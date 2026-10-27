package engine

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/itsgiahung/502Drive/internal/drive"
	"github.com/itsgiahung/502Drive/internal/state"
)

func TestMirrorIsOptInAndNeverTrashesDestinationAdditions(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "off", true: "on"}[enabled], func(t *testing.T) {
			e := newEnv(t)
			src := e.fake.Folder("Source", "root")
			gone := e.fake.File("gone.txt", src.ID, 4)
			e.fake.File("stay.txt", src.ID, 5)
			base, root := e.baseline(src)
			db, err := sql.Open("sqlite", filepath.Join(e.store.DataDir(), "state-v2.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if _, err := db.ExecContext(context.Background(), "UPDATE collections SET body=json_set(body,'$.protection',json(?)) WHERE id=?", map[bool]string{false: `{"mirror":false}`, true: `{"mirror":true}`}[enabled], base.CollectionID); err != nil {
				t.Fatal(err)
			}
			copy := find(e.fake, root, "gone.txt")[0]
			stranger := e.fake.File("user-added.txt", root, 9)
			e.fake.Mutate(gone.ID, func(f *drive.File) { f.Trashed = true })
			j := e.runUntil(e.runNow(base.CollectionID).ID, terminal)
			got, _ := e.fake.Get(copy.ID)
			if got.Trashed != enabled || j.State != state.JobSucceeded {
				t.Fatalf("mirror=%v: trashed=%v job=%s error=%+v", enabled, got.Trashed, j.State, j.LastError)
			}
			if f, _ := e.fake.Get(stranger.ID); f.Trashed {
				t.Fatal("mirror trashed a destination addition")
			}
		})
	}
}

func setProtection(t *testing.T, e *env, col string, mirror, snapshots bool) {
	t.Helper()
	c, err := e.store.GetCollection(t.Context(), "owner", col)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = e.store.SetProtection(t.Context(), "owner", "owner", fmt.Sprintf("opts-%d", c.Revision), col, c.Revision, state.ProtectionOptions{Mirror: mirror, Snapshots: snapshots}); err != nil {
		t.Fatal(err)
	}
}

func TestMirrorRefusesMissingIntentNonemptyFoldersAndIncompleteScans(t *testing.T) {
	for _, failure := range []string{"marker", "addition", "list", "mass"} {
		t.Run(failure, func(t *testing.T) {
			e := newEnv(t)
			src := e.fake.Folder("Source", "root")
			sub := e.fake.Folder("sub", src.ID)
			gone := e.fake.File("gone", sub.ID, 2)
			for n := 0; n < 12; n++ {
				e.fake.File(fmt.Sprintf("keep%d", n), src.ID, 1)
			}
			base, root := e.baseline(src)
			setProtection(t, e, base.CollectionID, true, false)
			mapped := e.itemOf(base.CollectionID, gone.ID)
			submap := e.itemOf(base.CollectionID, sub.ID)
			e.fake.Mutate(gone.ID, func(f *drive.File) { f.Trashed = true })
			switch failure {
			case "marker":
				e.fake.Mutate(mapped.DestID, func(f *drive.File) { f.AppProperties = nil })
			case "addition":
				e.fake.Mutate(sub.ID, func(f *drive.File) { f.Parents = []string{"root"} })
				e.fake.File("user-added", submap.DestID, 3)
			case "list":
				e.fake.Before = func(op string, f drive.File) error {
					if (op == "list" || op == "list_batch") && f.ID == sub.ID {
						return &drive.Error{Kind: drive.KindPermission, Status: 403}
					}
					return nil
				}
			case "mass":
				for _, child := range e.fake.Children(src.ID) {
					e.fake.Mutate(child.ID, func(f *drive.File) { f.Trashed = true })
				}
			}
			j := e.runUntil(e.runNow(base.CollectionID).ID, terminal)
			if j.State == state.JobSucceeded {
				t.Fatalf("unsafe %s succeeded", failure)
			}
			if failure != "addition" {
				if f, _ := e.fake.Get(mapped.DestID); f.Trashed {
					t.Fatal("unsafe scan/ownership trashed copy")
				}
			}
			if f, _ := e.fake.Get(submap.DestID); f.Trashed {
				t.Fatal("folder with retained children trashed")
			}
			if f, _ := e.fake.Get(root); f.Trashed {
				t.Fatal("root trashed")
			}
		})
	}
}

