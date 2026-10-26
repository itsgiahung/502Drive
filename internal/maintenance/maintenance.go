// Package maintenance implements the offline data-directory operations:
// cold backup, restore into a new directory and cleanup of abandoned temp
// files. Every entry point refuses to run while another process holds the
// instance lock, because a live SQLite writer cannot be snapshotted safely.
package maintenance

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gofrs/flock"
	_ "modernc.org/sqlite"

	"github.com/GiaHung07/502Drive/internal/state"
)

// Layout of a backup directory.
const (
	ManifestName = "MANIFEST.json"
	DatabaseName = "state-v2.db"
	ConfigName   = "config.json"
	SecretsDir   = "secrets"
	KeyName      = "master.key"
	TokenName    = "google-token.enc"
	LockName     = "instance.lock"
)

// Errors callers map to exit codes and messages.
var (
	ErrWriterRunning = errors.New("the data directory is in use by a running daemon")
	ErrTargetExists  = errors.New("the target already exists")
	ErrManifest      = errors.New("the backup manifest is missing or invalid")
	ErrIntegrity     = errors.New("the database failed its integrity check")
	ErrSchema        = errors.New("the backup schema is newer than this binary")
	ErrSecret        = errors.New("the backup secrets cannot be used")
	ErrUnsafePath    = errors.New("unsafe path in the backup")
)

// FileEntry is one hashed file of a backup set. Only paths and digests are
// recorded: never file contents.
type FileEntry struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// Manifest describes a backup set.
type Manifest struct {
	Tool          string      `json:"tool"`
	Version       string      `json:"version"`
	CreatedAt     time.Time   `json:"created_at"`
	SchemaVersion int         `json:"schema_version"`
	SourceDataDir string      `json:"source_data_dir"`
	Files         []FileEntry `json:"files"`
	// TokenIncluded reports whether an encrypted Google token was copied. A
	// restore refuses to invent one.
	TokenIncluded bool `json:"token_included"`
}

type Options struct {
	DataDir    string
	ConfigPath string
	Version    string
	Now        func() time.Time
}

func (o Options) now() time.Time {
	if o.Now != nil {
		return o.Now()
	}
	return time.Now()
}

// LockDataDir holds the instance lock, proving no daemon is using the
// directory. The caller must release it when the operation is done.
func LockDataDir(dir string) (func(), error) {
	info, err := os.Lstat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("the data directory must be a real directory")
	}
	l := flock.New(filepath.Join(dir, LockName), flock.SetPermissions(0o600))
	ok, err := l.TryLock()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrWriterRunning
	}
	return func() { _ = l.Unlock() }, nil
}

// Backup snapshots a stopped data directory into a new directory: the
// database (via VACUUM INTO), the master key, the encrypted Google token and
// the config file, plus a manifest with a SHA-256 per file.
func Backup(ctx context.Context, o Options, dst string) (Manifest, error) {
	if o.DataDir == "" || dst == "" {
		return Manifest{}, errors.New("a data directory and a backup path are required")
	}
	absData, err := filepath.Abs(o.DataDir)
	if err != nil {
		return Manifest{}, err
	}
	absDst, err := filepath.Abs(dst)
	if err != nil {
		return Manifest{}, err
	}
	if inside(absDst, absData) {
		return Manifest{}, fmt.Errorf("%w: the backup must not live inside the data directory", ErrUnsafePath)
	}
	if _, err = os.Lstat(absDst); err == nil {
		return Manifest{}, fmt.Errorf("%w: %s", ErrTargetExists, absDst)
	} else if !errors.Is(err, os.ErrNotExist) {
		return Manifest{}, err
	}
	if _, err = os.Stat(filepath.Join(absData, DatabaseName)); err != nil {
		return Manifest{}, fmt.Errorf("no database in %s: %w", absData, err)
	}
	unlock, err := LockDataDir(absData)
	if err != nil {
		return Manifest{}, err
	}
	defer unlock()

	if err = os.Mkdir(absDst, 0o700); err != nil {
		return Manifest{}, err
	}
	ok := false
	defer func() {
		if !ok {
			_ = os.RemoveAll(absDst)
		}
	}()

	m := Manifest{Tool: "502drive", Version: o.Version, CreatedAt: o.now().UTC(), SourceDataDir: absData}
	if err = vacuumInto(ctx, absData, filepath.Join(absDst, DatabaseName)); err != nil {
		return Manifest{}, err
	}
	if m.SchemaVersion, err = readSchemaVersion(ctx, filepath.Join(absDst, DatabaseName)); err != nil {
		return Manifest{}, err
	}

	// Secrets are optional: a data directory that never logged in has none.
	secrets := filepath.Join(absData, SecretsDir)
	for _, name := range []string{KeyName, TokenName} {
		src := filepath.Join(secrets, name)
		if _, statErr := os.Lstat(src); errors.Is(statErr, os.ErrNotExist) {
			continue
		}
		rel := filepath.Join(SecretsDir, name)
		if err = copyFile(src, filepath.Join(absDst, rel)); err != nil {
			return Manifest{}, err
		}
		m.TokenIncluded = m.TokenIncluded || name == TokenName
	}
	if o.ConfigPath != "" {
		if err = copyFile(o.ConfigPath, filepath.Join(absDst, ConfigName)); err != nil {
			return Manifest{}, err
		}
	}
	if m.Files, err = hashFiles(absDst); err != nil {
		return Manifest{}, err
	}
	if err = writeManifest(absDst, &m); err != nil {
		return Manifest{}, err
	}
	ok = true
	return m, nil
}

