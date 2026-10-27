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
