package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/itsgiahung/502Drive/internal/schedule"
)

// Subscription states (match the HTTP contract).
const (
	SyncWaitingBaseline = "waiting_for_baseline"
	SyncBootstrapping   = "bootstrapping"
	SyncActive          = "active"
	SyncPaused          = "paused"
	SyncStopping        = "stopping"
	SyncStopped         = "stopped"
	SyncNeedsReconcile  = "needs_reconcile"
	SyncError           = "error"

	ModeScheduled = "scheduled"
	ModeRealtime  = "realtime"

	PolicyVersioned = "versioned_copy"
	PolicyReplace   = "replace_copy"
)

var (
	ErrNeedsAction = errors.New("needs_action")
	// ErrDuplicateSlot means the occurrence was already claimed.
	ErrDuplicateSlot = errors.New("duplicate_occurrence")
)

type Subscription struct {
	ID               string
	CollectionID     string
	Mode             string
	State            string
	Schedule         *schedule.Spec
	ScheduleRevision int
	ContentPolicy    string
	Revision         int
	NextDue          *time.Time
	NextKey          string
	LastAttemptAt    *time.Time
	LastSuccessAt    *time.Time
	LastError        *Failure
	Cursor           string
	RerunRequested   bool
	NextPollAt       int64
	// FullReconcileEvery is the periodic full reconcile interval in seconds
	// (0 disables it); NextFullReconcileAt is epoch milliseconds.
	FullReconcileEvery  int64
	NextFullReconcileAt int64
	// ReplayOverflow means the replay buffer hit its cap while a baseline was
	// running; the next activation asks for a full reconcile instead of
	// replaying a set that is known to be incomplete.
	ReplayOverflow bool
	// ReplayCount/ReplayBytes mirror the buffer size so the cap check is a
	// column read, not a COUNT(*) per poll.
	ReplayCount int64
	ReplayBytes int64
}

const subCols = "id,collection_id,mode,state,schedule,schedule_revision,content_policy,revision,next_due,next_key,last_attempt_at,last_success_at,last_error,cursor,rerun_requested,next_poll_at,full_reconcile_every,next_full_reconcile_at,replay_overflow,replay_count,replay_bytes"

func msTime(v sql.NullInt64) *time.Time {
	if !v.Valid {
		return nil
	}
	t := time.UnixMilli(v.Int64).UTC()
	return &t
}

func msVal(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UnixMilli()
}

func scanSub(r interface{ Scan(...any) error }) (Subscription, error) {
	var s Subscription
	var sched, lastErr string
	var nd, la, ls sql.NullInt64
	var rerun, overflow int
	err := r.Scan(&s.ID, &s.CollectionID, &s.Mode, &s.State, &sched, &s.ScheduleRevision, &s.ContentPolicy, &s.Revision, &nd, &s.NextKey, &la, &ls, &lastErr, &s.Cursor, &rerun, &s.NextPollAt,
		&s.FullReconcileEvery, &s.NextFullReconcileAt, &overflow, &s.ReplayCount, &s.ReplayBytes)
	if err != nil {
		return s, err
	}
	s.NextDue, s.LastAttemptAt, s.LastSuccessAt, s.RerunRequested = msTime(nd), msTime(la), msTime(ls), rerun == 1
	s.ReplayOverflow = overflow == 1
	if sched != "" {
		var sp schedule.Spec
		if json.Unmarshal([]byte(sched), &sp) == nil {
			s.Schedule = &sp
		}
	}
	if lastErr != "" {
		var f Failure
		if json.Unmarshal([]byte(lastErr), &f) == nil {
			s.LastError = &f
		}
	}
	return s, nil
}

func getSub(ctx context.Context, q queryer, collectionID string) (Subscription, error) {
	s, err := scanSub(q.QueryRowContext(ctx, "SELECT "+subCols+" FROM sync_subscriptions WHERE collection_id=?", collectionID))
	if errors.Is(err, sql.ErrNoRows) {
		return s, ErrNotFound
	}
	return s, err
}

func putSub(ctx context.Context, q queryer, s Subscription) error {
	sched, lastErr := "", ""
	if s.Schedule != nil {
		b, _ := json.Marshal(s.Schedule)
		sched = string(b)
	}
	if s.LastError != nil {
		b, _ := json.Marshal(s.LastError)
		lastErr = string(b)
	}
	rerun := 0
	if s.RerunRequested {
		rerun = 1
	}
	overflow := 0
	if s.ReplayOverflow {
		overflow = 1
	}
	_, err := q.ExecContext(ctx, `INSERT INTO sync_subscriptions(`+subCols+`) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(collection_id) DO UPDATE SET mode=excluded.mode,state=excluded.state,schedule=excluded.schedule,schedule_revision=excluded.schedule_revision,
		content_policy=excluded.content_policy,revision=excluded.revision,next_due=excluded.next_due,next_key=excluded.next_key,last_attempt_at=excluded.last_attempt_at,
		last_success_at=excluded.last_success_at,last_error=excluded.last_error,cursor=excluded.cursor,rerun_requested=excluded.rerun_requested,next_poll_at=excluded.next_poll_at,
		full_reconcile_every=excluded.full_reconcile_every,next_full_reconcile_at=excluded.next_full_reconcile_at,replay_overflow=excluded.replay_overflow,
		replay_count=excluded.replay_count,replay_bytes=excluded.replay_bytes`,
		s.ID, s.CollectionID, s.Mode, s.State, sched, s.ScheduleRevision, s.ContentPolicy, s.Revision, msVal(s.NextDue), s.NextKey, msVal(s.LastAttemptAt), msVal(s.LastSuccessAt), lastErr, s.Cursor, rerun, s.NextPollAt,
		s.FullReconcileEvery, s.NextFullReconcileAt, overflow, s.ReplayCount, s.ReplayBytes)
	return err
}

func (s *Store) GetSubscription(ctx context.Context, collectionID string) (Subscription, error) {
	return getSub(ctx, s.db, collectionID)
}

