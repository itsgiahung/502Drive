package state

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// AfterCopySync is one pending "configure sync for the collection the copy
// created" request (S8). The row is durable, so a restart between the initial
// job and the subscription does not lose it.
type AfterCopySync struct {
	CollectionID  string
	Principal     string
	Mode          string
	State         string
	Attempts      int
	NextAttemptAt time.Time
	LastError     string
	Warning       string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

const afterCopyCols = "collection_id,principal,mode,state,attempts,next_attempt_at,last_error,warning,created_at,updated_at"

func scanAfterCopy(row interface{ Scan(...any) error }) (AfterCopySync, error) {
	var a AfterCopySync
	var next, created, updated int64
	err := row.Scan(&a.CollectionID, &a.Principal, &a.Mode, &a.State, &a.Attempts, &next, &a.LastError, &a.Warning, &created, &updated)
	if err != nil {
		return a, err
	}
	a.NextAttemptAt = time.UnixMilli(next).UTC()
	a.CreatedAt = time.UnixMilli(created).UTC()
	a.UpdatedAt = time.UnixMilli(updated).UTC()
	return a, nil
}

// enqueueAfterCopySync records the request in the job-result transaction. It
// skips when a row or a subscription already exists, so two runs on one
// collection cannot create two subscriptions.
func enqueueAfterCopySync(ctx context.Context, tx *sql.Tx, j Job, now time.Time) error {
	if j.Trigger != "initial" || j.Kind != "" || j.State != JobSucceeded {
		return nil
	}
	mode := j.AfterCopySync
	if mode == "" {
		p, err := preferencesQuery(ctx, tx, j.Principal)
		if err != nil {
			return err
		}
		mode = p.AfterCopySyncMode
	}
	if !ValidAfterCopySyncMode(mode) || mode == AfterCopyOff || mode == "" {
		return nil
	}
	var sub string
	err := tx.QueryRowContext(ctx, "SELECT id FROM sync_subscriptions WHERE collection_id=?", j.CollectionID).Scan(&sub)
	if err == nil {
		return nil // an explicit subscription wins
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO after_copy_sync(collection_id,principal,mode,state,attempts,next_attempt_at,created_at,updated_at)
		VALUES(?,?,?,'pending',0,?,?,?) ON CONFLICT(collection_id) DO NOTHING`,
		j.CollectionID, j.Principal, mode, now.UnixMilli(), now.UnixMilli(), now.UnixMilli())
	return err
}

// ClaimDueAfterCopySync takes up to limit due requests. A row left "applying"
// by a crash becomes due again after its lease, and the idempotent key makes
// the retry harmless.
func (s *Store) ClaimDueAfterCopySync(ctx context.Context, now time.Time, limit int) ([]AfterCopySync, error) {
	if limit < 1 {
		return nil, nil
	}
	var out []AfterCopySync
	err := s.tx(ctx, func(tx *sql.Tx) error {
		rows, e := tx.QueryContext(ctx, "SELECT "+afterCopyCols+" FROM after_copy_sync WHERE (state='pending' OR (state='applying' AND next_attempt_at<=?)) AND next_attempt_at<=? ORDER BY created_at LIMIT ?",
			now.UnixMilli(), now.UnixMilli(), limit)
		if e != nil {
			return e
		}
		var ids []string
		for rows.Next() {
			a, e := scanAfterCopy(rows)
			if e != nil {
				rows.Close()
				return e
			}
			out = append(out, a)
			ids = append(ids, a.CollectionID)
		}
		rows.Close()
		if e = rows.Err(); e != nil {
			return e
		}
		for i := range out {
			out[i].State, out[i].Attempts = "applying", out[i].Attempts+1
			out[i].NextAttemptAt = now.Add(afterCopyLease)
			if _, e = tx.ExecContext(ctx, "UPDATE after_copy_sync SET state='applying', attempts=attempts+1, next_attempt_at=?, updated_at=? WHERE collection_id=?",
				out[i].NextAttemptAt.UnixMilli(), now.UnixMilli(), ids[i]); e != nil {
				return e
			}
		}
		return nil
	})
	return out, err
}

const (
	afterCopyLease    = time.Minute
	afterCopyMaxTries = 5
)

// CompleteAfterCopySync marks the request applied. warning records a skipped
// but harmless outcome (for example an explicit subscription already existed).
func (s *Store) CompleteAfterCopySync(ctx context.Context, collectionID, warning string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE after_copy_sync SET state='applied', warning=?, last_error='', updated_at=? WHERE collection_id=?",
		warning, time.Now().UTC().UnixMilli(), collectionID)
	return err
}

// FailAfterCopySync records a failed attempt. It retries with a simple backoff
// until afterCopyMaxTries, then stays failed; the copy job is never touched.
func (s *Store) FailAfterCopySync(ctx context.Context, a AfterCopySync, msg string) error {
	state, next := "pending", time.Now().Add(time.Duration(a.Attempts)*time.Minute).UTC()
	if a.Attempts >= afterCopyMaxTries {
		state, next = "failed", time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, "UPDATE after_copy_sync SET state=?, last_error=?, next_attempt_at=?, updated_at=? WHERE collection_id=?",
		state, msg, next.UnixMilli(), time.Now().UTC().UnixMilli(), a.CollectionID)
	return err
}

// AfterCopySyncOf reads the request for one collection, for adapters that show
// why an automatic subscription was not applied.
func (s *Store) AfterCopySyncOf(ctx context.Context, collectionID string) (AfterCopySync, error) {
	a, err := scanAfterCopy(s.db.QueryRowContext(ctx, "SELECT "+afterCopyCols+" FROM after_copy_sync WHERE collection_id=?", collectionID))
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrNotFound
	}
	return a, err
}
