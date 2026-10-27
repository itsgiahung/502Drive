package state

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// bootstrappingSync creates the smallest realtime subscription whose baseline
// reconcile run has not finished yet, so changes read from the feed must be
// stored instead of applied.
func bootstrappingSync(t *testing.T, s *Store, now time.Time) (Collection, Subscription, string) {
	t.Helper()
	col, _ := auditFixture(t, s)
	sub, _, err := s.ConfigureSync(context.Background(), "owner", "", "cfg", col.ID, SyncChange{
		Mode: ModeRealtime, ContentPolicy: PolicyVersioned, ExpectedCollection: col.Revision,
		Cursor: "7", Now: now, FullReconcileEvery: int64(DefaultReconcileForTests / time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	if sub.State != SyncBootstrapping {
		t.Fatalf("state = %q, want bootstrapping", sub.State)
	}
	job, err := s.ActiveRunID(context.Background(), col.ID)
	if err != nil || job == "" {
		t.Fatalf("reconcile job = %q err=%v", job, err)
	}
	return col, sub, job
}

const DefaultReconcileForTests = 7 * 24 * time.Hour

// activate ends the baseline reconcile and lets the subscription become active.
func activate(t *testing.T, s *Store, col Collection, job string) Subscription {
	t.Helper()
	ctx := context.Background()
	if _, err := s.UpdateJob(ctx, job, func(j *Job) (string, bool) {
		j.State = JobSucceeded
		return "finished", true
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunFinished(ctx, job); err != nil {
		t.Fatal(err)
	}
	sub, err := s.GetSubscription(ctx, col.ID)
	if err != nil {
		t.Fatal(err)
	}
	if sub.State != SyncActive {
		t.Fatalf("state after baseline = %q", sub.State)
	}
	return sub
}

func pendingCount(t *testing.T, s *Store, collectionID string) int {
	t.Helper()
	var n int
	if err := s.db.QueryRowContext(context.Background(), "SELECT count(*) FROM sync_pending_changes WHERE collection_id=?", collectionID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// A database at schema 006 with data upgrades to 007 with the data intact and a
// clean foreign_key_check.
func TestMigration007UpgradesFromSchema006WithData(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "state-v2.db"))
	if err != nil {
		t.Fatal(err)
	}
	{
		ms, err := loadMigrations()
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range ms {
			if m.version > 6 {
				break
			}
			if _, err = db.Exec(m.sql); err != nil {
				t.Fatalf("migration %d: %v", m.version, err)
			}
		}
		if _, err = db.Exec("PRAGMA user_version=6; PRAGMA application_id=502002"); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec("INSERT INTO plans VALUES(?,?,?,NULL)", "plan1", "owner", []byte(`{"id":"plan1"}`)); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec("INSERT INTO collections VALUES(?,?,?,?)", "col1", "owner", "plan1", []byte(`{"id":"col1","source_id":"src","source_kind":"folder","state":"ready"}`)); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(`INSERT INTO sync_subscriptions(collection_id,id,mode,state,cursor)
			VALUES('col1','sub1','realtime','bootstrapping','42')`); err != nil {
			t.Fatal(err)
		}
		if err = db.Close(); err != nil {
			t.Fatal(err)
		}
	}

	s, err := Open(dir)
	if err != nil {
		t.Fatalf("open after migration: %v", err)
	}
	defer s.Close()
	var ver int
	if err = s.db.QueryRowContext(context.Background(), "PRAGMA user_version").Scan(&ver); err != nil || ver < 7 {
		t.Fatalf("user_version = %d err=%v", ver, err)
	}
	sub, err := s.GetSubscription(context.Background(), "col1")
	if err != nil {
		t.Fatal(err)
	}
	if sub.Cursor != "42" || sub.FullReconcileEvery != int64((7*24*time.Hour)/time.Second) || sub.NextFullReconcileAt != 0 || sub.ReplayOverflow {
		t.Fatalf("upgraded subscription = %+v", sub)
	}
	fk, err := s.db.QueryContext(context.Background(), "PRAGMA foreign_key_check")
	if err != nil {
		t.Fatal(err)
	}
	if fk.Next() {
		t.Fatal("foreign_key_check is not clean")
	}
	fk.Close()
	// The new table is usable through the normal path.
	if _, err = s.AppendPendingChanges(context.Background(), "col1", []ReplayChange{{FileID: "f1", Parents: []string{"src"}}}, "43", time.Now()); err != nil {
		t.Fatalf("append on upgraded db: %v", err)
	}
	if n := pendingCount(t, s, "col1"); n != 1 {
		t.Fatalf("pending rows = %d", n)
	}
}

func TestAppendPendingChangesRespectsTheRowCap(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	col, _, _ := bootstrappingSync(t, s, time.Now())

	batch := make([]ReplayChange, MaxReplayChanges)
	for i := range batch {
		batch[i] = ReplayChange{FileID: "f" + strings.Repeat("x", 4) + strconv.Itoa(i), Parents: []string{"src"}}
	}
	overflow, err := s.AppendPendingChanges(ctx, col.ID, batch, "100", time.Now())
	if err != nil || overflow {
		t.Fatalf("append at the cap: overflow=%v err=%v", overflow, err)
	}
	if n := pendingCount(t, s, col.ID); n != MaxReplayChanges {
		t.Fatalf("rows = %d, want %d", n, MaxReplayChanges)
	}
	overflow, err = s.AppendPendingChanges(ctx, col.ID, []ReplayChange{{FileID: "one-too-many"}}, "101", time.Now())
	if err != nil || !overflow {
		t.Fatalf("append over the cap: overflow=%v err=%v", overflow, err)
	}
	if n := pendingCount(t, s, col.ID); n != MaxReplayChanges {
		t.Fatalf("overflow stored a row: %d", n)
	}
	sub, err := s.GetSubscription(ctx, col.ID)
	if err != nil || !sub.ReplayOverflow || sub.Cursor != "7" {
		t.Fatalf("subscription after overflow = %+v err=%v", sub, err)
	}
}

func TestAppendPendingChangesRespectsTheByteCap(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	col, _, _ := bootstrappingSync(t, s, time.Now())

	// Each change marshals to slightly more than 4 KiB, so ~1024 of them hit
	// the 4 MiB cap even though the row cap is far away.
	batch := make([]ReplayChange, MaxReplayChanges)
	for i := range batch {
		batch[i] = ReplayChange{FileID: strings.Repeat("a", 4096)}
	}
	overflow, err := s.AppendPendingChanges(ctx, col.ID, batch, "9", time.Now())
	if err != nil || !overflow {
		t.Fatalf("append: overflow=%v err=%v", overflow, err)
	}
	n := pendingCount(t, s, col.ID)
	if n == 0 || n == MaxReplayChanges {
		t.Fatalf("byte cap stored %d rows", n)
	}
	var bytes int64
	if err := s.db.QueryRowContext(ctx, "SELECT sum(length(change_json)) FROM sync_pending_changes WHERE collection_id=?", col.ID).Scan(&bytes); err != nil {
		t.Fatal(err)
	}
	if bytes > MaxReplayBytes {
		t.Fatalf("stored %d bytes over the cap", bytes)
	}
}

func TestLastPendingCursorTracksTheNewestPage(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	col, _, _ := bootstrappingSync(t, s, time.Now())

	if _, ok, err := s.LastPendingCursor(ctx, col.ID); err != nil || ok {
		t.Fatalf("empty buffer: ok=%v err=%v", ok, err)
	}
	if _, err := s.AppendPendingChanges(ctx, col.ID, []ReplayChange{{FileID: "a"}}, "11", time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendPendingChanges(ctx, col.ID, []ReplayChange{{FileID: "b"}}, "12", time.Now()); err != nil {
		t.Fatal(err)
	}
	last, ok, err := s.LastPendingCursor(ctx, col.ID)
	if err != nil || !ok || last != "12" {
		t.Fatalf("last = %q ok=%v err=%v", last, ok, err)
	}
}

// Replay advances the cursor, clears the buffer and admits exactly one run in a
// single transaction.
func TestFinishReplayMovesCursorAndAdmitsOneRun(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	col, sub, job := bootstrappingSync(t, s, time.Now())
	if _, err := s.AppendPendingChanges(ctx, col.ID, []ReplayChange{{FileID: "f1"}, {FileID: "f2"}}, "9", time.Now()); err != nil {
		t.Fatal(err)
	}
	sub = activate(t, s, col, job)
	before := jobsFor(t, s, col.ID)

	run, err := s.FinishReplay(ctx, col.ID, sub.Cursor, "9", true, time.Now().Add(time.Minute))
	if err != nil || run == nil {
		t.Fatalf("finish replay: run=%+v err=%v", run, err)
	}
	if run.Trigger != "realtime" {
		t.Fatalf("trigger = %q", run.Trigger)
	}
	after, err := s.GetSubscription(ctx, col.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Cursor != "9" || after.ReplayOverflow {
		t.Fatalf("subscription after replay = %+v", after)
	}
	if n := pendingCount(t, s, col.ID); n != 0 {
		t.Fatalf("pending rows = %d", n)
	}
	if got := jobsFor(t, s, col.ID); got != before+1 {
		t.Fatalf("jobs = %d, want %d", got, before+1)
	}
}

func TestFinishReplayKeepsTheBufferForAPausedSubscription(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	col, sub, job := bootstrappingSync(t, s, time.Now())
	if _, err := s.AppendPendingChanges(ctx, col.ID, []ReplayChange{{FileID: "f1"}}, "9", time.Now()); err != nil {
		t.Fatal(err)
	}
	sub = activate(t, s, col, job)
	now := time.Now().UTC()
	if _, _, err := s.SyncAction(ctx, "owner", "", "pause-1", col.ID, "pause", sub.Revision, now, nil); err != nil {
		t.Fatal(err)
	}

	if _, err := s.FinishReplay(ctx, col.ID, sub.Cursor, "9", true, now); !errors.Is(err, ErrTransition) {
		t.Fatalf("finish replay on a paused subscription: %v", err)
	}
	if n := pendingCount(t, s, col.ID); n != 1 {
		t.Fatalf("paused replay dropped %d rows", n)
	}
	after, _ := s.GetSubscription(ctx, col.ID)
	if after.Cursor != "7" {
		t.Fatalf("cursor moved while paused: %q", after.Cursor)
	}
}

// A baseline that finished with an overflowed buffer asks for a full reconcile
// instead of activating with a known-incomplete feed.
func TestRunFinishedTurnsAnOverflowIntoAReconcile(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	col, _, job := bootstrappingSync(t, s, time.Now())
	if _, err := s.AppendPendingChanges(ctx, col.ID, []ReplayChange{{FileID: "a"}}, "9", time.Now()); err != nil {
		t.Fatal(err)
	}
	// Reaching the cap without writing 5000 rows: set the flag directly, the
	// append path is covered separately.
	if _, err := s.db.ExecContext(ctx, "UPDATE sync_subscriptions SET replay_overflow=1 WHERE collection_id=?", col.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateJob(ctx, job, func(j *Job) (string, bool) {
		j.State = JobSucceeded
		return "finished", true
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunFinished(ctx, job); err != nil {
		t.Fatal(err)
	}
	sub, err := s.GetSubscription(ctx, col.ID)
	if err != nil {
		t.Fatal(err)
	}
	if sub.State != SyncNeedsReconcile || sub.ReplayOverflow || sub.LastError == nil || sub.LastError.Code != "replay_overflow" {
		t.Fatalf("state=%q overflow=%v err=%+v", sub.State, sub.ReplayOverflow, sub.LastError)
	}
	if n := pendingCount(t, s, col.ID); n != 0 {
		t.Fatalf("overflow kept %d rows", n)
	}
}

// The periodic full reconcile is jittered by ±10% and never overlaps a run.
func TestClaimDueFullReconcilesSchedulesJitteredSlots(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	col, _, job := bootstrappingSync(t, s, now)
	sub := activate(t, s, col, job)

	every := time.Duration(sub.FullReconcileEvery) * time.Second
	want := time.UnixMilli(sub.NextFullReconcileAt).UTC()
	if d := want.Sub(now); d < every*9/10 || d > every*11/10 {
		t.Fatalf("first slot %v outside ±10%% of %v", d, every)
	}
	if jobs, err := s.ClaimDueFullReconciles(ctx, now.Add(every/2)); err != nil || len(jobs) != 0 {
		t.Fatalf("early claim: %d jobs err=%v", len(jobs), err)
	}
	due := now.Add(every * 11 / 10)
	jobs, err := s.ClaimDueFullReconciles(ctx, due)
	if err != nil || len(jobs) != 1 || jobs[0].Trigger != "full_reconcile" {
		t.Fatalf("due claim: %+v err=%v", jobs, err)
	}
	after, err := s.GetSubscription(ctx, col.ID)
	if err != nil {
		t.Fatal(err)
	}
	next := time.UnixMilli(after.NextFullReconcileAt).UTC()
	if d := next.Sub(due); d < every*9/10 || d > every*11/10 {
		t.Fatalf("next slot %v outside ±10%% of %v", d, every)
	}
	// The slot moved before the run started: a second tick cannot duplicate it.
	if jobs, err := s.ClaimDueFullReconciles(ctx, due); err != nil || len(jobs) != 0 {
		t.Fatalf("second claim: %d jobs err=%v", len(jobs), err)
	}
}

func TestFullReconcileCanBeDisabled(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	col, _ := auditFixture(t, s)
	sub, _, err := s.ConfigureSync(ctx, "owner", "", "off", col.ID, SyncChange{
		Mode: ModeRealtime, ContentPolicy: PolicyVersioned, ExpectedCollection: col.Revision,
		Cursor: "7", Now: now, FullReconcileEvery: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	if sub.FullReconcileEvery != 0 || sub.NextFullReconcileAt != 0 {
		t.Fatalf("disabled subscription = %+v", sub)
	}
	job, err := s.ActiveRunID(ctx, col.ID)
	if err != nil || job == "" {
		t.Fatal("no reconcile run")
	}
	activate(t, s, col, job)
	if jobs, err := s.ClaimDueFullReconciles(ctx, now.Add(100*365*24*time.Hour)); err != nil || len(jobs) != 0 {
		t.Fatalf("disabled claim: %d jobs err=%v", len(jobs), err)
	}
}

func jobsFor(t *testing.T, s *Store, collectionID string) int {
	t.Helper()
	var n int
	if err := s.db.QueryRowContext(context.Background(), "SELECT count(*) FROM jobs WHERE collection_id=?", collectionID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
