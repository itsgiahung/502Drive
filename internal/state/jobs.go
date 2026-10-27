package state

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	maxActiveTotal   = 100
	maxActivePerUser = 20
)

type queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func hash(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// idempotent runs fn at most once per (principal, path, key). A repeat with the
// same request returns the stored response; a different request is a mismatch.
func (s *Store) idempotent(ctx context.Context, principal, path, key string, req any, fn func(tx *sql.Tx) (any, error)) (body []byte, replayed bool, err error) {
	if principal == "" {
		return nil, false, errors.New("principal required")
	}
	h := hash(req)
	err = s.tx(ctx, func(tx *sql.Tx) error {
		var previous string
		e := tx.QueryRowContext(ctx, "SELECT hash,body FROM idempotency WHERE principal=? AND path=? AND key=?", principal, path, key).Scan(&previous, &body)
		if e == nil {
			if previous != h {
				return ErrMismatch
			}
			replayed = true
			return nil
		}
		if !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		v, e := fn(tx)
		if e != nil {
			return e
		}
		if body, e = json.Marshal(v); e != nil {
			return e
		}
		_, e = tx.ExecContext(ctx, "INSERT INTO idempotency VALUES(?,?,?,?,?)", principal, path, key, h, body)
		return e
	})
	if err != nil {
		return nil, false, err
	}
	return body, replayed, nil
}

// PutPlan stores a plan. Call it through PutPlanIdempotent for API requests.
func (s *Store) PutPlan(ctx context.Context, p Plan) error {
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, "INSERT INTO plans(id,principal,body) VALUES(?,?,?)", p.ID, p.Principal, b)
	return err
}

// PutPlanIdempotent stores the plan produced by build once per key; a replay
// returns the original plan without refreshing its expiry.
func (s *Store) PutPlanIdempotent(ctx context.Context, principal, key string, req any, p Plan) (Plan, bool, error) {
	body, replayed, err := s.idempotent(ctx, principal, "POST /v1/plans", key, req, func(tx *sql.Tx) (any, error) {
		b, e := json.Marshal(p)
		if e != nil {
			return nil, e
		}
		_, e = tx.ExecContext(ctx, "INSERT INTO plans(id,principal,body) VALUES(?,?,?)", p.ID, p.Principal, b)
		return p, e
	})
	if err != nil {
		return Plan{}, false, err
	}
	var out Plan
	return out, replayed, json.Unmarshal(body, &out)
}

func (s *Store) GetPlan(ctx context.Context, principal, id string) (Plan, error) {
	var b []byte
	var p Plan
	err := s.db.QueryRowContext(ctx, "SELECT body FROM plans WHERE id=? AND principal=?", id, principal).Scan(&b)
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrNotFound
	}
	if err != nil {
		return p, err
	}
	return p, json.Unmarshal(b, &p)
}

func appendEvent(ctx context.Context, q queryer, j Job, kind string, data map[string]any) error {
	if data == nil {
		data = map[string]any{}
	}
	data["state"] = j.State
	e := Event{JobID: j.ID, Kind: kind, Revision: j.Revision, Timestamp: time.Now().UTC(), Data: data}
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	_, err = q.ExecContext(ctx, "INSERT INTO events(job_id,body) VALUES(?,?)", j.ID, b)
	return err
}

func saveJob(ctx context.Context, q queryer, j Job) error {
	j.Counts = Counts{} // counts are derived from items on read
	b, err := json.Marshal(j)
	if err != nil {
		return err
	}
	_, err = q.ExecContext(ctx, "UPDATE jobs SET state=?,body=? WHERE id=?", j.State, b, j.ID)
	return err
}

func readJob(ctx context.Context, q queryer, principal, id string) (Job, error) {
	var b []byte
	var j Job
	err := q.QueryRowContext(ctx, "SELECT body FROM jobs WHERE id=? AND (?='' OR principal=?)", id, principal, principal).Scan(&b)
	if errors.Is(err, sql.ErrNoRows) {
		return j, ErrNotFound
	}
	if err != nil {
		return j, err
	}
	if err = json.Unmarshal(b, &j); err != nil {
		return j, err
	}
	return j, fillCounts(ctx, q, &j)
}

