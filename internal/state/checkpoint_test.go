package state

import (
	"context"
	"testing"
)

func TestCheckpointTruncateAndOptimize(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()
	ctx := context.Background()

	// Insert some data to generate WAL pages
	if err := s.SetSetting(ctx, "test_key", "test_value"); err != nil {
		t.Fatalf("SetSetting failed: %v", err)
	}

	if err := s.CheckpointTruncate(ctx); err != nil {
		t.Fatalf("CheckpointTruncate failed: %v", err)
	}

	if s.LastCheckpoint().IsZero() {
		t.Fatal("LastCheckpoint should not be zero after checkpoint")
	}

	if err := s.Optimize(ctx); err != nil {
		t.Fatalf("Optimize failed: %v", err)
	}
}
