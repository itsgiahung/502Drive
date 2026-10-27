package maintenance

import (
	"context"
	"testing"

	"github.com/itsgiahung/502Drive/internal/state"
)

func TestVacuumOfflineSucceeds(t *testing.T) {
	dir := t.TempDir()
	s, err := state.Open(dir)
	if err != nil {
		t.Fatalf("state.Open failed: %v", err)
	}
	ctx := context.Background()
	// Insert settings then delete to create fragmented pages
	for i := range 100 {
		_ = s.SetSetting(ctx, "key_"+string(rune('A'+i)), "large_value_padding_text_1234567890_abcdefghijklmnopqrstuvwxyz")
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}

	report, err := Vacuum(ctx, dir)
	if err != nil {
		t.Fatalf("Vacuum failed: %v", err)
	}
	if report.DataDir != dir {
		t.Fatalf("report DataDir = %q, want %q", report.DataDir, dir)
	}
	if report.PreBytes <= 0 || report.PostBytes <= 0 {
		t.Fatalf("unexpected sizes: pre=%d post=%d", report.PreBytes, report.PostBytes)
	}
}

func TestVacuumRefusesWhenDaemonLocked(t *testing.T) {
	dir := t.TempDir()
	s, err := state.Open(dir)
	if err != nil {
		t.Fatalf("state.Open failed: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	_, err = Vacuum(ctx, dir)
	if err != ErrWriterRunning {
		t.Fatalf("Vacuum should fail with ErrWriterRunning while store is open, got: %v", err)
	}
}