func fillCounts(ctx context.Context, q queryer, j *Job) error {
	var observed, done, failed, blocked, skipped, listing int
	var bytes int64
	err := q.QueryRowContext(ctx, `SELECT
		count(*),
		coalesce(sum(state='done'),0),
		coalesce(sum(state='failed'),0),
		coalesce(sum(state='blocked'),0),
		coalesce(sum(state IN ('skipped','cancelled')),0),
		coalesce(sum(kind='folder' AND list_state IN ('none','partial') AND scan_job=''),0),
		coalesce(sum(CASE WHEN state='done' THEN size END),0)
		FROM items WHERE job_id=? AND kind<>'wrapper'`, j.ID).Scan(&observed, &done, &failed, &blocked, &skipped, &listing, &bytes)
	if err != nil {
		return err
	}
	j.Counts = Counts{Observed: observed, Completed: done, Failed: failed, Blocked: blocked, Skipped: skipped, Bytes: bytes}
	// The total is unknown until every folder has been fully listed.
	if observed > 0 && listing == 0 && (j.Phase == nil || *j.Phase != "discovering") {
		t := observed
		j.Counts.Total = &t
	}
	return nil
}

func rootSource(p Plan) string {
	if p.SourceKind == KindFile {
		return "wrapper:" + p.SourceID
	}
	return p.SourceID
}

func seedRoot(ctx context.Context, tx *sql.Tx, c Collection, j Job, p Plan) error {
	ins := `INSERT INTO items(collection_id,source_id,job_id,parent_source_id,kind,name,mime_type,resource_key,size,fingerprint,state,dest_parent_id,list_state)
		VALUES(?,?,?,?,?,?,?,?,?,?, 'pending', ?, ?)`
	switch p.SourceKind {
	case KindFolder:
		_, err := tx.ExecContext(ctx, ins, c.ID, p.SourceID, j.ID, "", KindFolder, p.RootName, p.SourceMime, p.ResourceKey, nil, p.SourceVersion, p.DestinationFolder, "none")
		return err
	case KindFile:
		w := "wrapper:" + p.SourceID
		if _, err := tx.ExecContext(ctx, ins, c.ID, w, j.ID, "", KindWrapper, p.RootName, "", "", nil, "", p.DestinationFolder, "done"); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, ins, c.ID, p.SourceID, j.ID, w, KindFile, p.SourceName, p.SourceMime, p.ResourceKey, p.SourceSize, p.SourceVersion, "", "done")
		return err
	}
	return nil
}

// Admit atomically consumes a plan into collection + baseline job + root
// intent + first event + idempotency record. Only after it returns may a
// caller report "accepted".
func (s *Store) Admit(ctx context.Context, principal, key, planID string, revision int) (Result, error) {
	return s.AdmitGuarded(ctx, principal, key, planID, revision, nil)
}

// Querier is the read/write handle given to admission guards.
type Querier = queryer

