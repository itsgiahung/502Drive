package state

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func seedJob(t *testing.T, s *Store, id, principal, st string, age time.Duration, withItem bool) {
	t.Helper()
	ctx := context.Background()
	at := time.Now().UTC().Add(-age)
	pb, _ := json.Marshal(Plan{ID: "plan_h", Principal: "owner", ExpiresAt: time.Now().Add(time.Hour), Revision: 1})
	cb, _ := json.Marshal(Collection{ID: "col_h", SourceID: "src_h", RootName: "Root"})
	if _, err := s.db.ExecContext(ctx, "INSERT OR IGNORE INTO plans(id, principal, body, consumed_job) VALUES('plan_h','owner',?,NULL)", pb); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, "INSERT OR IGNORE INTO collections(id, principal, plan_id, body) VALUES('col_h','owner','plan_h',?)", cb); err != nil {
		t.Fatal(err)
	}
	j := Job{ID: id, Principal: principal, CollectionID: "col_h", State: st, CreatedAt: at, UpdatedAt: at}
	b, _ := json.Marshal(j)
	if _, err := s.db.ExecContext(ctx, "INSERT INTO jobs(id, principal, collection_id, state, body) VALUES(?,?,?,?,?)", id, principal, "col_h", st, b); err != nil {
		t.Fatal(err)
	}
	if withItem {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO items(collection_id, source_id, job_id, parent_source_id, kind, name, state, dest_id, intent_key)
			VALUES('col_h', ?, ?, '', 'file', 'a.txt', 'done', 'd1', 'k1')`, "src_"+id, id); err != nil {
			t.Fatal(err)
		}
	}
}

func TestClearHistoryHidesFinishedJobsAndKeepsMappings(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	seedJob(t, s, "j_done", "owner", "succeeded", time.Hour, true) // referenced by an item
	seedJob(t, s, "j_old", "owner", "failed", 40*24*time.Hour, false)
	seedJob(t, s, "j_run", "owner", "running", time.Hour, false)
	seedJob(t, s, "j_op", "op1", "succeeded", time.Hour, false)

	dry, err := s.ClearHistory(ctx, "", time.Time{}, nil, true)
	if err != nil || !dry.DryRun || dry.Hidden != 3 {
		t.Fatalf("dry run = %+v err=%v, want 3 finished jobs and nothing changed", dry, err)
	}
	if jobs, _, _ := s.ListJobs(ctx, "", "", "", 50, 0); len(jobs) != 4 {
		t.Fatalf("a dry run must not hide anything, listed %d", len(jobs))
	}

	got, err := s.ClearHistory(ctx, "", time.Time{}, nil, false)
	if err != nil || got.Hidden != 3 {
		t.Fatalf("clear = %+v err=%v", got, err)
	}
	jobs, _, _ := s.ListJobs(ctx, "", "", "", 50, 0)
	if len(jobs) != 1 || jobs[0].ID != "j_run" {
		t.Fatalf("only the running job should stay listed: %+v", jobs)
	}
	if _, err := s.GetJob(ctx, "", "j_done"); err != nil {
		t.Fatalf("a hidden job is still readable by id (its items point at it): %v", err)
	}
	var items int
	_ = s.db.QueryRowContext(ctx, "SELECT count(*) FROM items").Scan(&items)
	if items != 1 {
		t.Fatalf("mappings must survive, items = %d", items)
	}
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM jobs WHERE id='j_old'").Scan(&items); err != nil || items != 0 {
		t.Fatalf("an unreferenced finished job is deleted for real, found %d", items)
	}
}

func TestClearHistoryByAgeAndByIDsRespectsScope(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	seedJob(t, s, "j_new", "owner", "succeeded", time.Hour, true)
	seedJob(t, s, "j_old", "owner", "succeeded", 40*24*time.Hour, true)
	seedJob(t, s, "j_mine", "op1", "succeeded", time.Hour, true)
	seedJob(t, s, "j_run", "op1", "queued", time.Hour, false)

	got, err := s.ClearHistory(ctx, "", time.Now().Add(-30*24*time.Hour), nil, false)
	if err != nil || got.Hidden != 1 {
		t.Fatalf("older than 30 days hides only the old one: %+v err=%v", got, err)
	}
	// An operator reaches only their own jobs, and only finished ones.
	got, err = s.ClearHistory(ctx, "op1", time.Time{}, []string{"j_mine", "j_run", "j_new", "nope"}, false)
	if err != nil || got.Hidden != 1 || got.Skipped != 3 {
		t.Fatalf("by ids = %+v err=%v, want 1 hidden and 3 skipped", got, err)
	}
	jobs, _, _ := s.ListJobs(ctx, "", "", "", 50, 0)
	ids := map[string]bool{}
	for _, j := range jobs {
		ids[j.ID] = true
	}
	if !ids["j_new"] || !ids["j_run"] || ids["j_old"] || ids["j_mine"] {
		t.Fatalf("listed = %v", ids)
	}
}

// The cutoff is compared as a point in time, not as text: a job updated half a
// second after the cutoff is newer, even though its string sorts lower.
func TestClearHistoryComparesTimesChronologically(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := t.Context()
	cutoff := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	seedJob(t, s, "j_after", "owner", "succeeded", time.Since(cutoff.Add(500*time.Millisecond)), false)
	seedJob(t, s, "j_before", "owner", "succeeded", time.Since(cutoff.Add(-500*time.Millisecond)), false)
	got, err := s.ClearHistory(ctx, "", cutoff, nil, true)
	if err != nil || got.Hidden != 1 {
		t.Fatalf("only the job older than the cutoff counts, hidden=%d err=%v", got.Hidden, err)
	}
}
