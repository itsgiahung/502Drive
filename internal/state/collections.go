package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
)

// collectionChunk bounds one SQL fetch while scanning for text matches.
const collectionChunk = 100

// CollectionPage lists collections newest first with a rowid cursor. The
// principal and sync-mode filters run in SQL; the text query is matched in Go
// because SQLite's lower() is ASCII-only and Drive names may be Vietnamese.
func (s *Store) CollectionPage(ctx context.Context, principal, query, mode string, all bool, limit int, cursor int64) ([]Collection, int64, error) {
	if limit < 1 || limit > 100 {
		return nil, 0, errors.New("invalid limit")
	}
	if cursor <= 0 {
		cursor = 1 << 62
	}
	needle := strings.ToLower(query)
	scan := cursor
	out := make([]collectionRow, 0, limit)
	for {
		rows, err := s.collectionChunk(ctx, principal, mode, all, scan, collectionChunk)
		if err != nil {
			return nil, 0, err
		}
		if len(rows) == 0 {
			break
		}
		short := len(rows) < collectionChunk
		for _, row := range rows {
			scan = row.row
			if needle != "" && !strings.Contains(strings.ToLower(row.c.SourceName), needle) {
				continue
			}
			if len(out) == limit {
				// The page is full: resume at the last returned row so the next
				// page starts with this one.
				return stripRows(out), out[len(out)-1].row, nil
			}
			out = append(out, row)
		}
		if short {
			break
		}
	}
	return stripRows(out), 0, nil
}

type collectionRow struct {
	row int64
	c   Collection
}

func (s *Store) collectionChunk(ctx context.Context, principal, mode string, all bool, before int64, limit int) ([]collectionRow, error) {
	allInt := 0
	if all {
		allInt = 1
	}
	rows, err := s.db.QueryContext(ctx, `SELECT c.rowid, c.body
		FROM collections c
		LEFT JOIN sync_subscriptions sub ON sub.collection_id = c.id
		WHERE (?='' OR c.principal=?)
		  AND (?=1 OR c.retired_at IS NULL)
		  AND (?='' OR ? = CASE WHEN sub.collection_id IS NULL THEN 'off' ELSE sub.mode END)
		  AND c.rowid < ?
		ORDER BY c.rowid DESC
		LIMIT ?`,
		principal, principal, allInt, mode, mode, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []collectionRow
	for rows.Next() {
		var row collectionRow
		var body []byte
		if err = rows.Scan(&row.row, &body); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(body, &row.c); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// stripRows drops the cursor bookkeeping from the result.
func stripRows(rows []collectionRow) []Collection {
	out := make([]Collection, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.c)
	}
	return out
}

// ErrExistingCopy is returned by an admission guard when the plan's source is
// already copied to the same destination. The app answers it by mode (L2).
type ErrExistingCopy struct {
	Copy Collection
}

func (e ErrExistingCopy) Error() string { return "existing_copy: " + e.Copy.ID }

// ExistingCopy returns the newest collection of principal that copies the
// plan's source to the plan's destination. It runs inside the admission
// transaction, so a concurrent admission of the same plan cannot slip past it.
func ExistingCopy(ctx context.Context, q Querier, principal string, p Plan) (Collection, bool, error) {
	var cb []byte
	err := q.QueryRowContext(ctx, `SELECT body FROM collections
		WHERE principal=? AND json_extract(body,'$.source_id')=? AND json_extract(body,'$.destination_id')=?
		ORDER BY rowid DESC LIMIT 1`, principal, p.SourceID, p.DestinationID).Scan(&cb)
	if errors.Is(err, sql.ErrNoRows) {
		return Collection{}, false, nil
	}
	if err != nil {
		return Collection{}, false, err
	}
	c, err := decodeCollection(cb)
	return c, err == nil, err
}