// AdmitGuarded is Admit with a check that runs inside the admission
// transaction, after replay detection and before the plan is consumed.
func (s *Store) AdmitGuarded(ctx context.Context, principal, key, planID string, revision int, guard func(ctx context.Context, q Querier, p Plan) error) (Result, error) {
	body, replayed, err := s.idempotent(ctx, principal, "POST /v1/jobs", key, []any{planID, revision}, func(tx *sql.Tx) (any, error) {
		var raw []byte
		var consumed sql.NullString
		e := tx.QueryRowContext(ctx, "SELECT body,consumed_job FROM plans WHERE id=? AND principal=?", planID, principal).Scan(&raw, &consumed)
		if errors.Is(e, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		if e != nil {
			return nil, e
		}
		var p Plan
		if e = json.Unmarshal(raw, &p); e != nil {
			return nil, e
		}
		if p.Revision != revision {
			return nil, ErrStale
		}
		if consumed.Valid {
			return readJob(ctx, tx, principal, consumed.String)
		}
		if !p.ExpiresAt.After(time.Now()) {
			return nil, ErrExpired
		}
		if guard != nil {
			if e = guard(ctx, tx, p); e != nil {
				return nil, e
			}
		}
		var total, own int
		e = tx.QueryRowContext(ctx, "SELECT count(*),coalesce(sum(principal=?),0) FROM jobs WHERE state IN ('queued','running','pausing','cancelling','reconciling')", principal).Scan(&total, &own)
		if e != nil {
			return nil, e
		}
		if total >= maxActiveTotal || own >= maxActivePerUser {
			return nil, ErrCapacity
		}
		now := time.Now().UTC()
		j := Job{ID: ID("job"), Principal: principal, CollectionID: ID("col"), Trigger: "initial", SourceName: p.SourceName, DestinationID: p.DestinationID, RootName: p.RootName, State: JobQueued, DesiredState: "run", Revision: 1, CreatedAt: now, UpdatedAt: now}
		c := Collection{ID: j.CollectionID, SourceID: p.SourceID, SourceName: p.SourceName, SourceKind: p.SourceKind, SourceMime: p.SourceMime, ResourceKey: p.ResourceKey, DestinationID: p.DestinationID, RootName: p.RootName, BaselineJobID: j.ID, State: "copying", Revision: 1}
		cb, _ := json.Marshal(c)
		if _, e = tx.ExecContext(ctx, "INSERT INTO collections(id, principal, plan_id, body) VALUES(?,?,?,?)", c.ID, principal, p.ID, cb); e != nil {
			return nil, e
		}
		jb, _ := json.Marshal(j)
		if _, e = tx.ExecContext(ctx, "INSERT INTO jobs VALUES(?,?,?,?,?)", j.ID, principal, c.ID, j.State, jb); e != nil {
			return nil, e
		}
		if e = seedRoot(ctx, tx, c, j, p); e != nil {
			return nil, e
		}
		if _, e = tx.ExecContext(ctx, "UPDATE plans SET consumed_job=? WHERE id=?", j.ID, p.ID); e != nil {
			return nil, e
		}
		if e = appendEvent(ctx, tx, j, "admitted", nil); e != nil {
			return nil, e
		}
		return j, nil
	})
	if err != nil {
		return Result{}, err
	}
	var j Job
	if err = json.Unmarshal(body, &j); err != nil {
		return Result{}, err
	}
	return Result{Job: j, Replayed: replayed}, nil
}

// Action applies a guarded control command (pause, resume, cancel, retry).
func (s *Store) Action(ctx context.Context, principal, key, id, action string, revision int) (Result, error) {
	return s.ActionAs(ctx, principal, principal, key, id, action, revision)
}

// ActionAs lets an authorised actor (the owner) control a job that belongs to
// another principal; idempotency is scoped to the actor.
func (s *Store) ActionAs(ctx context.Context, actor, owner, key, id, action string, revision int) (Result, error) {
	path := "POST /v1/jobs/" + id + "/" + action
	body, replayed, err := s.idempotent(ctx, actor, path, key, revision, func(tx *sql.Tx) (any, error) {
		j, e := readJob(ctx, tx, owner, id)
		if e != nil {
			return nil, e
		}
		if j.Revision != revision {
			return nil, ErrStale
		}
		kind := action
		switch action {
		case "pause":
			switch j.State {
			case JobQueued:
				j.State = JobPaused
			case JobRunning, JobReconciling:
				j.State = JobPausing
			default:
				return nil, ErrTransition
			}
			j.DesiredState = "pause"
		case "resume":
			if j.State != JobPaused && j.State != JobBlocked {
				return nil, ErrTransition
			}
			var blocked int
			if e = tx.QueryRowContext(ctx, "SELECT count(*) FROM items WHERE job_id=? AND state='blocked'", j.ID).Scan(&blocked); e != nil {
				return nil, e
			}
			if blocked > 0 { // unknown remote outcomes need an explicit retry decision
				return nil, ErrTransition
			}
			j.State, j.DesiredState = JobQueued, "run"
		case "cancel":
			switch j.State {
			case JobQueued, JobPaused:
				j.State = JobCancelled
			case JobRunning, JobPausing, JobReconciling, JobBlocked:
				j.State = JobCancelling
			default:
				return nil, ErrTransition
			}
			j.DesiredState = "cancel"
		case "retry":
			if j.State != JobPartial && j.State != JobFailed && j.State != JobBlocked {
				return nil, ErrTransition
			}
			if e = resetForRetry(ctx, tx, j); e != nil {
				return nil, e
			}
			j.State, j.DesiredState, j.LastError, j.Phase = JobQueued, "run", nil, nil
		default:
			return nil, ErrNotFound
		}
		j.Revision++
		j.UpdatedAt = time.Now().UTC()
		if e = saveJob(ctx, tx, j); e != nil {
			if strings.Contains(e.Error(), "UNIQUE") {
				return nil, ErrTransition
			}
			return nil, e
		}
		if e = appendEvent(ctx, tx, j, kind, nil); e != nil {
			return nil, e
		}
		return j, nil
	})
	if err != nil {
		return Result{}, err
	}
	var j Job
	if err = json.Unmarshal(body, &j); err != nil {
		return Result{}, err
	}
	return Result{Job: j, Replayed: replayed}, nil
}

// resetForRetry re-arms only the items that did not finish: failed ones are
// retried, blocked ones go back through reconciliation, failed listings resume
// from their saved page token. Done items are never touched.
func resetForRetry(ctx context.Context, tx *sql.Tx, j Job) error {
	for _, q := range []string{
		"UPDATE items SET state='pending', error_code='', next_attempt_at=0 WHERE job_id=? AND state IN ('failed','skipped','cancelled')",
		"UPDATE items SET state='reconciling', error_code=CASE WHEN error_code='unknown_remote_outcome' THEN 'retry_confirmed' ELSE '' END WHERE job_id=? AND state='blocked'",
		"UPDATE items SET list_state=CASE WHEN page_token='' THEN 'none' ELSE 'partial' END, error_code='' WHERE job_id=? AND list_state='failed'",
	} {
		if _, err := tx.ExecContext(ctx, q, j.ID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) GetJob(ctx context.Context, principal, id string) (Job, error) {
	return readJob(ctx, s.db, principal, id)
}

// ListJobs returns newest first. cursor is the opaque value from a previous
// call ("" for the first page). principal "" lists every principal's jobs
// and is for owner-scoped callers only.
func (s *Store) ListJobs(ctx context.Context, principal, state string, limit int, cursor int64) ([]Job, int64, error) {
	if limit < 1 || limit > 100 {
		return nil, 0, errors.New("invalid limit")
	}
	if cursor <= 0 {
		cursor = 1 << 62
	}
	rows, err := s.db.QueryContext(ctx, "SELECT rowid,id FROM jobs WHERE (?='' OR principal=?) AND (?='' OR state=?) AND rowid<? ORDER BY rowid DESC LIMIT ?", principal, principal, state, state, cursor, limit+1)
	if err != nil {
		return nil, 0, err
	}
	type ref struct {
		row int64
		id  string
	}
	var refs []ref
	for rows.Next() {
		var r ref
		if err = rows.Scan(&r.row, &r.id); err != nil {
			rows.Close()
			return nil, 0, err
		}
		refs = append(refs, r)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, 0, err
	}
	var next int64
	if len(refs) > limit {
		refs = refs[:limit]
		next = refs[limit-1].row
	}
	out := []Job{}
	for _, r := range refs {
		j, e := readJob(ctx, s.db, "", r.id)
		if e != nil {
			return nil, 0, e
		}
		out = append(out, j)
	}
	return out, next, nil
}

func (s *Store) Events(ctx context.Context, principal, id string, after int64, limit int) ([]Event, error) {
	if _, e := s.GetJob(ctx, principal, id); e != nil {
		return nil, e
	}
	if after < 0 || limit < 1 || limit > 100 {
		return nil, errors.New("invalid pagination")
	}
	rows, e := s.db.QueryContext(ctx, "SELECT sequence,body FROM events WHERE job_id=? AND sequence>? ORDER BY sequence LIMIT ?", id, after, limit)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var seq int64
		var b []byte
		var v Event
		if e = rows.Scan(&seq, &b); e != nil {
			return nil, e
		}
		if e = json.Unmarshal(b, &v); e != nil {
			return nil, e
		}
		v.Sequence = seq
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) Collections(ctx context.Context, principal string, all bool) ([]Collection, error) {
	allInt := 0
	if all {
		allInt = 1
	}
	rows, e := s.db.QueryContext(ctx, "SELECT body FROM collections WHERE (?='' OR principal=?) AND (?=1 OR retired_at IS NULL) ORDER BY rowid DESC LIMIT 100", principal, principal, allInt)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Collection{}
	for rows.Next() {
		var b []byte
		var c Collection
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		if e = json.Unmarshal(b, &c); e != nil {
			return nil, e
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) GetCollection(ctx context.Context, principal, id string) (Collection, error) {
	var b []byte
	var c Collection
	err := s.db.QueryRowContext(ctx, "SELECT body FROM collections WHERE id=? AND (?='' OR principal=?)", id, principal, principal).Scan(&b)
	if errors.Is(err, sql.ErrNoRows) {
		return c, ErrNotFound
	}
	if err != nil {
		return c, err
	}
	return c, json.Unmarshal(b, &c)
}

func ValidateKey(key string) error {
	if len(key) == 0 || len(key) > 128 {
		return fmt.Errorf("invalid idempotency key")
	}
	for _, r := range key {
		if r < 33 || r > 126 {
			return fmt.Errorf("invalid idempotency key")
		}
	}
	return nil
}

func IsConflict(err error) bool {
	var busy ErrCollectionBusy
	return errors.Is(err, ErrMismatch) || errors.Is(err, ErrStale) || errors.Is(err, ErrExpired) || errors.Is(err, ErrTransition) || errors.Is(err, ErrCollectionNotRetired) || errors.As(err, &busy)
}

func IsTerminal(s string) bool {
	return strings.Contains("|succeeded|partial|failed|cancelled|", "|"+s+"|")
}

func encode(v any) ([]byte, error) { return json.Marshal(v) }

func decodeCollection(b []byte) (Collection, error) {
	var c Collection
	return c, json.Unmarshal(b, &c)
}
