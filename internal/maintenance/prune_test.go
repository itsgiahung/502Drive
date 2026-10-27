package maintenance

import (
	"context"
	"testing"
	"time"

	"github.com/itsgiahung/502Drive/internal/state"
)

func TestPruneOfflineSucceeds(t *testing.T) {
	dir := t.TempDir()
	s, err := state.Open(dir)
	if err != nil {
		t.Fatalf("state.Open failed: %v", err)
	}
	ctx := context.Background()
	_ = s.SetSetting(ctx, "k", "v")
	if err := s.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}

	report, err := Prune(ctx, dir, state.PruneOptions{OlderThan: 90 * 24 * time.Hour, DryRun: false})
	if err != nil {
		t.Fatalf("Prune failed: %v", err)
	}
	if report.DryRun {
		t.Fatal("expected dryRun false")
	}
}

func TestPruneRefusesWhenDaemonLocked(t *testing.T) {
	dir := t.TempDir()
	s, err := state.Open(dir)
	if err != nil {
		t.Fatalf("state.Open failed: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	_, err = Prune(ctx, dir, state.PruneOptions{OlderThan: 90 * 24 * time.Hour, DryRun: false})
	if err != ErrWriterRunning {
		t.Fatalf("Prune should fail with ErrWriterRunning while store is open, got: %v", err)
	}
}
