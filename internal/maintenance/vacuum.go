package maintenance

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// VacuumReport describes the size reduction after an offline vacuum compaction.
type VacuumReport struct {
	DataDir   string `json:"data_dir"`
	PreBytes  int64  `json:"pre_bytes"`
	PostBytes int64  `json:"post_bytes"`
	Reclaimed int64  `json:"reclaimed_bytes"`
}

// Vacuum compacts the SQLite database offline. It flushes the WAL via TRUNCATE,
// executes VACUUM, and runs PRAGMA optimize. It refuses to run if instance.lock
// is held by an active daemon.
func Vacuum(ctx context.Context, dataDir string) (VacuumReport, error) {
	unlock, err := LockDataDir(dataDir)
	if err != nil {
		return VacuumReport{}, err
	}
	defer unlock()

	dbPath := filepath.Join(dataDir, DatabaseName)
	fiPre, err := os.Stat(dbPath)
	if err != nil {
		return VacuumReport{}, err
	}
	preSize := fiPre.Size()

	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath)+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return VacuumReport{}, err
	}
	defer db.Close()

	if _, err := db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		return VacuumReport{}, fmt.Errorf("wal_checkpoint: %w", err)
	}
	if _, err := db.ExecContext(ctx, "VACUUM"); err != nil {
		return VacuumReport{}, fmt.Errorf("vacuum: %w", err)
	}
	if _, err := db.ExecContext(ctx, "PRAGMA optimize"); err != nil {
		return VacuumReport{}, fmt.Errorf("optimize: %w", err)
	}
	_ = db.Close()

	fiPost, err := os.Stat(dbPath)
	if err != nil {
		return VacuumReport{}, err
	}
	postSize := fiPost.Size()

	reclaimed := preSize - postSize
	if reclaimed < 0 {
		reclaimed = 0
	}

	return VacuumReport{
		DataDir:   dataDir,
		PreBytes:  preSize,
		PostBytes: postSize,
		Reclaimed: reclaimed,
	}, nil
}
