package maintenance

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/GiaHung07/502Drive/internal/state"
)

// setupDir builds a stopped data directory with one queued job, a master key,
// an encrypted token and a config file pointing at it.
func setupDir(t *testing.T) (dataDir, configPath string) {
	t.Helper()
	dataDir = t.TempDir()
	store, err := state.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	plan := state.Plan{ID: "plan_1", Principal: "owner", SourceID: "src_1", SourceName: "Alpha", SourceKind: state.KindFolder,
		DestinationID: "dst_1", DestinationFolder: "dest_1", RootName: "Alpha", ExpiresAt: time.Now().Add(time.Hour), Revision: 1}
	if err = store.PutPlan(ctx, plan); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Admit(ctx, "owner", "admit-1", plan.ID, 1); err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	secrets := filepath.Join(dataDir, SecretsDir)
	if err = os.MkdirAll(secrets, 0o700); err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	if _, err = rand.Read(key); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(secrets, KeyName), []byte(base64.RawStdEncoding.EncodeToString(key)), 0o600); err != nil {
		t.Fatal(err)
	}
	token, err := sealToken(key, []byte(`{"access_token":"fixture"}`))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(secrets, TokenName), token, 0o600); err != nil {
		t.Fatal(err)
	}
	configPath = filepath.Join(t.TempDir(), ConfigName)
	cfg := fmt.Sprintf("{\"listen\":\"127.0.0.1:5020\",\"data_dir\":%q,\"token_file\":%q}\n", dataDir, filepath.Join(dataDir, "api-token"))
	if err = os.WriteFile(configPath, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	return dataDir, configPath
}

// sealToken mirrors the envelope written by internal/drive/auth.go.
func sealToken(key, plain []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return nil, err
	}
	sealed := gcm.Seal(nil, nonce, plain, []byte(tokenFormat))
	return json.Marshal(struct {
		Format string `json:"format"`
		Nonce  []byte `json:"nonce"`
		Data   []byte `json:"data"`
	}{Format: tokenFormat, Nonce: nonce, Data: sealed})
}

func countFiles(t *testing.T, dir string) int {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	return len(entries)
}

func TestBackupRestoreRoundtripKeepsAnUnfinishedJob(t *testing.T) {
	dataDir, configPath := setupDir(t)
	backup := filepath.Join(t.TempDir(), "backup-1")
	m, err := Backup(context.Background(), Options{DataDir: dataDir, ConfigPath: configPath, Version: "test"}, backup)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	if !m.TokenIncluded || m.SchemaVersion != state.MaxSchemaVersion() {
		t.Fatalf("manifest = %+v", m)
	}
	for _, f := range m.Files {
		if f.SHA256 == "" {
			t.Fatalf("file %s has no digest", f.Path)
		}
	}

	target := filepath.Join(t.TempDir(), "restored")
	report, err := Restore(context.Background(), RestoreOptions{Backup: backup, Target: target})
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if report.SchemaVersion != m.SchemaVersion || !report.ConfigWritten {
		t.Fatalf("report = %+v", report)
	}

	store, err := state.Open(target)
	if err != nil {
		t.Fatalf("open restored directory: %v", err)
	}
	defer store.Close()
	jobs, _, err := store.ListJobs(context.Background(), "", "", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].State != state.JobQueued {
		t.Fatalf("restored jobs = %+v", jobs)
	}
	// The engine admits work from the restored queue.
	claimed, err := store.ClaimQueued(context.Background())
	if err != nil || claimed.ID != jobs[0].ID {
		t.Fatalf("claim after restore: %+v %v", claimed, err)
	}
	if jobs, _, err = store.ListJobs(context.Background(), "", "", 10, 0); err != nil {
		t.Fatal(err)
	}
	var resumed state.Job
	for _, j := range jobs {
		resumed = j
	}
	if resumed.State != state.JobRunning {
		t.Fatalf("restored job state = %q", resumed.State)
	}

	raw, err := os.ReadFile(filepath.Join(target, ConfigName))
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err = json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg["data_dir"] != target {
		t.Fatalf("restored config points at %v, want %s", cfg["data_dir"], target)
	}
}

