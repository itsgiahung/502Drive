package state

import (
	"testing"
	"time"
)

func TestBatchNoticeSealAndOutboxSurviveRestart(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Unix(1700000000, 0)
	if err = s.SealBatchNotice(t.Context(), "owner", "batch-synthetic", at); err != nil {
		t.Fatal(err)
	}
	if err = s.FinishBatchNotice(t.Context(), "owner", "batch-synthetic", 42, at); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.SealBatchNotice(t.Context(), "owner", "batch-synthetic", at); err != nil {
		t.Fatal(err)
	}
	open, err := s.OpenBatchNotices(t.Context())
	if err != nil || len(open) != 0 {
		t.Fatalf("open=%v err=%v", open, err)
	}
	due, err := s.DueOutbox(t.Context(), at.Add(time.Hour), 10)
	if err != nil || len(due) != 1 {
		t.Fatalf("due=%v err=%v", due, err)
	}
	claimed, err := s.FenceBatchSend(t.Context(), due[0].ID)
	if err != nil || !claimed {
		t.Fatal(claimed, err)
	}
	due, err = s.DueOutbox(t.Context(), at.Add(time.Hour), 10)
	if err != nil || len(due) != 0 {
		t.Fatal("unknown send was retried", due, err)
	}
}
