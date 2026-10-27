package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const (
	JobKindSnapshot = "snapshot"
	JobKindRestore  = "restore"
)

// itemJSON freezes a mapping in SQL without loading a source tree into RAM.
const itemJSON = `json_object('CollectionID',collection_id,'SourceID',source_id,'JobID',job_id,'ParentSourceID',parent_source_id,'Kind',kind,'Name',name,'MimeType',mime_type,'ResourceKey',resource_key,'Size',size,'ShortcutTarget',shortcut_target,'ShortcutTargetMim',shortcut_target_mime,'Fingerprint',fingerprint,'Generation',generation,'IntentKey',intent_key,'State',state,'DestParentID',dest_parent_id,'DestID',dest_id,'Attempts',attempts,'NextAttemptAt',next_attempt_at,'ErrorCode',error_code,'ListState',list_state,'PageToken',page_token,'Op',op,'SeenJob',seen_job,'ScanJob',scan_job,'OldDestID',old_dest_id,'ConflictAction',conflict_action,'DestName',dest_name,'RelPath',rel_path)`

const versionJoin = ` JOIN (SELECT collection_id v_collection,source_id v_source,generation v_generation,dest_id v_dest FROM item_versions WHERE state<>'trashed') v ON v_collection=collection_id AND v_source=source_id `
const versionItemJSON = `json_set(` + itemJSON + `,'$.Generation',v_generation,'$.DestID',v_dest,'$.IntentKey',coalesce((SELECT json_extract(si.body,'$.IntentKey') FROM snapshot_items si WHERE si.dest_id=v_dest LIMIT 1),''))`

type ProtectionOptions struct {
	Mirror    bool `json:"mirror"`
	Snapshots bool `json:"snapshots"`
	KeepDays  int  `json:"keep_days"`
	KeepCount int  `json:"keep_count"`
}

func (o ProtectionOptions) Defaults() ProtectionOptions {
	if o.KeepDays == 0 {
		o.KeepDays = 30
	}
	if o.KeepCount == 0 {
		o.KeepCount = 10
	}
	return o
}

func (o ProtectionOptions) Validate() error {
	if o.KeepDays < 1 || o.KeepDays > 3650 || o.KeepCount < 1 || o.KeepCount > 1000 {
		return fmt.Errorf("keep_days must be 1–3650 and keep_count 1–1000")
	}
	return nil
}

func collectionQuery(ctx context.Context, q queryer, scope, id string) (Collection, error) {
	var b []byte
	err := q.QueryRowContext(ctx, "SELECT body FROM collections WHERE id=? AND (?='' OR principal=?)", id, scope, scope).Scan(&b)
	if errors.Is(err, sql.ErrNoRows) {
		return Collection{}, ErrNotFound
	}
	if err != nil {
		return Collection{}, err
	}
	return decodeCollection(b)
}

func (s *Store) SetProtection(ctx context.Context, actor, scope, key, id string, expected int, opts ProtectionOptions) (Collection, bool, error) {
	opts = opts.Defaults()
	if err := opts.Validate(); err != nil {
		return Collection{}, false, err
	}
	b, replay, err := s.idempotent(ctx, actor, "PUT /v1/collections/"+id+"/protection", key, []any{expected, opts}, func(tx *sql.Tx) (any, error) {
		c, err := collectionQuery(ctx, tx, scope, id)
		if err != nil {
			return nil, err
		}
		if c.Revision != expected {
			return nil, ErrStale
		}
		if c.State == CollectionRetired {
			return nil, ErrTransition
		}
		if active, err := activeRun(ctx, tx, id); err != nil {
			return nil, err
		} else if active != "" {
			return nil, ErrCollectionBusy{active}
		}
		c.Protection = opts
		c.Revision++
		b, _ := json.Marshal(c)
		_, err = tx.ExecContext(ctx, "UPDATE collections SET body=? WHERE id=?", b, id)
		return c, err
	})
	var c Collection
	if err == nil {
		err = json.Unmarshal(b, &c)
	}
	return c, replay, err
}

type Snapshot struct {
	ID           string    `json:"id"`
	CollectionID string    `json:"collection_id"`
	JobID        string    `json:"job_id"`
	State        string    `json:"state"`
	Pinned       bool      `json:"pinned"`
	Revision     int       `json:"revision"`
	CreatedAt    time.Time `json:"created_at"`
	ExpiresAt    time.Time `json:"expires_at"`
	Items        int       `json:"items"`
}

