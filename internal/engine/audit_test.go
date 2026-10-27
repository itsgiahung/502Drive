package engine

import (
	"context"
	"fmt"
	"testing"

	"github.com/itsgiahung/502Drive/internal/drive"
	"github.com/itsgiahung/502Drive/internal/state"
)

// writeOps are the fake provider ops that change Drive. An audit must never
// touch them.
var writeOps = []string{"folder", "copy", "shortcut", "trash", "rename", "move"}

type fakeSnap map[string]int

func (e *env) snapshotWrites() fakeSnap {
	s := fakeSnap{}
	for _, op := range writeOps {
		s[op] = e.fake.Calls[op]
	}
	return s
}

func (s fakeSnap) diff(other fakeSnap) map[string]int {
	d := map[string]int{}
	for op, n := range s {
		if other[op] != n {
			d[op] = other[op] - n
		}
	}
	return d
}

// copyTree builds src "Photos" with a.txt, sub/b.txt, sub2/c.txt, d.txt and
// e.txt, runs the baseline copy and returns the collection plus the done
// mapping items by source name (the wrapper root excluded).
func (e *env) copyTree(name string) (src drive.File, col state.Collection, items map[string]state.Item) {
	e.t.Helper()
	src = e.fake.Folder(name, "root")
	sub := e.fake.Folder("sub", src.ID)
	sub2 := e.fake.Folder("sub2", src.ID)
	e.fake.File("a.txt", src.ID, 10)
	e.fake.File("b.txt", sub.ID, 20)
	e.fake.File("c.txt", sub2.ID, 30)
	e.fake.File("d.txt", sub2.ID, 40)
	e.fake.File("e.txt", sub2.ID, 50)
	job := e.submit(src)
	e.runUntil(job.ID, terminal)
	col, err := e.store.GetCollection(context.Background(), "owner", job.CollectionID)
	if err != nil {
		e.t.Fatal(err)
	}
	if col.RootID == nil {
		e.t.Fatal("baseline copy left no destination root")
	}
	all, err := e.store.Items(context.Background(), col.ID)
	if err != nil {
		e.t.Fatal(err)
	}
	items = map[string]state.Item{}
	for _, it := range all {
		if it.State == state.ItemDone && it.ParentSourceID != "" {
			items[it.Name] = it
		}
	}
	if len(items) != 7 {
		e.t.Fatalf("fixture mapping = %d items", len(items))
	}
	return src, col, items
}

func (e *env) itemOf(colID, sourceID string) state.Item {
	e.t.Helper()
	all, err := e.store.Items(context.Background(), colID)
	if err != nil {
		e.t.Fatal(err)
	}
	for _, it := range all {
		if it.SourceID == sourceID {
			return it
		}
	}
	e.t.Fatalf("no item %s in %s", sourceID, colID)
	return state.Item{}
}

func (e *env) runAudit(col state.Collection) (state.Audit, []state.Finding) {
	e.t.Helper()
	e.n++
	a, job, replayed, err := e.store.AdmitAudit(context.Background(), "owner", "owner", fmt.Sprintf("audit-%d", e.n), col.ID)
	if err != nil || replayed {
		e.t.Fatalf("admit audit: %v replayed=%v", err, replayed)
	}
	e.runUntil(job.ID, terminal)
	got, err := e.store.GetAudit(context.Background(), "owner", a.ID)
	if err != nil {
		e.t.Fatal(err)
	}
	fs, _, err := e.store.ListFindings(context.Background(), a.ID, "", 100, 0)
	if err != nil {
		e.t.Fatal(err)
	}
	return got, fs
}

func (e *env) runRepair(a state.Audit, dryRun bool) state.Job {
	e.t.Helper()
	e.n++
	job, replayed, err := e.store.AdmitRepair(context.Background(), "owner", "owner", fmt.Sprintf("repair-%d", e.n), a.ID, a.Revision, dryRun)
	if err != nil || replayed {
		e.t.Fatalf("admit repair: %v replayed=%v", err, replayed)
	}
	return e.runUntil(job.ID, terminal)
}

