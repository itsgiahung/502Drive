package state

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Queue settings (S6). The pause flag is a durable setting, so a restarted
// daemon keeps the queue paused until someone resumes it.
const (
	queuePausedKey = "queue_paused"
	MinPriority    = -5
	MaxPriority    = 5
)

// claimQueuedSQL lists queued jobs best first:
//  1. higher priority;
//  2. fair share: fewer active jobs in the same group (a batch, or a lone
//     collection), so a 20-link batch cannot take every slot while a single
//     job waits;
//  3. FIFO by queue_order, then admission order.
//
// It uses jobs_queue (state, priority, order) and jobs_fair (state, group).
const claimQueuedSQL = `SELECT j.id FROM jobs j WHERE j.state='queued'
	ORDER BY j.queue_priority DESC,
		(SELECT count(*) FROM jobs r WHERE r.state IN ('running','reconciling','pausing','cancelling') AND r.fair_group=j.fair_group) ASC,
		j.queue_order ASC, j.rowid ASC
	LIMIT 1`

// ClaimQueued moves the best queued job to running. It returns ErrNotFound when
// there is nothing to start or the queue is paused.
func (s *Store) ClaimQueued(ctx context.Context) (Job, error) {
	var out Job
	err := s.tx(ctx, func(tx *sql.Tx) error {
		paused, e := queuePausedQuery(ctx, tx)
		if e != nil {
			return e
		}
		if paused {
			return ErrNotFound
		}
		var id string
		e = tx.QueryRowContext(ctx, claimQueuedSQL).Scan(&id)
		if errors.Is(e, sql.ErrNoRows) {
			return ErrNotFound
		}
		if e != nil {
			return e
		}
		j, e := readJob(ctx, tx, "", id)
		if e != nil {
			return e
		}
		phase := "discovering"
		j.State, j.Phase, j.Revision, j.UpdatedAt = JobRunning, &phase, j.Revision+1, time.Now().UTC()
		if e = saveJob(ctx, tx, j); e != nil {
			return e
		}
		if e = appendEvent(ctx, tx, j, "started", nil); e != nil {
			return e
		}
		out = j
		return nil
	})
	return out, err
}

func queuePausedQuery(ctx context.Context, q queryer) (bool, error) {
	var v string
	err := q.QueryRowContext(ctx, "SELECT value FROM settings WHERE key=?", queuePausedKey).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return v == "1", err
}

// QueuePaused reports the durable global queue pause.
func (s *Store) QueuePaused(ctx context.Context) (bool, error) {
	return queuePausedQuery(ctx, s.db)
}

// QueueChange reports what a queue pause or resume did.
type QueueChange struct {
	Paused bool
	// Changed is false when the queue was already in the requested state.
	Changed bool
	// Affected counts the jobs moved to pausing/paused or back to queued.
	Affected int
}

// SetQueuePaused pauses or resumes the whole queue in one transaction. Pausing
// moves running jobs to pausing (the engine settles them at a safe point with
// every item and cursor intact). Resuming returns only the jobs the queue pause
// itself stopped; jobs the user paused stay paused.
func (s *Store) SetQueuePaused(ctx context.Context, paused bool) (QueueChange, error) {
	out := QueueChange{Paused: paused}
	err := s.tx(ctx, func(tx *sql.Tx) error {
		cur, e := queuePausedQuery(ctx, tx)
		if e != nil {
			return e
		}
		if cur == paused {
			return nil
		}
		value := "0"
		if paused {
			value = "1"
		}
		if _, e = tx.ExecContext(ctx, "INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", queuePausedKey, value); e != nil {
			return e
		}
		out.Changed = true
		if paused {
			return pauseRunningForQueue(ctx, tx, &out)
		}
		return resumeQueuePaused(ctx, tx, &out)
	})
	return out, err
}

func pauseRunningForQueue(ctx context.Context, tx *sql.Tx, out *QueueChange) error {
	rows, err := tx.QueryContext(ctx, "SELECT id FROM jobs WHERE state IN ('running','reconciling','pausing') ORDER BY rowid")
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	for _, id := range ids {
		j, e := readJob(ctx, tx, "", id)
		if e != nil {
			return e
		}
		if j.State == JobPausing {
			continue // already on its way to paused, possibly by a user's choice
		}
		j.State, j.DesiredState, j.QueuePaused = JobPausing, "pause", true
		j.Revision++
		j.UpdatedAt = time.Now().UTC()
		if e = saveJob(ctx, tx, j); e != nil {
			return e
		}
		if e = appendEvent(ctx, tx, j, "pause", map[string]any{"reason": "queue"}); e != nil {
			return e
		}
		out.Affected++
	}
	return nil
}