const snapshotCols = "s.id,s.collection_id,s.job_id,s.state,s.pinned,s.revision,s.created_at,s.expires_at,(SELECT count(*) FROM snapshot_items i WHERE i.snapshot_id=s.id)"

func scanSnapshot(r interface{ Scan(...any) error }) (Snapshot, error) {
	var s Snapshot
	var created, expires int64
	err := r.Scan(&s.ID, &s.CollectionID, &s.JobID, &s.State, &s.Pinned, &s.Revision, &created, &expires, &s.Items)
	s.CreatedAt = time.UnixMilli(created).UTC()
	s.ExpiresAt = time.UnixMilli(expires).UTC()
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	return s, err
}

func snapshotQuery(ctx context.Context, q queryer, scope, id string) (Snapshot, error) {
	return scanSnapshot(q.QueryRowContext(ctx, "SELECT "+snapshotCols+" FROM snapshots s JOIN collections c ON c.id=s.collection_id WHERE s.id=? AND (?='' OR c.principal=?)", id, scope, scope))
}

func (s *Store) Snapshot(ctx context.Context, scope, id string) (Snapshot, error) {
	return snapshotQuery(ctx, s.db, scope, id)
}

func (s *Store) Snapshots(ctx context.Context, scope, col string, limit int, cursor int64) ([]Snapshot, int64, error) {
	if _, err := s.GetCollection(ctx, scope, col); err != nil {
		return nil, 0, err
	}
	if limit < 1 || limit > 100 {
		return nil, 0, fmt.Errorf("limit must be 1–100")
	}
	if cursor == 0 {
		cursor = 1 << 62
	}
	rows, err := s.db.QueryContext(ctx, "SELECT s.rowid,"+snapshotCols+" FROM snapshots s WHERE s.collection_id=? AND s.rowid<? ORDER BY s.rowid DESC LIMIT ?", col, cursor, limit+1)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []Snapshot{}
	var last, next int64
	for rows.Next() {
		var v Snapshot
		var created, expires int64
		if err = rows.Scan(&last, &v.ID, &v.CollectionID, &v.JobID, &v.State, &v.Pinned, &v.Revision, &created, &expires, &v.Items); err != nil {
			return nil, 0, err
		}
		v.CreatedAt = time.UnixMilli(created).UTC()
		v.ExpiresAt = time.UnixMilli(expires).UTC()
		if len(out) == limit {
			next = cursor
			break
		}
		out = append(out, v)
		cursor = last
	}
	return out, next, rows.Err()
}

type SnapshotItem struct {
	SourceID string `json:"source_id"`
	DestID   string `json:"destination_id"`
	Name     string `json:"name"`
	ParentID string `json:"parent_id"`
	Path     string `json:"path"`
	Identity string `json:"identity"`
	Item     Item   `json:"-"`
}

