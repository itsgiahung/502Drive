package state

import (
	"context"
	"strings"
	"testing"
)

// TestExistingCopyMatchesSourceAndDestination covers the L2 lookup used inside
// the admission transaction: the same source to the same destination finds the
// copy, another destination does not, and the plan's source/destination are
// the only keys. The lookup must use the expression index.
func TestExistingCopyMatchesSourceAndDestination(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if err := s.PutPlan(ctx, Plan{ID: "plan-a", Principal: "owner", SourceID: "src", DestinationID: "dst", Revision: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO collections(id, principal, plan_id, body) VALUES(?,?,?,?)`,
		"col-a", "owner", "plan-a", []byte(`{"id":"col-a","source_id":"src","destination_id":"dst","state":"ready","revision":3}`)); err != nil {
		t.Fatal(err)
	}

	got, found, err := ExistingCopy(ctx, s.db, "owner", Plan{SourceID: "src", DestinationID: "dst"})
	if err != nil || !found || got.ID != "col-a" || got.Revision != 3 {
		t.Fatalf("same source and destination: found=%v got=%+v err=%v", found, got, err)
	}
	if _, found, _ := ExistingCopy(ctx, s.db, "owner", Plan{SourceID: "src", DestinationID: "dst-2"}); found {
		t.Fatal("another destination must not count as the same copy")
	}
	if _, found, _ := ExistingCopy(ctx, s.db, "other", Plan{SourceID: "src", DestinationID: "dst"}); found {
		t.Fatal("another principal must not see this copy")
	}

	rows, err := s.db.QueryContext(ctx, `EXPLAIN QUERY PLAN SELECT body FROM collections
		WHERE principal=? AND json_extract(body,'$.source_id')=? AND json_extract(body,'$.destination_id')=?`, "owner", "src", "dst")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	used := false
	for rows.Next() {
		var id, parent, notused int
		var detail string
		if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(detail, "collections_source_dest") {
			used = true
		}
	}
	if !used {
		t.Fatal("ExistingCopy does not use collections_source_dest")
	}
}
