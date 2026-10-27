package state

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func newAPITestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func admitTestJob(t *testing.T, s *Store, ctx context.Context, principal, key, planID string) Job {
	t.Helper()
	p := Plan{ID: planID, Principal: principal, SourceID: planID + "-src", SourceName: planID, SourceKind: KindFolder,
		DestinationID: "d1", RootName: planID, ExpiresAt: time.Now().Add(time.Hour), Revision: 1}
	if err := s.PutPlan(ctx, p); err != nil {
		t.Fatal(err)
	}
	res, err := s.Admit(ctx, principal, key, planID, 1)
	if err != nil {
		t.Fatal(err)
	}
	return res.Job
}

func TestCollectionPageIncludeRetiredKeepsPrincipalScope(t *testing.T) {
	s := newAPITestStore(t)
	ctx := context.Background()
	_, _, _ = s.AddDestination(ctx, "owner", "k-dest", "dest", Destination{ID: "d1", FolderID: "root", Name: "My Drive"})
	mine := admitTestJob(t, s, ctx, "owner", "k1", "p1")
	other := admitTestJob(t, s, ctx, "operator", "k2", "p2")

	// Finish the baseline jobs so retirement is allowed.
	for _, j := range []Job{mine, other} {
		if _, err := s.Action(ctx, j.Principal, "cancel-"+j.ID, j.ID, "cancel", j.Revision); err != nil {
			t.Fatal(err)
		}
	}
	col, err := s.GetCollection(ctx, "owner", mine.CollectionID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.RetireCollection(ctx, "owner", "retire-1", col.ID, col.Revision); err != nil {
		t.Fatal(err)
	}

	visible, _, err := s.CollectionPage(ctx, "owner", "", "", false, false, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range visible {
		if c.ID == col.ID {
			t.Fatal("retired collection listed without include_retired")
		}
	}
	withRetired, _, err := s.CollectionPage(ctx, "owner", "", "", false, true, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	foundOwn, foundOther := false, false
	for _, c := range withRetired {
		switch c.ID {
		case col.ID:
			foundOwn = true
		case other.CollectionID:
			foundOther = true
		}
	}
	if !foundOwn {
		t.Fatal("include_retired did not list the retired collection")
	}
	if foundOther {
		t.Fatal("include_retired must not widen the principal scope")
	}
}

func TestJobListFiltersByCollectionAndPagesNewestFirst(t *testing.T) {
	s := newAPITestStore(t)
	ctx := context.Background()
	first := admitTestJob(t, s, ctx, "owner", "k1", "p1")
	second := admitTestJob(t, s, ctx, "owner", "k2", "p2")

	all, _, err := s.ListJobs(ctx, "owner", "", "", 10, 0)
	if err != nil || len(all) != 2 {
		t.Fatalf("all = %d, %v", len(all), err)
	}
	if all[0].ID != second.ID || all[1].ID != first.ID {
		t.Fatalf("newest first order = %s,%s", all[0].ID, all[1].ID)
	}
	only, _, err := s.ListJobs(ctx, "owner", "", first.CollectionID, 10, 0)
	if err != nil || len(only) != 1 || only[0].ID != first.ID {
		t.Fatalf("collection filter = %+v, %v", only, err)
	}
	// The cursor resumes at the last returned row.
	page1, next, err := s.ListJobs(ctx, "owner", "", "", 1, 0)
	if err != nil || len(page1) != 1 || next == 0 {
		t.Fatalf("page1 = %+v next=%d err=%v", page1, next, err)
	}
	page2, _, err := s.ListJobs(ctx, "owner", "", "", 1, next)
	if err != nil || len(page2) != 1 || page2[0].ID == page1[0].ID {
		t.Fatalf("page2 = %+v err=%v", page2, err)
	}

	// Recorded query plans: filtered listings must use an index and must not
	// sort the whole table (the prompt forbids loading the table per poll).
	for _, tc := range []struct {
		name, query string
		args        []any
	}{
		{"collection", "SELECT rowid,id FROM jobs WHERE rowid<? AND collection_id=? ORDER BY rowid DESC LIMIT ?", []any{int64(1 << 62), first.CollectionID, 21}},
		{"state", "SELECT rowid,id FROM jobs WHERE rowid<? AND state=? ORDER BY rowid DESC LIMIT ?", []any{int64(1 << 62), JobQueued, 21}},
	} {
		rows, err := s.db.QueryContext(ctx, "EXPLAIN QUERY PLAN "+tc.query, tc.args...)
		if err != nil {
			t.Fatal(err)
		}
		var plan strings.Builder
		for rows.Next() {
			var id, parent, notUsed int
			var detail string
			if err = rows.Scan(&id, &parent, &notUsed, &detail); err != nil {
				t.Fatal(err)
			}
			plan.WriteString(detail + "\n")
		}
		rows.Close()
		if strings.Contains(plan.String(), "TEMP B-TREE") {
			t.Errorf("%s filter still sorts:\n%s", tc.name, plan.String())
		}
	}
}

func TestLastAuditCountsUnrepairedRepairableFindings(t *testing.T) {
	s := newAPITestStore(t)
	ctx := context.Background()
	j := admitTestJob(t, s, ctx, "owner", "k1", "p1")

	a := Audit{ID: "aud1", CollectionID: j.CollectionID, JobID: j.ID, State: AuditRunning, Counts: map[string]int{}, StartedAt: time.Now().UTC(), Revision: 1}
	if _, err := s.db.ExecContext(ctx, "INSERT INTO audits(id,collection_id,job_id,state,counts,started_at) VALUES(?,?,?,?,?,?)",
		a.ID, a.CollectionID, a.JobID, a.State, "{}", a.StartedAt.UnixMilli()); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordFindings(ctx, a.ID, []Finding{
		{Kind: FindingMissing, SourceID: "s1", Repairable: true},
		{Kind: FindingName, SourceID: "s2", Repairable: true},
		{Kind: FindingExtra, DestID: "d1", Repairable: false},
	}); err != nil {
		t.Fatal(err)
	}
	last, err := s.LastAudit(ctx, "owner", j.CollectionID)
	if err != nil {
		t.Fatal(err)
	}
	if last.Repairable != 2 {
		t.Fatalf("repairable = %d, want 2", last.Repairable)
	}
	if err := s.MarkFindingsRepaired(ctx, a.ID, []Finding{{Kind: FindingMissing, SourceID: "s1"}}); err != nil {
		t.Fatal(err)
	}
	last, err = s.LastAudit(ctx, "owner", j.CollectionID)
	if err != nil || last.Repairable != 1 {
		t.Fatalf("after repair repairable = %d, %v", last.Repairable, err)
	}
	if _, err = s.LastAudit(ctx, "other", j.CollectionID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("last audit leaked to another principal: %v", err)
	}
}
