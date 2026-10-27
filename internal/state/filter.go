package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/itsgiahung/502Drive/internal/filter"
)

// SetCollectionFilter replaces the filter of a collection. It changes only
// what later runs decide: copies already at the destination are not touched,
// and a running job keeps the filter it was admitted with.
func (s *Store) SetCollectionFilter(ctx context.Context, principal, collectionID string, spec filter.Spec, expected int) (Collection, error) {
	var out Collection
	err := s.tx(ctx, func(tx *sql.Tx) error {
		var b []byte
		err := tx.QueryRowContext(ctx, "SELECT body FROM collections WHERE id=? AND (?='' OR principal=?)", collectionID, principal, principal).Scan(&b)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if err := json.Unmarshal(b, &out); err != nil {
			return err
		}
		if expected != out.Revision {
			return ErrStale
		}
		out.Filter = spec
		out.Revision++
		nb, err := json.Marshal(out)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "UPDATE collections SET body=? WHERE id=?", nb, collectionID)
		return err
	})
	return out, err
}

// pruneEmptySQL uses items_collection_kind_state for the outer scan and
// items_parent for the existence check (see the EXPLAIN test).
const pruneEmptySQL = `UPDATE items SET state='skipped', error_code=?
	WHERE collection_id=? AND state='pending' AND kind IN ('folder','wrapper') AND (kind<>'folder' OR parent_source_id<>'')
	AND NOT EXISTS (SELECT 1 FROM items c WHERE c.collection_id=items.collection_id AND c.parent_source_id=items.source_id AND c.state<>'skipped')`

// PruneEmptyFolders marks as filtered every pending folder (and wrapper) of a
// collection whose children were all excluded, so no empty folder is created.
// The copy root of a folder source is never pruned. It repeats until nothing
// changes, which is one pass per level of filtered depth. It returns how many
// folders were pruned.
func (s *Store) PruneEmptyFolders(ctx context.Context, collectionID string) (int64, error) {
	var total int64
	for pass := 0; pass < 10000; pass++ {
		res, err := s.db.ExecContext(ctx, pruneEmptySQL, SkipFiltered, collectionID)
		if err != nil {
			return total, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return total, err
		}
		if n == 0 {
			return total, nil
		}
		total += n
	}
	return total, nil
}
