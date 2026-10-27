package engine

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/itsgiahung/502Drive/internal/drive"
	"github.com/itsgiahung/502Drive/internal/state"
)

// A guardrail for the ownership rules the review found missing: a repair, an
// align or a replace_copy retirement may only touch an object that still
// carries the intent 502Drive wrote on it.
func TestSyncNeverTrashesACopyThatLostOurIntent(t *testing.T) {
	e := newEnv(t)
	src := e.fake.Folder("P", "root")
	f := e.fake.File("doc", src.ID, 1)
	base, root := e.baseline(src)
	c := mustColl(e, base.CollectionID)
	if _, _, err := e.store.ConfigureSync(context.Background(), "owner", "", "cfg", base.CollectionID,
		state.SyncChange{Mode: state.ModeRealtime, ContentPolicy: state.PolicyReplace, ExpectedCollection: c.Revision, Cursor: "0"}); err != nil {
		t.Fatal(err)
	}
	e.runUntil(findRun(e, base.CollectionID), terminal) // the reconcile run ConfigureSync admits

	old := find(e.fake, root, "doc")
	if len(old) != 1 {
		t.Fatalf("baseline copies = %d", len(old))
	}
	e.fake.Mutate(f.ID, func(x *drive.File) { x.Version = "9"; x.MD5 = "new" })
	// The user took the old copy out of our hands: its marker is gone.
	e.fake.Mutate(old[0].ID, func(x *drive.File) { x.AppProperties = nil })
	if j := e.runUntil(e.runNow(base.CollectionID).ID, terminal); j.State != state.JobSucceeded {
		t.Fatalf("replace run = %s %+v", j.State, j.LastError)
	}
	if n := len(find(e.fake, root, "doc")); n != 2 {
		t.Fatalf("replace_copy left %d copies, want both", n)
	}

	// The next run retires superseded generations; this one is not ours.
	e.runUntil(e.runNow(base.CollectionID).ID, terminal)
	if x, ok := e.fake.Get(old[0].ID); !ok || x.Trashed {
		t.Fatalf("trashed a copy that no longer carries our intent: %+v", x)
	}

	// With the marker back, the superseded generation is trashed again.
	e.fake.Mutate(old[0].ID, func(x *drive.File) {
		x.AppProperties = map[string]string{drive.IntentProperty: "carried-over"}
	})
	e.runUntil(e.runNow(base.CollectionID).ID, terminal)
	if x, ok := e.fake.Get(old[0].ID); !ok || !x.Trashed {
		t.Fatalf("superseded copy not retired: %+v", x)
	}
}

// align renames or moves an existing object; it must re-check the intent at the
// point of the write, not trust the scan that queued it.
func TestAlignLeavesAnObjectThatLostOurIntentAlone(t *testing.T) {
	e := newEnv(t)
	src := e.fake.Folder("P", "root")
	dir := e.fake.Folder("dirA", src.ID)
	e.fake.File("inside", dir.ID, 1)
	base, root := e.baseline(src)
	dest := find(e.fake, root, "dirA")
	if len(dest) != 1 {
		t.Fatalf("baseline copies = %d", len(dest))
	}
	e.fake.Mutate(dir.ID, func(x *drive.File) { x.Name = "dirA-renamed" })
	e.fake.Mutate(dest[0].ID, func(x *drive.File) { x.AppProperties = nil })

	j := e.runUntil(e.runNow(base.CollectionID).ID, terminal)
	if j.State != state.JobPartial {
		t.Fatalf("align of a foreign object finished %s", j.State)
	}
	if x, _ := e.fake.Get(dest[0].ID); x.Name != "dirA" {
		t.Fatalf("renamed an object that no longer carries our intent: %+v", x)
	}
	if n := e.fake.Calls["rename"] + e.fake.Calls["move"]; n != 0 {
		t.Fatalf("wrote %d rename/move calls", n)
	}
	it := e.itemOf(base.CollectionID, dir.ID)
	if it.State != state.ItemFailed || it.ErrorCode != "destination_not_ours" {
		t.Fatalf("item = %+v", it)
	}
}

// The repair planner re-checks every finding before acting on it: a marker
// that disappeared after the audit must not reach the write plan.
func TestRepairWillNotAlignACopyThatLostOurIntent(t *testing.T) {
	e := newEnv(t)
	_, col, items := e.copyTree("Photos")
	e.fake.Mutate(items["b.txt"].DestID, func(f *drive.File) { f.Name = "b-renamed.txt" })
	a, _ := e.runAudit(col)
	e.fake.Mutate(items["b.txt"].DestID, func(f *drive.File) { f.AppProperties = nil })

	j := e.runRepair(a, false)
	if x, _ := e.fake.Get(items["b.txt"].DestID); x.Name != "b-renamed.txt" {
		t.Fatalf("repair renamed an object it cannot prove it made: %+v", x)
	}
	if n := e.fake.Calls["rename"] + e.fake.Calls["move"]; n != 0 {
		t.Fatalf("repair wrote %d rename/move calls", n)
	}
	if j.State == state.JobSucceeded {
		t.Fatalf("repair claimed success without repairing the finding: %+v", j)
	}
}

