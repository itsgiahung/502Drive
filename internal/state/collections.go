package state

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
)

// collectionChunk bounds one SQL fetch while scanning for text matches.
const collectionChunk = 100

// CollectionPage lists collections newest first with a rowid cursor. The
// principal and sync-mode filters run in SQL; the text query is matched in Go
// because SQLite's lower() is ASCII-only and Drive names may be Vietnamese.
func (s *Store) CollectionPage(ctx context.Context, principal, query, mode string, limit int, cursor int64) ([]Collection, int64, error) {
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
		rows, err := s.collectionChunk(ctx, principal, mode, scan, collectionChunk)
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

func (s *Store) collectionChunk(ctx context.Context, principal, mode string, before int64, limit int) ([]collectionRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT c.rowid, c.body
		FROM collections c
		LEFT JOIN sync_subscriptions sub ON sub.collection_id = c.id
		WHERE (?='' OR c.principal=?)
		  AND (?='' OR ? = CASE WHEN sub.collection_id IS NULL THEN 'off' ELSE sub.mode END)
		  AND c.rowid < ?
		ORDER BY c.rowid DESC
		LIMIT ?`,
		principal, principal, mode, mode, before, limit)
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
