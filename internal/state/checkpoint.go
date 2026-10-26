package state

import "context"

// Checkpoint asks SQLite to copy WAL pages into the main database without
// blocking writers (PASSIVE). The daemon calls it before the machine sleeps so
// the WAL that must be replayed after a crash stays small.
func (s *Store) Checkpoint(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, "PRAGMA wal_checkpoint(PASSIVE)")
	return err
}
