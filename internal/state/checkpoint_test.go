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

// Doctor reads LastCheckpoint while the maintenance loop and suspend lifecycle
// write it; the access must be race-free (run with -race).
func TestCheckpointTimestampIsReadableWhileItIsWritten(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 100; i++ {
			_ = s.Checkpoint(context.Background())
			_ = s.CheckpointTruncate(context.Background())
		}
	}()
	for i := 0; i < 200; i++ {
		if s.LastCheckpoint().IsZero() {
			t.Fatal("checkpoint timestamp disappeared while being written")
		}
	}
	<-done
}
