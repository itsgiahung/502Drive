package state

import "testing"

func TestQueuePositionsFollowActualFairClaims(t *testing.T) {
	s := openStore(t)
	ctx := t.Context()
	a := queueJob(t, s, 1, "batchAAAA1")
	b := queueJob(t, s, 2, "batchAAAA1")
	c := queueJob(t, s, 3, "")
	positions, err := s.QueuePositions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for i, j := range []Job{a, c, b} {
		if positions[j.ID] != i+1 {
			t.Fatalf("position %s=%d want %d", j.ID, positions[j.ID], i+1)
		}
		if got := claimID(t, s); got != j.ID {
			t.Fatalf("claim=%s want %s", got, j.ID)
		}
	}
	positions, err = s.QueuePositions(ctx)
	if err != nil || len(positions) != 0 {
		t.Fatalf("positions=%v err=%v", positions, err)
	}
}

func TestFindingPathsStayWithinAuthorizedCollection(t *testing.T) {
	s := openStore(t)
	a := queueJob(t, s, 1, "")
	b := queueJob(t, s, 2, "")
	if _, err := s.db.ExecContext(t.Context(), "UPDATE items SET rel_path=? WHERE collection_id=? AND source_id=?", "docs/synthetic.pdf", a.CollectionID, "srcplan1"); err != nil {
		t.Fatal(err)
	}
	found := []Finding{{SourceID: "srcplan1"}}
	if err := s.ResolveFindingPaths(t.Context(), a.CollectionID, found); err != nil {
		t.Fatal(err)
	}
	if found[0].Path != "docs/synthetic.pdf" {
		t.Fatal(found)
	}
	found = []Finding{{SourceID: "srcplan1"}}
	if err := s.ResolveFindingPaths(t.Context(), b.CollectionID, found); err != nil {
		t.Fatal(err)
	}
	if found[0].Path != "" {
		t.Fatal("path crossed collection scope")
	}
}

func TestReadingQueuePositionsDoesNotSignalAWrite(t *testing.T) {
	s := openStore(t)
	queueJob(t, s, 1, "")
	before, _ := s.Version()
	if _, err := s.QueuePositions(t.Context()); err != nil {
		t.Fatal(err)
	}
	after, _ := s.Version()
	if after != before {
		t.Fatal("queue read caused an SSE refresh loop")
	}
}