func resumeQueuePaused(ctx context.Context, tx *sql.Tx, out *QueueChange) error {
	rows, err := tx.QueryContext(ctx, "SELECT id FROM jobs WHERE state='paused' AND json_extract(body,'$.queue_paused')=1 ORDER BY rowid")
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	for _, id := range ids {
		j, e := readJob(ctx, tx, "", id)
		if e != nil {
			return e
		}
		j.State, j.DesiredState, j.QueuePaused = JobQueued, "run", false
		j.Revision++
		j.UpdatedAt = time.Now().UTC()
		if e = saveJob(ctx, tx, j); e != nil {
			return e
		}
		if e = appendEvent(ctx, tx, j, "resumed", map[string]any{"reason": "queue"}); e != nil {
			return e
		}
		out.Affected++
	}
	return nil
}

// Move is a queue reordering of one queued job: place it before or after
// another queued job, and/or set its priority. Only one of Before/After is set.
type Move struct {
	Before   string
	After    string
	Priority *int
}

// MoveJob applies m to a queued job. revision must match the job's current
// revision. A running or finished job cannot be moved (ErrTransition).
func (s *Store) MoveJob(ctx context.Context, principal, id string, revision int, m Move) (Result, error) {
	err := s.tx(ctx, func(tx *sql.Tx) error {
		j, e := readJob(ctx, tx, principal, id)
		if e != nil {
			return e
		}
		if j.Revision != revision {
			return ErrStale
		}
		if j.State != JobQueued {
			return ErrTransition
		}
		if m.Before != "" && m.After != "" {
			return ErrInvalidMove
		}
		if m.Before == "" && m.After == "" && m.Priority == nil {
			return ErrInvalidMove
		}
		if m.Priority != nil {
			if *m.Priority < MinPriority || *m.Priority > MaxPriority {
				return ErrInvalidMove
			}
			j.Priority = *m.Priority
		}
		// The queue as it stands without this job, best first.
		rows, e := tx.QueryContext(ctx, "SELECT id, queue_priority FROM jobs WHERE state='queued' AND id<>? ORDER BY queue_priority DESC, queue_order ASC, rowid ASC", id)
		if e != nil {
			return e
		}
		var ids []string
		var prios []int
		for rows.Next() {
			var qid string
			var p int
			if e = rows.Scan(&qid, &p); e != nil {
				rows.Close()
				return e
			}
			ids = append(ids, qid)
			prios = append(prios, p)
		}
		rows.Close()
		if e = rows.Err(); e != nil {
			return e
		}
		at := 0
		if target := m.Before + m.After; target != "" {
			idx := -1
			for i, qid := range ids {
				if qid == target {
					idx = i
					break
				}
			}
			if idx < 0 {
				return ErrTransition // the target is not waiting in the queue
			}
			if m.Priority == nil {
				j.Priority = prios[idx]
			}
			at = idx
			if m.After != "" {
				at = idx + 1
			}
		} else {
			// Priority change only: the job joins the end of its new band.
			for _, p := range prios {
				if p >= j.Priority {
					at++
				} else {
					break
				}
			}
		}
		list := make([]string, 0, len(ids)+1)
		list = append(list, ids[:at]...)
		list = append(list, id)
		list = append(list, ids[at:]...)
		for i, qid := range list {
			if qid == id {
				continue
			}
			if _, e = tx.ExecContext(ctx, "UPDATE jobs SET body=json_set(body,'$.queue_order',?) WHERE id=?", i+1, qid); e != nil {
				return e
			}
		}
		j.QueueOrder = int64(at + 1)
		j.Revision++
		j.UpdatedAt = time.Now().UTC()
		if e = saveJob(ctx, tx, j); e != nil {
			return e
		}
		return appendEvent(ctx, tx, j, "moved", map[string]any{"position": at + 1, "priority": j.Priority})
	})
	if err != nil {
		return Result{}, err
	}
	j, err := readJob(ctx, s.db, principal, id)
	return Result{Job: j}, err
}

// BatchJobs returns the jobs of one batch, oldest first, scoped to principal.
func (s *Store) BatchJobs(ctx context.Context, principal, batchID string, limit int) ([]Job, error) {
	if limit < 1 || limit > 500 {
		return nil, errors.New("invalid limit")
	}
	rows, err := s.db.QueryContext(ctx, "SELECT id FROM jobs WHERE principal=? AND batch_id=? ORDER BY rowid LIMIT ?", principal, batchID, limit)
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
	if err = rows.Err(); err != nil {
		return nil, err
	}
	out := []Job{}
	for _, id := range ids {
		j, e := readJob(ctx, s.db, principal, id)
		if e != nil {
			return nil, e
		}
		out = append(out, j)
	}
	return out, nil
}
