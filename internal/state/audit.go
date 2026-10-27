package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// Audit states.
const (
	AuditRunning   = "running"
	AuditCompleted = "completed"
	AuditFailed    = "failed"
)

// Job kinds. Copy jobs carry no kind at all, so pre-S1 job bodies decode
// unchanged.
const (
	JobKindAudit  = "audit"
	JobKindRepair = "repair"
)

// Stable finding kinds reported by an integrity audit.
const (
	FindingMissing    = "missing_at_destination"
	FindingExtra      = "extra_at_destination"
	FindingName       = "name_mismatch"
	FindingSize       = "size_or_md5_mismatch"
	FindingParent     = "wrong_parent"
	FindingDuplicate  = "duplicate_at_destination"
	FindingRemoved    = "source_removed"
	FindingUnreadable = "unreadable_source"
)

// Audit is one integrity scan of a collection.
type Audit struct {
	ID           string          `json:"id"`
	CollectionID string          `json:"collection_id"`
	JobID        string          `json:"job_id"`
	State        string          `json:"state"`
	Counts       map[string]int  `json:"counts"`
	Plan         json.RawMessage `json:"plan,omitempty"`
	Scanned      int             `json:"scanned_folders"`
	// Repairable counts the findings a repair run can fix. It is computed on
	// read so a running scan reports the count so far.
	Repairable int       `json:"repairable_findings"`
	Revision   int       `json:"revision"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
}

// Finding is one divergence between source and destination.
type Finding struct {
	AuditID    string `json:"audit_id"`
	Kind       string `json:"kind"`
	SourceID   string `json:"source_id,omitempty"`
	DestID     string `json:"dest_id,omitempty"`
	Repairable bool   `json:"repairable"`
	Repaired   bool   `json:"repaired"`
	Detail     string `json:"detail,omitempty"`
}

// RepairOp is one action a repair job takes on the mapping. action is "recopy"
// (write the object again through the normal intent path) or "align" (rename
// and/or move the object we already own). OldDestID records a superseded copy
// that the cleanup phase may trash once the new one is confirmed.
type RepairOp struct {
	SourceID  string
	Action    string
	OldDestID string
}

const auditCols = "id,collection_id,job_id,state,counts,plan,scanned,revision,started_at,finished_at"

func scanAudit(row interface{ Scan(...any) error }) (Audit, error) {
	var a Audit
	var counts, plan string
	var started, finished sql.NullInt64
	err := row.Scan(&a.ID, &a.CollectionID, &a.JobID, &a.State, &counts, &plan, &a.Scanned, &a.Revision, &started, &finished)
	if err != nil {
		return Audit{}, err
	}
	a.Counts = map[string]int{}
	_ = json.Unmarshal([]byte(counts), &a.Counts)
	if plan != "" {
		a.Plan = json.RawMessage(plan)
	}
	if started.Valid {
		a.StartedAt = time.UnixMilli(started.Int64).UTC()
	}
	if finished.Valid {
		a.FinishedAt = time.UnixMilli(finished.Int64).UTC()
	}
	return a, nil
}

func fillRepairable(ctx context.Context, q queryer, a *Audit) error {
	if a.ID == "" {
		return nil
	}
	return q.QueryRowContext(ctx, "SELECT count(*) FROM audit_findings WHERE audit_id=? AND repairable=1 AND repaired=0", a.ID).Scan(&a.Repairable)
}

func auditScope(ctx context.Context, q queryer, auditID, principal string) (Audit, error) {
	a, err := scanAudit(q.QueryRowContext(ctx,
		"SELECT "+auditCols+" FROM audits WHERE id=? AND (?='' OR collection_id IN (SELECT id FROM collections WHERE principal=?))",
		auditID, principal, principal))
	if errors.Is(err, sql.ErrNoRows) {
		return Audit{}, ErrNotFound
	}
	if err != nil {
		return Audit{}, err
	}
	return a, fillRepairable(ctx, q, &a)
}

// AdmitAudit creates the audit job and its audit row in one admission
// transaction, or replays the earlier audit for the same key. The collection's
// one-active-run rule applies: an audit cannot overlap a copy, sync, audit or
// repair job on the same collection.
func (s *Store) AdmitAudit(ctx context.Context, actor, scopePrincipal, key, collectionID string) (Audit, Job, bool, error) {
	var out struct {
		A Audit
		J Job
	}
	body, replayed, err := s.idempotent(ctx, actor, "POST /v1/collections/"+collectionID+"/audits", key, collectionID, func(tx *sql.Tx) (any, error) {
		var cb []byte
		e := tx.QueryRowContext(ctx, "SELECT body FROM collections WHERE id=? AND (?='' OR principal=?)", collectionID, scopePrincipal, scopePrincipal).Scan(&cb)
		if errors.Is(e, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		if e != nil {
			return nil, e
		}
		c, _ := decodeCollection(cb)
		if id, e := activeRun(ctx, tx, c.ID); e != nil {
			return nil, e
		} else if id != "" {
			return nil, ErrTransition
		}
		var principal string
		if e := tx.QueryRowContext(ctx, "SELECT principal FROM collections WHERE id=?", c.ID).Scan(&principal); e != nil {
			return nil, e
		}
		var total, own int
		if e := tx.QueryRowContext(ctx, "SELECT count(*),coalesce(sum(principal=?),0) FROM jobs WHERE state IN ('queued','running','pausing','cancelling','reconciling')", principal).Scan(&total, &own); e != nil {
			return nil, e
		}
		if total >= maxActiveTotal || own >= maxActivePerUser {
			return nil, ErrCapacity
		}
		now := time.Now().UTC()
		j := Job{ID: ID("job"), Principal: principal, CollectionID: c.ID, Kind: JobKindAudit, Trigger: "audit",
			SourceName: c.SourceName, DestinationID: c.DestinationID, RootName: c.RootName,
			State: JobQueued, DesiredState: "run", Revision: 1, CreatedAt: now, UpdatedAt: now}
		jb, _ := json.Marshal(j)
		if _, e := tx.ExecContext(ctx, "INSERT INTO jobs VALUES(?,?,?,?,?)", j.ID, principal, c.ID, j.State, jb); e != nil {
			return nil, e
		}
		a := Audit{ID: ID("aud"), CollectionID: c.ID, JobID: j.ID, State: AuditRunning, Counts: map[string]int{}, Revision: 1, StartedAt: now}
		if _, e := tx.ExecContext(ctx, "INSERT INTO audits(id,collection_id,job_id,state,counts,started_at) VALUES(?,?,?,?,?,?)",
			a.ID, a.CollectionID, a.JobID, a.State, "{}", now.UnixMilli()); e != nil {
			return nil, e
		}
		if e := appendEvent(ctx, tx, j, "admitted", map[string]any{"trigger": "audit"}); e != nil {
			return nil, e
		}
		out.A, out.J = a, j
		return j, nil
	})
	if err != nil {
		return Audit{}, Job{}, false, err
	}
	if replayed {
		j, err := readJob(ctx, s.db, "", bodyToID(body))
		if err != nil {
			return Audit{}, Job{}, true, err
		}
		a, err := s.auditByJob(ctx, j.ID)
		return a, j, true, err
	}
	return out.A, out.J, false, nil
}

func bodyToID(body []byte) string {
	var j Job
	_ = json.Unmarshal(body, &j)
	return j.ID
}

func (s *Store) auditByJob(ctx context.Context, jobID string) (Audit, error) {
	a, err := scanAudit(s.db.QueryRowContext(ctx, "SELECT "+auditCols+" FROM audits WHERE job_id=?", jobID))
	if errors.Is(err, sql.ErrNoRows) {
		return Audit{}, ErrNotFound
	}
	if err != nil {
		return Audit{}, err
	}
	return a, fillRepairable(ctx, s.db, &a)
}

// GetAudit reads one audit within the principal's scope.
func (s *Store) GetAudit(ctx context.Context, scopePrincipal, id string) (Audit, error) {
	return auditScope(ctx, s.db, id, scopePrincipal)
}

// AuditByJob finds the audit a job belongs to.
func (s *Store) AuditByJob(ctx context.Context, jobID string) (Audit, error) {
	return s.auditByJob(ctx, jobID)
}

// LastAudit returns the newest audit of a collection, or ErrNotFound.
func (s *Store) LastAudit(ctx context.Context, scopePrincipal, collectionID string) (Audit, error) {
	a, err := scanAudit(s.db.QueryRowContext(ctx,
		"SELECT "+auditCols+" FROM audits WHERE collection_id=? AND (?='' OR collection_id IN (SELECT id FROM collections WHERE principal=?)) ORDER BY started_at DESC, id LIMIT 1",
		collectionID, scopePrincipal, scopePrincipal))
	if errors.Is(err, sql.ErrNoRows) {
		return Audit{}, ErrNotFound
	}
	if err != nil {
		return Audit{}, err
	}
	return a, fillRepairable(ctx, s.db, &a)
}

// AuditScanProgress records the folders scanned and the per-kind finding counts.
func (s *Store) AuditScanProgress(ctx context.Context, auditID string, scanned int, counts map[string]int) error {
	b, _ := json.Marshal(counts)
	_, err := s.db.ExecContext(ctx, "UPDATE audits SET scanned=?, counts=? WHERE id=?", scanned, string(b), auditID)
	return err
}

// CompleteAudit finishes an audit: state, final counts, and (for a repair run)
// the plan. It bumps the audit revision so control races are caught. Only a
// running audit can complete.
func (s *Store) CompleteAudit(ctx context.Context, auditID, state string, counts map[string]int, plan json.RawMessage) (Audit, error) {
	cb, _ := json.Marshal(counts)
	planStr := ""
	if len(plan) > 0 {
		planStr = string(plan)
	}
	res, err := s.db.ExecContext(ctx,
		"UPDATE audits SET state=?, counts=?, plan=CASE WHEN ?='' THEN plan ELSE ? END, finished_at=?, revision=revision+1 WHERE id=? AND state=?",
		state, string(cb), planStr, planStr, time.Now().UTC().UnixMilli(), auditID, AuditRunning)
	if err != nil {
		return Audit{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Audit{}, ErrNotFound
	}
	a, err := scanAudit(s.db.QueryRowContext(ctx, "SELECT "+auditCols+" FROM audits WHERE id=?", auditID))
	if err != nil {
		return Audit{}, err
	}
	return a, fillRepairable(ctx, s.db, &a)
}

// RecordFindings upserts findings; the primary key keeps a re-scan (restart)
// idempotent instead of duplicating rows.
func (s *Store) RecordFindings(ctx context.Context, auditID string, findings []Finding) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		for _, f := range findings {
			rep := 0
			if f.Repairable {
				rep = 1
			}
			if _, e := tx.ExecContext(ctx, `INSERT INTO audit_findings(audit_id,kind,source_id,dest_id,repairable,repaired,detail) VALUES(?,?,?,?,?,?,?)
				ON CONFLICT(audit_id,kind,source_id,dest_id) DO UPDATE SET repairable=excluded.repairable, detail=excluded.detail`,
				auditID, f.Kind, f.SourceID, f.DestID, rep, 0, f.Detail); e != nil {
				return e
			}
		}
		return nil
	})
}

// MarkFindingsRepaired flags the given findings as fixed.
func (s *Store) MarkFindingsRepaired(ctx context.Context, auditID string, findings []Finding) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		for _, f := range findings {
			if _, e := tx.ExecContext(ctx, "UPDATE audit_findings SET repaired=1 WHERE audit_id=? AND kind=? AND source_id=? AND dest_id=?",
				auditID, f.Kind, f.SourceID, f.DestID); e != nil {
				return e
			}
		}
		return nil
	})
}

// FindingsByAudit returns the findings of an audit, optionally only the
// repairable ones, in stable order.
func (s *Store) FindingsByAudit(ctx context.Context, auditID string, repairableOnly bool) ([]Finding, error) {
	q := "SELECT audit_id,kind,source_id,dest_id,repairable,repaired,detail FROM audit_findings WHERE audit_id=?"
	if repairableOnly {
		q += " AND repairable=1"
	}
	q += " ORDER BY kind,source_id,dest_id"
	rows, err := s.db.QueryContext(ctx, q, auditID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Finding
	for rows.Next() {
		var f Finding
		var rep, fixed int
		if e := rows.Scan(&f.AuditID, &f.Kind, &f.SourceID, &f.DestID, &rep, &fixed, &f.Detail); e != nil {
			return nil, e
		}
		f.Repairable, f.Repaired = rep == 1, fixed == 1
		out = append(out, f)
	}
	return out, rows.Err()
}

// ListFindings pages the findings of an audit, newest scan order being stable
// (kind, source_id, dest_id). The cursor is a row offset.
func (s *Store) ListFindings(ctx context.Context, auditID, kind string, limit int, cursor int64) ([]Finding, int64, error) {
	if limit < 1 || limit > 500 {
		return nil, 0, errors.New("limit must be between 1 and 500")
	}
	if cursor < 0 {
		return nil, 0, errors.New("bad cursor")
	}
	q := "SELECT audit_id,kind,source_id,dest_id,repairable,repaired,detail FROM audit_findings WHERE audit_id=?"
	args := []any{auditID}
	if kind != "" {
		q += " AND kind=?"
		args = append(args, kind)
	}
	q += " ORDER BY kind,source_id,dest_id LIMIT ? OFFSET ?"
	args = append(args, limit+1, cursor)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Finding
	for rows.Next() {
		var f Finding
		var rep, fixed int
		if e := rows.Scan(&f.AuditID, &f.Kind, &f.SourceID, &f.DestID, &rep, &fixed, &f.Detail); e != nil {
			return nil, 0, e
		}
		f.Repairable, f.Repaired = rep == 1, fixed == 1
		out = append(out, f)
	}
	if err = rows.Err(); err != nil {
		return nil, 0, err
	}
	next := int64(0)
	if len(out) > limit {
		out = out[:limit]
		next = cursor + int64(limit)
	}
	return out, next, nil
}

// SaveRepairPlan stores the latest repair plan and bumps the audit revision,
// so a second repair request needs the new revision.
func (s *Store) SaveRepairPlan(ctx context.Context, auditID string, plan json.RawMessage) error {
	_, err := s.db.ExecContext(ctx, "UPDATE audits SET plan=?, revision=revision+1 WHERE id=?", string(plan), auditID)
	return err
}

// AdmitRepair creates the repair job for a completed audit. dryRun plans
// without writing anything. The audit revision protects against repairing a
// report that changed meanwhile.
func (s *Store) AdmitRepair(ctx context.Context, actor, scopePrincipal, key, auditID string, expectedRevision int, dryRun bool) (Job, bool, error) {
	body, replayed, err := s.idempotent(ctx, actor, "POST /v1/audits/"+auditID+"/repair", key, []any{auditID, expectedRevision, dryRun}, func(tx *sql.Tx) (any, error) {
		a, e := auditScope(ctx, tx, auditID, scopePrincipal)
		if e != nil {
			return nil, e
		}
		if a.State != AuditCompleted {
			return nil, ErrTransition
		}
		if a.Revision != expectedRevision {
			return nil, ErrStale
		}
		if id, e := activeRun(ctx, tx, a.CollectionID); e != nil {
			return nil, e
		} else if id != "" {
			return nil, ErrTransition
		}
		var principal string
		if e := tx.QueryRowContext(ctx, "SELECT principal FROM collections WHERE id=?", a.CollectionID).Scan(&principal); e != nil {
			return nil, e
		}
		var total, own int
		if e := tx.QueryRowContext(ctx, "SELECT count(*),coalesce(sum(principal=?),0) FROM jobs WHERE state IN ('queued','running','pausing','cancelling','reconciling')", principal).Scan(&total, &own); e != nil {
			return nil, e
		}
		if total >= maxActiveTotal || own >= maxActivePerUser {
			return nil, ErrCapacity
		}
		var c Collection
		var cb []byte
		if e := tx.QueryRowContext(ctx, "SELECT body FROM collections WHERE id=?", a.CollectionID).Scan(&cb); e != nil {
			return nil, e
		}
		c, _ = decodeCollection(cb)
		now := time.Now().UTC()
		j := Job{ID: ID("job"), Principal: principal, CollectionID: a.CollectionID, Kind: JobKindRepair, AuditID: a.ID, DryRun: dryRun,
			Trigger: "repair", SourceName: c.SourceName, DestinationID: c.DestinationID, RootName: c.RootName,
			State: JobQueued, DesiredState: "run", Revision: 1, CreatedAt: now, UpdatedAt: now}
		jb, _ := json.Marshal(j)
		if _, e := tx.ExecContext(ctx, "INSERT INTO jobs VALUES(?,?,?,?,?)", j.ID, principal, a.CollectionID, j.State, jb); e != nil {
			return nil, e
		}
		if e := appendEvent(ctx, tx, j, "admitted", map[string]any{"trigger": "repair", "dry_run": dryRun}); e != nil {
			return nil, e
		}
		return j, nil
	})
	if err != nil {
		return Job{}, false, err
	}
	var j Job
	return j, replayed, json.Unmarshal(body, &j)
}

// PrepareRepair re-arms mapping items for a repair job: recopy clears the
// destination (keeping the old id for the cleanup) and align re-runs the
// rename/move. All inside one transaction, and only for items whose last write
// has a settled outcome: an executing, reconciling or blocked item is left for
// reconciliation, because its intent is the only way to find that copy again.
func (s *Store) PrepareRepair(ctx context.Context, jobID, collectionID string, ops []RepairOp) error {
	const settled = "state NOT IN ('executing','reconciling','blocked')"
	return s.tx(ctx, func(tx *sql.Tx) error {
		for _, op := range ops {
			switch op.Action {
			case "recopy":
				// A new generation: the previous copy stays in item_versions
				// as superseded so the cleanup can trash it (and retry after
				// a crash) without losing track of it.
				if _, e := tx.ExecContext(ctx,
					"UPDATE items SET old_dest_id=CASE WHEN dest_id='' THEN old_dest_id ELSE dest_id END, generation=generation+1, intent_key='', state='pending', dest_id='', error_code='', attempts=0, next_attempt_at=0, job_id=? WHERE collection_id=? AND source_id=? AND "+settled,
					jobID, collectionID, op.SourceID); e != nil {
					return e
				}
			case "align":
				if _, e := tx.ExecContext(ctx,
					"UPDATE items SET state='pending', op='align', error_code='', attempts=0, next_attempt_at=0, job_id=? WHERE collection_id=? AND source_id=? AND "+settled,
					jobID, collectionID, op.SourceID); e != nil {
					return e
				}
			default:
				return errors.New("unknown repair action " + op.Action)
			}
		}
		return nil
	})
}

// AbandonedRepairItems returns mapping items stuck on a repair job that no
// longer runs (cancelled); the next repair adopts them.
func (s *Store) AbandonedRepairItems(ctx context.Context, collectionID string) ([]Item, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+itemCols+" FROM items WHERE collection_id=? AND state IN ('pending','cancelled') AND old_dest_id<>''", collectionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Item
	for rows.Next() {
		it, e := scanItem(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, it)
	}
	return out, rows.Err()
}