func TestBackupRefusesRunningDaemon(t *testing.T) {
	dataDir, configPath := setupDir(t)
	store, err := state.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_, err = Backup(context.Background(), Options{DataDir: dataDir, ConfigPath: configPath}, filepath.Join(t.TempDir(), "backup"))
	if err == nil {
		t.Fatal("backup with a running daemon must fail")
	}
	if !errors.Is(err, ErrWriterRunning) {
		t.Fatalf("backup error = %v", err)
	}
}

func TestBackupRefusesAnExistingTarget(t *testing.T) {
	dataDir, configPath := setupDir(t)
	dst := t.TempDir()
	if _, err := Backup(context.Background(), Options{DataDir: dataDir, ConfigPath: configPath}, dst); !errors.Is(err, ErrTargetExists) {
		t.Fatalf("error = %v, want target exists", err)
	}
	if countFiles(t, dst) != 0 {
		t.Fatal("an existing target was modified")
	}
}

func TestBackupRefusesTargetInsideDataDir(t *testing.T) {
	dataDir, configPath := setupDir(t)
	if _, err := Backup(context.Background(), Options{DataDir: dataDir, ConfigPath: configPath}, filepath.Join(dataDir, "snap")); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("error = %v", err)
	}
}

func TestRestoreRejectsABadChecksum(t *testing.T) {
	dataDir, configPath := setupDir(t)
	backup := filepath.Join(t.TempDir(), "backup")
	if _, err := Backup(context.Background(), Options{DataDir: dataDir, ConfigPath: configPath}, backup); err != nil {
		t.Fatal(err)
	}
	// Append a byte: the size and digest no longer match.
	f, err := os.OpenFile(filepath.Join(backup, DatabaseName), os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.Write([]byte{0}); err != nil {
		t.Fatal(err)
	}
	f.Close()

	target := filepath.Join(t.TempDir(), "restored")
	if _, err = Restore(context.Background(), RestoreOptions{Backup: backup, Target: target}); err == nil {
		t.Fatal("a tampered backup restored")
	}
	if _, statErr := os.Lstat(target); !os.IsNotExist(statErr) {
		t.Fatalf("a failed restore left %s behind", target)
	}
}

func TestRestoreRejectsAMissingKey(t *testing.T) {
	dataDir, configPath := setupDir(t)
	backup := filepath.Join(t.TempDir(), "backup")
	if _, err := Backup(context.Background(), Options{DataDir: dataDir, ConfigPath: configPath}, backup); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(backup, SecretsDir, KeyName)); err != nil {
		t.Fatal(err)
	}
	writeTestManifest(t, backup, state.MaxSchemaVersion())

	target := filepath.Join(t.TempDir(), "restored")
	if _, err := Restore(context.Background(), RestoreOptions{Backup: backup, Target: target}); !errors.Is(err, ErrSecret) {
		t.Fatalf("error = %v, want a secret failure", err)
	}
	if _, statErr := os.Lstat(target); !os.IsNotExist(statErr) {
		t.Fatalf("a failed restore left %s behind", target)
	}
}

func TestRestoreRejectsANewerSchema(t *testing.T) {
	dataDir, configPath := setupDir(t)
	backup := filepath.Join(t.TempDir(), "backup")
	if _, err := Backup(context.Background(), Options{DataDir: dataDir, ConfigPath: configPath}, backup); err != nil {
		t.Fatal(err)
	}
	writeTestManifest(t, backup, state.MaxSchemaVersion()+1)
	if _, err := Restore(context.Background(), RestoreOptions{Backup: backup, Target: filepath.Join(t.TempDir(), "r")}); !errors.Is(err, ErrSchema) {
		t.Fatalf("error = %v, want a schema failure", err)
	}
}

