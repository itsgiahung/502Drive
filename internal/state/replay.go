package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math/rand/v2"
	"time"
)

// Replay buffer limits. They are deliberately small: the buffer exists to
// bridge one baseline scan, not to become a second change log. When it fills,
// the subscription asks for a full reconcile instead of replaying a set that is
// known to be incomplete.
const (
	MaxReplayChanges = 5000
	MaxReplayBytes   = 4 << 20
)

// ReplayChange is the minimal change needed to decide whether a feed entry
// concerns a copied tree. Full file metadata is not kept.
type ReplayChange struct {
	FileID  string   `json:"file_id"`
	Removed bool     `json:"removed"`
	Parents []string `json:"parents,omitempty"`
	Trashed bool     `json:"trashed,omitempty"`
}

// nextFullReconcile spreads periodic reconciles by ±10% so a fleet of
// collections does not wake up in the same minute.
func nextFullReconcile(now time.Time, everySeconds int64) int64 {
	if everySeconds <= 0 {
		return 0
	}
	f := 0.9 + 0.2*rand.Float64()
	return now.Add(time.Duration(float64(everySeconds) * f * float64(time.Second))).UnixMilli()
}

// AppendPendingChanges stores one feed page while the subscription is
// bootstrapping. It never advances the subscription cursor: the cursor moves
// with the replay (FinishReplay), so a crash cannot skip a page. It returns
// overflow=true when the buffer cap was hit; the rows stored so far are kept
// only for the "discard and reconcile" path.
func (s *Store) AppendPendingChanges(ctx context.Context, collectionID string, changes []ReplayChange, cursorAfter string, nextPoll time.Time) (overflow bool, err error) {
	err = s.tx(ctx, func(tx *sql.Tx) error {
		sub, e := getSub(ctx, tx, collectionID)
		if errors.Is(e, ErrNotFound) {
			return ErrTransition
		}
		if e != nil {
			return e
		}
		if sub.State != SyncBootstrapping || sub.Mode != ModeRealtime {
			return ErrTransition
		}
		if sub.ReplayOverflow {
			overflow = true
			return nil
		}
		for _, ch := range changes {
			b, e := json.Marshal(ch)
			if e != nil {
				return e
			}
			if sub.ReplayCount >= MaxReplayChanges || sub.ReplayBytes+int64(len(b)) > MaxReplayBytes {
				overflow = true
				break
			}
			sub.ReplayCount++
			if _, e = tx.ExecContext(ctx, "INSERT INTO sync_pending_changes(collection_id,seq,cursor_after,change_json) VALUES(?,?,?,?)",
				collectionID, sub.ReplayCount, cursorAfter, string(b)); e != nil {
				return e
			}
			sub.ReplayBytes += int64(len(b))
		}
		sub.NextPollAt = nextPoll.UnixMilli()
		if overflow {
			sub.ReplayOverflow = true
		}
		return putSub(ctx, tx, sub)
	})
	if errors.Is(err, ErrTransition) {
		// Paused, stopped or reconfigured while the page was read: the page is
		// not stored, the old cursor stays, the next active poll re-reads it.
		return false, nil
	}
	return overflow, err
}

// ReplayBatch reads the next slice of the pending buffer in feed order.
func (s *Store) ReplayBatch(ctx context.Context, collectionID string, limit int) (changes []ReplayChange, finalCursor string, overflow bool, err error) {
	if limit < 1 || limit > MaxReplayChanges {
		limit = MaxReplayChanges
	}
	sub, err := s.GetSubscription(ctx, collectionID)
	if err != nil {
		return nil, "", false, err
	}
	rows, err := s.db.QueryContext(ctx, "SELECT change_json,cursor_after FROM sync_pending_changes WHERE collection_id=? ORDER BY seq LIMIT ?", collectionID, limit)
	if err != nil {
		return nil, "", sub.ReplayOverflow, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw, after string
		if err = rows.Scan(&raw, &after); err != nil {
			return nil, "", sub.ReplayOverflow, err
		}
		var ch ReplayChange
		if err = json.Unmarshal([]byte(raw), &ch); err != nil {
			return nil, "", sub.ReplayOverflow, err
		}
		changes = append(changes, ch)
		finalCursor = after
	}
	return changes, finalCursor, sub.ReplayOverflow, rows.Err()
}