func (s *Store) SnapshotItems(ctx context.Context, id string, limit int, after string) ([]SnapshotItem, error) {
	if limit < 1 || limit > 500 {
		return nil, fmt.Errorf("limit must be 1–500")
	}
	rows, err := s.db.QueryContext(ctx, "SELECT source_id,dest_id,body,identity FROM snapshot_items WHERE snapshot_id=? AND source_id>? ORDER BY source_id LIMIT ?", id, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SnapshotItem{}
	for rows.Next() {
		var v SnapshotItem
		var b []byte
		if err = rows.Scan(&v.SourceID, &v.DestID, &b, &v.Identity); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(b, &v.Item); err != nil {
			return nil, err
		}
		v.Name = v.Item.TargetName()
		v.ParentID = v.Item.DestParentID
		v.Path = v.Item.RelPath
		out = append(out, v)
	}
	return out, rows.Err()
}

// BeginSnapshot freezes the mapping before remote metadata is read. Retrying the
// same run reuses the snapshot and its capture checkpoints.
func (s *Store) BeginSnapshot(ctx context.Context, jobID string) (Snapshot, error) {
	var out Snapshot
	err := s.tx(ctx, func(tx *sql.Tx) error {
		if v, e := scanSnapshot(tx.QueryRowContext(ctx, "SELECT "+snapshotCols+" FROM snapshots s WHERE s.job_id=?", jobID)); e == nil {
			out = v
			return nil
		} else if !errors.Is(e, ErrNotFound) {
			return e
		}
		j, err := readJob(ctx, tx, "", jobID)
		if err != nil {
			return err
		}
		c, err := collectionQuery(ctx, tx, "", j.CollectionID)
		if err != nil {
			return err
		}
		if c.RootID == nil {
			return ErrNeedsAction
		}
		o := c.Protection.Defaults()
		now := time.Now().UTC()
		out = Snapshot{ID: ID("snap"), CollectionID: c.ID, JobID: j.ID, State: "capturing", Revision: 1, CreatedAt: now, ExpiresAt: now.AddDate(0, 0, o.KeepDays)}
		if _, err = tx.ExecContext(ctx, "INSERT INTO snapshots(id,collection_id,job_id,state,created_at,expires_at) VALUES(?,?,?,'capturing',?,?)", out.ID, c.ID, j.ID, now.UnixMilli(), out.ExpiresAt.UnixMilli()); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO snapshot_items(snapshot_id,source_id,dest_id,body)
          SELECT ?,source_id,dest_id,`+itemJSON+` FROM items WHERE collection_id=? AND state='done' AND dest_id<>''`, out.ID, c.ID)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		out.Items = int(n)
		if err != nil {
			return err
		}
		if j.Kind == JobKindSnapshot {
			_, err = tx.ExecContext(ctx, "UPDATE jobs SET body=json_set(body,'$.counts.total_items',?,'$.counts.observed_items',?) WHERE id=?", n, n, jobID)
			if err != nil {
				return err
			}
		}
		return nil
	})
	return out, err
}

func (s *Store) RecordSnapshotItem(ctx context.Context, id string, it Item, identity string) error {
	b, err := json.Marshal(it)
	if err != nil {
		return err
	}
	return s.tx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, "UPDATE snapshot_items SET identity=?,body=? WHERE snapshot_id=? AND source_id=? AND identity=''", identity, string(b), id, it.SourceID)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil || n == 0 {
			return err
		}
		_, err = tx.ExecContext(ctx, "UPDATE jobs SET body=json_set(body,'$.counts.completed_items',coalesce(json_extract(body,'$.counts.completed_items'),0)+1) WHERE id=(SELECT job_id FROM snapshots WHERE id=?) AND json_extract(body,'$.kind')='snapshot'", id)
		return err
	})
}

func (s *Store) SnapshotReferences(ctx context.Context, dest string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM snapshot_items i JOIN snapshots s ON s.id=i.snapshot_id WHERE i.dest_id=? AND (s.state='capturing' OR s.pinned=1 OR s.expires_at>?)", dest, time.Now().UnixMilli()).Scan(&n)
	return n > 0, err
}

func (s *Store) PinSnapshot(ctx context.Context, actor, scope, key, id string, expected int, pin bool) (Snapshot, bool, error) {
	b, replay, err := s.idempotent(ctx, actor, "POST /v1/snapshots/"+id+"/pin", key, []any{expected, pin}, func(tx *sql.Tx) (any, error) {
		v, err := snapshotQuery(ctx, tx, scope, id)
		if err != nil {
			return nil, err
		}
		if v.Revision != expected {
			return nil, ErrStale
		}
		if v.State != "ready" {
			return nil, ErrTransition
		}
		if active, err := activeRun(ctx, tx, v.CollectionID); err != nil {
			return nil, err
		} else if active != "" {
			return nil, ErrCollectionBusy{active}
		}
		v.Pinned = pin
		v.Revision++
		_, err = tx.ExecContext(ctx, "UPDATE snapshots SET pinned=?,revision=? WHERE id=?", pin, v.Revision, id)
		return v, err
	})
	var v Snapshot
	if err == nil {
		err = json.Unmarshal(b, &v)
	}
	return v, replay, err
}

// Pruning removes only metadata; it never deletes physical Drive copies.
func (s *Store) PruneSnapshots(ctx context.Context, col string, opts ProtectionOptions) error {
	o := opts.Defaults()
	_, err := s.db.ExecContext(ctx, `DELETE FROM snapshots WHERE collection_id=? AND pinned=0 AND state='ready' AND
 (expires_at<=? OR id IN (SELECT id FROM snapshots WHERE collection_id=? AND pinned=0 AND state='ready' ORDER BY created_at DESC,rowid DESC LIMIT -1 OFFSET ?))
 AND NOT EXISTS (SELECT 1 FROM jobs j WHERE j.collection_id=snapshots.collection_id AND json_extract(j.body,'$.snapshot_id')=snapshots.id AND j.state IN ('queued','running','pausing','paused','cancelling','reconciling','blocked'))`, col, time.Now().UnixMilli(), col, o.KeepCount)
	return err
}

type DestinationAction struct {
	Ordinal int          `json:"ordinal"`
	Action  string       `json:"action"`
	Target  SnapshotItem `json:"target"`
	State   string       `json:"state"`
}

func (s *Store) DestinationActionPage(ctx context.Context, jobID string, after, limit int) ([]DestinationAction, error) {
	if after < 0 || limit < 1 || limit > 500 {
		return nil, ErrTransition
	}
	rows, err := s.db.QueryContext(ctx, "SELECT ordinal,action,body,state FROM destination_actions WHERE job_id=? AND ordinal>=? ORDER BY ordinal LIMIT ?", jobID, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DestinationAction{}
	for rows.Next() {
		var a DestinationAction
		var b []byte
		var body struct {
			Item     Item   `json:"item"`
			Identity string `json:"identity"`
		}
		if err = rows.Scan(&a.Ordinal, &a.Action, &b, &a.State); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(b, &body); err != nil {
			return nil, err
		}
		it := body.Item
		a.Target = SnapshotItem{SourceID: it.SourceID, DestID: it.DestID, Name: it.TargetName(), ParentID: it.DestParentID, Path: it.RelPath, Identity: body.Identity, Item: it}
		out = append(out, a)
	}
	return out, rows.Err()
}

// FreezeRestoreActions records the whole immutable plan atomically, parents
// first on restore and children first on removal. No source tree is loaded in RAM.
func (s *Store) FreezeRestoreActions(ctx context.Context, job, col, snapshot string) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM destination_actions WHERE job_id=?", job).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return nil
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO destination_actions(job_id,ordinal,action,body)
   SELECT ?,row_number() OVER(ORDER BY priority,depth,source)-1,action,body FROM (
    SELECT 0 priority,length(json_extract(body,'$.RelPath'))-length(replace(json_extract(body,'$.RelPath'),'/','')) + CASE WHEN json_extract(body,'$.ParentSourceID')='' THEN -1 ELSE 0 END depth,
      source_id source,'restore' action,json_object('item',json(body),'identity',identity) body FROM snapshot_items WHERE snapshot_id=?
    UNION ALL
    SELECT 1 priority,-length(rel_path)+length(replace(rel_path,'/',''))-CASE WHEN parent_source_id='' THEN 0 ELSE 1 END depth,source_id source,'remove' action,json_object('item',`+itemJSON+`,'identity','') body
      FROM items WHERE collection_id=? AND state='done' AND dest_id<>'' AND dest_id NOT IN(SELECT dest_id FROM snapshot_items WHERE snapshot_id=?)
    UNION ALL
    SELECT 1 priority,-length(rel_path)+length(replace(rel_path,'/',''))-CASE WHEN parent_source_id='' THEN 0 ELSE 1 END depth,source_id source,'remove' action,json_object('item',`+versionItemJSON+`,'identity','') body
      FROM items `+versionJoin+` WHERE collection_id=? AND v_dest<>dest_id AND v_dest NOT IN(SELECT dest_id FROM snapshot_items WHERE snapshot_id=?)
   )`, job, snapshot, col, snapshot, col, snapshot)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "UPDATE jobs SET body=json_set(body,'$.counts.total_items',(SELECT count(*) FROM destination_actions WHERE job_id=?)) WHERE id=?", job, job)
		return err
	})
}