func TestRestoreRefusesANonEmptyTarget(t *testing.T) {
	dataDir, configPath := setupDir(t)
	backup := filepath.Join(t.TempDir(), "backup")
	if _, err := Backup(context.Background(), Options{DataDir: dataDir, ConfigPath: configPath}, backup); err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	if err := os.WriteFile(filepath.Join(target, "keep.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Restore(context.Background(), RestoreOptions{Backup: backup, Target: target}); !errors.Is(err, ErrTargetExists) {
		t.Fatalf("error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "keep.txt")); err != nil {
		t.Fatal("the existing target was modified")
	}
}

func TestCleanupSkipsSymlinksAndKeepsState(t *testing.T) {
	dataDir, _ := setupDir(t)
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("precious"), 0o600); err != nil {
		t.Fatal(err)
	}
	abandoned := filepath.Join(dataDir, "abandoned.tmp")
	if err := os.WriteFile(abandoned, []byte("scratch"), 0o600); err != nil {
		t.Fatal(err)
	}
	recent := filepath.Join(dataDir, "recent.tmp")
	if err := os.WriteFile(recent, []byte("in use"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dataDir, "evil.tmp")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	old := time.Now().Add(-48 * time.Hour)
	for _, p := range []string{abandoned, recent, link} {
		if err := os.Chtimes(p, old, old); err != nil {
			// Chtimes follows the symlink; set it on the target instead.
			if err := os.Chtimes(outside, old, old); err != nil {
				t.Fatal(err)
			}
		}
	}
	// Keep the one we want retained inside the age window.
	if err := os.Chtimes(recent, time.Now(), time.Now()); err != nil {
		t.Fatal(err)
	}

	report, err := Cleanup(context.Background(), CleanupOptions{DataDir: dataDir, Apply: true})
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if len(report.Removed) != 1 || report.Removed[0] != abandoned {
		t.Fatalf("removed = %v, want only %s", report.Removed, abandoned)
	}
	if _, err = os.Stat(abandoned); !os.IsNotExist(err) {
		t.Fatal("the abandoned temp file survived")
	}
	if _, err = os.Stat(recent); err != nil {
		t.Fatal("a recent temp file was removed")
	}
	if _, err = os.Stat(outside); err != nil {
		t.Fatal("cleanup followed a symlink out of the data directory")
	}
	if _, err = os.Stat(filepath.Join(dataDir, DatabaseName)); err != nil {
		t.Fatal("cleanup removed the database")
	}
	if _, err = os.Stat(filepath.Join(dataDir, SecretsDir, KeyName)); err != nil {
		t.Fatal("cleanup removed the master key")
	}
	var sawSymlink bool
	for _, s := range report.Skipped {
		sawSymlink = sawSymlink || s.Reason == "symlink"
	}
	if !sawSymlink {
		t.Fatalf("the symlink was not reported: %+v", report.Skipped)
	}
}

func TestCleanupDryRunDoesNotDelete(t *testing.T) {
	dataDir, _ := setupDir(t)
	path := filepath.Join(dataDir, "old.tmp")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-72 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	report, err := Cleanup(context.Background(), CleanupOptions{DataDir: dataDir})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Candidates) != 1 || len(report.Removed) != 0 {
		t.Fatalf("dry run report = %+v", report)
	}
	if _, err = os.Stat(path); err != nil {
		t.Fatal("a dry run deleted a file")
	}
}

func TestCleanupRefusesARunningDaemon(t *testing.T) {
	dataDir, _ := setupDir(t)
	store, err := state.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err = Cleanup(context.Background(), CleanupOptions{DataDir: dataDir, Apply: true}); !errors.Is(err, ErrWriterRunning) {
		t.Fatalf("error = %v", err)
	}
}

// writeTestManifest hashes the files present and stores a manifest, which
// lets a test control the declared schema version.
func writeTestManifest(t *testing.T, dir string, schema int) {
	t.Helper()
	var files []FileEntry
	for _, name := range []string{DatabaseName, filepath.Join(SecretsDir, KeyName), filepath.Join(SecretsDir, TokenName), ConfigName} {
		path := filepath.Join(dir, name)
		info, err := os.Lstat(path)
		if err != nil {
			continue
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(raw)
		files = append(files, FileEntry{Path: name, Size: info.Size(), SHA256: hex.EncodeToString(sum[:])})
	}
	m := Manifest{Tool: "502drive", Version: "test", CreatedAt: time.Now().UTC(), SchemaVersion: schema, Files: files}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, ManifestName), append(b, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}
