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

func TestMassRemovalGuardSurvivesLaterRuns(t *testing.T) {
	for _, protection := range []string{"off", "mirror", "snapshots"} {
		t.Run(protection, func(t *testing.T) {
			e := newEnv(t)
			src := e.fake.Folder("Source", "root")
			var files []drive.File
			for i := range 20 {
				files = append(files, e.fake.File(fmt.Sprintf("file%d", i), src.ID, 1))
			}
			base, root := e.baseline(src)
			setProtection(t, e, base.CollectionID, protection != "off", protection == "snapshots")
			if protection == "snapshots" {
				e.runUntil(e.runNow(base.CollectionID).ID, terminal)
			}
			for _, f := range files[:9] {
				e.fake.Mutate(f.ID, func(f *drive.File) { f.Trashed = true })
			}
			writes := e.fake.Writes
			for attempt := range 2 {
				j := e.runUntil(e.runNow(base.CollectionID).ID, terminal)
				if j.State != state.JobPartial || j.LastError == nil || j.LastError.Code != "source_mass_removal" || j.SourceRemoved != 9 {
					t.Fatalf("run %d: state=%s removed=%d error=%+v", attempt, j.State, j.SourceRemoved, j.LastError)
				}
				// Let the read-only watch audit finish, then start a fresh engine/run.
				if id, _ := e.store.ActiveRunID(t.Context(), base.CollectionID); id != "" {
					e.runUntil(id, terminal)
				}
				if e.fake.Writes != writes || len(e.fake.Children(root)) != 20 {
					t.Fatal("a guarded run modified the destination")
				}
			}
			for _, f := range files[:9] {
				e.fake.Mutate(f.ID, func(f *drive.File) { f.Trashed = false })
			}
			if j := e.runUntil(e.runNow(base.CollectionID).ID, terminal); j.State != state.JobSucceeded {
				t.Fatalf("recovered source did not clear guard: %s %+v", j.State, j.LastError)
			}
		})
	}
}

func TestMassRemovalGuardUsesBaselineBeforeDiscovery(t *testing.T) {
	for _, scenario := range []string{"new_files", "earlier_mirror"} {
		t.Run(scenario, func(t *testing.T) {
			e := newEnv(t)
			src := e.fake.Folder("Source", "root")
			var files []drive.File
			for i := range 20 {
				files = append(files, e.fake.File(fmt.Sprintf("file%d", i), src.ID, 1))
			}
			base, _ := e.baseline(src)
			setProtection(t, e, base.CollectionID, true, false)
			removed := 9
			if scenario == "earlier_mirror" {
				for _, f := range files[:5] {
					e.fake.Mutate(f.ID, func(f *drive.File) { f.Trashed = true })
				}
				if j := e.runUntil(e.runNow(base.CollectionID).ID, terminal); j.State != state.JobSucceeded {
					t.Fatalf("small churn: %s %+v", j.State, j.LastError)
				}
				files, removed = files[5:], 5
			} else {
				for i := range 40 {
					e.fake.File(fmt.Sprintf("new%d", i), src.ID, 1)
				}
			}
			for _, f := range files[:removed] {
				e.fake.Mutate(f.ID, func(f *drive.File) { f.Trashed = true })
			}
			writes := e.fake.Writes
			j := e.runUntil(e.runNow(base.CollectionID).ID, terminal)
			if j.State != state.JobPartial || j.LastError == nil || j.LastError.Code != "source_mass_removal" || e.fake.Writes != writes {
				t.Fatalf("baseline loss was diluted: %s %+v writes=%d", j.State, j.LastError, e.fake.Writes-writes)
			}
		})
	}
}

func TestReplaceCopyRetiresWithinTheSameRun(t *testing.T) {
	for _, failTrash := range []bool{false, true} {
		t.Run(fmt.Sprint(failTrash), func(t *testing.T) {
			e := newEnv(t)
			src := e.fake.File("file", "root", 1)
			base, root := e.baseline(src)
			old := find(e.fake, root, "file")[0]
			c := mustColl(e, base.CollectionID)
			if _, _, err := e.store.ConfigureSync(t.Context(), "owner", "", "sync", c.ID, state.SyncChange{Mode: state.ModeRealtime, ContentPolicy: state.PolicyReplace, ExpectedCollection: c.Revision, Cursor: "0"}); err != nil {
				t.Fatal(err)
			}
			e.runUntil(findRun(e, c.ID), terminal)
			e.fake.Mutate(src.ID, func(f *drive.File) { f.MD5 = "changed"; f.Version = "2" })
			e.fake.Before = func(op string, f drive.File) error {
				if failTrash && op == "trash" && f.ID == old.ID {
					return &drive.Error{Kind: drive.KindPermission}
				}
				return nil
			}
			j := e.runUntil(e.runNow(c.ID).ID, terminal)
			got, _ := e.fake.Get(old.ID)
			if failTrash {
				if got.Trashed || j.State != state.JobPartial || j.LastError == nil || j.LastError.Code != "cleanup_failed" {
					t.Fatalf("failed retirement: trashed=%v state=%s error=%+v", got.Trashed, j.State, j.LastError)
				}
			} else if !got.Trashed || j.State != state.JobSucceeded || len(e.fake.Children(root)) != 1 {
				t.Fatalf("old copy survived first run: trashed=%v state=%s", got.Trashed, j.State)
			}
		})
	}
}