// Reset may only resend a write after a successful lookup confirmed no match:
// a reconciliation that never reached Drive proves nothing.
func TestRetryNeverResendsWithoutASuccessfulLookup(t *testing.T) {
	e := newEnv(t)
	f := e.fake.File("report.pdf", "root", 99)
	lost := false
	e.fake.LoseResponse = func(op string, _ drive.File) bool {
		if op == "copy" && !lost {
			lost = true
			return true
		}
		return false
	}
	e.fake.Before = func(op string, _ drive.File) error {
		if op == "find" {
			return &drive.Error{Kind: drive.KindServer, Status: 503}
		}
		return nil
	}
	j := e.runUntil(e.submit(f).ID, terminal)
	if j.State != state.JobBlocked {
		t.Fatalf("first run = %s %+v", j.State, j.LastError)
	}
	copies := e.fake.CallCount("copy")
	if copies != 1 {
		t.Fatalf("copies = %d", copies)
	}

	// The user retries while the intent lookup still fails: no resend.
	if _, err := e.store.Action(context.Background(), "owner", "retry-1", j.ID, "retry", j.Revision); err != nil {
		t.Fatal(err)
	}
	j2 := e.runUntil(j.ID, terminal)
	if j2.State != state.JobBlocked {
		t.Fatalf("retry without a confirmed outcome = %s %+v", j2.State, j2.LastError)
	}
	if got := e.fake.CallCount("copy"); got != copies {
		t.Fatalf("resent a copy after a failed reconciliation: %d -> %d", copies, got)
	}
	it := e.itemOf(j.CollectionID, f.ID)
	if it.State != state.ItemBlocked || it.ErrorCode != "unknown_remote_outcome" || it.IntentKey == "" {
		t.Fatalf("item after retry = %+v", it)
	}

	// Once the lookup works again the existing copy is adopted, not duplicated.
	e.fake.Before = nil
	if _, err := e.store.Action(context.Background(), "owner", "retry-2", j.ID, "retry", j2.Revision); err != nil {
		t.Fatal(err)
	}
	j3 := e.runUntil(j.ID, terminal)
	if j3.State != state.JobSucceeded {
		t.Fatalf("reconciled retry = %s %+v", j3.State, j3.LastError)
	}
	if got := e.fake.CallCount("copy"); got != copies {
		t.Fatalf("duplicated an existing copy: %d -> %d", copies, got)
	}
}

// A repair cut off between files.copy and the mapping commit must reconcile the
// generation it already prepared, never prepare another one.
func TestRepairRestartReconcilesThePreparedGeneration(t *testing.T) {
	e := newEnv(t)
	_, col, items := e.copyTree("Photos")
	e.fake.Mutate(items["a.txt"].DestID, func(f *drive.File) { f.Trashed = true })
	a, _ := e.runAudit(col)

	// The repair sends files.copy, Google creates the copy, the reply is lost
	// and the lookups that would find it keep failing until the process dies.
	e.fake.LoseResponse = func(op string, _ drive.File) bool { return op == "copy" }
	reached := make(chan struct{})
	var once sync.Once
	e.fake.Before = func(op string, _ drive.File) error {
		if op == "find" {
			once.Do(func() { close(reached) })
			return &drive.Error{Kind: drive.KindUnavailable}
		}
		return nil
	}
	e.n++
	job, _, err := e.store.AdmitRepair(context.Background(), "owner", "owner", "repair-crash", a.ID, a.Revision, false)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	eng := e.engine()
	go func() { eng.Run(ctx); close(done) }()
	select {
	case <-reached:
	case <-time.After(10 * time.Second):
		t.Fatal("the repair never sent a copy")
	}
	cancel()
	<-done
	e.fake.Before = nil
	e.fake.LoseResponse = nil

	it := e.itemOf(col.ID, items["a.txt"].SourceID)
	if it.State != state.ItemReconciling || it.IntentKey == "" {
		t.Fatalf("item after the crash = %+v", it)
	}
	copies := e.fake.CallCount("copy")

	// Restart: the stranded repair reconciles the prepared generation first.
	j := e.runUntil(job.ID, terminal)
	if j.State != state.JobSucceeded {
		t.Fatalf("repair after restart = %s %+v", j.State, j.LastError)
	}
	if got := e.fake.CallCount("copy"); got != copies {
		t.Fatalf("repair sent another copy instead of reconciling: %d -> %d", copies, got)
	}
	it = e.itemOf(col.ID, items["a.txt"].SourceID)
	if it.State != state.ItemDone || it.DestID == "" {
		t.Fatalf("prepared generation not adopted: %+v", it)
	}
	if x, ok := e.fake.Get(it.DestID); !ok || x.Trashed {
		t.Fatalf("mapped copy = %+v ok=%v", x, ok)
	}
}

