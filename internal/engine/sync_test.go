package engine

import (
	"context"
	"fmt"
	"testing"

	"github.com/GiaHung07/502Drive/internal/drive"
	"github.com/GiaHung07/502Drive/internal/state"
)

func (e *env) runNow(collection string) state.Job {
	e.t.Helper()
	c, err := e.store.GetCollection(context.Background(), "", collection)
	if err != nil {
		e.t.Fatal(err)
	}
	out, _, err := e.store.RunNow(context.Background(), "owner", "", fmt.Sprintf("run-%d-%d", c.Revision, e.n), collection, c.Revision)
	e.n++
	if err != nil {
		e.t.Fatal(err)
	}
	return out.Job
}

func (e *env) baseline(src drive.File) (state.Job, string) {
	j := e.runUntil(e.submit(src).ID, terminal)
	if j.State != state.JobSucceeded {
		e.t.Fatalf("baseline %s %+v", j.State, j.LastError)
	}
	return j, *mustColl(e, j.CollectionID).RootID
}

func mustColl(e *env, id string) state.Collection {
	c, err := e.store.GetCollection(context.Background(), "", id)
	if err != nil {
		e.t.Fatal(err)
	}
	return c
}

func find(f interface{ Children(string) []drive.File }, parent, name string) []drive.File {
	var out []drive.File
	for _, c := range f.Children(parent) {
		if c.Name == name {
			out = append(out, c)
		}
	}
	return out
}

func TestDeltaRunReusesRootAndAppliesChangesWithoutDeleting(t *testing.T) {
	e := newEnv(t)
	src := e.fake.Folder("Proj", "root")
	keep := e.fake.File("keep.txt", src.ID, 1)
	edit := e.fake.File("edit.txt", src.ID, 2)
	gone := e.fake.File("gone.txt", src.ID, 3)
	dirA := e.fake.Folder("dirA", src.ID)
	dirB := e.fake.Folder("dirB", src.ID)
	moving := e.fake.File("moving.txt", dirA.ID, 4)
	base, root := e.baseline(src)
	writes := e.fake.Writes

	// nothing changed: a run must not write anything
	j := e.runUntil(e.runNow(base.CollectionID).ID, terminal)
	if j.State != state.JobSucceeded || e.fake.Writes != writes {
		t.Fatalf("idle run wrote: %s writes %d->%d", j.State, writes, e.fake.Writes)
	}

	// source changes
	e.fake.File("new.txt", src.ID, 5)
	e.fake.Mutate(edit.ID, func(x *drive.File) { x.Version = "2"; x.MD5 = "changed" })
	e.fake.Mutate(gone.ID, func(x *drive.File) { x.Trashed = true })
	e.fake.Mutate(dirA.ID, func(x *drive.File) { x.Name = "dirA-renamed" })
	e.fake.Mutate(moving.ID, func(x *drive.File) { x.Parents = []string{dirB.ID} })
	newDir := e.fake.Folder("fresh", src.ID)
	e.fake.File("inside.txt", newDir.ID, 6)

	j = e.runUntil(e.runNow(base.CollectionID).ID, terminal)
	if j.State != state.JobSucceeded {
		t.Fatalf("delta %s %+v", j.State, j.LastError)
	}
	c := mustColl(e, base.CollectionID)
	if *c.RootID != root || len(e.fake.Children(e.dest.ID)) != 1 {
		t.Fatal("a sync run created a new root")
	}
	if len(find(e.fake, root, "keep.txt")) != 1 || keep.ID == "" {
		t.Fatal("unchanged file duplicated")
	}
	if got := find(e.fake, root, "edit.txt"); len(got) != 2 {
		t.Fatalf("versioned_copy must keep old and new copy, got %d", len(got))
	}
	if len(find(e.fake, root, "gone.txt")) != 1 {
		t.Fatal("source deletion must not delete the destination copy")
	}
	if len(find(e.fake, root, "new.txt")) != 1 {
		t.Fatal("new file not copied")
	}
	renamed := find(e.fake, root, "dirA-renamed")
	if len(renamed) != 1 || len(find(e.fake, root, "dirA")) != 0 {
		t.Fatal("folder rename not applied to the existing destination folder")
	}
	dirBCopy := find(e.fake, root, "dirB")[0]
	if len(find(e.fake, dirBCopy.ID, "moving.txt")) != 1 || len(find(e.fake, renamed[0].ID, "moving.txt")) != 0 {
		t.Fatal("move not applied")
	}
	fresh := find(e.fake, root, "fresh")
	if len(fresh) != 1 || len(find(e.fake, fresh[0].ID, "inside.txt")) != 1 {
		t.Fatal("new folder with content not copied")
	}
	// the run is idempotent
	w := e.fake.Writes
	e.runUntil(e.runNow(base.CollectionID).ID, terminal)
	if e.fake.Writes != w {
		t.Fatalf("second run after convergence wrote %d objects", e.fake.Writes-w)
	}
}

