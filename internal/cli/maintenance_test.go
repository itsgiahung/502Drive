package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/itsgiahung/502Drive/internal/config"
	"github.com/itsgiahung/502Drive/internal/maintenance"
	"github.com/itsgiahung/502Drive/internal/state"
)

// TestMaintenanceCommands exercises backup/restore/cleanup through the CLI:
// they must work without a daemon and refuse when the instance is in use.
func TestMaintenanceCommands(t *testing.T) {
	home := t.TempDir()
	dataDir := filepath.Join(home, "data")
	store, err := state.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(home, "config.json")
	cfg := config.Config{Listen: "127.0.0.1:5020", DataDir: dataDir, TokenFile: filepath.Join(home, "api-token")}
	if err = config.Save(cfgPath, cfg); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) result {
		t.Helper()
		var out, errb bytes.Buffer
		full := append([]string{"--config", cfgPath}, args...)
		code := Execute(context.Background(), full, IO{In: strings.NewReader(""), Out: &out, Err: &errb, Width: 80})
		return result{code, out.String(), errb.String()}
	}

	backup := filepath.Join(home, "backup")
	if res := run("backup", backup); res.code != ExitOK {
		t.Fatalf("backup: %d %s %s", res.code, res.out, res.errOut)
	}
	if _, err := os.Stat(filepath.Join(backup, maintenance.ManifestName)); err != nil {
		t.Fatalf("backup has no manifest: %v", err)
	}
	if res := run("backup", backup); res.code != ExitConflict {
		t.Fatalf("second backup to the same path: exit %d %s", res.code, res.errOut)
	}

	target := filepath.Join(home, "restored")
	if res := run("restore", backup, "--to", target); res.code != ExitOK {
		t.Fatalf("restore: %d %s %s", res.code, res.out, res.errOut)
	}
	if _, err := os.Stat(filepath.Join(target, maintenance.DatabaseName)); err != nil {
		t.Fatalf("restored directory has no database: %v", err)
	}
	if res := run("restore", backup, "--to", target); res.code != ExitConflict {
		t.Fatalf("restore over a used directory: exit %d", res.code)
	}

	junk := filepath.Join(dataDir, "abandoned.tmp")
	if err := os.WriteFile(junk, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-72 * time.Hour)
	if err := os.Chtimes(junk, old, old); err != nil {
		t.Fatal(err)
	}
	res := run("cleanup")
	if res.code != ExitOK || !strings.Contains(res.out, "abandoned.tmp") {
		t.Fatalf("cleanup dry run: %d %s %s", res.code, res.out, res.errOut)
	}
	if _, err := os.Stat(junk); err != nil {
		t.Fatal("the dry run deleted a file")
	}
	if res = run("cleanup", "--apply"); res.code != ExitOK {
		t.Fatalf("cleanup apply: %d %s", res.code, res.errOut)
	}
	if _, err := os.Stat(junk); !os.IsNotExist(err) {
		t.Fatal("cleanup --apply kept the abandoned file")
	}
	if res = run("cleanup", "--apply", "--dry-run"); res.code != ExitUsage {
		t.Fatalf("both modes: exit %d", res.code)
	}

	// Prune offline test
	if res = run("maintenance", "prune", "--dry-run", "--older-than", "90d"); res.code != ExitOK || !strings.Contains(res.out, "dry-run") {
		t.Fatalf("maintenance prune dry-run: %d %s %s", res.code, res.out, res.errOut)
	}
	if res = run("prune", "--older-than", "90d"); res.code != ExitOK || !strings.Contains(res.out, "apply") {
		t.Fatalf("prune apply alias: %d %s %s", res.code, res.out, res.errOut)
	}
	if res = run("prune", "--older-than", "invalid_duration"); res.code != ExitUsage {
		t.Fatalf("prune with invalid duration should fail with usage error, got exit %d", res.code)
	}

	// A running daemon (here: the open store holding the lock) blocks both
	// backup and cleanup.
	live, err := state.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	if res = run("backup", filepath.Join(home, "backup-2")); res.code != ExitUnavailable {
		t.Fatalf("backup with a writer: exit %d %s", res.code, res.errOut)
	}
	if res = run("cleanup", "--apply"); res.code != ExitUnavailable {
		t.Fatalf("cleanup with a writer: exit %d %s", res.code, res.errOut)
	}
	if res = run("maintenance", "prune"); res.code != ExitUnavailable {
		t.Fatalf("prune with a writer: exit %d %s", res.code, res.errOut)
	}
}
