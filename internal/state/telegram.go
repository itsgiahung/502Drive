package state

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Telegram adapter state: a durable update inbox, one-shot callback states and
// a notification outbox. The adapter never touches these tables directly; it
// goes through the methods below.

var (
	// ErrCallbackStale means the button is expired, already used or from
	// another chat. Adapters must not reveal which.
	ErrCallbackStale = errors.New("callback_stale")
)

// Inbox is one raw Telegram update waiting to be handled.
type Inbox struct {
	UpdateID int64
	Body     []byte
}

// TelegramInstance returns the stable id of this bot instance, creating it on
// first use. The id keys the inbox and idempotency keys, so it must survive
// restarts.
func (s *Store) TelegramInstance(ctx context.Context) (string, error) {
	var instance string
	err := s.db.QueryRowContext(ctx, "SELECT instance FROM telegram_meta WHERE id=1").Scan(&instance)
	if err == nil {
		return instance, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	instance = ID("bot")
	_, err = s.db.ExecContext(ctx, "INSERT OR IGNORE INTO telegram_meta(id,instance,next_update) VALUES(1,?,0)", instance)
	if err != nil {
		return "", err
	}
	err = s.db.QueryRowContext(ctx, "SELECT instance FROM telegram_meta WHERE id=1").Scan(&instance)
	return instance, err
}

// TelegramOffset is the next update_id to request. It only advances after the
// batch it covers is durably stored.
func (s *Store) TelegramOffset(ctx context.Context, instance string) (int64, error) {
	var next int64
	err := s.db.QueryRowContext(ctx, "SELECT next_update FROM telegram_meta WHERE id=1 AND instance=?", instance).Scan(&next)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return next, err
}

// IngestUpdates stores a batch and advances the offset in one transaction. It
// returns how many updates were new; duplicates from a redelivered batch are
// ignored.
func (s *Store) IngestUpdates(ctx context.Context, instance string, updates []Inbox, next int64) (int, error) {
	inserted := 0
	err := s.tx(ctx, func(tx *sql.Tx) error {
		now := time.Now().UnixMilli()
		for _, u := range updates {
			res, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO telegram_inbox(bot,update_id,body,state,received_at) VALUES(?,?,?,'pending',?)", instance, u.UpdateID, u.Body, now)
			if err != nil {
				return err
			}
			if n, _ := res.RowsAffected(); n == 1 {
				inserted++
			}
		}
		_, err := tx.ExecContext(ctx, "UPDATE telegram_meta SET next_update=? WHERE id=1 AND instance=? AND next_update<?", next, instance, next)
		return err
	})
	return inserted, err
}

// PendingUpdates returns the oldest updates that were stored but not handled.
func (s *Store) PendingUpdates(ctx context.Context, instance string, limit int) ([]Inbox, error) {
	if limit < 1 {
		limit = 1
	}
	rows, err := s.db.QueryContext(ctx, "SELECT update_id,body FROM telegram_inbox WHERE bot=? AND state='pending' ORDER BY update_id LIMIT ?", instance, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Inbox
	for rows.Next() {
		var u Inbox
		if err = rows.Scan(&u.UpdateID, &u.Body); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// FinishUpdate marks an update handled. A non-empty code keeps it pending for
// a bounded number of retries, then marks it failed; the job itself is
// protected by the update-derived idempotency key.
func (s *Store) FinishUpdate(ctx context.Context, instance string, updateID int64, code string) error {
	if code == "" {
		_, err := s.db.ExecContext(ctx, "UPDATE telegram_inbox SET state='processed', error_code='' WHERE bot=? AND update_id=?", instance, updateID)
		return err
	}
	_, err := s.db.ExecContext(ctx, `UPDATE telegram_inbox
		SET attempts=attempts+1, error_code=?,
		    state=CASE WHEN attempts+1>=5 THEN 'failed' ELSE 'pending' END
		WHERE bot=? AND update_id=?`, code, instance, updateID)
	return err
}

// Callback is one inline-keyboard action. Mutating actions are consumed once.
type Callback struct {
	ID        string
	Principal string
	ChatID    int64
	MessageID int
	Action    string
	Object    string
	Revision  int
	Payload   string
	ExpiresAt time.Time
	Consumed  bool
	CreatedAt time.Time
}

// PutCallback stores a new callback state.
func (s *Store) PutCallback(ctx context.Context, c Callback) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO callback_states(id,principal,chat_id,message_id,action,object,revision,payload,expires_at,consumed,created_at)
		VALUES(?,?,?,?,?,?,?,?,?,0,?)`,
		c.ID, c.Principal, c.ChatID, c.MessageID, c.Action, c.Object, c.Revision, c.Payload, c.ExpiresAt.UnixMilli(), c.CreatedAt.UnixMilli())
	if err == nil {
		// Keep the table bounded: rows older than the retention window can no
		// longer be claimed anyway.
		_ = s.PruneCallbacks(ctx, c.CreatedAt)
	}
	return err
}

// BindCallbackMessage records the message a callback was delivered in. It is
// informational: claiming still checks principal and chat.
func (s *Store) BindCallbackMessage(ctx context.Context, ids []string, messageID int) error {
	for _, id := range ids {
		if _, err := s.db.ExecContext(ctx, "UPDATE callback_states SET message_id=? WHERE id=?", messageID, id); err != nil {
			return err
		}
	}
	return nil
}

// ClaimCallback authorises a click. Mutating actions are consumed inside the
// transaction so two clicks cannot both act. Expired, consumed and foreign
// callbacks all return ErrCallbackStale.
func (s *Store) ClaimCallback(ctx context.Context, id, principal string, chatID int64, messageID int, mutating bool, now time.Time) (Callback, error) {
	var c Callback
	err := s.tx(ctx, func(tx *sql.Tx) error {
		var expires, created int64
		var consumed int
		err := tx.QueryRowContext(ctx, `SELECT principal,chat_id,message_id,action,object,revision,payload,expires_at,consumed,created_at
			FROM callback_states WHERE id=?`, id).
			Scan(&c.Principal, &c.ChatID, &c.MessageID, &c.Action, &c.Object, &c.Revision, &c.Payload, &expires, &consumed, &created)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrCallbackStale
		}
		if err != nil {
			return err
		}
		c.ID, c.ExpiresAt, c.Consumed, c.CreatedAt = id, time.UnixMilli(expires).UTC(), consumed == 1, time.UnixMilli(created).UTC()
		// A callback is bound to the message it was sent in (0 = not yet bound).
		if !c.ExpiresAt.After(now) || c.Principal != principal || c.ChatID != chatID || (c.MessageID != 0 && c.MessageID != messageID) {
			return ErrCallbackStale
		}
		if !mutating {
			return nil
		}
		if consumed == 1 {
			return ErrCallbackStale
		}
		res, err := tx.ExecContext(ctx, "UPDATE callback_states SET consumed=1 WHERE id=? AND consumed=0", id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return ErrCallbackStale
		}
		c.Consumed = true
		return nil
	})
	if err != nil {
		return Callback{}, err
	}
	return c, nil
}

// PruneCallbacks deletes callbacks that expired more than a day before now.
func (s *Store) PruneCallbacks(ctx context.Context, now time.Time) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM callback_states WHERE expires_at<?", now.Add(-24*time.Hour).UnixMilli())
	return err
}

// Outbox is one durable outgoing message. Progress rows coalesce per
// (principal, job) and are edited in place; every terminal decision is made
// from the job row, never from the message.
type Outbox struct {
	ID          string
	Kind        string
	Principal   string
	JobID       string
	ChatID      int64
	MessageID   int
	State       string
	Attempts    int
	NextSend    time.Time
	LastHash    string
	LastState   string
	ErrorCode   string
	Note        string
	CoalesceKey string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

const outboxCols = "id,kind,principal,job_id,chat_id,message_id,state,attempts,next_send,last_hash,last_state,error_code,note,coalesce_key,created_at,updated_at"

func scanOutbox(row interface{ Scan(...any) error }) (Outbox, error) {
	var o Outbox
	var next, created, updated int64
	err := row.Scan(&o.ID, &o.Kind, &o.Principal, &o.JobID, &o.ChatID, &o.MessageID, &o.State, &o.Attempts, &next,
		&o.LastHash, &o.LastState, &o.ErrorCode, &o.Note, &o.CoalesceKey, &created, &updated)
	o.NextSend = time.UnixMilli(next).UTC()
	o.CreatedAt = time.UnixMilli(created).UTC()
	o.UpdatedAt = time.UnixMilli(updated).UTC()
	return o, err
}

// EnqueueProgress ensures exactly one pending progress panel per chat and job.
func (s *Store) EnqueueProgress(ctx context.Context, principal, jobID string, chatID int64, at time.Time) error {
	key := "progress:" + principal + ":" + jobID
	now := time.Now().UnixMilli()
	_, err := s.db.ExecContext(ctx, `INSERT INTO notification_outbox(id,kind,principal,job_id,chat_id,state,next_send,coalesce_key,created_at,updated_at)
		VALUES(?,?,?,?,?,'pending',?,?,?,?)
		ON CONFLICT(coalesce_key) WHERE coalesce_key <> '' DO UPDATE SET
		  next_send=MIN(next_send, excluded.next_send),
		  updated_at=excluded.updated_at
		WHERE notification_outbox.state='pending'`, ID("out"), "progress", principal, jobID, chatID, at.UnixMilli(), key, now, now)
	return err
}

// BindProgress points an existing pending progress panel at the message that
// was just sent, so the job updates edit it instead of sending a second one.
func (s *Store) BindProgress(ctx context.Context, principal, jobID string, messageID int, hash, jobState string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE notification_outbox
		SET message_id=?, last_hash=?, last_state=?, updated_at=?
		WHERE coalesce_key=? AND state='pending'`,
		messageID, hash, jobState, time.Now().UnixMilli(), "progress:"+principal+":"+jobID)
	return err
}

// DueOutbox returns pending messages that are ready to send, oldest first.
func (s *Store) DueOutbox(ctx context.Context, now time.Time, limit int) ([]Outbox, error) {
	if limit < 1 {
		limit = 1
	}
	rows, err := s.db.QueryContext(ctx, "SELECT "+outboxCols+" FROM notification_outbox WHERE state='pending' AND next_send<=? ORDER BY next_send, created_at LIMIT ?", now.UnixMilli(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Outbox
	for rows.Next() {
		o, err := scanOutbox(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// DeferOutbox keeps a progress row pending: it will be sent again no earlier
// than at, and the last rendered text is remembered so unchanged panels are
// not edited.
func (s *Store) DeferOutbox(ctx context.Context, id string, at time.Time, messageID int, hash, jobState string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE notification_outbox
		SET next_send=?, message_id=?, last_hash=?, last_state=?, updated_at=?, error_code=''
		WHERE id=?`, at.UnixMilli(), messageID, hash, jobState, time.Now().UnixMilli(), id)
	return err
}

// SentOutbox finishes a row. For progress panels this means the job reached a
// final state and the last edit carried the final counts.
func (s *Store) SentOutbox(ctx context.Context, id string, messageID int) error {
	_, err := s.db.ExecContext(ctx, "UPDATE notification_outbox SET state='sent', message_id=?, updated_at=? WHERE id=?", messageID, time.Now().UnixMilli(), id)
	return err
}

// RetryOutbox schedules another attempt, for example after a 429.
func (s *Store) RetryOutbox(ctx context.Context, id string, at time.Time, code string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE notification_outbox
		SET attempts=attempts+1, next_send=?, error_code=?, updated_at=?
		WHERE id=?`, at.UnixMilli(), code, time.Now().UnixMilli(), id)
	return err
}

// DeadOutbox marks a message undeliverable (blocked bot, 403). The job is not
// touched.
func (s *Store) DeadOutbox(ctx context.Context, id string, code string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE notification_outbox SET state='dead', error_code=?, updated_at=? WHERE id=?", code, time.Now().UnixMilli(), id)
	return err
}