func TestReplaceCopyRetiresOldVersionOnlyAfterNewOne(t *testing.T) {
	e := newEnv(t)
	src := e.fake.Folder("P", "root")
	f := e.fake.File("doc", src.ID, 1)
	base, root := e.baseline(src)
	c := mustColl(e, base.CollectionID)
	// a replace_copy subscription (paused: only the policy matters here)
	if _, _, err := e.store.ConfigureSync(context.Background(), "owner", "", "cfg", base.CollectionID, state.SyncChange{Mode: state.ModeRealtime, ContentPolicy: state.PolicyReplace, ExpectedCollection: c.Revision, Cursor: "0"}); err != nil {
		t.Fatal(err)
	}
	e.runUntil(findRun(e, base.CollectionID), terminal) // reconcile run admitted by ConfigureSync
	e.fake.Mutate(f.ID, func(x *drive.File) { x.Version = "9"; x.MD5 = "new" })
	failTrash := true
	e.fake.Before = func(op string, _ drive.File) error {
		if op == "trash" && failTrash {
			return &drive.Error{Kind: drive.KindPermission, Status: 403}
		}
		return nil
	}
	j := e.runUntil(e.runNow(base.CollectionID).ID, terminal)
	if j.State != state.JobSucceeded && j.State != state.JobPartial {
		t.Fatalf("%s", j.State)
	}
	if n := len(find(e.fake, root, "doc")); n != 2 {
		t.Fatalf("both copies must survive a failed cleanup, got %d", n)
	}
	failTrash = false
	e.runUntil(e.runNow(base.CollectionID).ID, terminal)
	if n := len(find(e.fake, root, "doc")); n != 1 {
		t.Fatalf("old copy not retired on retry: %d", n)
	}
}

func findRun(e *env, collection string) string {
	id, err := e.store.ActiveRunID(context.Background(), collection)
	if err != nil || id == "" {
		e.t.Fatalf("no run for %s: %v", collection, err)
	}
	return id
}

func TestRunNowCoalescesWithActiveRunAndRefusesWhileBlocked(t *testing.T) {
	e := newEnv(t)
	src := e.fake.Folder("P", "root")
	e.fake.File("a", src.ID, 1)
	base, _ := e.baseline(src)
	c := mustColl(e, base.CollectionID)
	first, _, err := e.store.RunNow(context.Background(), "owner", "", "k1", base.CollectionID, c.Revision)
	if err != nil || first.Coalesced {
		t.Fatal(err)
	}
	second, _, err := e.store.RunNow(context.Background(), "owner", "", "k2", base.CollectionID, c.Revision)
	if err != nil || !second.Coalesced || second.Job.ID != first.Job.ID {
		t.Fatalf("not coalesced: %+v %v", second, err)
	}
}

func TestSingleFileCollectionPicksUpNewVersion(t *testing.T) {
	e := newEnv(t)
	f := e.fake.File("report", "root", 10)
	base, root := e.baseline(f)
	e.fake.Mutate(f.ID, func(x *drive.File) { x.Version = "2"; x.MD5 = "v2" })
	j := e.runUntil(e.runNow(base.CollectionID).ID, terminal)
	if j.State != state.JobSucceeded {
		t.Fatalf("%s %+v", j.State, j.LastError)
	}
	if n := len(find(e.fake, root, "report")); n != 2 {
		t.Fatalf("wrapper holds %d copies", n)
	}
}

func TestStoppedOrCancelledSyncRunDoesNotBreakTheCollection(t *testing.T) {
	e := newEnv(t)
	src := e.fake.Folder("P", "root")
	e.fake.File("a", src.ID, 1)
	base, _ := e.baseline(src)
	before := mustColl(e, base.CollectionID)

	run := e.runNow(base.CollectionID)
	if _, err := e.store.Action(context.Background(), "owner", "c1", run.ID, "cancel", run.Revision); err != nil {
		t.Fatal(err)
	}
	after := mustColl(e, base.CollectionID)
	if after.State != "ready" || after.Revision != before.Revision {
		t.Fatalf("collection changed by a cancelled run: %+v -> %+v", before, after)
	}
	// a finished delta run does not bump the revision either, so a client that
	// just read it can still act
	j := e.runUntil(e.runNow(base.CollectionID).ID, terminal)
	if j.State != state.JobSucceeded {
		t.Fatalf("%s", j.State)
	}
	if got := mustColl(e, base.CollectionID); got.Revision != before.Revision {
		t.Fatalf("revision moved %d -> %d", before.Revision, got.Revision)
	}
}
