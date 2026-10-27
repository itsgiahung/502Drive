package state

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// queueJob admits one single-plan copy of its own source into batch (may be "").
func queueJob(t *testing.T, s *Store, n int, batch string) Job {
	t.Helper()
	ctx := context.Background()
	if _, err := s.GetDestination(ctx, "d1"); err != nil {
		if _, _, err = s.AddDestination(ctx, "owner", "k-dest", "dest", Destination{ID: "d1", FolderID: "root", Name: "My Drive"}); err != nil {
			t.Fatalf("destination: %v", err)
		}
	}
	planID := fmt.Sprintf("plan%d", n)
	if _, _, err := s.PutPlanIdempotent(ctx, "owner", "kp"+planID, "src"+planID, Plan{ID: planID, Principal: "owner", SourceID: "src" + planID,
		SourceName: "Src" + planID, SourceKind: KindFolder, DestinationID: "d1", RootName: "Src" + planID, ExpiresAt: time.Now().Add(time.Hour), Revision: 1}); err != nil {
		t.Fatalf("plan: %v", err)
	}
	res, err := s.AdmitInBatch(ctx, "owner", "kj"+planID, planID, 1, batch, nil)
	if err != nil {
		t.Fatalf("admit %d: %v", n, err)
	}
	return res.Job
}

func claimID(t *testing.T, s *Store) string {
	t.Helper()
	j, err := s.ClaimQueued(context.Background())
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	return j.ID
}

// engineSettle mimics the engine finishing a pause: running -> paused.
func engineSettle(t *testing.T, s *Store, id string) {
	t.Helper()
	if _, err := s.UpdateJob(context.Background(), id, func(j *Job) (string, bool) {
		j.State = JobPaused
		return "paused", true
	}); err != nil {
		t.Fatal(err)
	}
}

func TestQueueFairShareDoesNotLetBatchStarveSingleJob(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	a := make([]Job, 4)
	for i := range a {
		a[i] = queueJob(t, s, i+1, "batchAAAA1")
	}
	single := queueJob(t, s, 9, "")

	// FIFO for the first pick: the batch was admitted first.
	if got := claimID(t, s); got != a[0].ID {
		t.Fatalf("first claim = %s, want %s", got, a[0].ID)
	}
	// The batch now has one running job; the single job has none: it goes next,
	// ahead of the batch's second link.
	if got := claimID(t, s); got != single.ID {
		t.Fatalf("second claim = %s, want the single job %s", got, single.ID)
	}
	if got := claimID(t, s); got != a[1].ID {
		t.Fatalf("third claim = %s, want %s", got, a[1].ID)
	}
	if _, err := s.ClaimQueued(ctx); err != nil {
		t.Fatalf("fourth claim: %v", err)
	}
}

func TestQueuePriorityBeatsFIFOAndMoveRenumbers(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	j1 := queueJob(t, s, 1, "")
	j2 := queueJob(t, s, 2, "")
	j3 := queueJob(t, s, 3, "")

	// Move j3 before j1: order j3, j1, j2.
	moved, err := s.MoveJob(ctx, "owner", j3.ID, j3.Revision, Move{Before: j1.ID})
	if err != nil {
		t.Fatalf("move before: %v", err)
	}
	if moved.Job.Revision != j3.Revision+1 {
		t.Fatalf("move must bump the job revision: %d -> %d", j3.Revision, moved.Job.Revision)
	}
	if got := claimID(t, s); got != j3.ID {
		t.Fatalf("after move before, first claim = %s, want %s", got, j3.ID)
	}
	if got := claimID(t, s); got != j1.ID {
		t.Fatalf("second claim = %s, want %s", got, j1.ID)
	}
	if got := claimID(t, s); got != j2.ID {
		t.Fatalf("third claim = %s, want %s", got, j2.ID)
	}

	// Priority wins over position; the bounds are enforced.
	j4 := queueJob(t, s, 4, "")
	j5 := queueJob(t, s, 5, "")
	prio := 2
	if _, err = s.MoveJob(ctx, "owner", j5.ID, j5.Revision, Move{Priority: &prio}); err != nil {
		t.Fatalf("priority: %v", err)
	}
	if got := claimID(t, s); got != j5.ID {
		t.Fatalf("higher priority claim = %s, want %s", got, j5.ID)
	}
	bad := 9
	if _, err = s.MoveJob(ctx, "owner", j4.ID, j4.Revision, Move{Priority: &bad}); err == nil {
		t.Fatal("priority outside -5..5 must be refused")
	}
}

