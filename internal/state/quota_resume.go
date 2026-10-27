package state

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

func (s *Store) QuotaResume(ctx context.Context, principal string, now time.Time) (*time.Time, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT json_extract(body,'$.resume_after') FROM jobs INDEXED BY jobs_state_rowid
 WHERE state='blocked' AND principal=? AND json_extract(body,'$.last_error.code')='quota_exceeded'
 AND julianday(json_extract(body,'$.resume_after'))>julianday(?)
 ORDER BY julianday(json_extract(body,'$.resume_after')) LIMIT 1`, principal, now.UTC().Format(time.RFC3339Nano)).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	at, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return nil, err
	}
	return &at, nil
}
