package state

import (
	"strings"
	"testing"
)

func TestMultiStateUsesIndexAndPreservesOtherStates(t *testing.T) {
	s := openStore(t)
	ctx := t.Context()
	a := queueJob(t, s, 1, "")
	b := queueJob(t, s, 2, "")
	if _, err := s.UpdateJob(ctx, a.ID, func(j *Job) (string, bool) { j.State = JobFailed; return "finished", true }); err != nil {
		t.Fatal(err)
	}
	jobs, _, err := s.ListJobs(ctx, "owner", "failed,queued", "", 1, 0)
	if err != nil || len(jobs) != 1 || jobs[0].ID != b.ID {
		t.Fatalf("jobs=%v err=%v", jobs, err)
	}
	rows, err := s.db.QueryContext(ctx, "EXPLAIN QUERY PLAN SELECT rowid,id FROM jobs WHERE rowid<? AND state IN (?,?) ORDER BY rowid DESC LIMIT ?", int64(1<<62), "queued", "failed", 21)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var details []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err = rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		details = append(details, detail)
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	plan := strings.Join(details, "\n")
	t.Log(plan)
	if !strings.Contains(plan, "SEARCH jobs USING INDEX jobs_state_rowid") {
		t.Fatal(plan)
	}
}
