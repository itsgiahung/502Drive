package state

import (
	"context"
	"database/sql"
	"errors"
)

// ResolveFindingPaths reads only the already-authorized collection mapping.
func (s *Store) ResolveFindingPaths(ctx context.Context, collection string, findings []Finding) error {
	for i := range findings {
		f := &findings[i]
		if f.SourceID == "" {
			continue
		}
		err := s.db.QueryRowContext(ctx, "SELECT rel_path FROM items WHERE collection_id=? AND source_id=?", collection, f.SourceID).Scan(&f.Path)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	return nil
}
