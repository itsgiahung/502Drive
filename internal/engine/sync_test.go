package engine

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/itsgiahung/502Drive/internal/drive"
	"github.com/itsgiahung/502Drive/internal/state"
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

// Losing more than 30% of the mapped source in one scan stops the run before
// any copy and asks for a read-only audit; the destination is never touched.
func TestMassSourceRemovalStopsAndQueuesAnAudit(t *testing.T) {
	e := newEnv(t)
	src := e.fake.Folder("Bulk", "root")
	files := make([]drive.File, 20)
	for i := range files {
		files[i] = e.fake.File(fmt.Sprintf("f%02d.txt", i), src.ID, int64(i+1))
	}
	base, root := e.baseline(src)
	if got := len(e.fake.Children(root)); got != 20 {
		t.Fatalf("baseline files = %d", got)
	}

	for i := range 9 {
		e.fake.Mutate(files[i].ID, func(x *drive.File) { x.Trashed = true })
	}
	writes := e.fake.Writes
	j := e.runUntil(e.runNow(base.CollectionID).ID, terminal)
	if j.State != state.JobPartial || j.LastError == nil || j.LastError.Code != "source_mass_removal" {
		t.Fatalf("state=%s err=%+v", j.State, j.LastError)
	}
	if j.SourceRemoved != 9 {
		t.Fatalf("source_removed = %d", j.SourceRemoved)
	}
	if e.fake.Writes != writes {
		t.Fatalf("the suspect run wrote %d objects", e.fake.Writes-writes)
	}
	if got := len(e.fake.Children(root)); got != 20 {
		t.Fatalf("destination lost files: %d", got)
	}

	jobs, _, err := e.store.ListJobs(context.Background(), "", "", "", 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	var audit *state.Job
	for i := range jobs {
		if jobs[i].Kind == state.JobKindAudit && jobs[i].CollectionID == base.CollectionID {
			audit = &jobs[i]
		}
	}
	if audit == nil || audit.Trigger != "reconcile_watch" {
		t.Fatalf("no watch audit was queued: %+v", jobs)
	}
}

// A loss under the threshold is normal source churn: the scan flags the gone
// items, copies what remains and does not raise an audit.
func TestSmallSourceRemovalSyncsNormally(t *testing.T) {
	e := newEnv(t)
	src := e.fake.Folder("Churn", "root")
	files := make([]drive.File, 20)
	for i := range files {
		files[i] = e.fake.File(fmt.Sprintf("f%02d.txt", i), src.ID, int64(i+1))
	}
	base, root := e.baseline(src)
	for i := range 5 {
		e.fake.Mutate(files[i].ID, func(x *drive.File) { x.Trashed = true })
	}

	j := e.runUntil(e.runNow(base.CollectionID).ID, terminal)
	if j.State != state.JobSucceeded {
		t.Fatalf("state=%s err=%+v", j.State, j.LastError)
	}
	if j.SourceRemoved != 0 {
		t.Fatalf("below-threshold loss was reported as mass removal: %d", j.SourceRemoved)
	}
	if got := len(e.fake.Children(root)); got != 20 {
		t.Fatalf("destination changed under churn: %d", got)
	}
	jobs, _, err := e.store.ListJobs(context.Background(), "", "", "", 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range jobs {
		if x.Kind == state.JobKindAudit && x.CollectionID == base.CollectionID {
			t.Fatal("churn queued an audit")
		}
	}
}

// A pause seen at the top of a page read must stop the scan, not produce an
// empty successful page: committing that would mark the folder fully listed and
// silently drop the rest of the tree (found while pausing the 10k load probe).
func TestPausedListingDoesNotReportAnEmptyPage(t *testing.T) {
	e := newEnv(t)
	e.fake.PageSize = 5
	src := e.fake.Folder("Paged", "root")
	for i := range 12 {
		e.fake.Folder(fmt.Sprintf("d%02d", i), src.ID)
	}
	job := e.submit(src)
	ctx := context.Background()
	eng := e.engine()
	eng.defaults()
	r := &run{e: eng, db: ctx, id: job.ID, p: e.fake}
	if _, err := e.store.UpdateJob(ctx, job.ID, func(j *state.Job) (string, bool) {
		j.DesiredState = "pause"
		return "", false
	}); err != nil {
		t.Fatal(err)
	}
	items, err := e.store.Items(ctx, job.CollectionID)
	if err != nil {
		t.Fatal(err)
	}
	var root state.Item
	for _, it := range items {
		if it.Kind == state.KindFolder && it.ParentSourceID == "" {
			root = it
		}
	}
	files, next, err := r.listPage(ctx, root)
	if err == nil || len(files) != 0 || next != "" {
		t.Fatalf("paused listPage = %d files, next %q, err %v (an empty success would truncate the tree)", len(files), next, err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}

// Renaming a source file changes its fingerprint: the new generation must be
// named from the source again, not copied under the previous generation's name.
func TestRenamedFileRecopyUsesTheNewDestinationName(t *testing.T) {
	e := newEnv(t)
	src := e.fake.Folder("P", "root")
	f := e.fake.File("a.txt", src.ID, 1)
	base, root := e.baseline(src)
	if got := find(e.fake, root, "a.txt"); len(got) != 1 {
		t.Fatalf("baseline copies named a.txt: %d", len(got))
	}

	// Renamed and edited: a new generation, named after the new source name.
	e.fake.Mutate(f.ID, func(x *drive.File) { x.Name, x.MD5 = "b.txt", "edited" })
	j := e.runUntil(e.runNow(base.CollectionID).ID, terminal)
	if j.State != state.JobSucceeded {
		t.Fatalf("rename run = %s %+v", j.State, j.LastError)
	}
	if n := len(find(e.fake, root, "b.txt")); n != 1 {
		t.Fatalf("second generation used the old destination name: b.txt copies = %d", n)
	}
	if n := len(find(e.fake, root, "a.txt")); n != 1 {
		t.Fatalf("versioned_copy must keep exactly the old copy: a.txt copies = %d", n)
	}
}

// The single-file (wrapper) recopy path has its own name decision per
// generation; a renamed source must not be copied under the old name either.
func TestRenamedSingleFileRecopyUsesTheNewDestinationName(t *testing.T) {
	e := newEnv(t)
	f := e.fake.File("report.txt", "root", 10)
	base, root := e.baseline(f)
	e.fake.Mutate(f.ID, func(x *drive.File) { x.Name = "renamed.txt"; x.Version = "2"; x.MD5 = "edited" })
	j := e.runUntil(e.runNow(base.CollectionID).ID, terminal)
	if j.State != state.JobSucceeded {
		t.Fatalf("rename run = %s %+v", j.State, j.LastError)
	}
	if n := len(find(e.fake, root, "renamed.txt")); n != 1 {
		t.Fatalf("wrapper used the old destination name: renamed.txt copies = %d", n)
	}
	if n := len(find(e.fake, root, "report.txt")); n != 1 {
		t.Fatalf("versioned_copy must keep exactly the old copy: report.txt copies = %d", n)
	}
}

// A queue pause resumed while the job was still reaching its safe point must
// not settle the job as paused: settle reports "continue" and the worker's run
// loop starts over instead of leaving it stuck.
func TestSettlingALiftedQueuePauseKeepsTheRunGoing(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	src := e.fake.Folder("P", "root")
	e.fake.File("a", src.ID, 1)
	job := e.submit(src)
	if _, err := e.store.ClaimQueued(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.SetQueuePaused(ctx, true); err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.SetQueuePaused(ctx, false); err != nil {
		t.Fatal(err)
	}
	r := &run{e: e.engine(), db: ctx, id: job.ID}
	r.settle(ctx)
	if !r.resumed {
		t.Fatal("settle paused a job whose queue pause had been lifted")
	}
	got, err := e.store.GetJob(ctx, "owner", job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != state.JobRunning || got.DesiredState != "run" {
		t.Fatalf("job after lifted queue pause = %s/%s", got.State, got.DesiredState)
	}
}

// A pause the user requested still settles to paused.
func TestSettlingAUserPausePausesTheRun(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	src := e.fake.Folder("P", "root")
	e.fake.File("a", src.ID, 1)
	job := e.submit(src)
	if _, err := e.store.ClaimQueued(ctx); err != nil {
		t.Fatal(err)
	}
	cur, _ := e.store.GetJob(ctx, "owner", job.ID)
	if _, err := e.store.Action(ctx, "owner", "pause-key", job.ID, "pause", cur.Revision); err != nil {
		t.Fatal(err)
	}
	r := &run{e: e.engine(), db: ctx, id: job.ID}
	r.settle(ctx)
	if r.resumed {
		t.Fatal("settle resumed a job the user paused")
	}
	if got, _ := e.store.GetJob(ctx, "owner", job.ID); got.State != state.JobPaused {
		t.Fatalf("user pause settled as %s", got.State)
	}
}

// Drive bumps a file's version for changes the user never made (sharing the
// parent, being copied). For a Google native file (no md5) only modifiedTime
// says the content changed, so a version bump alone must not re-copy it.
func TestDeltaRunIgnoresAVersionBumpWithoutAnEdit(t *testing.T) {
	e := newEnv(t)
	src := e.fake.Folder("Docs", "root")
	doc := e.fake.File("note", src.ID, 0)
	when := time.Date(2026, 10, 28, 1, 0, 0, 0, time.UTC)
	e.fake.Mutate(doc.ID, func(x *drive.File) {
		x.MimeType, x.MD5, x.Size, x.Modified = "application/vnd.google-apps.document", "", nil, when
	})
	base, root := e.baseline(src)
	writes := e.fake.Writes

	e.fake.Mutate(doc.ID, func(x *drive.File) { x.Version = "9" })
	if j := e.runUntil(e.runNow(base.CollectionID).ID, terminal); j.State != state.JobSucceeded || e.fake.Writes != writes {
		t.Fatalf("a version bump re-copied the doc: %s writes %d->%d", j.State, writes, e.fake.Writes)
	}

	e.fake.Mutate(doc.ID, func(x *drive.File) { x.Version, x.Modified = "10", when.Add(time.Minute) })
	if j := e.runUntil(e.runNow(base.CollectionID).ID, terminal); j.State != state.JobSucceeded {
		t.Fatalf("edit run %s", j.State)
	}
	if got := find(e.fake, root, "note"); len(got) != 2 {
		t.Fatalf("an edited doc must get a new generation (versioned_copy keeps both), got %d", len(got))
	}
}

// Renaming a file whose bytes did not change renames the existing copy; it is
// not copied again (no second copy under versioned_copy).
func TestDeltaRunRenamesTheCopyOnAPureRename(t *testing.T) {
	e := newEnv(t)
	src := e.fake.Folder("Ren", "root")
	f := e.fake.File("b.txt", src.ID, 25)
	base, root := e.baseline(src)
	copies, renames := e.fake.Calls["copy"], e.fake.Calls["rename"]

	e.fake.Mutate(f.ID, func(x *drive.File) { x.Name = "b-renamed.txt"; x.Version = "5" })
	if j := e.runUntil(e.runNow(base.CollectionID).ID, terminal); j.State != state.JobSucceeded {
		t.Fatalf("delta %s %+v", j.State, j.LastError)
	}
	if e.fake.Calls["copy"] != copies || e.fake.Calls["rename"] != renames+1 {
		t.Fatalf("copy %d->%d rename %d->%d, want no copy and one rename", copies, e.fake.Calls["copy"], renames, e.fake.Calls["rename"])
	}
	if len(find(e.fake, root, "b-renamed.txt")) != 1 || len(find(e.fake, root, "b.txt")) != 0 {
		t.Fatal("the copy was not renamed in place")
	}
	// Editing the bytes afterwards still makes a new generation.
	e.fake.Mutate(f.ID, func(x *drive.File) { x.MD5 = "edited" })
	if j := e.runUntil(e.runNow(base.CollectionID).ID, terminal); j.State != state.JobSucceeded || e.fake.Calls["copy"] != copies+1 {
		t.Fatalf("an edit after the rename must copy once: %s copies=%d", j.State, e.fake.Calls["copy"]-copies)
	}
}

// A single-file collection renames its copy in place on a pure rename too.
func TestSingleFilePureRenameRenamesTheCopy(t *testing.T) {
	e := newEnv(t)
	f := e.fake.File("report.txt", "root", 10)
	base, root := e.baseline(f)
	copies := e.fake.Calls["copy"]
	e.fake.Mutate(f.ID, func(x *drive.File) { x.Name = "renamed.txt"; x.Version = "2" })
	if j := e.runUntil(e.runNow(base.CollectionID).ID, terminal); j.State != state.JobSucceeded {
		t.Fatalf("rename run = %s %+v", j.State, j.LastError)
	}
	if e.fake.Calls["copy"] != copies || len(find(e.fake, root, "renamed.txt")) != 1 || len(find(e.fake, root, "report.txt")) != 0 {
		t.Fatalf("pure rename of a single file: copies +%d", e.fake.Calls["copy"]-copies)
	}
}
