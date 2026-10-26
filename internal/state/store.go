package state

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/gofrs/flock"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

const applicationID = 502002

// Store is the single writer for a data directory.
type Store struct {
	db   *dbConn
	lock *flock.Flock
}

type migration struct {
	version int
	sql     string
}

func loadMigrations() ([]migration, error) {
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return nil, err
	}
	var out []migration
	for _, e := range entries {
		n, err := strconv.Atoi(strings.SplitN(e.Name(), "_", 2)[0])
		if err != nil {
			return nil, fmt.Errorf("bad migration name %s", e.Name())
		}
		b, err := migrationFS.ReadFile("migrations/" + e.Name())
		if err != nil {
			return nil, err
		}
		out = append(out, migration{n, string(b)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	return out, nil
}

// Open takes the instance lock, opens state-v2.db and applies forward-only
// migrations. It refuses databases it does not recognise (including the
// legacy Rust database) and databases newer than this binary.
func Open(dir string) (s *Store, err error) {
	if err = os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("data directory must be a real directory")
	}
	lockPath := filepath.Join(dir, "instance.lock")
	if i, e := os.Lstat(lockPath); e == nil && (!i.Mode().IsRegular() || i.Mode()&os.ModeSymlink != 0) {
		return nil, errors.New("unsafe lock file")
	}
	l := flock.New(lockPath, flock.SetPermissions(0o600))
	ok, err := l.TryLock()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("data directory already has a writer")
	}
	defer func() {
		if err != nil {
			_ = l.Unlock()
		}
	}()
	path := filepath.Join(dir, "state-v2.db")
	if i, e := os.Lstat(path); e == nil && (!i.Mode().IsRegular() || i.Mode()&os.ModeSymlink != 0) {
		return nil, errors.New("unsafe database file")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err = f.Close(); err != nil {
		return nil, err
	}
	u := url.URL{Scheme: "file", Path: path}
	q := u.Query()
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "busy_timeout(5000)")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	defer func() {
		if err != nil {
			_ = db.Close()
		}
	}()
	migs, err := loadMigrations()
	if err != nil {
		return nil, err
	}
	var version, appID int
	if err = db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return nil, err
	}
	if err = db.QueryRow("PRAGMA application_id").Scan(&appID); err != nil {
		return nil, err
	}
	if version > migs[len(migs)-1].version || (appID != 0 && appID != applicationID) {
		return nil, errors.New("unsupported database version or application")
	}
	if appID == 0 {
		var tables int
		if err = db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'").Scan(&tables); err != nil {
			return nil, err
		}
		if tables != 0 {
			return nil, errors.New("refusing an unrecognized database")
		}
	}
	if _, err = db.Exec("PRAGMA journal_mode=WAL; PRAGMA synchronous=FULL;"); err != nil {
		return nil, err
	}
	for _, m := range migs {
		if m.version <= version {
			continue
		}
		tx, e := db.Begin()
		if e != nil {
			return nil, e
		}
		if _, e = tx.Exec(m.sql); e == nil {
			_, e = tx.Exec(fmt.Sprintf("PRAGMA user_version=%d", m.version))
		}
		if e == nil && m.version == 1 {
			_, e = tx.Exec(fmt.Sprintf("PRAGMA application_id=%d", applicationID))
		}
		if e != nil {
			tx.Rollback()
			return nil, fmt.Errorf("migration %d: %w", m.version, e)
		}
		if err = tx.Commit(); err != nil {
			return nil, err
		}
	}
	return &Store{db: &dbConn{DB: db, c: newChanges()}, lock: l}, nil
}

func (s *Store) Close() error { return errors.Join(s.db.Close(), s.lock.Unlock()) }

func (s *Store) Check(ctx context.Context) error { return s.db.PingContext(ctx) }

// SchemaVersion reports the applied migration level.
func (s *Store) SchemaVersion(ctx context.Context) (int, error) {
	var v int
	err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&v)
	return v, err
}

func (s *Store) tx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = fn(tx); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	s.db.c.bump()
	return nil
}
