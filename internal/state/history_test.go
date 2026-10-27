package state

import (
	"context"
	"strings"
	"testing"
	"time"
)

// setJobUpdated writes the updated_at of a job body directly, so history tests
// can span a window without sleeping.
func setJobUpdated(t *testing.T, s *Store, id string, at time.Time) {
	t.Helper()
	if _, err := s.db.ExecContext(context.Background(),
		`UPDATE jobs SET body=json_set(body,'$.updated_at',?) WHERE id=?`, at.UTC().Format(time.RFC3339Nano), id); err != nil {
		t.Fatal(err)
	}
}

func TestHistoryOrdersByUpdatedAtAndFiltersSince(t *testing.T) {
	s := newAPITestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	var mine []Job
	for i := range 4 {
		j := admitTestJob(t, s, ctx, "owner", "h"+string(rune('a'+i)), "hp"+string(rune('a'+i)))
		setJobUpdated(t, s, j.ID, base.Add(time.Duration(i)*time.Minute).Add(500*time.Millisecond))
		mine = append(mine, j)
	}
	other := admitTestJob(t, s, ctx, "operator", "ho", "hpo")
	setJobUpdated(t, s, other.ID, base.Add(10*time.Minute))

	// Half-second stamps must sort after the whole second, not before it.
	first, _, err := s.History(ctx, "owner", time.Time{}, 2, HistoryCursor{})
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 2 || first[0].ID != mine[3].ID || first[1].ID != mine[2].ID {
		t.Fatalf("first page = %v", historyIDs(first))
	}
	// Use the cursor returned by the store instead of guessing milliseconds.
	_, cur, err := s.History(ctx, "owner", time.Time{}, 2, HistoryCursor{})
	if err != nil {
		t.Fatal(err)
	}
	second, next, err := s.History(ctx, "owner", time.Time{}, 2, cur)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 2 || second[0].ID != mine[1].ID || second[1].ID != mine[0].ID {
		t.Fatalf("second page = %v", historyIDs(second))
	}
	if next.RowID != 0 {
		t.Fatalf("cursor past the end = %+v", next)
	}
	// since excludes everything older than base+2m.
	cut := base.Add(2 * time.Minute)
	recent, _, err := s.History(ctx, "owner", cut, 10, HistoryCursor{})
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 2 || recent[0].ID != mine[3].ID || recent[1].ID != mine[2].ID {
		t.Fatalf("since page = %v", historyIDs(recent))
	}
	// Principal scope: owner must not see the operator's newer job.
	for _, j := range recent {
		if j.ID == other.ID {
			t.Fatal("history leaked another principal's job")
		}
	}
}

func TestHistoryQueryPlanUsesIndex(t *testing.T) {
	s := newAPITestStore(t)
	expr := historyUpdatedExpr
	rows, err := s.db.QueryContext(context.Background(),
		"EXPLAIN QUERY PLAN SELECT rowid,"+expr+" FROM jobs WHERE "+expr+" >= ? AND ("+expr+", rowid) < (?, ?) ORDER BY "+expr+" DESC, rowid DESC LIMIT 5",
		0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err = rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		plan = append(plan, detail)
	}
	if len(plan) == 0 {
		t.Fatal("empty query plan")
	}
	joined := ""
	for _, d := range plan {
		joined += d + "\n"
	}
	if !strings.Contains(joined, "jobs_history") {
		t.Fatalf("history query does not use jobs_history:\n%s", joined)
	}
	if strings.Contains(joined, "TEMP B-TREE") {
		t.Fatalf("history query sorts the whole table:\n%s", joined)
	}
}

func historyIDs(js []Job) []string {
	out := make([]string, 0, len(js))
	for _, j := range js {
		out = append(out, j.ID)
	}
	return out
}