// hashFiles digests the known members of a backup set. Restore checks the
// same list, so a tampered or truncated file is caught before anything is
// switched.
func hashFiles(dir string) ([]FileEntry, error) {
	entries := make([]FileEntry, 0)
	for _, name := range []string{DatabaseName, filepath.Join(SecretsDir, KeyName), filepath.Join(SecretsDir, TokenName), ConfigName} {
		path := filepath.Join(dir, name)
		info, statErr := os.Lstat(path)
		if errors.Is(statErr, os.ErrNotExist) {
			continue
		}
		if statErr != nil {
			return nil, statErr
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("%w: %s is not a regular file", ErrUnsafePath, name)
		}
		sum, size, sumErr := fileSHA256(path)
		if sumErr != nil {
			return nil, sumErr
		}
		entries = append(entries, FileEntry{Path: name, Size: size, SHA256: sum})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, nil
}

// ReadManifest loads MANIFEST.json from a backup directory.
func ReadManifest(dir string) (Manifest, error) {
	path := filepath.Join(dir, ManifestName)
	info, err := os.Lstat(path)
	if err != nil {
		return Manifest{}, ErrManifest
	}
	if !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return Manifest{}, ErrManifest
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, ErrManifest
	}
	var m Manifest
	if json.Unmarshal(raw, &m) != nil || m.Tool != "502drive" || m.SchemaVersion <= 0 || len(m.Files) == 0 {
		return Manifest{}, ErrManifest
	}
	return m, nil
}

func writeManifest(dir string, m *Manifest) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(filepath.Join(dir, ManifestName), append(b, '\n'))
}

// vacuumInto copies the database with VACUUM INTO, which produces a
// transactionally consistent file without copying a hot WAL.
func vacuumInto(ctx context.Context, dataDir, dst string) error {
	u := url.URL{Scheme: "file", Path: filepath.Join(dataDir, DatabaseName)}
	q := u.Query()
	q.Add("_pragma", "busy_timeout(5000)")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err = db.ExecContext(ctx, "VACUUM INTO ?", dst); err != nil {
		return fmt.Errorf("vacuum into the backup: %w", err)
	}
	return os.Chmod(dst, 0o600)
}

func readSchemaVersion(ctx context.Context, path string) (int, error) {
	u := url.URL{Scheme: "file", Path: path}
	q := u.Query()
	q.Add("mode", "ro")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return 0, err
	}
	defer db.Close()
	var v int
	return v, db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&v)
}

func copyFile(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: %s is not a regular file", ErrUnsafePath, src)
	}
	if err = os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	in, err := os.OpenFile(src, os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	if err == nil {
		err = out.Sync()
	}
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	return err
}

func writeFile(path string, b []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, err = f.Write(b)
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	return err
}

func fileSHA256(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// inside reports whether path is dir or below it.
func inside(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// maxSchema is the schema level this binary understands.
func maxSchema() int { return state.MaxSchemaVersion() }