// A copy sitting in the trash is missing, not repaired: files.get still returns
// its metadata with trashed=true.
func TestRepairRecopiesACopyThatIsInTheTrash(t *testing.T) {
	e := newEnv(t)
	_, col, items := e.copyTree("Photos")
	e.fake.TrashedVisible = true
	e.fake.Mutate(items["a.txt"].DestID, func(f *drive.File) { f.Trashed = true })

	a, fs := e.runAudit(col)
	if findKind(fs, state.FindingMissing, items["a.txt"].SourceID) == nil {
		t.Fatalf("no missing finding: %+v", fs)
	}
	j := e.runRepair(a, false)
	if j.State != state.JobSucceeded {
		t.Fatalf("repair = %s %+v", j.State, j.LastError)
	}
	it := e.itemOf(col.ID, items["a.txt"].SourceID)
	if it.State != state.ItemDone || it.DestID == "" || it.DestID == items["a.txt"].DestID {
		t.Fatalf("a trashed copy counted as repaired: %+v", it)
	}
	if x, ok := e.fake.Get(it.DestID); !ok || x.Trashed {
		t.Fatalf("new copy = %+v ok=%v", x, ok)
	}
}

// The audit reads every page of every folder: dropping nextPageToken hides a
// whole subtree and reports false divergences.
func TestAuditReadsEveryPageOfEveryFolder(t *testing.T) {
	e := newEnv(t)
	_, col, _ := e.copyTree("Photos")
	e.fake.PageSize = 1 // every folder needs several pages

	a, fs := e.runAudit(col)
	if a.State != state.AuditCompleted {
		t.Fatalf("audit = %+v", a)
	}
	if len(fs) != 0 {
		t.Fatalf("a healthy tree produced findings: %+v", fs)
	}
}

// A saved page token can expire. Retrying it forever leaves the folder only
// partly listed; the listing must restart from its first page.
func TestRejectedPageTokenRestartsTheListing(t *testing.T) {
	e := newEnv(t)
	e.fake.PageSize = 2
	src := e.fake.Folder("Big", "root")
	for i := range 5 {
		e.fake.File(fmt.Sprintf("f%d", i), src.ID, int64(i+1))
	}
	var mu sync.Mutex
	lists := 0
	e.fake.Before = func(op string, _ drive.File) error {
		if op != "list" {
			return nil
		}
		mu.Lock()
		defer mu.Unlock()
		lists++
		if lists >= 2 {
			return &drive.Error{Kind: drive.KindServer, Status: 503}
		}
		return nil
	}
	j := e.runUntil(e.submit(src).ID, terminal)
	e.fake.Before = nil
	if j.State != state.JobPartial {
		t.Fatalf("first run = %s %+v", j.State, j.LastError)
	}
	it := e.itemOf(j.CollectionID, src.ID)
	if it.ListState != "failed" || it.PageToken == "" {
		t.Fatalf("listing checkpoint = %+v", it)
	}

	// The saved token expires; every request that carries it is rejected.
	e.fake.TokenEpoch++
	if _, err := e.store.Action(context.Background(), "owner", "retry", j.ID, "retry", j.Revision); err != nil {
		t.Fatal(err)
	}
	j2 := e.runUntil(j.ID, terminal)
	if j2.State != state.JobSucceeded {
		t.Fatalf("retry with a rejected page token = %s %+v", j2.State, j2.LastError)
	}
	if n := len(e.fake.Children(*mustColl(e, j.CollectionID).RootID)); n != 5 {
		t.Fatalf("copied %d of 5 files", n)
	}
	if x := e.itemOf(j.CollectionID, src.ID); x.ListState != "done" || x.PageToken != "" {
		t.Fatalf("listing did not finish: %+v", x)
	}
}

// A read that failed is not proof that a copy came back: the finding stays open
// and the repair does not claim success.
func TestRepairDoesNotCountAFailedReadAsFixed(t *testing.T) {
	e := newEnv(t)
	_, col, items := e.copyTree("Photos")
	e.fake.TrashedVisible = true
	e.fake.Mutate(items["a.txt"].DestID, func(f *drive.File) { f.Trashed = true })
	a, _ := e.runAudit(col)

	e.fake.Before = func(op string, f drive.File) error {
		if op == "get" && f.ID == items["a.txt"].DestID {
			return &drive.Error{Kind: drive.KindServer, Status: 503}
		}
		return nil
	}
	j := e.runRepair(a, false)
	e.fake.Before = nil
	if j.State == state.JobSucceeded {
		t.Fatalf("repair reported success without confirming the copy: %+v", j)
	}
	fs2, _, err := e.store.ListFindings(context.Background(), a.ID, "", 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	if f := findKind(fs2, state.FindingMissing, items["a.txt"].SourceID); f == nil || f.Repaired {
		t.Fatalf("a failed read counted as proof of repair: %+v", f)
	}
	it := e.itemOf(col.ID, items["a.txt"].SourceID)
	if it.State != state.ItemDone || it.DestID != items["a.txt"].DestID {
		t.Fatalf("the mapping was re-armed from an unverified finding: %+v", it)
	}
}