func TestMoveRefusesRunningJobAndStaleRevision(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	j1 := queueJob(t, s, 1, "")
	j2 := queueJob(t, s, 2, "")
	if got := claimID(t, s); got != j1.ID {
		t.Fatalf("claim = %s", got)
	}
	// The claim moved the job to revision 2, so revision 1 is stale.
	if _, err := s.MoveJob(ctx, "owner", j1.ID, j1.Revision, Move{Before: j2.ID}); err != ErrStale {
		t.Fatalf("stale move err = %v", err)
	}
	if _, err := s.MoveJob(ctx, "owner", j1.ID, j1.Revision, Move{}); err != ErrStale {
		t.Fatalf("stale move err = %v", err)
	}
	cur, _ := s.GetJob(ctx, "owner", j1.ID)
	if _, err := s.MoveJob(ctx, "owner", j1.ID, cur.Revision, Move{Before: j2.ID}); err != ErrTransition {
		t.Fatalf("running job move err = %v, want ErrTransition", err)
	}
}

func TestQueuePauseHoldsQueueAndPausesRunningWithoutLoss(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	j1 := queueJob(t, s, 1, "")
	j2 := queueJob(t, s, 2, "")
	if got := claimID(t, s); got != j1.ID {
		t.Fatalf("claim = %s", got)
	}

	res, err := s.SetQueuePaused(ctx, true)
	if err != nil {
		t.Fatalf("pause queue: %v", err)
	}
	if !res.Changed || res.Affected != 1 {
		t.Fatalf("pause result = %+v", res)
	}
	cur, _ := s.GetJob(ctx, "owner", j1.ID)
	if cur.State != JobPausing || cur.DesiredState != "pause" || !cur.QueuePaused {
		t.Fatalf("running job after queue pause = %s/%s queue_paused=%v", cur.State, cur.DesiredState, cur.QueuePaused)
	}
	if _, err = s.ClaimQueued(ctx); err == nil {
		t.Fatal("paused queue must not claim queued work")
	}
	// Admission still works while paused; the new job just waits.
	j3 := queueJob(t, s, 3, "")
	if cur, _ = s.GetJob(ctx, "owner", j3.ID); cur.State != JobQueued {
		t.Fatalf("admitted during pause = %s", cur.State)
	}
	// Idempotent: a second pause changes nothing.
	if res, err = s.SetQueuePaused(ctx, true); err != nil || res.Changed {
		t.Fatalf("second pause = %+v err=%v", res, err)
	}

	// The engine settles the running job at a safe point: pausing -> paused.
	engineSettle(t, s, j1.ID)
	if cur, _ = s.GetJob(ctx, "owner", j1.ID); cur.State != JobPaused {
		t.Fatalf("settled job = %s", cur.State)
	}

	// Resume returns the queue-paused job to the front of the line, unchanged.
	if res, err = s.SetQueuePaused(ctx, false); err != nil || !res.Changed || res.Affected != 1 {
		t.Fatalf("resume queue = %+v err=%v", res, err)
	}
	cur, _ = s.GetJob(ctx, "owner", j1.ID)
	if cur.State != JobQueued || cur.QueuePaused {
		t.Fatalf("resumed job = %s queue_paused=%v", cur.State, cur.QueuePaused)
	}
	if got := claimID(t, s); got != j1.ID {
		t.Fatalf("first claim after resume = %s, want %s", got, j1.ID)
	}
	if got := claimID(t, s); got != j2.ID {
		t.Fatalf("second claim = %s, want %s", got, j2.ID)
	}
}