func TestSingleFileSourceReadFailureNeverSucceeds(t *testing.T) {
	for _, kind := range []drive.Kind{drive.KindPermission, drive.KindServer, drive.KindAuth, drive.KindQuota, drive.KindUnreachable} {
		t.Run(string(kind), func(t *testing.T) {
			e := newEnv(t)
			src := e.fake.File("file", "root", 1)
			base, _ := e.baseline(src)
			c := mustColl(e, base.CollectionID)
			_, _, err := e.store.ConfigureSync(t.Context(), "owner", "", "sync", c.ID, state.SyncChange{Mode: state.ModeRealtime, ContentPolicy: state.PolicyVersioned, ExpectedCollection: c.Revision, Cursor: "0"})
			if err != nil {
				t.Fatal(err)
			}
			e.runUntil(findRun(e, c.ID), terminal)
			before, _ := e.store.GetSubscription(t.Context(), c.ID)
			e.fake.Before = func(op string, f drive.File) error {
				if op == "get" && f.ID == src.ID {
					return &drive.Error{Kind: kind}
				}
				return nil
			}
			j := e.runUntil(e.runNow(c.ID).ID, terminal)
			if j.State == state.JobSucceeded || j.LastError == nil {
				t.Fatalf("unread source reported %s: %+v", j.State, j.LastError)
			}
			after, _ := e.store.GetSubscription(t.Context(), c.ID)
			if before.LastSuccessAt == nil || after.LastSuccessAt == nil || !before.LastSuccessAt.Equal(*after.LastSuccessAt) {
				t.Fatal("failed read advanced last_success_at")
			}
			if kind == drive.KindQuota && (j.State != state.JobBlocked || j.ResumeAfter == nil) {
				t.Fatalf("quota did not block with resume time: %+v", j)
			}
		})
	}
}

func TestCleanupObservesControlAfterRootRead(t *testing.T) {
	for _, action := range []string{"cancel", "pause"} {
		t.Run(action, func(t *testing.T) {
			e := newEnv(t)
			_, col, _ := e.copyTree("Photos")
			retired, err := e.store.RetireCollection(t.Context(), "owner", "retire", col.ID, col.Revision)
			if err != nil {
				t.Fatal(err)
			}
			job, err := e.store.AdmitCleanup(t.Context(), "owner", "cleanup", col.ID, retired.Revision, false)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = e.store.ClaimQueued(t.Context()); err != nil {
				t.Fatal(err)
			}
			e.fake.Before = func(op string, f drive.File) error {
				if op == "get" && f.ID == *col.RootID {
					j, err := e.store.GetJob(t.Context(), "owner", job.ID)
					if err != nil {
						return err
					}
					_, err = e.store.Action(t.Context(), "owner", "control", j.ID, action, j.Revision)
					return err
				}
				return nil
			}
			eng := e.engine()
			eng.defaults()
			r := &run{e: eng, db: context.Background(), id: job.ID}
			r.execute(t.Context())
			got, _ := e.store.GetJob(t.Context(), "owner", job.ID)
			want := state.JobCancelled
			if action == "pause" {
				want = state.JobPaused
			}
			f, _ := e.fake.Get(*col.RootID)
			if got.State != want || f.Trashed || e.fake.CallCount("trash") != 0 {
				t.Fatalf("%s: state=%s trashed=%v", action, got.State, f.Trashed)
			}
		})
	}
}

func TestSyncDoesNotWriteWhenDestinationRootCannotBeChecked(t *testing.T) {
	e := newEnv(t)
	src := e.fake.File("file", "root", 1)
	base, root := e.baseline(src)
	e.fake.Mutate(src.ID, func(f *drive.File) { f.MD5 = "changed"; f.Version = "2" })
	writes := e.fake.Writes
	e.fake.Before = func(op string, f drive.File) error {
		if op == "get" && f.ID == root {
			return &drive.Error{Kind: drive.KindServer}
		}
		return nil
	}
	j := e.runUntil(e.runNow(base.CollectionID).ID, terminal)
	if j.State != state.JobBlocked || j.LastError == nil || e.fake.Writes != writes {
		t.Fatalf("unverified destination accepted writes: %s %+v writes=%d", j.State, j.LastError, e.fake.Writes-writes)
	}
}

func TestTrashStopsWhenJobControlCannotBeRead(t *testing.T) {
	e := newEnv(t)
	_, col, items := e.copyTree("Photos")
	db, err := sql.Open("sqlite", filepath.Join(e.store.DataDir(), "state-v2.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec("ALTER TABLE jobs RENAME TO unavailable_jobs"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := db.Exec("ALTER TABLE unavailable_jobs RENAME TO jobs"); err != nil {
			t.Error(err)
		}
	}()
	eng := e.engine()
	eng.defaults()
	r := &run{e: eng, db: context.Background(), id: col.BaselineJobID, p: e.fake}
	writes := e.fake.Writes
	err = r.trashOurs(t.Context(), items["a.txt"].DestID, items["a.txt"].IntentKey)
	f, _ := e.fake.Get(items["a.txt"].DestID)
	if err == nil || f.Trashed || e.fake.Writes != writes {
		t.Fatalf("unreadable job control allowed trash: error=%v writes=%d", err, e.fake.Writes-writes)
	}
}
