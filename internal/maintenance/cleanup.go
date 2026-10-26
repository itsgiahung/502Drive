package maintenance

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DefaultTempAge is the minimum age of a temp file before cleanup may touch
// it: an active export or staging directory is never that old.
const DefaultTempAge = 24 * time.Hour

// protected names are never deleted, even if they somehow match a temp
// pattern. Replay guards, keys and backups are not cleanup candidates.
var protected = map[string]bool{
	DatabaseName:              true,
	DatabaseName + "-wal":     true,
	DatabaseName + "-shm":     true,
	DatabaseName + "-journal": true,
	KeyName:                   true,
	TokenName:                 true,
	LockName:                  true,
	ManifestName:              true,
	ConfigName:                true,
}

type CleanupOptions struct {
	DataDir   string
	OlderThan time.Duration
	Apply     bool
	Now       func() time.Time
}

type Skipped struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type CleanupReport struct {
	DataDir    string    `json:"data_dir"`
	Apply      bool      `json:"apply"`
	OlderThan  string    `json:"older_than"`
	Candidates []string  `json:"candidates"`
	Removed    []string  `json:"removed"`
	Skipped    []Skipped `json:"skipped"`
	Bytes      int64     `json:"bytes"`
}

// Cleanup lists (and with Apply, deletes) abandoned temp files in a stopped
// data directory. It never follows symlinks, never leaves the data directory
// and never touches the database, keys, backups or sync mappings.
func Cleanup(ctx context.Context, o CleanupOptions) (CleanupReport, error) {
	report := CleanupReport{DataDir: o.DataDir, Apply: o.Apply}
	if o.DataDir == "" {
		return report, errors.New("a data directory is required")
	}
	age := o.OlderThan
	if age <= 0 {
		age = DefaultTempAge
	}
	report.OlderThan = age.String()
	now := time.Now()
	if o.Now != nil {
		now = o.Now()
	}
	cutoff := now.Add(-age)
	dir, err := filepath.Abs(o.DataDir)
	if err != nil {
		return report, err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return report, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return report, errors.New("the data directory must be a real directory")
	}
	unlock, err := LockDataDir(dir)
	if err != nil {
		return report, err
	}
	defer unlock()

	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if walkErr != nil {
			report.Skipped = append(report.Skipped, Skipped{Path: path, Reason: "cannot read"})
			return nil
		}
		// WalkDir does not follow symlinks; record them so an operator can
		// see why a matching name was left alone.
		if d.Type()&os.ModeSymlink != 0 {
			report.Skipped = append(report.Skipped, Skipped{Path: path, Reason: "symlink"})
			return nil
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if !isTempName(name) || protected[name] {
			return nil
		}
		if !inside(path, dir) {
			report.Skipped = append(report.Skipped, Skipped{Path: path, Reason: "outside the data directory"})
			return nil
		}
		entry, statErr := os.Lstat(path)
		if statErr != nil {
			report.Skipped = append(report.Skipped, Skipped{Path: path, Reason: "cannot stat"})
			return nil
		}
		if !entry.Mode().IsRegular() {
			report.Skipped = append(report.Skipped, Skipped{Path: path, Reason: "not a regular file"})
			return nil
		}
		if !entry.ModTime().Before(cutoff) {
			return nil // too new: it may still be in use
		}
		report.Candidates = append(report.Candidates, path)
		report.Bytes += entry.Size()
		if !o.Apply {
			return nil
		}
		if removeErr := os.Remove(path); removeErr != nil {
			report.Skipped = append(report.Skipped, Skipped{Path: path, Reason: fmt.Sprintf("remove: %v", removeErr)})
			return nil
		}
		report.Removed = append(report.Removed, path)
		return nil
	})
	if err != nil {
		return report, err
	}
	return report, nil
}

// isTempName matches only the temp files this program writes.
func isTempName(name string) bool {
	return strings.HasSuffix(name, ".tmp")
}