type loseMoveResponse struct {
	drive.Provider
	lost bool
}

func (p *loseMoveResponse) MoveFile(ctx context.Context, id, from, to string) error {
	if err := p.Provider.MoveFile(ctx, id, from, to); err != nil {
		return err
	}
	if !p.lost {
		p.lost = true
		return &drive.Error{Kind: drive.KindUnavailable}
	}
	return nil
}

func TestRestoreReconcilesLostMoveResponseAndRejectsChangedContent(t *testing.T) {
	e := newEnv(t)
	src := e.fake.Folder("Source", "root")
	f := e.fake.File("file", src.ID, 1)
	base, root := e.baseline(src)
	c, _ := e.store.GetCollection(t.Context(), "owner", base.CollectionID)
	j, _, err := e.store.AdmitProtectionJob(t.Context(), "owner", "owner", "snap", c.ID, "", c.Revision, false)
	if err != nil {
		t.Fatal(err)
	}
	e.runUntil(j.ID, terminal)
	snaps, _, _ := e.store.Snapshots(t.Context(), "owner", c.ID, 10, 0)
	snap := snaps[0]
	copied := find(e.fake, root, "file")[0]
	elsewhere := e.fake.Folder("Elsewhere", "root")
	e.fake.Mutate(copied.ID, func(x *drive.File) { x.Parents = []string{elsewhere.ID} })
	j, _, err = e.store.AdmitProtectionJob(t.Context(), "owner", "owner", "restore", c.ID, snap.ID, snap.Revision, false)
	if err != nil {
		t.Fatal(err)
	}
	p := &loseMoveResponse{Provider: e.fake}
	eng := e.engine()
	eng.defaults()
	run := &run{e: eng, db: context.Background(), id: j.ID, collection: c.ID, p: p}
	if _, err = e.store.ClaimQueued(t.Context()); err != nil {
		t.Fatal(err)
	}
	run.executeRestore(t.Context(), j)
	blocked, _ := e.store.GetJob(t.Context(), "owner", j.ID)
	if blocked.State != state.JobBlocked {
		t.Fatalf("expected blocked: %+v", blocked)
	}
	moves := e.fake.Calls["move"]
	if _, err = e.store.Action(t.Context(), "owner", "retry", j.ID, "retry", blocked.Revision); err != nil {
		t.Fatal(err)
	}
	if got := e.runUntil(j.ID, terminal); got.State != state.JobSucceeded {
		t.Fatalf("resume: %+v", got)
	}
	if e.fake.Calls["move"] != moves {
		t.Fatal("repeated an already applied move")
	}
	e.fake.Mutate(copied.ID, func(x *drive.File) { x.MD5 = "edited" })
	before := e.snapshotWrites()
	j, _, err = e.store.AdmitProtectionJob(t.Context(), "owner", "owner", "changed", c.ID, snap.ID, snap.Revision, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := e.runUntil(j.ID, terminal); got.State != state.JobBlocked {
		t.Fatalf("modified snapshot not blocked %+v", got)
	}
	if diff := before.diff(e.snapshotWrites()); len(diff) > 0 {
		t.Fatalf("changed content caused writes %v", diff)
	}
	_ = f
}

func TestSnapshotCaptureContinuesAfterDatabaseReopenAndUntrashes(t *testing.T) {
	e := newEnv(t)
	e.fake.TrashedVisible = true
	src := e.fake.Folder("Source", "root")
	e.fake.File("data", src.ID, 1)
	base, root := e.baseline(src)
	c, _ := e.store.GetCollection(t.Context(), "owner", base.CollectionID)
	j, _, err := e.store.AdmitProtectionJob(t.Context(), "owner", "owner", "capture", c.ID, "", c.Revision, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.store.ClaimQueued(t.Context()); err != nil {
		t.Fatal(err)
	}
	snap, err := e.store.BeginSnapshot(t.Context(), j.ID)
	if err != nil {
		t.Fatal(err)
	}
	dir := e.store.DataDir()
	if err = e.store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := state.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	e.store = reopened
	if got := e.runUntil(j.ID, terminal); got.State != state.JobSucceeded {
		t.Fatalf("restart %+v", got)
	}
	snaps, _, err := e.store.Snapshots(t.Context(), "owner", c.ID, 10, 0)
	if err != nil || len(snaps) != 1 || snaps[0].ID != snap.ID {
		t.Fatalf("restart snapshots %+v %v", snaps, err)
	}
	copied := find(e.fake, root, "data")[0]
	e.fake.Mutate(copied.ID, func(f *drive.File) { f.Trashed = true })
	before := e.fake.Calls["copy"]
	j, _, err = e.store.AdmitProtectionJob(t.Context(), "owner", "owner", "restore", c.ID, snap.ID, snaps[0].Revision, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := e.runUntil(j.ID, terminal); got.State != state.JobSucceeded {
		t.Fatalf("untrash %+v", got)
	}
	if got, _ := e.fake.Get(copied.ID); got.Trashed || e.fake.Calls["restore"] != 1 || e.fake.Calls["copy"] != before {
		t.Fatal("did not restore the same physical copy")
	}
}

func TestSnapshotRestoreReusesRetainedCopiesAndDefaultsToDryRun(t *testing.T) {
	e := newEnv(t)
	src := e.fake.Folder("Documents", "root")
	f := e.fake.File("a.txt", src.ID, 4)
	e.fake.File("keep.txt", src.ID, 5)
	base, root := e.baseline(src)
	c, _ := e.store.GetCollection(context.Background(), "owner", base.CollectionID)
	c, _, err := e.store.SetProtection(context.Background(), "owner", "owner", "opts", c.ID, c.Revision, state.ProtectionOptions{Mirror: true, Snapshots: true})
	if err != nil {
		t.Fatal(err)
	}
	old := find(e.fake, root, "a.txt")[0]
	j, _, err := e.store.AdmitProtectionJob(context.Background(), "owner", "owner", "capture", c.ID, "", c.Revision, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := e.runUntil(j.ID, terminal); got.State != state.JobSucceeded {
		t.Fatalf("capture: %+v", got)
	}
	snaps, _, err := e.store.Snapshots(context.Background(), "owner", c.ID, 10, 0)
	if err != nil || len(snaps) != 1 || snaps[0].State != "ready" {
		t.Fatalf("snapshots=%+v err=%v", snaps, err)
	}
	snap := snaps[0]
	// Retained restore points must also survive replace_copy cleanup.
	if _, _, err = e.store.ConfigureSync(context.Background(), "owner", "owner", "sync", c.ID, state.SyncChange{Mode: state.ModeRealtime, ContentPolicy: state.PolicyReplace, ExpectedCollection: c.Revision, Cursor: "0"}); err != nil {
		t.Fatal(err)
	}
	active, err := e.store.ActiveRunID(context.Background(), c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if active != "" {
		e.runUntil(active, terminal)
	}
	e.fake.Mutate(f.ID, func(x *drive.File) { x.MD5 = "new-content"; x.Version = "2" })
	if got := e.runUntil(e.runNow(c.ID).ID, terminal); got.State != state.JobSucceeded {
		t.Fatalf("update %+v", got)
	}
	e.fake.Mutate(f.ID, func(x *drive.File) { x.Trashed = true })
	if got := e.runUntil(e.runNow(c.ID).ID, terminal); got.State != state.JobSucceeded {
		t.Fatalf("mirror %+v", got)
	}
	if v, _ := e.fake.Get(old.ID); v.Trashed {
		t.Fatal("snapshot's physical copy was trashed")
	}
	before := e.snapshotWrites()
	j, _, err = e.store.AdmitProtectionJob(context.Background(), "owner", "owner", "preview", c.ID, snap.ID, snap.Revision, true)
	if err != nil {
		t.Fatal(err)
	}
	if got := e.runUntil(j.ID, terminal); got.State != state.JobSucceeded {
		t.Fatalf("preview %+v", got)
	}
	if diff := before.diff(e.snapshotWrites()); len(diff) > 0 {
		t.Fatalf("dry run wrote: %v", diff)
	}
	j, _, err = e.store.AdmitProtectionJob(context.Background(), "owner", "owner", "restore", c.ID, snap.ID, snap.Revision, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := e.runUntil(j.ID, terminal); got.State != state.JobSucceeded {
		t.Fatalf("restore %+v", got)
	}
	kids := find(e.fake, root, "a.txt")
	if len(kids) != 1 || kids[0].ID != old.ID {
		t.Fatalf("restored=%+v want original %s", kids, old.ID)
	}
	if got := e.itemOf(c.ID, f.ID); got.Generation < 2 {
		t.Fatalf("restore rewound generation and could reuse an intent: %+v", got)
	}
}
