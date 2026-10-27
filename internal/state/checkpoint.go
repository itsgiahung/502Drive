package state

import (
	"context"
	"time"
)

// Checkpoint asks SQLite to copy WAL pages into the main database without
// blocking writers (PASSIVE). The daemon calls it before the machine sleeps so
// the WAL that must be replayed after a crash stays small.
func (s *Store) Checkpoint(ctx context.Context) error {
	s.lastCheckpointAt.Store(time.Now().UnixNano())
	_, err := s.db.ExecContext(ctx, "PRAGMA wal_checkpoint(PASSIVE)")
	return err
}

// CheckpointTruncate asks SQLite to copy all WAL pages and truncate the WAL file
// to zero bytes. It is called during idle periods and on shutdown.
func (s *Store) CheckpointTruncate(ctx context.Context) error {
	s.lastCheckpointAt.Store(time.Now().UnixNano())
	_, err := s.db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)")
	return err
}

// Optimize runs PRAGMA optimize to update SQLite query planner statistics.
func (s *Store) Optimize(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, "PRAGMA optimize")
	return err
}
