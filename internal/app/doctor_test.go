package app

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

func TestDoctorFixCreatesDirectoriesAndRestrictsSecrets(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not enforced on Windows")
	}
	store, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	configDir := filepath.Join(t.TempDir(), "missing", "config")
	svc := &Service{Store: store, ConfigDir: configDir}
	fixed, _, err := svc.DoctorFix(context.Background(), Owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(fixed) == 0 {
		t.Fatal("no directory fix reported")
	}
	fi, err := os.Stat(configDir)
	if err != nil || !fi.IsDir() || fi.Mode().Perm() != 0o700 {
		t.Fatalf("config dir = %v (%v)", fi, err)
	}
	secret := filepath.Join(configDir, "token.json")
	if err = os.WriteFile(secret, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	fixed, checks, err := svc.DoctorFix(context.Background(), Owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(fixed) != 1 || !strings.Contains(fixed[0], "chmod 0600") {
		t.Fatalf("fixed = %v", fixed)
	}
	if fi, err = os.Stat(secret); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("secret mode = %v (%v)", fi.Mode().Perm(), err)
	}
	for _, c := range checks {
		if c.Name == "secret_permissions" && c.Status != "passed" {
			t.Fatalf("secret check = %+v", c)
		}
	}
}

func TestDoctorSystemdUserUnitIsActionable(t *testing.T) {
	dir := t.TempDir()
	store, err := state.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	svc := &Service{Store: store, ConfigDir: t.TempDir(),
		DiskSpaceFn: func(string) (uint64, uint64, error) { return 1 << 40, 1 << 41, nil },
		FDLimitFn:   func() (uint64, uint64, error) { return 1024, 4096, nil },
		Systemd: func() SystemdState {
			return SystemdState{Installed: true, Enabled: false, Unit: "/home/u/.config/systemd/user/502drive.service", Detail: "disabled"}
		}}
	checks, err := svc.Doctor(context.Background(), Owner, false)
	if err != nil {
		t.Fatal(err)
	}
	find := func() *Check {
		for i := range checks {
			if checks[i].Name == "systemd_user" {
				return &checks[i]
			}
		}
		return nil
	}
	c := find()
	if c == nil || c.Status != "failed" || !strings.Contains(c.Message, "systemctl --user enable") {
		t.Fatalf("disabled unit check = %+v", c)
	}
	svc.Systemd = func() SystemdState {
		return SystemdState{Installed: true, Enabled: true, Unit: "/home/u/.config/systemd/user/502drive.service", Detail: "enabled"}
	}
	checks, _ = svc.Doctor(context.Background(), Owner, false)
	if c = find(); c == nil || c.Status != "passed" {
		t.Fatalf("enabled unit check = %+v", c)
	}
	svc.Systemd = func() SystemdState { return SystemdState{Detail: "no user unit"} }
	checks, _ = svc.Doctor(context.Background(), Owner, false)
	if c = find(); c == nil || c.Status != "passed" {
		t.Fatalf("missing unit check = %+v", c)
	}
}
