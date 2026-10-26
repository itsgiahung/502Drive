package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"time"
)

type Operator struct {
	ID             string `json:"id"`
	TelegramUserID int64  `json:"telegram_user_id"`
	Enabled        bool   `json:"enabled"`
}

// AddOperator allows a Telegram user to use the service as an operator.
// Re-adding a disabled operator re-enables the same principal.
func (s *Store) AddOperator(ctx context.Context, actor, key string, telegramID int64) (Operator, bool, error) {
	body, replayed, err := s.idempotent(ctx, actor, "POST /v1/operators", key, telegramID, func(tx *sql.Tx) (any, error) {
		ext := strconv.FormatInt(telegramID, 10)
		var op Operator
		var role string
		var en int
		e := tx.QueryRowContext(ctx, "SELECT id,role,enabled FROM principals WHERE kind='telegram' AND external_id=?", ext).Scan(&op.ID, &role, &en)
		switch {
		case e == nil:
			if role != "operator" {
				return nil, ErrTransition
			}
			if _, e = tx.ExecContext(ctx, "UPDATE principals SET enabled=1 WHERE id=?", op.ID); e != nil {
				return nil, e
			}
		case errors.Is(e, sql.ErrNoRows):
			op.ID = ID("op")
			if _, e = tx.ExecContext(ctx, "INSERT INTO principals VALUES(?,?,?,?,1,?)", op.ID, "telegram", ext, "operator", time.Now().UnixMilli()); e != nil {
				return nil, e
			}
		default:
			return nil, e
		}
		op.TelegramUserID, op.Enabled = telegramID, true
		return op, nil
	})
	if err != nil {
		return Operator{}, false, err
	}
	var out Operator
	return out, replayed, json.Unmarshal(body, &out)
}

func (s *Store) Operators(ctx context.Context) ([]Operator, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id,external_id,enabled FROM principals WHERE kind='telegram' AND role='operator' ORDER BY created_at,id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Operator{}
	for rows.Next() {
		var o Operator
		var ext string
		var en int
		if err = rows.Scan(&o.ID, &ext, &en); err != nil {
			return nil, err
		}
		o.TelegramUserID, _ = strconv.ParseInt(ext, 10, 64)
		o.Enabled = en == 1
		out = append(out, o)
	}
	return out, rows.Err()
}

// DisableOperator revokes an operator. Owners cannot be removed this way.
func (s *Store) DisableOperator(ctx context.Context, actor, key, id string) error {
	_, _, err := s.idempotent(ctx, actor, "DELETE /v1/operators/"+id, key, id, func(tx *sql.Tx) (any, error) {
		res, e := tx.ExecContext(ctx, "UPDATE principals SET enabled=0 WHERE id=? AND role='operator'", id)
		if e != nil {
			return nil, e
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return nil, ErrNotFound
		}
		return map[string]string{"id": id}, nil
	})
	return err
}

// OperatorByTelegram resolves an enabled operator for a verified Telegram user.
func (s *Store) OperatorByTelegram(ctx context.Context, telegramID int64) (string, bool, error) {
	var id string
	err := s.db.QueryRowContext(ctx, "SELECT id FROM principals WHERE kind='telegram' AND external_id=? AND role='operator' AND enabled=1", strconv.FormatInt(telegramID, 10)).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return id, err == nil, err
}
