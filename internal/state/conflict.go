package state

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// Conflict policies: what to do when the destination name is already taken.
// Nothing is ever overwritten under any of them.
const (
	ConflictSkip   = "skip"
	ConflictRename = "rename" // default: " (2)", " (3)", ...
	ConflictUpdate = "update" // the same name on purpose; old objects are left alone
)

// Decisions stored on an item (items.conflict_action). They are written in the
// same transaction that reserves the name, so a restart never re-decides.
const (
	DecisionNone   = ""       // the planned name was free
	DecisionRename = "rename" // a numbered name was chosen (items.dest_name)
	DecisionUpdate = "update" // the same name was used on purpose (duplicate allowed)
	DecisionSkip   = "skip"   // not written; error_code says why
)

// holderPredicate selects items that currently reserve a destination name
// under the same parent. Skipped items and "skip" decisions reserve nothing.
const holderPredicate = `dest_parent_id=? AND dest_name=? AND dest_name<>'' AND conflict_action<>'skip' AND state<>'skipped' AND NOT (collection_id=? AND source_id=?)`

// ClaimDestName reserves name under the item's destination parent and records
// the decision. ok=false means another item already reserves that name. With
// force (the update policy) the uniqueness check is skipped on purpose.
func (s *Store) ClaimDestName(ctx context.Context, it Item, action, name string, force bool) (bool, error) {
	claimed := false
	err := s.tx(ctx, func(tx *sql.Tx) error {
		if !force {
			var taken bool
			if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM items WHERE `+holderPredicate+`)`,
				it.DestParentID, name, it.CollectionID, it.SourceID).Scan(&taken); err != nil {
				return err
			}
			if taken {
				return nil
			}
		}
		if _, err := tx.ExecContext(ctx, "UPDATE items SET conflict_action=?, dest_name=? WHERE collection_id=? AND source_id=?",
			action, name, it.CollectionID, it.SourceID); err != nil {
			return err
		}
		claimed = true
		return nil
	})
	return claimed, err
}

// NameHolder returns one other item that reserves the same destination name
// under the same parent, if any.
func (s *Store) NameHolder(ctx context.Context, it Item, name string) (Item, bool, error) {
	row := s.db.QueryRowContext(ctx, "SELECT "+itemCols+" FROM items WHERE "+holderPredicate+" LIMIT 1",
		it.DestParentID, name, it.CollectionID, it.SourceID)
	h, err := scanItem(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Item{}, false, nil
	}
	if err != nil {
		return Item{}, false, err
	}
	return h, true, nil
}

// SkipWithDecision marks an item skipped with a stable code and the decision
// in one statement, so a crash never leaves a skip half-recorded.
func (s *Store) SkipWithDecision(ctx context.Context, it Item, code, decision string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE items SET state='skipped', error_code=?, conflict_action=? WHERE collection_id=? AND source_id=?",
		code, decision, it.CollectionID, it.SourceID)
	return err
}

// ContentFingerprintMD5 extracts the md5 from an item fingerprint
// ("v<version>|<md5>|<name>"). It returns "" when Drive gave no md5 (Google
// native files), which must never count as "same content".
func ContentFingerprintMD5(fp string) string {
	parts := strings.SplitN(fp, "|", 3)
	if len(parts) < 2 {
		return ""
	}
	return parts[1]
}

// SameContent reports whether two file items are known to hold identical bytes:
// equal size and a non-empty, equal md5.
func SameContent(a, b Item) bool {
	if a.Kind != KindFile || b.Kind != KindFile || a.Size == nil || b.Size == nil || *a.Size != *b.Size {
		return false
	}
	ma := ContentFingerprintMD5(a.Fingerprint)
	return ma != "" && ma == ContentFingerprintMD5(b.Fingerprint)
}
