package state

import (
	"context"
	"database/sql"
	"time"
)

// addCopiedBytes credits one principal's hourly bucket inside the transaction
// that completed an item.
func (s *Store) addCopiedBytes(ctx context.Context, tx *sql.Tx, jobID string, bytes int64) error {
	var principal string
	if err := tx.QueryRowContext(ctx, "SELECT principal FROM jobs WHERE id=?", jobID).Scan(&principal); err != nil {
		return err
	}
	hour := time.Now().UTC().Unix() / 3600
	_, err := tx.ExecContext(ctx, `INSERT INTO copy_ledger(principal,hour,bytes) VALUES(?,?,?)
		ON CONFLICT(principal,hour) DO UPDATE SET bytes=bytes+excluded.bytes`, principal, hour, bytes)
	return err
}

// CopiedBytes24h reports the bytes credited to a principal in the current hour
// and the 23 before it. It is one indexed sum, never a scan over jobs/items.
func (s *Store) CopiedBytes24h(ctx context.Context, principal string, now time.Time) (int64, error) {
	hour := now.UTC().Unix() / 3600
	var total int64
	err := s.db.QueryRowContext(ctx, "SELECT coalesce(sum(bytes),0) FROM copy_ledger WHERE principal=? AND hour>=?", principal, hour-23).Scan(&total)
	return total, err
}

// ResumeDueBlocked moves quota-blocked jobs whose resume_after has passed back
// to the queue. desired_state stays "run"; the engine claims them on the next
// tick, so no new job (and no duplicate identity) is created.
func (s *Store) ResumeDueBlocked(ctx context.Context, now time.Time) (int, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE jobs SET state='queued',
		body=json_remove(json_set(body,'$.state','queued','$.revision',json_extract(body,'$.revision')+1,
			'$.updated_at',?),'$.resume_after','$.last_error','$.phase')
		WHERE state='blocked'
		AND json_extract(body,'$.desired_state')='run'
		AND json_extract(body,'$.resume_after') IS NOT NULL
		AND julianday(json_extract(body,'$.resume_after'))<=julianday(?)`,
		now.UTC().Format(time.RFC3339Nano), now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
