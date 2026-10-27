package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

const batchOpenPrefix = "batch_notice_open:"
const batchDonePrefix = "batch_notice_done:"
const BatchNoticePrefix = "batch-final:"

type BatchNotice struct {
	Principal, BatchID string
	At                 time.Time
}

func batchNoticeSuffix(principal, batch string) string { return principal + ":" + batch }
func (s *Store) SealBatchNotice(ctx context.Context, principal, batch string, at time.Time) error {
	suffix := batchNoticeSuffix(principal, batch)
	b, _ := json.Marshal(BatchNotice{principal, batch, at})
	_, err := s.db.ExecContext(ctx, `INSERT INTO settings(key,value) SELECT ?,? WHERE NOT EXISTS(SELECT 1 FROM settings WHERE key=?) ON CONFLICT(key) DO NOTHING`, batchOpenPrefix+suffix, string(b), batchDonePrefix+suffix)
	return err
}
func (s *Store) OpenBatchNotices(ctx context.Context) ([]BatchNotice, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT value FROM settings WHERE key>=? AND key<? ORDER BY key LIMIT 100", batchOpenPrefix, batchOpenPrefix+"~")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BatchNotice
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		var v BatchNotice
		if err = json.Unmarshal([]byte(raw), &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Store) BatchNoticeKnown(ctx context.Context, principal, batch string) (bool, error) {
	suffix := batchNoticeSuffix(principal, batch)
	var found bool
	err := s.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM settings WHERE key IN (?,?))", batchOpenPrefix+suffix, batchDonePrefix+suffix).Scan(&found)
	return found, err
}
func (s *Store) FinishBatchNotice(ctx context.Context, principal, batch string, chat int64, at time.Time) error {
	suffix := batchNoticeSuffix(principal, batch)
	return s.tx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, "INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO NOTHING", batchDonePrefix+suffix, "1")
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n > 0 && chat != 0 {
			_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO notification_outbox(id,kind,principal,job_id,chat_id,state,next_send,note,coalesce_key,created_at,updated_at) VALUES(?,'final',?,'',?,'pending',?,?,?,?,?)`, ID("out"), principal, chat, at.UnixMilli(), BatchNoticePrefix+batch, BatchNoticePrefix+suffix, at.UnixMilli(), at.UnixMilli())
			if err != nil {
				return err
			}
		}
		_, err = tx.ExecContext(ctx, "DELETE FROM settings WHERE key=?", batchOpenPrefix+suffix)
		return err
	})
}

// FenceBatchSend records a potentially uncertain send before it leaves the process.
func (s *Store) FenceBatchSend(ctx context.Context, id string) (bool, error) {
	res, err := s.db.ExecContext(ctx, "UPDATE notification_outbox SET state='sent',error_code='delivery_unknown' WHERE id=? AND state='pending' AND note LIKE 'batch-final:%'", id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

func (s *Store) RequeueRejectedBatchSend(ctx context.Context, id string, at time.Time) error {
	_, err := s.db.ExecContext(ctx, "UPDATE notification_outbox SET state='pending',next_send=?,error_code='rate_limited',attempts=attempts+1 WHERE id=? AND state='sent' AND error_code='delivery_unknown'", at.UnixMilli(), id)
	return err
}
func (s *Store) ConfirmBatchSend(ctx context.Context, id string, messageID int) error {
	_, err := s.db.ExecContext(ctx, "UPDATE notification_outbox SET state='sent',message_id=?,error_code='' WHERE id=?", messageID, id)
	return err
}
