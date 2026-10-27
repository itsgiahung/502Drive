package state

import (
	"context"
	"errors"
	"time"
)

// historyUpdatedExpr orders history by the job's last update. Text RFC3339
// ordering is wrong when one value has a fractional second and another does not
// ("Z" sorts after "."), so both the index and the query use integer
// milliseconds. rowid is the tie-breaker and the second keyset component.
const historyUpdatedExpr = `CAST((julianday(json_extract(body, '$.updated_at')) - 2440587.5) * 86400000 AS INTEGER)`

// HistoryCursor is the keyset position in History. The zero value starts at the
// newest job.
type HistoryCursor struct {
	// UpdatedMS is the millisecond timestamp of the last returned job.
	UpdatedMS int64
	// RowID is the rowid of the last returned job.
	RowID int64
}

// History lists jobs by last update, newest first, optionally inside a window.
// It is the export query: unlike ListJobs it orders by updated_at, so an old job
// that ran again recently appears at the top. principal empty means every
// principal (owner view).
func (s *Store) History(ctx context.Context, principal string, since time.Time, limit int, cur HistoryCursor) ([]Job, HistoryCursor, error) {
	if limit < 1 || limit > 200 {
		return nil, HistoryCursor{}, errors.New("limit must be between 1 and 200")
	}
	query := "SELECT rowid," + historyUpdatedExpr + ",id FROM jobs WHERE 1=1"
	var args []any
	if principal != "" {
		query += " AND principal=?"
		args = append(args, principal)
	}
	if !since.IsZero() {
		query += " AND " + historyUpdatedExpr + " >= ?"
		args = append(args, since.UnixMilli())
	}
	if cur.RowID > 0 {
		query += " AND (" + historyUpdatedExpr + ", rowid) < (?, ?)"
		args = append(args, cur.UpdatedMS, cur.RowID)
	}
	query += " ORDER BY " + historyUpdatedExpr + " DESC, rowid DESC LIMIT ?"
	args = append(args, limit+1)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, HistoryCursor{}, err
	}
	type ref struct {
		row int64
		ms  int64
		id  string
	}
	var refs []ref
	for rows.Next() {
		var r ref
		if err = rows.Scan(&r.row, &r.ms, &r.id); err != nil {
			rows.Close()
			return nil, HistoryCursor{}, err
		}
		refs = append(refs, r)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, HistoryCursor{}, err
	}
	var next HistoryCursor
	if len(refs) > limit {
		refs = refs[:limit]
		last := refs[limit-1]
		next = HistoryCursor{UpdatedMS: last.ms, RowID: last.row}
	}
	out := make([]Job, 0, len(refs))
	for _, r := range refs {
		j, e := readJob(ctx, s.db, "", r.id)
		if e != nil {
			return nil, HistoryCursor{}, e
		}
		out = append(out, j)
	}
	return out, next, nil
}