func (s *Store) FreezeMirrorActions(ctx context.Context, job, col string) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM destination_actions WHERE job_id=?", job).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return nil
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO destination_actions(job_id,ordinal,action,body)
   SELECT ?,row_number() OVER(ORDER BY depth DESC,source)-1,'remove',body FROM (
    SELECT length(rel_path)-length(replace(rel_path,'/',''))+CASE WHEN parent_source_id='' THEN 0 ELSE 1 END depth,source_id source,json_object('item',`+itemJSON+`,'identity','') body
     FROM items WHERE collection_id=? AND state='done' AND dest_id<>'' AND error_code='source_removed'
    UNION ALL
    SELECT length(rel_path)-length(replace(rel_path,'/',''))+CASE WHEN parent_source_id='' THEN 0 ELSE 1 END depth,source_id source,json_object('item',`+versionItemJSON+`,'identity','') body
     FROM items `+versionJoin+` WHERE collection_id=? AND v_dest<>dest_id AND error_code='source_removed'
   )`, job, col, col)
		return err
	})
}

func (s *Store) SetDestinationAction(ctx context.Context, jobID string, ordinal int, status string) error {
	if status != "executing" && status != "done" {
		return ErrTransition
	}
	_, err := s.db.ExecContext(ctx, "UPDATE destination_actions SET state=? WHERE job_id=? AND ordinal=?", status, jobID, ordinal)
	return err
}

func (s *Store) CompleteDestinationAction(ctx context.Context, job string, a DestinationAction, mirror bool) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, "UPDATE destination_actions SET state='done' WHERE job_id=? AND ordinal=? AND state<>'done'", job, a.Ordinal)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return nil
		}
		if mirror && a.Action == "remove" {
			it := a.Target.Item
			if _, err = tx.ExecContext(ctx, "UPDATE items SET state='skipped',error_code='mirror_removed' WHERE collection_id=? AND source_id=? AND dest_id=?", it.CollectionID, it.SourceID, it.DestID); err != nil {
				return err
			}
			if _, err = tx.ExecContext(ctx, "UPDATE item_versions SET state='superseded' WHERE collection_id=? AND source_id=? AND dest_id=?", it.CollectionID, it.SourceID, it.DestID); err != nil {
				return err
			}
		}
		_, err = tx.ExecContext(ctx, "UPDATE jobs SET body=json_set(body,'$.counts.completed_items',coalesce(json_extract(body,'$.counts.completed_items'),0)+1) WHERE id=? AND json_extract(body,'$.kind')='restore'", job)
		return err
	})
}

type CollectionArchive struct {
	Intent, ParentID, DestID string
	Attempted                bool
}

func (s *Store) Archive(ctx context.Context, col, parent string) (CollectionArchive, error) {
	_, err := s.db.ExecContext(ctx, "INSERT INTO collection_archives(collection_id,intent_key,parent_id) VALUES(?,?,?) ON CONFLICT(collection_id) DO NOTHING", col, ID("archive"), parent)
	if err != nil {
		return CollectionArchive{}, err
	}
	var a CollectionArchive
	err = s.db.QueryRowContext(ctx, "SELECT intent_key,parent_id,dest_id,attempted FROM collection_archives WHERE collection_id=?", col).Scan(&a.Intent, &a.ParentID, &a.DestID, &a.Attempted)
	return a, err
}
func (s *Store) RecordArchive(ctx context.Context, col, dest string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE collection_archives SET attempted=1,dest_id=? WHERE collection_id=?", dest, col)
	return err
}

func (s *Store) AdmitProtectionJob(ctx context.Context, actor, scope, key, col, snapshot string, expected int, dry bool) (Job, bool, error) {
	kind := JobKindSnapshot
	if snapshot != "" {
		kind = JobKindRestore
	}
	b, replay, err := s.idempotent(ctx, actor, "POST /v1/collections/"+col+"/"+kind, key, []any{snapshot, expected, dry}, func(tx *sql.Tx) (any, error) {
		c, err := collectionQuery(ctx, tx, scope, col)
		if err != nil {
			return nil, err
		}
		if snapshot != "" {
			v, err := snapshotQuery(ctx, tx, scope, snapshot)
			if err != nil {
				return nil, err
			}
			if v.CollectionID != col {
				return nil, ErrNotFound
			}
			if v.Revision != expected {
				return nil, ErrStale
			}
			if v.State != "ready" {
				return nil, ErrTransition
			}
		} else if c.Revision != expected {
			return nil, ErrStale
		}
		if c.State == CollectionRetired || c.RootID == nil {
			return nil, ErrTransition
		}
		if snapshot == "" && c.State != CollectionReady {
			return nil, ErrNeedsAction
		}
		if active, err := activeRun(ctx, tx, col); err != nil {
			return nil, err
		} else if active != "" {
			return nil, ErrCollectionBusy{active}
		}
		var owner string
		if err = tx.QueryRowContext(ctx, "SELECT principal FROM collections WHERE id=?", col).Scan(&owner); err != nil {
			return nil, err
		}
		var total, own int
		if err = tx.QueryRowContext(ctx, "SELECT count(*),coalesce(sum(principal=?),0) FROM jobs WHERE state IN ('queued','running','pausing','cancelling','reconciling')", owner).Scan(&total, &own); err != nil {
			return nil, err
		}
		if total >= maxActiveTotal || own >= maxActivePerUser {
			return nil, ErrCapacity
		}
		now := time.Now().UTC()
		j := Job{ID: ID("job"), Principal: owner, CollectionID: col, Kind: kind, Trigger: kind, SnapshotID: snapshot, DryRun: dry, SourceName: c.SourceName, DestinationID: c.DestinationID, RootName: c.RootName, State: JobQueued, DesiredState: "run", Revision: 1, CreatedAt: now, UpdatedAt: now}
		if snapshot != "" && !dry {
			if _, err = tx.ExecContext(ctx, "UPDATE sync_subscriptions SET state='paused',rerun_requested=0,revision=revision+1 WHERE collection_id=? AND state<>'stopped'", col); err != nil {
				return nil, err
			}
		}
		b, _ := json.Marshal(j)
		if _, err = tx.ExecContext(ctx, "INSERT INTO jobs(id,principal,collection_id,state,body) VALUES(?,?,?,?,?)", j.ID, owner, col, j.State, b); err != nil {
			return nil, err
		}
		return j, appendEvent(ctx, tx, j, "admitted", nil)
	})
	var j Job
	if err == nil {
		err = json.Unmarshal(b, &j)
	}
	return j, replay, err
}

func refuseRestoreWriter(ctx context.Context, q queryer, col string) error {
	id, err := activeRun(ctx, q, col)
	if err != nil || id == "" {
		return err
	}
	j, err := readJob(ctx, q, "", id)
	if err != nil {
		return err
	}
	if j.Kind == JobKindRestore {
		return ErrCollectionBusy{id}
	}
	return nil
}

func (s *Store) ApplyRestoredMapping(ctx context.Context, col, job, snapshot string) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "UPDATE items SET state='skipped',error_code='snapshot_extra' WHERE collection_id=? AND source_id NOT IN (SELECT source_id FROM snapshot_items WHERE snapshot_id=?)", col, snapshot); err != nil {
			return err
		}
		after := ""
		for {
			rows, err := tx.QueryContext(ctx, "SELECT body FROM snapshot_items WHERE snapshot_id=? AND source_id>? ORDER BY source_id LIMIT 200", snapshot, after)
			if err != nil {
				return err
			}
			var items []Item
			for rows.Next() {
				var b []byte
				if err = rows.Scan(&b); err != nil {
					rows.Close()
					return err
				}
				var it Item
				if err = json.Unmarshal(b, &it); err != nil {
					rows.Close()
					return err
				}
				items = append(items, it)
			}
			rows.Close()
			if err = rows.Err(); err != nil {
				return err
			}
			for _, it := range items {
				if _, err = tx.ExecContext(ctx, `UPDATE items SET job_id=?,parent_source_id=?,kind=?,name=?,mime_type=?,size=?,fingerprint=?,generation=max(generation,?),intent_key=?,state='done',dest_parent_id=?,dest_id=?,error_code='',dest_name=?,rel_path=?,op='create',old_dest_id='',list_state='done',page_token='' WHERE collection_id=? AND source_id=?`, job, it.ParentSourceID, it.Kind, it.Name, it.MimeType, it.Size, it.Fingerprint, it.Generation, it.IntentKey, it.DestParentID, it.DestID, it.DestName, it.RelPath, col, it.SourceID); err != nil {
					return err
				}
				if _, err = tx.ExecContext(ctx, "UPDATE item_versions SET state=CASE WHEN dest_id=? THEN 'current' ELSE 'superseded' END WHERE collection_id=? AND source_id=?", it.DestID, col, it.SourceID); err != nil {
					return err
				}
			}
			if len(items) < 200 {
				break
			}
			after = items[len(items)-1].SourceID
		}
		var root string
		if err := tx.QueryRowContext(ctx, "SELECT dest_id FROM snapshot_items WHERE snapshot_id=? AND json_extract(body,'$.ParentSourceID')=''", snapshot).Scan(&root); err != nil {
			return err
		}
		c, err := collectionQuery(ctx, tx, "", col)
		if err != nil {
			return err
		}
		link := "https://drive.google.com/drive/folders/" + root
		c.RootID, c.ResultURL, c.State = &root, &link, CollectionReady
		c.Revision++
		b, err := json.Marshal(c)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "UPDATE collections SET body=? WHERE id=?", b, col); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "UPDATE jobs SET body=json_set(body,'$.result_url',?) WHERE id=?", link, job); err != nil {
			return err
		}
		return nil
	})
}