// LastPendingCursor returns the cursor after the newest stored page, so a
// restarted poller resumes behind the buffer instead of re-reading (and
// duplicating) pages that are already durable.
func (s *Store) LastPendingCursor(ctx context.Context, collectionID string) (string, bool, error) {
	var c string
	err := s.db.QueryRowContext(ctx, "SELECT cursor_after FROM sync_pending_changes WHERE collection_id=? ORDER BY seq DESC LIMIT 1", collectionID).Scan(&c)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return c, err == nil, err
}

// PendingCollections lists collections with rows waiting for replay.
func (s *Store) PendingCollections(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT DISTINCT collection_id FROM sync_pending_changes ORDER BY collection_id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// DiscardReplay drops the buffer and clears the overflow flag. Used when the
// buffer overflowed (a full reconcile follows) or when replay is impossible.
func (s *Store) DiscardReplay(ctx context.Context, collectionID string) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		sub, err := getSub(ctx, tx, collectionID)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM sync_pending_changes WHERE collection_id=?", collectionID); err != nil {
			return err
		}
		sub.ReplayOverflow, sub.ReplayCount, sub.ReplayBytes = false, 0, 0
		return putSub(ctx, tx, sub)
	})
}

func clearPendingChanges(ctx context.Context, tx *sql.Tx, collectionID string) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM sync_pending_changes WHERE collection_id=?", collectionID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, "UPDATE sync_subscriptions SET replay_count=0, replay_bytes=0, replay_overflow=0 WHERE collection_id=?", collectionID)
	return err
}

// FinishReplay advances the cursor to the last replayed page and admits the
// run the changes need — all in one transaction, guarded by the cursor the
// caller read. A stale snapshot (pause/stop/another poll) returns ErrTransition
// and leaves the buffer untouched.
func (s *Store) FinishReplay(ctx context.Context, collectionID, expectedCursor, finalCursor string, changed bool, nextPoll time.Time) (*Job, error) {
	var out *Job
	err := s.tx(ctx, func(tx *sql.Tx) error {
		sub, e := getSub(ctx, tx, collectionID)
		if e != nil {
			return e
		}
		if sub.State != SyncActive || sub.Cursor != expectedCursor {
			return ErrTransition
		}
		if finalCursor != "" {
			sub.Cursor = finalCursor
		}
		sub.NextPollAt = nextPoll.UnixMilli()
		sub.ReplayOverflow, sub.ReplayCount, sub.ReplayBytes = false, 0, 0
		if _, e = tx.ExecContext(ctx, "DELETE FROM sync_pending_changes WHERE collection_id=?", collectionID); e != nil {
			return e
		}
		if changed {
			var cb []byte
			if e = tx.QueryRowContext(ctx, "SELECT body FROM collections WHERE id=?", collectionID).Scan(&cb); e != nil {
				return e
			}
			c, _ := decodeCollection(cb)
			j, coalesced, e := admitRun(ctx, tx, c, "realtime", "")
			if e != nil {
				if errors.Is(e, ErrNeedsAction) {
					sub.State, sub.LastError = SyncError, &Failure{Code: "needs_action", Message: "The copied source needs attention before it can sync."}
				} else {
					return e
				}
			} else if !coalesced {
				out = &j
				la := time.Now().UTC()
				sub.LastAttemptAt = &la
			}
			if e = keepRerun(ctx, tx, collectionID, &sub); e != nil {
				return e
			}
		}
		return putSub(ctx, tx, sub)
	})
	return out, err
}

