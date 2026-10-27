package app

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/itsgiahung/502Drive/internal/state"
)

func TestDoctorReliabilityChecks(t *testing.T) {
	dir := t.TempDir()
	store, err := state.Open(dir)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer store.Close()

	configDir := t.TempDir()
	// Create secure secret files (0600)
	tokenFile := filepath.Join(configDir, "token.json")
	if err := os.WriteFile(tokenFile, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}

	svc := &Service{
		Store:     store,
		ConfigDir: configDir,
		DiskSpaceFn: func(path string) (uint64, uint64, error) {
			return 10 * 1024 * 1024 * 1024, 100 * 1024 * 1024 * 1024, nil
		},
		FDLimitFn: func() (uint64, uint64, error) {
			return 1024, 4096, nil
		},
	}

	ctx := context.Background()
	checks, err := svc.Doctor(ctx, Owner, false)
	if err != nil {
		t.Fatalf("Doctor failed: %v", err)
	}

	findCheck := func(name string) *Check {
		for i := range checks {
			if checks[i].Name == name {
				return &checks[i]
			}
		}
		return nil
	}

	// 1. Healthy checks
	for _, name := range []string{"disk_space", "fd_limit", "db_wal", "checkpoint_age", "secret_permissions"} {
		c := findCheck(name)
		if c == nil {
			t.Fatalf("missing check %q", name)
		}
		if c.Status != "passed" {
			t.Errorf("check %q status = %q (msg: %s), want passed", name, c.Status, c.Message)
		}
	}

	// 2. Low disk space (< 512MB)
	svc.DiskSpaceFn = func(path string) (uint64, uint64, error) {
		return 100 * 1024 * 1024, 100 * 1024 * 1024 * 1024, nil // 100MB
	}
	checks, _ = svc.Doctor(ctx, Owner, false)
	if c := findCheck("disk_space"); c == nil || c.Status != "failed" {
		t.Errorf("low disk space should fail, got: %+v", c)
	}

	// 3. Low fd limit (< 128)
	svc.DiskSpaceFn = nil // reset
	svc.FDLimitFn = func() (uint64, uint64, error) {
		return 64, 1024, nil
	}
	checks, _ = svc.Doctor(ctx, Owner, false)
	if c := findCheck("fd_limit"); c == nil || c.Status != "failed" {
		t.Errorf("low fd limit should fail, got: %+v", c)
	}
	svc.FDLimitFn = nil // reset

	// 4. Large WAL (> 32MB)
	walPath := filepath.Join(dir, "state-v2.db-wal")
	f, err := os.Create(walPath)
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Truncate(35 * 1024 * 1024)
	_ = f.Close()
	checks, _ = svc.Doctor(ctx, Owner, false)
	if c := findCheck("db_wal"); c == nil || c.Status != "failed" {
		t.Errorf("large WAL should fail, got: %+v", c)
	}
	_ = os.Remove(walPath)

	// 5. Insecure secret permissions
	if runtime.GOOS != "windows" {
		_ = os.Chmod(tokenFile, 0o666)
		checks, _ = svc.Doctor(ctx, Owner, false)
		if c := findCheck("secret_permissions"); c == nil || c.Status != "failed" {
			t.Errorf("0666 token file should fail secret_permissions, got: %+v", c)
		}
	}
}

func TestCheckpointAgeCheck(t *testing.T) {
	dir := t.TempDir()
	store, err := state.Open(dir)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer store.Close()

	svc := &Service{
		Store: store,
	}

	ctx := context.Background()
	checks, err := svc.Doctor(ctx, Owner, false)
	if err != nil {
		t.Fatal(err)
	}
	var cpCheck *Check
	for i := range checks {
		if checks[i].Name == "checkpoint_age" {
			cpCheck = &checks[i]
		}
	}
	if cpCheck == nil || cpCheck.Status != "passed" {
		t.Fatalf("expected passed checkpoint check, got: %+v", cpCheck)
	}
}