// ActiveRunID returns the collection's in-flight job, if any.
func activeRun(ctx context.Context, q queryer, collectionID string) (string, error) {
	var id string
	err := q.QueryRowContext(ctx, "SELECT id FROM jobs WHERE collection_id=? AND state IN ('queued','running','pausing','paused','cancelling','reconciling','blocked') ORDER BY rowid DESC LIMIT 1", collectionID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return id, err
}

func (s *Store) ActiveRunID(ctx context.Context, collectionID string) (string, error) {
	return activeRun(ctx, s.db, collectionID)
}

// SyncChange describes a requested subscription configuration.
type SyncChange struct {
	Mode                 string
	Schedule             *schedule.Spec
	ContentPolicy        string
	ExpectedCollection   int
	ExpectedSubscription *int
	// Computed by the caller from the current time and the provider.
	NextDue *schedule.Slot
	Cursor  string
	Now     time.Time
	// FullReconcileEvery is the periodic full reconcile interval in seconds
	// (0 disables it). The caller reads it from config.
	FullReconcileEvery int64
}

// ConfigureSync creates or updates the single subscription of a collection.
// For realtime on a ready collection the cursor must have been captured by the
// caller before this call; a reconcile run is admitted in the same commit.
func (s *Store) ConfigureSync(ctx context.Context, actor, scopePrincipal, key, collectionID string, ch SyncChange) (Subscription, bool, error) {
	var out Subscription
	body, replayed, err := s.idempotent(ctx, actor, "PUT /v1/collections/"+collectionID+"/sync", key, ch.fingerprint(), func(tx *sql.Tx) (any, error) {
		var cb []byte
		e := tx.QueryRowContext(ctx, "SELECT body FROM collections WHERE id=? AND (?='' OR principal=?)", collectionID, scopePrincipal, scopePrincipal).Scan(&cb)
		if errors.Is(e, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		if e != nil {
			return nil, e
		}
		c, e := decodeCollection(cb)
		if e != nil {
			return nil, e
		}
		if c.Revision != ch.ExpectedCollection {
			return nil, ErrStale
		}
		cur, e := getSub(ctx, tx, collectionID)
		exists := e == nil
		if e != nil && !errors.Is(e, ErrNotFound) {
			return nil, e
		}
		if exists != (ch.ExpectedSubscription != nil) || (exists && cur.Revision != *ch.ExpectedSubscription) {
			return nil, ErrStale
		}
		sub := cur
		if !exists {
			sub = Subscription{ID: ID("sub"), CollectionID: collectionID, Revision: 0, ScheduleRevision: 0}
		}
		sub.Mode, sub.ContentPolicy, sub.Schedule = ch.Mode, ch.ContentPolicy, ch.Schedule
		sub.FullReconcileEvery = ch.FullReconcileEvery
		sub.ReplayOverflow, sub.ReplayCount, sub.ReplayBytes = false, 0, 0
		sub.Revision++
		sub.ScheduleRevision++
		sub.NextDue, sub.NextKey, sub.Cursor, sub.RerunRequested, sub.LastError = nil, "", "", false, nil
		sub.NextPollAt = 0
		sub.NextFullReconcileAt = 0
		ready := c.State == "ready"
		var admit bool
		switch {
		case !ready:
			sub.State = SyncWaitingBaseline
		case ch.Mode == ModeScheduled:
			sub.State = SyncActive
			sub.NextDue, sub.NextKey = &ch.NextDue.At, ch.NextDue.Key
		default:
			sub.State, sub.Cursor, admit = SyncBootstrapping, ch.Cursor, true
			sub.NextFullReconcileAt = nextFullReconcile(ch.Now, sub.FullReconcileEvery)
		}
		if e = putSub(ctx, tx, sub); e != nil {
			return nil, e
		}
		if admit {
			if _, _, e = admitRun(ctx, tx, c, "reconcile", ""); e != nil {
				return nil, e
			}
		}
		out = sub
		return subJSON(sub), nil
	})
	if err != nil {
		return Subscription{}, false, err
	}
	var got Subscription
	if replayed {
		if got, err = s.GetSubscription(ctx, collectionID); err != nil {
			return Subscription{}, true, err
		}
		return got, true, nil
	}
	_ = body
	return out, false, nil
}

func (c SyncChange) fingerprint() any {
	sp, _ := json.Marshal(c.Schedule)
	return []any{c.Mode, string(sp), c.ContentPolicy, c.ExpectedCollection, c.ExpectedSubscription, c.FullReconcileEvery}
}

func subJSON(s Subscription) map[string]any {
	return map[string]any{"id": s.ID, "revision": s.Revision}
}

// SyncAction applies pause, resume or stop with a revision guard.
func (s *Store) SyncAction(ctx context.Context, actor, scopePrincipal, key, collectionID, action string, expected int, now time.Time, next func(Subscription) (*schedule.Slot, error)) (Subscription, bool, error) {
	var out Subscription
	_, replayed, err := s.idempotent(ctx, actor, "POST /v1/collections/"+collectionID+"/sync/"+action, key, expected, func(tx *sql.Tx) (any, error) {
		var one int
		e := tx.QueryRowContext(ctx, "SELECT 1 FROM collections WHERE id=? AND (?='' OR principal=?)", collectionID, scopePrincipal, scopePrincipal).Scan(&one)
		if errors.Is(e, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		if e != nil {
			return nil, e
		}
		sub, e := getSub(ctx, tx, collectionID)
		if e != nil {
			return nil, e
		}
		if sub.Revision != expected {
			return nil, ErrStale
		}
		switch action {
		case "pause":
			if sub.State != SyncActive && sub.State != SyncBootstrapping && sub.State != SyncNeedsReconcile {
				return nil, ErrTransition
			}
			sub.State, sub.RerunRequested = SyncPaused, false
		case "resume":
			if sub.State != SyncPaused && sub.State != SyncStopped && sub.State != SyncError && sub.State != SyncNeedsReconcile {
				return nil, ErrTransition
			}
			var cs []byte
			if e = tx.QueryRowContext(ctx, "SELECT body FROM collections WHERE id=?", collectionID).Scan(&cs); e != nil {
				return nil, e
			}
			c, _ := decodeCollection(cs)
			switch {
			case c.State != "ready":
				sub.State = SyncWaitingBaseline
			case sub.Mode == ModeScheduled:
				sub.State = SyncActive
				// A schedule that was due while paused catches up exactly once.
				if sub.NextDue == nil || sub.NextDue.After(now) {
					slot, e := next(sub)
					if e != nil {
						return nil, ErrTransition
					}
					sub.NextDue, sub.NextKey = &slot.At, slot.Key
				} else {
					sub.NextKey = "catchup-" + now.UTC().Format(time.RFC3339)
				}
			case sub.State == SyncNeedsReconcile || sub.Cursor == "":
				sub.State, sub.NextPollAt = SyncNeedsReconcile, 0
			default:
				sub.State, sub.NextPollAt = SyncActive, 0
			}
			sub.LastError = nil
		case "stop":
			if sub.State == SyncStopped {
				return nil, ErrTransition
			}
			sub.NextDue, sub.NextKey, sub.RerunRequested = nil, "", false
			sub.State = SyncStopped
			// Automatic work that has not started is cancelled; files stay.
			if _, e = tx.ExecContext(ctx, `UPDATE jobs SET state='cancelled', body=json_set(body,'$.state','cancelled','$.desired_state','cancel','$.revision',json_extract(body,'$.revision')+1)
				WHERE collection_id=? AND state IN ('queued','paused') AND json_extract(body,'$.trigger') IN ('scheduled','realtime','reconcile','coalesced','full_reconcile')`, collectionID); e != nil {
				return nil, e
			}
			res, e := tx.ExecContext(ctx, `UPDATE jobs SET state='cancelling', body=json_set(body,'$.state','cancelling','$.desired_state','cancel','$.revision',json_extract(body,'$.revision')+1)
				WHERE collection_id=? AND state IN ('running','pausing','reconciling') AND json_extract(body,'$.trigger') IN ('scheduled','realtime','reconcile','coalesced','full_reconcile')`, collectionID)
			if e != nil {
				return nil, e
			}
			if n, _ := res.RowsAffected(); n > 0 {
				sub.State = SyncStopping
			}
		default:
			return nil, ErrNotFound
		}
		sub.Revision++
		if e = putSub(ctx, tx, sub); e != nil {
			return nil, e
		}
		out = sub
		return subJSON(sub), nil
	})
	if err != nil {
		return Subscription{}, false, err
	}
	if replayed {
		got, e := s.GetSubscription(ctx, collectionID)
		return got, true, e
	}
	return out, false, nil
}

// RunOutcome is returned by AdmitRun.
type RunOutcome struct {
	Job       Job
	Coalesced bool
}

// admitRun creates a delta run for a collection, or coalesces into the active one.
func admitRun(ctx context.Context, tx *sql.Tx, c Collection, trigger, occurrence string) (Job, bool, error) {
	if id, err := activeRun(ctx, tx, c.ID); err != nil {
		return Job{}, false, err
	} else if id != "" {
		if _, err = tx.ExecContext(ctx, "UPDATE sync_subscriptions SET rerun_requested=1 WHERE collection_id=? AND state IN ('active','bootstrapping')", c.ID); err != nil {
			return Job{}, false, err
		}
		j, err := readJob(ctx, tx, "", id)
		return j, true, err
	}
	var blocked int
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM items WHERE collection_id=? AND state='blocked'", c.ID).Scan(&blocked); err != nil {
		return Job{}, false, err
	}
	if blocked > 0 || (c.State != "ready" && c.State != "partial" && c.State != "cancelled") {
		return Job{}, false, ErrNeedsAction
	}
	var principal string
	if err := tx.QueryRowContext(ctx, "SELECT principal FROM collections WHERE id=?", c.ID).Scan(&principal); err != nil {
		return Job{}, false, err
	}
	var total, own int
	if err := tx.QueryRowContext(ctx, "SELECT count(*),coalesce(sum(principal=?),0) FROM jobs WHERE state IN ('queued','running','pausing','cancelling','reconciling')", principal).Scan(&total, &own); err != nil {
		return Job{}, false, err
	}
	if total >= maxActiveTotal || own >= maxActivePerUser {
		return Job{}, false, ErrCapacity
	}
	now := time.Now().UTC()
	j := Job{ID: ID("job"), Principal: principal, CollectionID: c.ID, Trigger: trigger, SourceName: c.SourceName, DestinationID: c.DestinationID, RootName: c.RootName,
		State: JobQueued, DesiredState: "run", Shortcuts: c.Shortcuts, OnConflict: c.OnConflict, Filter: c.Filter, Revision: 1, CreatedAt: now, UpdatedAt: now}
	jb, _ := json.Marshal(j)
	if _, err := tx.ExecContext(ctx, "INSERT INTO jobs VALUES(?,?,?,?,?)", j.ID, principal, c.ID, j.State, jb); err != nil {
		return Job{}, false, err
	}
	// Re-arm: every mapped folder is scanned again; earlier failures get another try.
	if _, err := tx.ExecContext(ctx, "UPDATE items SET list_state='none', page_token='', scan_job=? WHERE collection_id=? AND kind='folder' AND state='done'", j.ID, c.ID); err != nil {
		return Job{}, false, err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE items SET state='pending', job_id=?, error_code='', next_attempt_at=0 WHERE collection_id=? AND state IN ('failed','skipped','cancelled')", j.ID, c.ID); err != nil {
		return Job{}, false, err
	}
	// Work left pending by a run that was cancelled before it started is adopted.
	if _, err := tx.ExecContext(ctx, "UPDATE items SET job_id=? WHERE collection_id=? AND state='pending' AND job_id<>?", j.ID, c.ID, j.ID); err != nil {
		return Job{}, false, err
	}
	if occurrence != "" {
		var rev int
		if err := tx.QueryRowContext(ctx, "SELECT schedule_revision FROM sync_subscriptions WHERE collection_id=?", c.ID).Scan(&rev); err != nil {
			return Job{}, false, err
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO sync_occurrences(collection_id,schedule_revision,occurrence_key,job_id) VALUES(?,?,?,?)", c.ID, rev, occurrence, j.ID); err != nil {
			if strings.Contains(err.Error(), "UNIQUE") {
				return Job{}, false, ErrDuplicateSlot
			}
			return Job{}, false, err
		}
	}
	if err := appendEvent(ctx, tx, j, "admitted", map[string]any{"trigger": trigger}); err != nil {
		return Job{}, false, err
	}
	return j, false, nil
}

// keepRerun copies the transaction's rerun_requested into the caller's
// snapshot. admitRun writes that column inside the same transaction, after the
// caller read its snapshot; without this re-read the following putSub would
// write the stale false back and silently drop a coalesced follow-up run.
func keepRerun(ctx context.Context, tx *sql.Tx, collectionID string, sub *Subscription) error {
	var rerun int
	if err := tx.QueryRowContext(ctx, "SELECT rerun_requested FROM sync_subscriptions WHERE collection_id=?", collectionID).Scan(&rerun); err != nil {
		return err
	}
	sub.RerunRequested = rerun == 1
	return nil
}

// RunNow starts (or joins) a delta run on a collection regardless of sync mode.
func (s *Store) RunNow(ctx context.Context, actor, scopePrincipal, key, collectionID string, expected int) (RunOutcome, bool, error) {
	var out RunOutcome
	body, replayed, err := s.idempotent(ctx, actor, "POST /v1/collections/"+collectionID+"/runs", key, expected, func(tx *sql.Tx) (any, error) {
		var cb []byte
		e := tx.QueryRowContext(ctx, "SELECT body FROM collections WHERE id=? AND (?='' OR principal=?)", collectionID, scopePrincipal, scopePrincipal).Scan(&cb)
		if errors.Is(e, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		if e != nil {
			return nil, e
		}
		c, _ := decodeCollection(cb)
		if c.Revision != expected {
			return nil, ErrStale
		}
		j, coalesced, e := admitRun(ctx, tx, c, "manual", "")
		if e != nil {
			return nil, e
		}
		out = RunOutcome{Job: j, Coalesced: coalesced}
		return j, nil
	})
	if err != nil {
		return RunOutcome{}, false, err
	}
	if replayed {
		var j Job
		if err = json.Unmarshal(body, &j); err != nil {
			return RunOutcome{}, true, err
		}
		cur, err := s.GetJob(ctx, "", j.ID)
		return RunOutcome{Job: cur}, true, err
	}
	return out, false, nil
}

// ClaimDueSchedules admits a run for every due scheduled subscription. Each
// slot is claimed once (persistent occurrence key); missed slots collapse into
// a single catch-up run and the next due time is computed from now.
func (s *Store) ClaimDueSchedules(ctx context.Context, now time.Time) ([]Job, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT collection_id FROM sync_subscriptions WHERE mode='scheduled' AND state='active' AND next_due IS NOT NULL AND next_due<=? ORDER BY next_due LIMIT 50", now.UnixMilli())
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
			if e != nil || sub.State != SyncActive || sub.NextDue == nil || sub.NextDue.After(now) || sub.Schedule == nil {
				return e
			}
			slot, e := sub.Schedule.Next(now)
			if e != nil {
				return e
			}
			var cb []byte
			if e = tx.QueryRowContext(ctx, "SELECT body FROM collections WHERE id=?", id).Scan(&cb); e != nil {
				return e
			}
			c, _ := decodeCollection(cb)
			if c.State == CollectionRetired {
				return nil
			}
			run, coalesced, e := admitRun(ctx, tx, c, "scheduled", sub.NextKey)
			switch {
			case errors.Is(e, ErrCapacity):
				return e // keep the slot due; try again next tick
			case errors.Is(e, ErrDuplicateSlot):
				// slot already ran (clock moved backwards or DST fold): skip it
			case errors.Is(e, ErrNeedsAction):
				sub.State, sub.LastError = SyncError, &Failure{Code: "needs_action", Message: "The copied source needs attention before it can sync."}
			case e != nil:
				return e
			default:
				j, made = run, !coalesced
				la := now.UTC()
				sub.LastAttemptAt = &la
			}
			if e = keepRerun(ctx, tx, sub.CollectionID, &sub); e != nil {
				return e
			}
			sub.NextDue, sub.NextKey = &slot.At, slot.Key
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

// DuePolls lists subscriptions whose poll time has come. A bootstrapping
// subscription is polled too: the feed is stored raw for replay once the
// baseline finishes, so changes made during the scan are never lost.
func (s *Store) DuePolls(ctx context.Context, now time.Time) ([]Subscription, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+subCols+" FROM sync_subscriptions WHERE mode='realtime' AND state IN ('active','bootstrapping') AND next_poll_at<=? ORDER BY next_poll_at LIMIT 50", now.UnixMilli())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Subscription
	for rows.Next() {
		sub, e := scanSub(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, sub)
	}
	return out, rows.Err()
}

// RelevantChange reports whether a changed Drive object belongs to a
// collection's source tree (it is known, or sits in a known source folder).
func (s *Store) RelevantChange(ctx context.Context, collectionID, fileID string, parents []string) (bool, error) {
	var n int
	args := []any{collectionID, fileID}
	q := "SELECT count(*) FROM items WHERE collection_id=? AND (source_id=?"
	for _, p := range parents {
		q += " OR (kind='folder' AND source_id=?)"
		args = append(args, p)
	}
	q += ")"
	if err := s.db.QueryRowContext(ctx, q, args...).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}

// AdvancePoll stores the new feed cursor and, when relevant changes were seen,
// admits (or joins) a run — in the same transaction, so a cursor never moves
// past changes that have no run behind them.
func (s *Store) AdvancePoll(ctx context.Context, collectionID, oldCursor, newCursor string, changed bool, nextPoll time.Time) (*Job, error) {
	var out *Job
	err := s.tx(ctx, func(tx *sql.Tx) error {
		sub, e := getSub(ctx, tx, collectionID)
		if e != nil {
			return e
		}
		if sub.State != SyncActive || sub.Cursor != oldCursor {
			return ErrTransition
		}
		sub.Cursor, sub.NextPollAt = newCursor, nextPoll.UnixMilli()
		if changed {
			var cb []byte
			if e = tx.QueryRowContext(ctx, "SELECT body FROM collections WHERE id=?", collectionID).Scan(&cb); e != nil {
				return e
			}
			c, _ := decodeCollection(cb)
			if c.State == CollectionRetired {
				return nil
			}
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

// PollLater only moves the next poll time (nothing relevant happened).
func (s *Store) PollLater(ctx context.Context, collectionID string, at time.Time) error {
	_, err := s.db.ExecContext(ctx, "UPDATE sync_subscriptions SET next_poll_at=? WHERE collection_id=?", at.UnixMilli(), collectionID)
	return err
}

// MarkNeedsReconcile records that the feed cannot be trusted (expired cursor,
// lost permission). A fresh cursor taken before the reconcile scan is stored.
func (s *Store) MarkNeedsReconcile(ctx context.Context, collectionID, newCursor string, f Failure) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		sub, e := getSub(ctx, tx, collectionID)
		if e != nil {
			return e
		}
		if sub.State != SyncActive && sub.State != SyncNeedsReconcile {
			return nil
		}
		sub.LastError = &f
		if newCursor == "" {
			sub.State = SyncNeedsReconcile
			return putSub(ctx, tx, sub)
		}
		sub.Cursor, sub.State = newCursor, SyncBootstrapping
		var cb []byte
		if e = tx.QueryRowContext(ctx, "SELECT body FROM collections WHERE id=?", collectionID).Scan(&cb); e != nil {
			return e
		}
		c, _ := decodeCollection(cb)
		if _, _, e = admitRun(ctx, tx, c, "reconcile", ""); e != nil && !errors.Is(e, ErrNeedsAction) {
			return e
		}
		if e = keepRerun(ctx, tx, collectionID, &sub); e != nil {
			return e
		}
		return putSub(ctx, tx, sub)
	})
}

// RunFinished updates a subscription after any run on its collection ends and
// returns a follow-up run if changes were coalesced meanwhile.
func (s *Store) RunFinished(ctx context.Context, jobID string) (*Job, error) {
	var next *Job
	err := s.tx(ctx, func(tx *sql.Tx) error {
		j, e := readJob(ctx, tx, "", jobID)
		if e != nil {
			return e
		}
		var cb []byte
		if e = tx.QueryRowContext(ctx, "SELECT body FROM collections WHERE id=?", j.CollectionID).Scan(&cb); e != nil {
			return e
		}
		c, _ := decodeCollection(cb)
		// A delta scan that lost a large part of the source stops rather than
		// syncing silently; the read-only audit explains what happened. This
		// is independent of a subscription (RunNow can start a delta run), and
		// the finished job has already left the active set, so admission is
		// legal.
		if j.SourceRemoved > 0 {
			if _, e := admitWatchAudit(ctx, tx, c); e != nil && !errors.Is(e, ErrCapacity) && !errors.Is(e, ErrTransition) {
				return e
			}
		}
		sub, e := getSub(ctx, tx, j.CollectionID)
		if errors.Is(e, ErrNotFound) {
			// S8: the initial copy created this collection; a successful run
			// records the job's explicit mode or the principal's preference
			// in the same transaction, so a restart cannot lose it.
			if e := enqueueAfterCopySync(ctx, tx, j, time.Now().UTC()); e != nil {
				return e
			}
			_, e := tx.ExecContext(ctx, "DELETE FROM job_bookkeeping WHERE job_id=?", jobID)
			return e
		}
		if e != nil {
			return e
		}
		now := time.Now().UTC()
		sub.LastAttemptAt = &now
		ok := j.State == JobSucceeded
		if ok {
			sub.LastSuccessAt, sub.LastError = &now, nil
		} else if j.LastError != nil {
			sub.LastError = j.LastError
		}
		switch sub.State {
		case SyncWaitingBaseline:
			if j.Trigger == "initial" && ok {
				if sub.Mode == ModeScheduled {
					if sub.Schedule == nil {
						// Nothing to schedule; the job's bookkeeping is done.
						_, e := tx.ExecContext(ctx, "DELETE FROM job_bookkeeping WHERE job_id=?", jobID)
						return e
					}
					slot, e := sub.Schedule.Next(now)
					if e != nil {
						return e
					}
					sub.State, sub.NextDue, sub.NextKey = SyncActive, &slot.At, slot.Key
				} else {
					sub.State, sub.NextPollAt = SyncNeedsReconcile, 0 // cursor is captured by the syncer
				}
			}
		case SyncBootstrapping:
			if ok && sub.ReplayOverflow {
				// The replay buffer hit its cap: do not pretend the feed is
				// covered. Take a fresh cursor and run a full reconcile.
				sub.State, sub.NextPollAt, sub.ReplayOverflow = SyncNeedsReconcile, 0, false
				sub.LastError = &Failure{Code: "replay_overflow", Message: "Changes during the baseline exceeded the replay buffer; a full reconcile was queued."}
				if e := clearPendingChanges(ctx, tx, j.CollectionID); e != nil {
					return e
				}
			} else if ok {
				sub.State, sub.NextPollAt = SyncActive, 0
				if sub.FullReconcileEvery > 0 && sub.NextFullReconcileAt == 0 {
					sub.NextFullReconcileAt = nextFullReconcile(now, sub.FullReconcileEvery)
				}
			} else if j.State != JobCancelled {
				sub.State = SyncNeedsReconcile
			}
		case SyncStopping:
			sub.State = SyncStopped
		}
		if sub.RerunRequested && sub.State == SyncActive {
			sub.RerunRequested = false
			run, coalesced, e := admitRun(ctx, tx, c, "coalesced", "")
			if e == nil && !coalesced {
				next = &run
			}
			// A coalesced request was written to the active run's subscription
			// inside admitRun; keep it in this snapshot so putSub cannot clear it.
			sub.RerunRequested = e == nil && coalesced
		}
		sub.Revision++
		if e = putSub(ctx, tx, sub); e != nil {
			return e
		}
		_, e = tx.ExecContext(ctx, "DELETE FROM job_bookkeeping WHERE job_id=?", jobID)
		return e
	})
	return next, err
}

// FinishJob commits a terminal job state together with the collection state it
// implies and a durable bookkeeping marker, in one transaction. RunFinished
// consumes the marker after it records the subscription or after-copy request;
// FinishPendingBookkeeping replays any marker a crash left behind, so a restart
// cannot leave the collection stale or drop an after-copy sync request.
func (s *Store) FinishJob(ctx context.Context, id string, fn func(j *Job) (string, bool)) (Job, error) {
	var out Job
	err := s.tx(ctx, func(tx *sql.Tx) error {
		j, e := readJob(ctx, tx, "", id)
		if e != nil {
			return e
		}
		kind, bump := fn(&j)
		if bump {
			j.Revision++
		}
		j.UpdatedAt = time.Now().UTC()
		if e = saveJob(ctx, tx, j); e != nil {
			return e
		}
		if kind != "" {
			if e = appendEvent(ctx, tx, j, kind, nil); e != nil {
				return e
			}
		}
		if e = fillCounts(ctx, tx, &j); e != nil {
			return e
		}
		if j.Trigger == "initial" {
			// Same rule the engine used after its own terminal write: the
			// baseline copy defines the collection state.
			cs := CollectionReady
			if j.State != JobSucceeded {
				cs = j.State
			}
			var cb []byte
			if e = tx.QueryRowContext(ctx, "SELECT body FROM collections WHERE id=?", j.CollectionID).Scan(&cb); e != nil {
				return e
			}
			c, e := decodeCollection(cb)
			if e != nil {
				return e
			}
			c.State = cs
			c.Revision++
			nb, _ := encode(c)
			if _, e = tx.ExecContext(ctx, "UPDATE collections SET body=? WHERE id=?", nb, j.CollectionID); e != nil {
				return e
			}
		}
		_, e = tx.ExecContext(ctx, "INSERT INTO job_bookkeeping(job_id,created_at) VALUES(?,?) ON CONFLICT(job_id) DO NOTHING", j.ID, time.Now().UnixMilli())
		out = j
		return e
	})
	return out, err
}

// FinishPendingBookkeeping replays sync bookkeeping for terminal jobs whose
// commit did not reach RunFinished (a crash in between). It is idempotent: a
// job without a marker is skipped, and RunFinished removes the marker in its
// own transaction.
func (s *Store) FinishPendingBookkeeping(ctx context.Context) (int, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT job_id FROM job_bookkeeping ORDER BY created_at LIMIT 200")
	if err != nil {
		return 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return 0, err
	}
	done := 0
	for _, id := range ids {
		if _, e := s.RunFinished(ctx, id); e != nil {
			if errors.Is(e, ErrNotFound) {
				// The job disappeared (pruned) before the replay; do not keep
				// retrying a row that can never be applied.
				_, _ = s.db.ExecContext(ctx, "DELETE FROM job_bookkeeping WHERE job_id=?", id)
				continue
			}
			return done, e
		}
		done++
	}
	return done, nil
}

// ---- delta scanning ----

// DeltaStats summarises a page diff.
type DeltaStats struct{ New, Changed, Moved int }

// CommitDeltaPage diffs one page of a folder listing against the mapping and
// stores what must be done, plus the page checkpoint, in one transaction.
func (s *Store) CommitDeltaPage(ctx context.Context, run string, folder Item, children []Item, nextToken string) (DeltaStats, error) {
	var st DeltaStats
	err := s.tx(ctx, func(tx *sql.Tx) error {
		// The folder may have been renamed or moved earlier in this run, and
		// the copy of its row that the scanner holds can be older than that:
		// read its stored path inside this transaction.
		folderRel := folder.RelPath
		if e := tx.QueryRowContext(ctx, "SELECT rel_path FROM items WHERE collection_id=? AND source_id=?", folder.CollectionID, folder.SourceID).Scan(&folderRel); e != nil && !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		for _, c := range children {
			c.RelPath = joinRel(folderRel, c.Name)
			ex, e := scanItem(tx.QueryRowContext(ctx, "SELECT "+itemCols+" FROM items WHERE collection_id=? AND source_id=?", folder.CollectionID, c.SourceID))
			size := sql.NullInt64{}
			if c.Size != nil {
				size = sql.NullInt64{Int64: *c.Size, Valid: true}
			}
			if errors.Is(e, sql.ErrNoRows) {
				stState := ItemPending
				if c.State != "" {
					stState = c.State
				}
				ls := "done"
				if c.Kind == KindFolder && stState == ItemPending {
					ls = "none"
				}
				_, e = tx.ExecContext(ctx, `INSERT INTO items(collection_id,source_id,job_id,parent_source_id,kind,name,mime_type,resource_key,size,shortcut_target,shortcut_target_mime,fingerprint,state,error_code,list_state,seen_job,rel_path)
					VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, folder.CollectionID, c.SourceID, run, folder.SourceID, c.Kind, c.Name, c.MimeType, c.ResourceKey, size, c.ShortcutTarget, c.ShortcutTargetMim, c.Fingerprint, stState, c.ErrorCode, ls, run, c.RelPath)
				if e != nil {
					return e
				}
				st.New++
				continue
			}
			if e != nil {
				return e
			}
			switch {
			case c.ErrorCode == SkipFiltered:
				if e = applyFilterDecision(ctx, tx, run, folder, c, ex); e != nil {
					return e
				}
				continue
			case ex.State == ItemSkipped && ex.ErrorCode == SkipFiltered:
				if c.State == ItemSkipped { // excluded for another reason now: stays out
					if _, e = tx.ExecContext(ctx, "UPDATE items SET seen_job=? WHERE collection_id=? AND source_id=?", run, ex.CollectionID, ex.SourceID); e != nil {
						return e
					}
					continue
				}
				if e = applyFilterDecision(ctx, tx, run, folder, c, ex); e != nil {
					return e
				}
				st.New++
				continue
			}
			if c.RelPath != "" && ex.RelPath != c.RelPath {
				if e = moveRelPath(ctx, tx, ex.CollectionID, ex.SourceID, ex.RelPath, c.RelPath); e != nil {
					return e
				}
			}
			moved := ex.ParentSourceID != folder.SourceID
			if c.Kind == KindFolder {
				renamed := ex.Name != c.Name
				if (renamed || moved) && ex.State == ItemDone {
					if _, e = tx.ExecContext(ctx, "UPDATE items SET name=?, dest_name=CASE WHEN name<>? THEN '' ELSE dest_name END, conflict_action=CASE WHEN name<>? THEN '' ELSE conflict_action END, parent_source_id=?, state='pending', op='align', job_id=?, seen_job=?, error_code='' WHERE collection_id=? AND source_id=?", c.Name, c.Name, c.Name, folder.SourceID, run, run, ex.CollectionID, ex.SourceID); e != nil {
						return e
					}
					st.Moved++
					continue
				}
			} else if ex.State == ItemDone && ex.Name != c.Name && SameBytes(ex.Fingerprint, c.Fingerprint) {
				// A pure rename (same md5): rename the copy in place rather than
				// copying the same bytes again. The new name is decided afresh.
				if _, e = tx.ExecContext(ctx, `UPDATE items SET name=?, fingerprint=?, dest_name='', conflict_action='', parent_source_id=?,
					state='pending', op='align', job_id=?, seen_job=?, error_code='' WHERE collection_id=? AND source_id=?`,
					c.Name, c.Fingerprint, folder.SourceID, run, run, ex.CollectionID, ex.SourceID); e != nil {
					return e
				}
				st.Moved++
				continue
			} else if !SameSource(ex.Fingerprint, c.Fingerprint) && ex.State != ItemExecuting && ex.State != ItemReconciling {
				// New content (or a renamed Google file): a new generation is copied; the
				// previous copy is kept (versioned_copy) or retired after success.
				// A renamed source must be named again for the new generation; a
				// same-name recopy keeps the stored decision (restart stability).
				if _, e = tx.ExecContext(ctx, `UPDATE items SET name=?, mime_type=?, size=?, fingerprint=?, parent_source_id=?, generation=generation+1, intent_key='',
					dest_name=CASE WHEN name<>? THEN '' ELSE dest_name END,
					conflict_action=CASE WHEN name<>? THEN '' ELSE conflict_action END,
					state='pending', op='create',
					old_dest_id=CASE WHEN dest_id<>'' THEN dest_id ELSE old_dest_id END, job_id=?, seen_job=?, error_code='', attempts=0, next_attempt_at=0
					WHERE collection_id=? AND source_id=?`, c.Name, c.MimeType, size, c.Fingerprint, folder.SourceID, c.Name, c.Name, run, run, ex.CollectionID, ex.SourceID); e != nil {
					return e
				}
				st.Changed++
				continue
			} else if moved && ex.State == ItemDone {
				if _, e = tx.ExecContext(ctx, "UPDATE items SET parent_source_id=?, state='pending', op='align', job_id=?, seen_job=?, error_code='' WHERE collection_id=? AND source_id=?", folder.SourceID, run, run, ex.CollectionID, ex.SourceID); e != nil {
					return e
				}
				st.Moved++
				continue
			}
			// Unchanged; a fingerprint in the old format is rewritten here.
			if _, e = tx.ExecContext(ctx, "UPDATE items SET seen_job=?, fingerprint=CASE WHEN state IN ('executing','reconciling') THEN fingerprint ELSE ? END WHERE collection_id=? AND source_id=?", run, c.Fingerprint, ex.CollectionID, ex.SourceID); e != nil {
				return e
			}
		}
		ls := "partial"
		if nextToken == "" {
			ls = "done"
		}
		_, e := tx.ExecContext(ctx, "UPDATE items SET list_state=?, page_token=?, error_code='', seen_job=? WHERE collection_id=? AND source_id=?", ls, nextToken, run, folder.CollectionID, folder.SourceID)
		return e
	})
	return st, err
}

// joinRel is the path of an entry named name below a folder at rel.
func joinRel(rel, name string) string {
	if rel == "" {
		return name
	}
	return rel + "/" + name
}

// moveRelPath rewrites the stored path of an object and of everything below
// it after a rename or move, so path rules keep matching the current source.
// Descendants keep their suffix: "old/x" becomes "new/x".
func moveRelPath(ctx context.Context, tx *sql.Tx, collectionID, sourceID, oldRel, newRel string) error {
	if oldRel == "" {
		_, err := tx.ExecContext(ctx, "UPDATE items SET rel_path=? WHERE collection_id=? AND source_id=?", newRel, collectionID, sourceID)
		return err
	}
	_, err := tx.ExecContext(ctx, `WITH RECURSIVE sub(source_id) AS (
			SELECT ? UNION ALL SELECT i.source_id FROM items i JOIN sub ON i.collection_id=? AND i.parent_source_id=sub.source_id)
		UPDATE items SET rel_path = ? || substr(rel_path, ?) WHERE collection_id=? AND source_id IN (SELECT source_id FROM sub)`,
		sourceID, collectionID, newRel, utf8.RuneCountInString(oldRel)+1, collectionID)
	return err
}

// applyFilterDecision reconciles an existing object with the filter as it is
// now. A filter change never deletes or re-copies what was already copied:
//   - still filtered and already copied, or already excluded: left as it is;
//   - still filtered and not yet written (pending or failed): excluded now;
//   - in flight (executing, reconciling): left alone until it settles;
//   - was excluded and is now included: it becomes copyable (a folder is listed).
func applyFilterDecision(ctx context.Context, tx *sql.Tx, run string, folder Item, c, ex Item) error {
	if c.ErrorCode == SkipFiltered {
		switch ex.State {
		case ItemPending, ItemFailed, ItemBlocked:
			if c.Kind == KindFolder {
				break // a folder with children already listed is not pulled out from under them
			}
			_, e := tx.ExecContext(ctx, "UPDATE items SET state='skipped', error_code=?, seen_job=? WHERE collection_id=? AND source_id=?", SkipFiltered, run, ex.CollectionID, ex.SourceID)
			return e
		}
		_, e := tx.ExecContext(ctx, "UPDATE items SET seen_job=? WHERE collection_id=? AND source_id=?", run, ex.CollectionID, ex.SourceID)
		return e
	}
	// Included now, excluded before and never written: make it copyable.
	ls := "done"
	if c.Kind == KindFolder {
		ls = "none"
	}
	size := sql.NullInt64{}
	if c.Size != nil {
		size = sql.NullInt64{Int64: *c.Size, Valid: true}
	}
	_, e := tx.ExecContext(ctx, `UPDATE items SET name=?, mime_type=?, size=?, fingerprint=?, parent_source_id=?, rel_path=?, state='pending', error_code='', list_state=?, op='create', job_id=?, seen_job=?, attempts=0, next_attempt_at=0
		WHERE collection_id=? AND source_id=?`, c.Name, c.MimeType, size, c.Fingerprint, folder.SourceID, c.RelPath, ls, run, run, ex.CollectionID, ex.SourceID)
	return e
}

// ApplySingleFile compares the one file of a file-source collection.
func (s *Store) ApplySingleFile(ctx context.Context, run, collectionID string, c Item, found bool) (changed bool, err error) {
	err = s.tx(ctx, func(tx *sql.Tx) error {
		ex, e := scanItem(tx.QueryRowContext(ctx, "SELECT "+itemCols+" FROM items WHERE collection_id=? AND source_id=?", collectionID, c.SourceID))
		if e != nil {
			return e
		}
		if !found {
			_, e = tx.ExecContext(ctx, "UPDATE items SET error_code='source_removed' WHERE collection_id=? AND source_id=? AND state='done'", collectionID, c.SourceID)
			return e
		}
		if SameSource(ex.Fingerprint, c.Fingerprint) {
			_, e = tx.ExecContext(ctx, "UPDATE items SET error_code=CASE WHEN error_code='source_removed' THEN '' ELSE error_code END, fingerprint=CASE WHEN state IN ('executing','reconciling') THEN fingerprint ELSE ? END WHERE collection_id=? AND source_id=?", c.Fingerprint, collectionID, c.SourceID)
			return e
		}
		changed = true
		if ex.State == ItemDone && ex.Name != c.Name && SameBytes(ex.Fingerprint, c.Fingerprint) {
			// A pure rename: rename the copy in place (see CommitDeltaPage).
			_, e = tx.ExecContext(ctx, `UPDATE items SET name=?, fingerprint=?, dest_name='', conflict_action='', state='pending', op='align', job_id=?, error_code=''
				WHERE collection_id=? AND source_id=?`, c.Name, c.Fingerprint, run, collectionID, c.SourceID)
			return e
		}
		size := sql.NullInt64{}
		if c.Size != nil {
			size = sql.NullInt64{Int64: *c.Size, Valid: true}
		}
		_, e = tx.ExecContext(ctx, `UPDATE items SET name=?, mime_type=?, size=?, fingerprint=?, generation=generation+1, intent_key='',
			dest_name=CASE WHEN name<>? THEN '' ELSE dest_name END,
			conflict_action=CASE WHEN name<>? THEN '' ELSE conflict_action END,
			state='pending', op='create',
			old_dest_id=CASE WHEN dest_id<>'' THEN dest_id ELSE old_dest_id END, job_id=?, error_code='', attempts=0, next_attempt_at=0 WHERE collection_id=? AND source_id=?`,
			c.Name, c.MimeType, size, c.Fingerprint, c.Name, c.Name, run, collectionID, c.SourceID)
		return e
	})
	return changed, err
}

// FinishScan marks items that a complete scan no longer saw. Nothing is ever
// deleted from the destination; the item is only flagged.
func (s *Store) FinishScan(ctx context.Context, run, collectionID string) (removed int, err error) {
	err = s.tx(ctx, func(tx *sql.Tx) error {
		res, e := tx.ExecContext(ctx, "UPDATE items SET error_code='source_removed' WHERE collection_id=? AND parent_source_id<>'' AND state='done' AND seen_job<>? AND error_code=''", collectionID, run)
		if e != nil {
			return e
		}
		n, _ := res.RowsAffected()
		removed = int(n)
		_, e = tx.ExecContext(ctx, "UPDATE items SET error_code='' WHERE collection_id=? AND seen_job=? AND error_code='source_removed'", collectionID, run)
		return e
	})
	return removed, err
}

type Superseded struct {
	CollectionID, SourceID string
	Generation             int
	DestID                 string
}

// SupersededVersions lists old copies that replace_copy still has to retire.
func (s *Store) SupersededVersions(ctx context.Context, collectionID string) ([]Superseded, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT collection_id,source_id,generation,dest_id FROM item_versions WHERE collection_id=? AND state='superseded' ORDER BY rowid", collectionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Superseded
	for rows.Next() {
		var v Superseded
		if err = rows.Scan(&v.CollectionID, &v.SourceID, &v.Generation, &v.DestID); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) MarkTrashed(ctx context.Context, v Superseded) error {
	_, err := s.db.ExecContext(ctx, "UPDATE item_versions SET state='trashed' WHERE collection_id=? AND source_id=? AND generation=?", v.CollectionID, v.SourceID, v.Generation)
	return err
}

// NeedsReconcileSubs lists realtime subscriptions waiting for a fresh cursor
// and a full reconcile scan.
func (s *Store) NeedsReconcileSubs(ctx context.Context) ([]Subscription, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+subCols+" FROM sync_subscriptions WHERE mode='realtime' AND state='needs_reconcile' ORDER BY rowid LIMIT 50")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Subscription
	for rows.Next() {
		sub, e := scanSub(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, sub)
	}
	return out, rows.Err()
}