// ClaimDueFullReconciles admits one delta run per realtime subscription whose
// periodic full reconcile is due. The next slot is jittered and stored before
// the run starts, so a slow run never causes a second one. A full queue keeps
// the slot due (retried next tick) instead of skipping the reconcile.
func (s *Store) ClaimDueFullReconciles(ctx context.Context, now time.Time) ([]Job, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT collection_id FROM sync_subscriptions
		WHERE mode='realtime' AND state='active' AND full_reconcile_every>0
		AND (next_full_reconcile_at=0 OR next_full_reconcile_at<=?) ORDER BY next_full_reconcile_at LIMIT 50`, now.UnixMilli())
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	var jobs []Job
	for _, id := range ids {
		var j Job
		var made bool
		err := s.tx(ctx, func(tx *sql.Tx) error {
			sub, e := getSub(ctx, tx, id)
			if e != nil || sub.State != SyncActive || sub.Mode != ModeRealtime || sub.FullReconcileEvery <= 0 {
				return e
			}
			if sub.NextFullReconcileAt == 0 {
				// A subscription from before this slot existed: schedule the
				// first reconcile one interval from now instead of firing a
				// fleet-wide reconcile on upgrade.
				sub.NextFullReconcileAt = nextFullReconcile(now, sub.FullReconcileEvery)
				sub.Revision++
				return putSub(ctx, tx, sub)
			}
			if sub.NextFullReconcileAt > now.UnixMilli() {
				return nil
			}
			var cb []byte
			if e = tx.QueryRowContext(ctx, "SELECT body FROM collections WHERE id=?", id).Scan(&cb); e != nil {
				return e
			}
			c, _ := decodeCollection(cb)
			run, coalesced, e := admitRun(ctx, tx, c, "full_reconcile", "")
			switch {
			case errors.Is(e, ErrCapacity):
				return e // keep the slot due
			case errors.Is(e, ErrNeedsAction):
				sub.LastError = &Failure{Code: "needs_action", Message: "The copied source needs attention before it can sync."}
			case e != nil:
				return e
			default:
				j, made = run, !coalesced
			}
			if e = keepRerun(ctx, tx, id, &sub); e != nil {
				return e
			}
			sub.NextFullReconcileAt = nextFullReconcile(now, sub.FullReconcileEvery)
			sub.Revision++
			return putSub(ctx, tx, sub)
		})
		if errors.Is(err, ErrCapacity) {
			continue
		}
		if err != nil {
			return jobs, err
		}
		if made {
			jobs = append(jobs, j)
		}
	}
	return jobs, nil
}

// ItemTotals counts the mapping: every non-root item and the ones already done.
// Used by the mass-removal guard; it is a single indexed aggregate, not a scan
// over the whole destination tree.
func (s *Store) ItemTotals(ctx context.Context, collectionID string) (total, done int, err error) {
	err = s.db.QueryRowContext(ctx, `SELECT count(*), coalesce(sum(state='done'),0) FROM items
		WHERE collection_id=? AND NOT (parent_source_id='' AND (kind='folder' OR kind='wrapper'))`, collectionID).Scan(&total, &done)
	return
}

// admitWatchAudit creates the read-only audit the engine requests after a
// suspicious reconcile. It bypasses the HTTP idempotency layer on purpose: the
// caller is the daemon, and the job is deduplicated by the one-active-run rule.
func admitWatchAudit(ctx context.Context, tx *sql.Tx, c Collection) (Job, error) {
	if id, err := activeRun(ctx, tx, c.ID); err != nil {
		return Job{}, err
	} else if id != "" {
		return Job{}, ErrTransition
	}
	var principal string
	if err := tx.QueryRowContext(ctx, "SELECT principal FROM collections WHERE id=?", c.ID).Scan(&principal); err != nil {
		return Job{}, err
	}
	var total, own int
	if err := tx.QueryRowContext(ctx, "SELECT count(*),coalesce(sum(principal=?),0) FROM jobs WHERE state IN ('queued','running','pausing','cancelling','reconciling')", principal).Scan(&total, &own); err != nil {
		return Job{}, err
	}
	if total >= maxActiveTotal || own >= maxActivePerUser {
		return Job{}, ErrCapacity
	}
	now := time.Now().UTC()
	j := Job{ID: ID("job"), Principal: principal, CollectionID: c.ID, Kind: JobKindAudit, Trigger: "reconcile_watch",
		SourceName: c.SourceName, DestinationID: c.DestinationID, RootName: c.RootName,
		State: JobQueued, DesiredState: "run", Revision: 1, CreatedAt: now, UpdatedAt: now}
	jb, _ := json.Marshal(j)
	if _, err := tx.ExecContext(ctx, "INSERT INTO jobs VALUES(?,?,?,?,?)", j.ID, principal, c.ID, j.State, jb); err != nil {
		return Job{}, err
	}
	a := Audit{ID: ID("aud"), CollectionID: c.ID, JobID: j.ID, State: AuditRunning, Counts: map[string]int{}, Revision: 1, StartedAt: now}
	if _, err := tx.ExecContext(ctx, "INSERT INTO audits(id,collection_id,job_id,state,counts,started_at) VALUES(?,?,?,?,?,?)",
		a.ID, a.CollectionID, a.JobID, a.State, "{}", now.UnixMilli()); err != nil {
		return Job{}, err
	}
	if err := appendEvent(ctx, tx, j, "admitted", map[string]any{"trigger": "reconcile_watch"}); err != nil {
		return Job{}, err
	}
	return j, nil
}
