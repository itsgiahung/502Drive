package state

import "context"

// ReleaseItemNoPenalty returns an item to pending without consuming an attempt.
// It is used when the write provably never left the machine (no network): the
// item did not fail, the machine was offline. ClaimItem incremented attempts
// before the call, so the decrement restores the pre-claim value.
func (s *Store) ReleaseItemNoPenalty(ctx context.Context, it Item, code string) error {
	_, err := s.db.ExecContext(ctx,
		"UPDATE items SET state='pending', error_code=?, next_attempt_at=0, attempts=MAX(attempts-1,0) WHERE collection_id=? AND source_id=?",
		code, it.CollectionID, it.SourceID)
	return err
}

// Items lists every item of a collection. The caller gets a copy; the engine's
// own claim path is unchanged.
func (s *Store) Items(ctx context.Context, collectionID string) ([]Item, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+itemCols+" FROM items WHERE collection_id=? ORDER BY rowid", collectionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Item
	for rows.Next() {
		it, e := scanItem(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, it)
	}
	return out, rows.Err()
}