func findKind(fs []state.Finding, kind, id string) *state.Finding {
	for i := range fs {
		if fs[i].Kind == kind && (fs[i].SourceID == id || fs[i].DestID == id) {
			return &fs[i]
		}
	}
	return nil
}

func TestAuditDetectsEveryFindingKind(t *testing.T) {
	e := newEnv(t)
	src, col, items := e.copyTree("Photos")

	// missing: destination a.txt is gone (trashed = not in any listing).
	e.fake.Mutate(items["a.txt"].DestID, func(f *drive.File) { f.Trashed = true })
	// name mismatch: b kept its object but was renamed.
	e.fake.Mutate(items["b.txt"].DestID, func(f *drive.File) { f.Name = "b-renamed.txt" })
	// wrong parent: c moved to a sibling folder inside the tree.
	e.fake.Mutate(items["c.txt"].DestID, func(f *drive.File) { f.Parents = []string{items["sub"].DestID} })
	// size/md5 mismatch on d.
	sz := int64(41)
	e.fake.Mutate(items["d.txt"].DestID, func(f *drive.File) { f.Size = &sz })
	// duplicate: a second live copy of b with the same intent.
	bSrc, _ := e.fake.Get(items["b.txt"].SourceID)
	if _, err := e.fake.CopyFile(context.Background(), bSrc, items["sub"].DestID, items["b.txt"].IntentKey); err != nil {
		t.Fatal(err)
	}
	// extra: a foreign file without our intent.
	stranger := e.fake.File("stranger.txt", *col.RootID, 3)
	// source removed: the source of e is gone, the copy stays.
	e.fake.Mutate(items["e.txt"].SourceID, func(f *drive.File) { f.Trashed = true })
	// unreadable source: one folder refuses to list.
	locked := e.fake.Folder("locked", src.ID)
	e.fake.File("hidden.txt", locked.ID, 1)

	e.fake.Before = func(op string, f drive.File) error {
		if op == "list" && f.ID == locked.ID {
			return &drive.Error{Kind: drive.KindPermission, Status: 403}
		}
		return nil
	}

	before := e.snapshotWrites()
	a, fs := e.runAudit(col)
	after := e.snapshotWrites()
	if d := before.diff(after); len(d) != 0 {
		t.Fatalf("the audit wrote to Drive: %v", d)
	}
	if a.State != state.AuditCompleted || a.Scanned == 0 {
		t.Fatalf("audit = %+v", a)
	}

	// One unreadable folder makes the audit job partial, not successful.
	job, err := e.store.GetJob(context.Background(), "owner", a.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if job.State != state.JobPartial {
		t.Fatalf("audit job state = %s (%+v)", job.State, job)
	}

	for _, tc := range []struct{ kind, id string }{
		{"missing_at_destination", items["a.txt"].SourceID},
		{"name_mismatch", items["b.txt"].SourceID},
		{"wrong_parent", items["c.txt"].SourceID},
		{"size_or_md5_mismatch", items["d.txt"].SourceID},
		{"duplicate_at_destination", items["b.txt"].SourceID},
		{"extra_at_destination", stranger.ID},
		{"source_removed", items["e.txt"].SourceID},
		{"unreadable_source", locked.ID},
	} {
		if findKind(fs, tc.kind, tc.id) == nil {
			t.Errorf("no %s finding for %s in %+v", tc.kind, tc.id, fs)
		}
	}
	notRepairable := []struct{ kind, id string }{
		{"extra_at_destination", stranger.ID},
		{"source_removed", items["e.txt"].SourceID},
		{"unreadable_source", locked.ID},
	}
	for _, tc := range notRepairable {
		if f := findKind(fs, tc.kind, tc.id); f != nil && f.Repairable {
			t.Errorf("%s must never be repairable", tc.kind)
		}
	}
	if f := findKind(fs, "missing_at_destination", items["a.txt"].SourceID); f == nil || !f.Repairable {
		t.Error("missing_at_destination must exist and be repairable")
	}
	if got := a.Counts["missing_at_destination"]; got != 1 {
		t.Errorf("counts = %v", a.Counts)
	}
}

func TestAuditRestartsAfterAKillAndKeepsTheMapping(t *testing.T) {
	e := newEnv(t)
	_, col, items := e.copyTree("Photos")

	// Kill the daemon while it is inside the source walk.
	reached, release := make(chan struct{}), make(chan struct{})
	e.fake.Before = func(op string, f drive.File) error {
		if op == "list" {
			select {
			case <-reached:
			default:
				close(reached)
			}
			<-release
		}
		return nil
	}
	a, job, _, err := e.store.AdmitAudit(context.Background(), "owner", "owner", "k", col.ID)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	eng := e.engine()
	go func() { eng.Run(ctx); close(done) }()
	<-reached
	cancel()
	close(release)
	<-done
	e.fake.Before = nil

	// The audit job is stranded in a running state and comes back on restart.
	stranded, err := e.store.GetJob(context.Background(), "owner", job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.IsTerminal(stranded.State) {
		t.Fatalf("stranded audit settled by itself: %s", stranded.State)
	}
	e.runUntil(job.ID, terminal)
	got, err := e.store.GetAudit(context.Background(), "owner", a.ID)
	if err != nil || got.State != state.AuditCompleted {
		t.Fatalf("audit after restart = %+v err=%v", got, err)
	}
	// The mapping was never touched by the audit.
	for _, it := range items {
		after := e.itemOf(col.ID, it.SourceID)
		if after.State != state.ItemDone || after.DestID != it.DestID {
			t.Fatalf("audit mutated item %s: %+v", it.SourceID, after)
		}
	}
}

func TestRepairDryRunWritesNothing(t *testing.T) {
	e := newEnv(t)
	_, col, items := e.copyTree("Photos")
	e.fake.Mutate(items["a.txt"].DestID, func(f *drive.File) { f.Trashed = true })

	a, fs := e.runAudit(col)
	if len(fs) == 0 {
		t.Fatal("fixture produced no findings")
	}
	before := e.snapshotWrites()
	job := e.runRepair(a, true)
	after := e.snapshotWrites()
	if d := before.diff(after); len(d) != 0 {
		t.Fatalf("a dry-run repair wrote to Drive: %v", d)
	}
	if job.State != state.JobSucceeded {
		t.Fatalf("dry-run job = %+v", job)
	}
	after2, err := e.store.GetAudit(context.Background(), "owner", a.ID)
	if err != nil || after2.Revision != a.Revision+1 || len(after2.Plan) == 0 {
		t.Fatalf("audit after dry-run = %+v err=%v", after2, err)
	}
	// The mapping still shows the hole: nothing was applied.
	it := e.itemOf(col.ID, items["a.txt"].SourceID)
	if it.DestID == "" || it.State != state.ItemDone {
		t.Fatalf("dry-run changed the mapping: %+v", it)
	}
}

func TestRepairFixesWhatIsSafeAndLeavesTheRest(t *testing.T) {
	e := newEnv(t)
	_, col, items := e.copyTree("Photos")

	e.fake.Mutate(items["a.txt"].DestID, func(f *drive.File) { f.Trashed = true })
	e.fake.Mutate(items["b.txt"].DestID, func(f *drive.File) { f.Name = "b-renamed.txt" })
	sz := int64(35)
	e.fake.Mutate(items["c.txt"].DestID, func(f *drive.File) { f.Size = &sz })
	dSrc, _ := e.fake.Get(items["d.txt"].SourceID)
	dup, err := e.fake.CopyFile(context.Background(), dSrc, *col.RootID, items["d.txt"].IntentKey)
	if err != nil {
		t.Fatal(err)
	}
	stranger := e.fake.File("stranger.txt", *col.RootID, 3)
	e.fake.Mutate(items["e.txt"].SourceID, func(f *drive.File) { f.Trashed = true })

	a, _ := e.runAudit(col)
	before := e.snapshotWrites()
	job := e.runRepair(a, false)
	after := e.snapshotWrites()
	if job.State != state.JobSucceeded {
		t.Fatalf("repair job = %+v", job)
	}

	// a: copied again through the normal intent path, under the right parent.
	ita := e.itemOf(col.ID, items["a.txt"].SourceID)
	if ita.State != state.ItemDone || ita.DestID == items["a.txt"].DestID {
		t.Fatalf("missing item not re-copied: %+v", ita)
	}
	if f, ok := e.fake.Get(ita.DestID); !ok || f.Name != "a.txt" || len(f.Parents) != 1 || f.Parents[0] != items["a.txt"].DestParentID {
		t.Fatalf("re-copied file is wrong: %+v", f)
	}
	// b: renamed back in place (same object id).
	fb, _ := e.fake.Get(items["b.txt"].DestID)
	if fb.Name != "b.txt" {
		t.Fatalf("b not renamed back: %+v", fb)
	}
	// c: re-copied and the bad old copy trashed (never deleted).
	itc := e.itemOf(col.ID, items["c.txt"].SourceID)
	if itc.DestID == "" || itc.DestID == items["c.txt"].DestID {
		t.Fatalf("mismatch item not re-copied: %+v", itc)
	}
	if fc, _ := e.fake.Get(items["c.txt"].DestID); !fc.Trashed {
		t.Fatalf("old wrong copy was not trashed: %+v", fc)
	}
	// d: the duplicate is trashed, the mapped copy lives.
	if fd, _ := e.fake.Get(dup.ID); !fd.Trashed {
		t.Fatalf("duplicate was not trashed: %+v", fd)
	}
	if fd, _ := e.fake.Get(items["d.txt"].DestID); fd.Trashed || fd.Name != "d.txt" {
		t.Fatalf("the mapped copy was touched: %+v", fd)
	}
	// stranger: untouched (no intent of ours).
	if fst, _ := e.fake.Get(stranger.ID); fst.ID == "" || fst.Trashed {
		t.Fatalf("a foreign object was touched: %+v", fst)
	}
	// removed source: the destination copy stays, nothing deleted.
	if fe, _ := e.fake.Get(items["e.txt"].DestID); fe.ID == "" || fe.Trashed {
		t.Fatalf("the copy of a removed source was touched: %+v", fe)
	}
	if d := before.diff(after); d["trash"] < 1 {
		t.Fatalf("no trash calls happened: %v", d)
	}

	// Findings record what was repaired.
	fs2, _, err := e.store.ListFindings(context.Background(), a.ID, "", 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ kind, id string }{
		{"missing_at_destination", items["a.txt"].SourceID},
		{"name_mismatch", items["b.txt"].SourceID},
		{"size_or_md5_mismatch", items["c.txt"].SourceID},
		{"duplicate_at_destination", items["d.txt"].SourceID},
	} {
		f := findKind(fs2, tc.kind, tc.id)
		if f == nil || !f.Repaired {
			t.Errorf("finding %s not marked repaired: %+v", tc.kind, f)
		}
	}
}

func TestRepairTwiceIsANoOp(t *testing.T) {
	e := newEnv(t)
	_, col, items := e.copyTree("Photos")
	e.fake.Mutate(items["a.txt"].DestID, func(f *drive.File) { f.Trashed = true })

	a, _ := e.runAudit(col)
	e.runRepair(a, false)
	a2, err := e.store.GetAudit(context.Background(), "owner", a.ID)
	if err != nil {
		t.Fatal(err)
	}
	before := e.snapshotWrites()
	writesBefore := e.fake.Writes
	job := e.runRepair(a2, false) // second run: everything is already fixed
	after := e.snapshotWrites()
	if d := before.diff(after); len(d) != 0 {
		t.Fatalf("the second repair wrote to Drive: %v", d)
	}
	if e.fake.Writes != writesBefore {
		t.Fatalf("second repair changed Drive: %d -> %d", writesBefore, e.fake.Writes)
	}
	if job.State != state.JobSucceeded {
		t.Fatalf("second repair job = %+v", job)
	}
}

func TestRepairTrashFailureIsRetriedOnTheNextRun(t *testing.T) {
	e := newEnv(t)
	_, col, items := e.copyTree("Photos")
	sz := int64(35)
	e.fake.Mutate(items["c.txt"].DestID, func(f *drive.File) { f.Size = &sz })

	a, _ := e.runAudit(col)
	// The trash of the superseded copy fails: the repair still re-copies,
	// reports the leftover honestly and never deletes.
	e.fake.Before = func(op string, f drive.File) error {
		if op == "trash" {
			return &drive.Error{Kind: drive.KindUnavailable, Status: 503}
		}
		return nil
	}
	job := e.runRepair(a, false)
	e.fake.Before = nil
	if job.State == state.JobSucceeded {
		t.Fatalf("a failed cleanup reported success: %+v", job)
	}
	if fc, _ := e.fake.Get(items["c.txt"].DestID); fc.ID == "" {
		t.Fatal("the old copy was removed instead of trashed")
	}
	itc := e.itemOf(col.ID, items["c.txt"].SourceID)
	if itc.DestID == items["c.txt"].DestID {
		t.Fatalf("the re-copy did not happen: %+v", itc)
	}

	// Second repair run finishes the cleanup: exactly one live copy remains.
	a2, err := e.store.GetAudit(context.Background(), "owner", a.ID)
	if err != nil {
		t.Fatal(err)
	}
	job2 := e.runRepair(a2, false)
	if job2.State != state.JobSucceeded {
		t.Fatalf("second repair job = %+v", job2)
	}
	// The re-copy carries a new generation's intent; read it fresh.
	want := e.itemOf(col.ID, items["c.txt"].SourceID).IntentKey
	count := 0
	var walk func(id string)
	walk = func(id string) {
		for _, c := range e.fake.Children(id) {
			if c.AppProperties[drive.IntentProperty] == want && !c.Trashed {
				count++
			}
			walk(c.ID)
		}
	}
	walk(*col.RootID)
	if count != 1 {
		t.Fatalf("live copies with the intent: %d", count)
	}
}

func TestAuditHonorsPauseAndCancel(t *testing.T) {
	e := newEnv(t)
	_, col, _ := e.copyTree("Photos")

	// Cancel while queued settles without touching Drive.
	_, jobA, _, err := e.store.AdmitAudit(context.Background(), "owner", "owner", "cancel-me", col.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.store.Action(context.Background(), "owner", "cancel-key", jobA.ID, "cancel", jobA.Revision); err != nil {
		t.Fatal(err)
	}
	j := e.runUntil(jobA.ID, terminal)
	if j.State != state.JobCancelled {
		t.Fatalf("cancelled audit = %+v", j)
	}

	// Pause lands between folders and the audit resumes to completion.
	a, jobB, _, err := e.store.AdmitAudit(context.Background(), "owner", "owner", "pause-me", col.ID)
	if err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	e.fake.Before = func(op string, f drive.File) error {
		if op == "list" {
			<-release
		}
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	eng := e.engine()
	go func() { eng.Run(ctx); close(done) }()
	if _, err = e.store.Action(context.Background(), "owner", "pause-key", jobB.ID, "pause", jobB.Revision); err != nil {
		t.Fatal(err)
	}
	close(release)
	paused := e.runUntil(jobB.ID, func(j state.Job) bool { return j.State == state.JobPaused })
	if paused.State != state.JobPaused {
		t.Fatalf("pause did not land: %+v", paused)
	}
	cancel()
	<-done
	e.fake.Before = nil

	if _, err = e.store.Action(context.Background(), "owner", "resume-key", jobB.ID, "resume", paused.Revision); err != nil {
		t.Fatal(err)
	}
	j2 := e.runUntil(jobB.ID, terminal)
	if j2.State != state.JobSucceeded {
		t.Fatalf("resumed audit = %+v", j2)
	}
	got, err := e.store.GetAudit(context.Background(), "owner", a.ID)
	if err != nil || got.State != state.AuditCompleted {
		t.Fatalf("audit after resume = %+v err=%v", got, err)
	}
}
