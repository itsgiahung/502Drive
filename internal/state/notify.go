package state

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"time"
)

// Notification bookkeeping for work the user did not start in a chat. The
// adapter decides who to notify; these helpers only read identities and keep
// the durable outbox + "already told once" markers.

// TelegramChat resolves the chat id of an enabled Telegram operator principal.
// The owner has no principals row: the daemon uses the configured owner id.
func (s *Store) TelegramChat(ctx context.Context, principal string) (int64, bool, error) {
	var external string
	err := s.db.QueryRowContext(ctx, `SELECT external_id FROM principals
		WHERE id=? AND kind='telegram' AND role='operator' AND enabled=1`, principal).Scan(&external)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	id, perr := strconv.ParseInt(external, 10, 64)
	if perr != nil || id <= 0 {
		return 0, false, nil
	}
	return id, true, nil
}

// CollectionOwner returns the principal that owns a collection.
func (s *Store) CollectionOwner(ctx context.Context, collectionID string) (string, error) {
	var principal string
	err := s.db.QueryRowContext(ctx, "SELECT principal FROM collections WHERE id=?", collectionID).Scan(&principal)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return principal, err
}

// HasProgressPanel reports whether a chat already follows this job; such jobs
// are reported by editing that panel, not with a second message.
func (s *Store) HasProgressPanel(ctx context.Context, principal, jobID string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM notification_outbox
		WHERE principal=? AND job_id=? AND kind='progress'`, principal, jobID).Scan(&n)
	return n > 0, err
}

// EnqueueFinal inserts one durable final notification. The coalesce key makes
// the hook idempotent; an empty key means every call is a new notification.
func (s *Store) EnqueueFinal(ctx context.Context, principal, jobID string, chatID int64, note, coalesceKey string, at time.Time) (bool, error) {
	if principal == "" || chatID == 0 {
		return false, errors.New("state: principal and chat are required for a notification")
	}
	now := time.Now().UnixMilli()
	res, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO notification_outbox
		(id,kind,principal,job_id,chat_id,state,next_send,note,coalesce_key,created_at,updated_at)
		VALUES(?,?,?,?,?,'pending',?,?,?,?,?)`,
		ID("out"), "final", principal, jobID, chatID, at.UnixMilli(), note, coalesceKey, now, now)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// JobTouchedItems counts the items this run created, changed or moved. A sync
// run that touched nothing changed nothing, so it is not announced.
func (s *Store) JobTouchedItems(ctx context.Context, jobID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM items WHERE job_id=? AND kind<>'wrapper'", jobID).Scan(&n)
	return n, err
}

// ClaimNotice records an error for a subject and reports whether it is new. A
// repeated identical code returns false, so the user is told once until the
// error changes or clears.
func (s *Store) ClaimNotice(ctx context.Context, principal, subject, code string) (bool, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO notify_markers(principal,subject,code,updated_at)
		VALUES(?,?,?,?)
		ON CONFLICT(principal,subject) DO UPDATE
		SET code=excluded.code, updated_at=excluded.updated_at
		WHERE notify_markers.code<>excluded.code`, principal, subject, code, time.Now().UnixMilli())
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// ClearNotice forgets the last error announced for a subject; the next failure
// is a new event for the user.
func (s *Store) ClearNotice(ctx context.Context, principal, subject string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM notify_markers WHERE principal=? AND subject=?", principal, subject)
	return err
}