func TestUserPausedJobIsNotResumedByQueueResume(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	j1 := queueJob(t, s, 1, "")
	claimID(t, s)
	cur, _ := s.GetJob(ctx, "owner", j1.ID)
	if _, err := s.Action(ctx, "owner", "k-user-pause", j1.ID, "pause", cur.Revision); err != nil {
		t.Fatalf("user pause: %v", err)
	}
	engineSettle(t, s, j1.ID)
	if _, err := s.SetQueuePaused(ctx, true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetQueuePaused(ctx, false); err != nil {
		t.Fatal(err)
	}
	if cur, _ := s.GetJob(ctx, "owner", j1.ID); cur.State != JobPaused {
		t.Fatalf("user-paused job was resumed by queue resume: %s", cur.State)
	}
}

func TestQueuePausedSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	queueJob(t, s, 1, "")
	if _, err = s.SetQueuePaused(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	s.Close()

	s, err = Open(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s.Close()
	if paused, err := s.QueuePaused(context.Background()); err != nil || !paused {
		t.Fatalf("queue_paused after restart = %v err=%v", paused, err)
	}
	if _, err = s.ClaimQueued(context.Background()); err == nil {
		t.Fatal("restarted daemon claimed work while the queue was paused")
	}
}

func TestBatchAdmissionReplayAndSummary(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	inBatch := queueJob(t, s, 1, "batchBBBB2")
	other := queueJob(t, s, 2, "batchCCCC3")
	single := queueJob(t, s, 3, "")

	// Replaying the same item key returns the same job, never a second one.
	again, err := s.AdmitInBatch(ctx, "owner", "kjplan1", "plan1", 1, "batchBBBB2", nil)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if !again.Replayed || again.Job.ID != inBatch.ID {
		t.Fatalf("replay = %+v, want job %s", again, inBatch.ID)
	}

	jobs, err := s.BatchJobs(ctx, "owner", "batchBBBB2", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].ID != inBatch.ID || jobs[0].BatchID != "batchBBBB2" {
		t.Fatalf("batch members = %+v", jobs)
	}
	if jobs, _ = s.BatchJobs(ctx, "intruder", "batchBBBB2", 100); len(jobs) != 0 {
		t.Fatal("batch lookup must be scoped to the principal")
	}
	_ = other
	_ = single
}

func TestQueueClaimUsesIndexes(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	queueJob(t, s, 1, "")
	plan, err := s.db.QueryContext(ctx, "EXPLAIN QUERY PLAN "+claimQueuedSQL)
	if err != nil {
		t.Fatal(err)
	}
	defer plan.Close()
	var lines []string
	for plan.Next() {
		var id, parent, notused int
		var detail string
		if err := plan.Scan(&id, &parent, &notused, &detail); err != nil {
			t.Fatal(err)
		}
		lines = append(lines, detail)
	}
	joined := strings.Join(lines, "\n")
	if strings.Contains(joined, "SCAN j") || strings.Contains(joined, "SCAN r") || strings.Contains(joined, "SCAN jobs") {
		t.Fatalf("claim query scans the jobs table:\n%s", joined)
	}
	if !strings.Contains(joined, "jobs_queue") || !strings.Contains(joined, "jobs_fair") {
		t.Fatalf("claim query does not use the queue indexes:\n%s", joined)
	}
}

func TestBatchMigrationUpgradesWithData(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "state-v2.db"))
	if err != nil {
		t.Fatal(err)
	}
	ms, err := loadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range ms {
		if m.version > 20 {
			break
		}
		if _, err = db.Exec(m.sql); err != nil {
			t.Fatalf("migration %d: %v", m.version, err)
		}
	}
	if _, err = db.Exec("PRAGMA user_version=20; PRAGMA application_id=502002"); err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"id":"job_old","principal_id":"owner","state":"queued","desired_state":"run","revision":1,"created_at":"2026-10-01T00:00:00Z","updated_at":"2026-10-01T00:00:00Z"}`)
	if _, err = db.Exec("INSERT INTO plans(id,principal,body) VALUES('p_old','owner','{}')"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("INSERT INTO collections(id, principal, plan_id, body) VALUES('col_old','owner','p_old','{\"id\":\"col_old\",\"state\":\"copying\"}')"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("INSERT INTO jobs(id,principal,collection_id,state,body) VALUES('job_old','owner','col_old','queued',?)", body); err != nil {
		t.Fatal(err)
	}
	db.Close()

	s, err := Open(dir)
	if err != nil {
		t.Fatalf("open after migration: %v", err)
	}
	defer s.Close()
	ctx := context.Background()
	j, err := s.GetJob(ctx, "owner", "job_old")
	if err != nil || j.State != JobQueued || j.BatchID != "" || j.Priority != 0 {
		t.Fatalf("old job after upgrade = %+v err=%v", j, err)
	}
	if got := claimID(t, s); got != "job_old" {
		t.Fatalf("old queued job not claimable after upgrade: %s", got)
	}
	fk, err := s.db.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		t.Fatal(err)
	}
	defer fk.Close()
	if fk.Next() {
		t.Fatal("foreign_key_check failed after upgrade")
	}
}

// RecentThroughput is bytes per second of the most recent succeeded jobs. The
// duration is the job's own created->updated span, so a crafted job pins it.
func TestRecentThroughputFromMeasuredJobs(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	if bps, n, err := s.RecentThroughput(ctx, "owner", 5); err != nil || n != 0 || bps != 0 {
		t.Fatalf("no jobs: bps=%v n=%d err=%v", bps, n, err)
	}
	col, job := auditFixture(t, s)
	_ = col
	start := time.Now().UTC().Add(-10 * time.Second).Truncate(time.Millisecond)
	if _, err := s.UpdateJob(ctx, job.ID, func(j *Job) (string, bool) {
		j.State, j.CreatedAt, j.UpdatedAt = JobSucceeded, start, start.Add(10*time.Second)
		return "succeeded", true
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, "UPDATE items SET size=1000, state='done' WHERE job_id=? AND source_id='f1'", job.ID); err != nil {
		t.Fatal(err)
	}
	bps, n, err := s.RecentThroughput(ctx, "owner", 5)
	if err != nil || n != 1 {
		t.Fatalf("throughput samples = %d err=%v", n, err)
	}
	if bps < 95 || bps > 105 {
		t.Fatalf("bytes/s = %v, want about 100", bps)
	}
}

// Resuming the queue before a pausing job reached its safe point must not leave
// it paused forever: the safe point re-checks the durable flag and keeps the
// job running so its worker can continue.
func TestQueueResumeWhileJobPausingKeepsItRunning(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	j1 := queueJob(t, s, 1, "")
	if got := claimID(t, s); got != j1.ID {
		t.Fatalf("claim = %s", got)
	}
	if _, err := s.SetQueuePaused(ctx, true); err != nil {
		t.Fatal(err)
	}
	if cur, _ := s.GetJob(ctx, "owner", j1.ID); cur.State != JobPausing || !cur.QueuePaused {
		t.Fatalf("after queue pause = %s queue_paused=%v", cur.State, cur.QueuePaused)
	}
	// The queue is resumed before the worker reaches its safe point; the paused
	// query in the resume transaction cannot see a pausing job.
	if res, err := s.SetQueuePaused(ctx, false); err != nil || !res.Changed {
		t.Fatalf("resume queue = %+v err=%v", res, err)
	}
	cur, err := s.SettlePause(ctx, j1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cur.State != JobRunning || cur.DesiredState != "run" || cur.QueuePaused {
		t.Fatalf("settled after resume = %s/%s queue_paused=%v", cur.State, cur.DesiredState, cur.QueuePaused)
	}
	// The claim index only lists queued work: the job is not requeued behind its
	// worker's back (that could be claimed while the first goroutine still runs).
	if j, err := s.ClaimQueued(ctx); err == nil {
		t.Fatalf("a running job was claimed a second time: %s", j.ID)
	}
}

// A pause the user requested is not turned into a run by the queue being off.
func TestSettlePauseKeepsUserPause(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	j := queueJob(t, s, 1, "")
	claimID(t, s)
	cur, _ := s.GetJob(ctx, "owner", j.ID)
	if _, err := s.Action(ctx, "owner", "k-user-pause", j.ID, "pause", cur.Revision); err != nil {
		t.Fatal(err)
	}
	got, err := s.SettlePause(ctx, j.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != JobPaused || got.DesiredState != "pause" {
		t.Fatalf("user pause settled as %s/%s", got.State, got.DesiredState)
	}
}
