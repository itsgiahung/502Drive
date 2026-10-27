package state

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

func intentKey(collection, source, destParent, kind string, generation int) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%d", collection, source, destParent, kind, generation)))
	return hex.EncodeToString(h[:16])
}

const itemCols = `collection_id,source_id,job_id,parent_source_id,kind,name,mime_type,resource_key,size,shortcut_target,shortcut_target_mime,fingerprint,generation,intent_key,state,dest_parent_id,dest_id,attempts,next_attempt_at,error_code,list_state,page_token,op,seen_job,scan_job,old_dest_id,conflict_action,dest_name,rel_path`

func scanItem(r interface{ Scan(...any) error }) (Item, error) {
	var it Item
	var size sql.NullInt64
	err := r.Scan(&it.CollectionID, &it.SourceID, &it.JobID, &it.ParentSourceID, &it.Kind, &it.Name, &it.MimeType, &it.ResourceKey, &size, &it.ShortcutTarget, &it.ShortcutTargetMim, &it.Fingerprint, &it.Generation, &it.IntentKey, &it.State, &it.DestParentID, &it.DestID, &it.Attempts, &it.NextAttemptAt, &it.ErrorCode, &it.ListState, &it.PageToken, &it.Op, &it.SeenJob, &it.ScanJob, &it.OldDestID, &it.ConflictAction, &it.DestName, &it.RelPath)
	if size.Valid {
		v := size.Int64
		it.Size = &v
	}
	return it, err
}

// ---- job lifecycle for the engine ----

// ClaimQueued moves the oldest queued job to running. Returns ErrNotFound when
// there is nothing to do.
func (s *Store) ClaimQueued(ctx context.Context) (Job, error) {
	var out Job
	err := s.tx(ctx, func(tx *sql.Tx) error {
		var id string
		e := tx.QueryRowContext(ctx, "SELECT id FROM jobs WHERE state='queued' ORDER BY rowid LIMIT 1").Scan(&id)
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

// ActiveJobs lists jobs a previous process left in a working state. Items that
// were mid-write become reconciling so no write is blindly repeated.
func (s *Store) ActiveJobs(ctx context.Context) ([]Job, error) {
	var out []Job
	err := s.tx(ctx, func(tx *sql.Tx) error {
		rows, e := tx.QueryContext(ctx, "SELECT id FROM jobs WHERE state IN ('running','pausing','cancelling','reconciling') ORDER BY rowid")
		if e != nil {
			return e
		}
		var ids []string
		for rows.Next() {
			var id string
			if e = rows.Scan(&id); e != nil {
				rows.Close()
				return e
			}
			ids = append(ids, id)
		}
		rows.Close()
		for _, id := range ids {
			if _, e = tx.ExecContext(ctx, "UPDATE items SET state='reconciling' WHERE job_id=? AND state='executing'", id); e != nil {
				return e
			}
			j, e := readJob(ctx, tx, "", id)
			if e != nil {
				return e
			}
			out = append(out, j)
		}
		return nil
	})
	return out, err
}

// UpdateJob applies fn to the job. Revision only moves when fn reports a
// user-visible state change, so progress does not invalidate control buttons.
func (s *Store) UpdateJob(ctx context.Context, id string, fn func(j *Job) (event string, bump bool)) (Job, error) {
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
		out = j
		return nil
	})
	return out, err
}

// ---- items ----

// NextToList returns a folder whose children still need listing.
func (s *Store) NextToList(ctx context.Context, jobID string) (Item, error) {
	it, err := scanItem(s.db.QueryRowContext(ctx, "SELECT "+itemCols+" FROM items WHERE kind='folder' AND list_state IN ('none','partial') AND (job_id=? OR scan_job=?) ORDER BY rowid LIMIT 1", jobID, jobID))
	if errors.Is(err, sql.ErrNoRows) {
		return it, ErrNotFound
	}
	return it, err
}

// NextToListPartial returns a folder whose listing has a page checkpoint. It
// must be continued on its own: a batch query's page token is not per folder.
func (s *Store) NextToListPartial(ctx context.Context, jobID string) (Item, error) {
	it, err := scanItem(s.db.QueryRowContext(ctx, "SELECT "+itemCols+" FROM items WHERE kind='folder' AND list_state='partial' AND (job_id=? OR scan_job=?) ORDER BY rowid LIMIT 1", jobID, jobID))
	if errors.Is(err, sql.ErrNoRows) {
		return it, ErrNotFound
	}
	return it, err
}

// NextToListBatch returns up to limit folders that have not been listed yet, in
// creation order. The caller combines their children into one files.list.
func (s *Store) NextToListBatch(ctx context.Context, jobID string, limit int) ([]Item, error) {
	if limit < 1 {
		limit = 1
	}
	if limit > 100 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, "SELECT "+itemCols+" FROM items WHERE kind='folder' AND list_state='none' AND (job_id=? OR scan_job=?) ORDER BY rowid LIMIT ?", jobID, jobID, limit)
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

// CommitPage stores a page of children and the folder's next-page checkpoint
// in one transaction, so replaying a page never duplicates items.
func (s *Store) CommitPage(ctx context.Context, folder Item, children []Item, nextToken string) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		for _, c := range children {
			size := sql.NullInt64{}
			if c.Size != nil {
				size = sql.NullInt64{Int64: *c.Size, Valid: true}
			}
			st := "pending"
			if c.State != "" {
				st = c.State
			}
			// A folder that is listed later must be pending; a folder the
			// filter removed is never listed, so its children are never read.
			ls := "done"
			if c.Kind == KindFolder && st == ItemPending {
				ls = "none"
			}
			_, e := tx.ExecContext(ctx, `INSERT OR IGNORE INTO items(collection_id,source_id,job_id,parent_source_id,kind,name,mime_type,resource_key,size,shortcut_target,shortcut_target_mime,fingerprint,state,error_code,list_state,rel_path)
				VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
				folder.CollectionID, c.SourceID, folder.JobID, folder.SourceID, c.Kind, c.Name, c.MimeType, c.ResourceKey, size, c.ShortcutTarget, c.ShortcutTargetMim, c.Fingerprint, st, c.ErrorCode, ls, c.RelPath)
			if e != nil {
				return e
			}
		}
		ls := "partial"
		if nextToken == "" {
			ls = "done"
		}
		_, e := tx.ExecContext(ctx, "UPDATE items SET list_state=?, page_token=?, error_code='' WHERE collection_id=? AND source_id=?", ls, nextToken, folder.CollectionID, folder.SourceID)
		return e
	})
}

// FailListing records a listing failure; the subtree stays "not scanned".
func (s *Store) FailListing(ctx context.Context, folder Item, code string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE items SET list_state='failed', error_code=? WHERE collection_id=? AND source_id=?", code, folder.CollectionID, folder.SourceID)
	return err
}

// SkipItem marks an item skipped with a stable error code.
func (s *Store) SkipItem(ctx context.Context, it Item, code string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE items SET state='skipped', error_code=? WHERE collection_id=? AND source_id=?", code, it.CollectionID, it.SourceID)
	return err
}

// ClaimItem picks the next pending item whose parent is already mapped, stamps
// its write intent and marks it executing — all in one commit before any
// remote call. now is epoch milliseconds.
func (s *Store) ClaimItem(ctx context.Context, jobID string, now int64) (Item, error) {
	var out Item
	err := s.tx(ctx, func(tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, `SELECT c.source_id, CASE WHEN c.parent_source_id='' THEN c.dest_parent_id ELSE p.dest_id END
			FROM items c LEFT JOIN items p ON p.collection_id=c.collection_id AND p.source_id=c.parent_source_id
			WHERE c.job_id=? AND c.state='pending' AND c.next_attempt_at<=?
			AND (c.parent_source_id='' OR p.state='done')
			ORDER BY c.rowid LIMIT 1`, jobID, now)
		var source, destParent string
		if e := row.Scan(&source, &destParent); errors.Is(e, sql.ErrNoRows) {
			return ErrNotFound
		} else if e != nil {
			return e
		}
		var col, kind string
		var gen int
		if e := tx.QueryRowContext(ctx, "SELECT collection_id,kind,generation FROM items WHERE job_id=? AND source_id=?", jobID, source).Scan(&col, &kind, &gen); e != nil {
			return e
		}
		key := intentKey(col, source, destParent, kind, gen)
		if _, e := tx.ExecContext(ctx, "UPDATE items SET state='executing', dest_parent_id=?, intent_key=?, attempts=attempts+1 WHERE collection_id=? AND source_id=?", destParent, key, col, source); e != nil {
			return e
		}
		it, e := scanItem(tx.QueryRowContext(ctx, "SELECT "+itemCols+" FROM items WHERE collection_id=? AND source_id=?", col, source))
		out = it
		return e
	})
	return out, err
}

// ItemsInState returns items of a job in the given state (recovery, retries).
func (s *Store) ItemsInState(ctx context.Context, jobID, state string) ([]Item, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+itemCols+" FROM items WHERE job_id=? AND state=? ORDER BY rowid", jobID, state)
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

// CompleteItem records the destination ID and, for the root, publishes the
// result URL on the job and collection in the same transaction.
func (s *Store) CompleteItem(ctx context.Context, it Item, destID string, rootURL func(id string) string) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		res, e := tx.ExecContext(ctx, "UPDATE items SET state='done', dest_id=?, error_code='' WHERE collection_id=? AND source_id=?", destID, it.CollectionID, it.SourceID)
		if e != nil {
			return e
		}
		done, _ := res.RowsAffected()
		isRoot := it.ParentSourceID == "" && (it.Kind == KindFolder || it.Kind == KindWrapper)
		// Q1: count bytes once, in the same transaction that marks the item
		// done. A repeated completion (crash/reconcile of an already done item)
		// does not inflate the ledger.
		if done > 0 && !isRoot && it.Kind == KindFile && it.Size != nil && *it.Size > 0 {
			if e = s.addCopiedBytes(ctx, tx, it.JobID, *it.Size); e != nil {
				return e
			}
		}
		if !isRoot {
			return nil
		}
		url := rootURL(destID)
		j, e := readJob(ctx, tx, "", it.JobID)
		if e != nil {
			return e
		}
		j.ResultURL = &url
		if e = saveJob(ctx, tx, j); e != nil {
			return e
		}
		var cb []byte
		if e = tx.QueryRowContext(ctx, "SELECT body FROM collections WHERE id=?", it.CollectionID).Scan(&cb); e != nil {
			return e
		}
		c, e := decodeCollection(cb)
		if e != nil {
			return e
		}
		c.RootID, c.ResultURL = &destID, &url
		nb, _ := encode(c)
		_, e = tx.ExecContext(ctx, "UPDATE collections SET body=? WHERE id=?", nb, it.CollectionID)
		return e
	})
}

// CompleteVersionedItem marks a copied file done, records its generation and
// the destination version in one transaction, and returns the destination of
// the copy this one supersedes (empty when there is none). One transaction
// halves the fsyncs per copied file, which keeps control reads responsive while
// several workers finish items.
func (s *Store) CompleteVersionedItem(ctx context.Context, it Item, destID string) (oldDest string, err error) {
	err = s.tx(ctx, func(tx *sql.Tx) error {
		res, e := tx.ExecContext(ctx, "UPDATE items SET state='done', dest_id=?, error_code='' WHERE collection_id=? AND source_id=?", destID, it.CollectionID, it.SourceID)
		if e != nil {
			return e
		}
		if done, _ := res.RowsAffected(); done > 0 && it.Kind == KindFile && it.Size != nil && *it.Size > 0 {
			if e = s.addCopiedBytes(ctx, tx, it.JobID, *it.Size); e != nil {
				return e
			}
		}
		if _, e = tx.ExecContext(ctx, "INSERT OR REPLACE INTO item_versions(collection_id,source_id,generation,dest_id,fingerprint,state) VALUES(?,?,?,?,?,'current')", it.CollectionID, it.SourceID, it.Generation, destID, it.Fingerprint); e != nil {
			return e
		}
		if it.OldDestID == "" {
			return nil
		}
		oldDest = it.OldDestID
		if _, e = tx.ExecContext(ctx, "UPDATE item_versions SET state='superseded' WHERE collection_id=? AND source_id=? AND generation<? AND state='current'", it.CollectionID, it.SourceID, it.Generation); e != nil {
			return e
		}
		_, e = tx.ExecContext(ctx, "UPDATE items SET old_dest_id='' WHERE collection_id=? AND source_id=?", it.CollectionID, it.SourceID)
		return e
	})
	return oldDest, err
}

// ReleaseItem puts an item back after an attempt that provably did not write.
// retryAt (epoch ms) of 0 means a terminal failure.
func (s *Store) ReleaseItem(ctx context.Context, it Item, code string, retryAt int64) error {
	state := ItemFailed
	if retryAt > 0 {
		state = ItemPending
	}
	_, err := s.db.ExecContext(ctx, "UPDATE items SET state=?, error_code=?, next_attempt_at=? WHERE collection_id=? AND source_id=?", state, code, retryAt, it.CollectionID, it.SourceID)
	return err
}

// ReleaseItemThrottled returns a throttled item to the pending queue without
// spending an attempt: a 429 says nothing about the item, only about the
// account's current request rate.
func (s *Store) ReleaseItemThrottled(ctx context.Context, it Item, code string, retryAt int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE items SET state='pending', error_code=?, next_attempt_at=?,
		attempts=CASE WHEN attempts>0 THEN attempts-1 ELSE 0 END
		WHERE collection_id=? AND source_id=?`, code, retryAt, it.CollectionID, it.SourceID)
	return err
}

// SetItemState moves an item to reconciling or blocked.
func (s *Store) SetItemState(ctx context.Context, it Item, state, code string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE items SET state=?, error_code=? WHERE collection_id=? AND source_id=?", state, code, it.CollectionID, it.SourceID)
	return err
}

// SkipOrphans marks pending items whose parent can never be mapped (failed,
// blocked, skipped) so the job can reach a truthful final state.
func (s *Store) SkipOrphans(ctx context.Context, jobID string) (int, error) {
	total := 0
	for {
		res, err := s.db.ExecContext(ctx, `UPDATE items SET state='skipped', error_code='parent_unavailable'
			WHERE job_id=? AND state='pending' AND parent_source_id<>'' AND parent_source_id IN
			(SELECT source_id FROM items p WHERE p.collection_id=items.collection_id AND p.state IN ('failed','blocked','skipped','cancelled'))`, jobID)
		if err != nil {
			return total, err
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return total, nil
		}
		total += int(n)
	}
}

// CancelPending marks all not-yet-started items cancelled. Output already
// created is left alone.
func (s *Store) CancelPending(ctx context.Context, jobID string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE items SET state='cancelled' WHERE job_id=? AND state IN ('pending')", jobID)
	return err
}

// Progress summarises what remains for a job.
type Progress struct {
	Pending, Active, Reconciling, Blocked, Failed, ListingLeft, ListingFailed int
	NextRetryAt                                                               int64
	RootDone                                                                  bool
}

func (s *Store) JobProgress(ctx context.Context, jobID string) (Progress, error) {
	var p Progress
	var next sql.NullInt64
	var root int
	err := s.db.QueryRowContext(ctx, `SELECT
		coalesce(sum(state='pending'),0),
		coalesce(sum(state='executing'),0),
		coalesce(sum(state='reconciling'),0),
		coalesce(sum(state='blocked'),0),
		coalesce(sum(state='failed'),0),
		coalesce(sum(kind='folder' AND list_state IN ('none','partial') AND scan_job=''),0),
		coalesce(sum(kind='folder' AND list_state='failed' AND scan_job=''),0),
		min(CASE WHEN state='pending' AND next_attempt_at>0 THEN next_attempt_at END),
		0
		FROM items WHERE job_id=?`, jobID).Scan(&p.Pending, &p.Active, &p.Reconciling, &p.Blocked, &p.Failed, &p.ListingLeft, &p.ListingFailed, &next, &root)
	if err != nil {
		return p, err
	}
	var scanLeft, scanFailed int
	if err = s.db.QueryRowContext(ctx, `SELECT
		coalesce(sum(list_state IN ('none','partial')),0), coalesce(sum(list_state='failed'),0)
		FROM items WHERE scan_job=? AND kind='folder'`, jobID).Scan(&scanLeft, &scanFailed); err != nil {
		return p, err
	}
	p.ListingLeft += scanLeft
	p.ListingFailed += scanFailed
	err = s.db.QueryRowContext(ctx, `SELECT coalesce(sum(i.parent_source_id='' AND i.state='done'),0)
		FROM items i JOIN jobs j ON j.collection_id=i.collection_id WHERE j.id=?`, jobID).Scan(&root)
	p.NextRetryAt = next.Int64
	p.RootDone = root > 0
	return p, err
}

// JobControl is a cheap read of the state and the user's desired state.
func (s *Store) JobControl(ctx context.Context, id string) (state, desired string, err error) {
	err = s.db.QueryRowContext(ctx, "SELECT state, coalesce(json_extract(body,'$.desired_state'),'run') FROM jobs WHERE id=?", id).Scan(&state, &desired)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	return
}

// SetCollectionState records the high-level state of a collection.
func (s *Store) SetCollectionState(ctx context.Context, id, state string) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		var b []byte
		if e := tx.QueryRowContext(ctx, "SELECT body FROM collections WHERE id=?", id).Scan(&b); e != nil {
			return e
		}
		c, e := decodeCollection(b)
		if e != nil {
			return e
		}
		c.State = state
		c.Revision++
		nb, _ := encode(c)
		_, e = tx.ExecContext(ctx, "UPDATE collections SET body=? WHERE id=?", nb, id)
		return e
	})
}

// StrandedJobs lists jobs asked to pause or cancel that no worker holds
// (for example a blocked job that was cancelled).
func (s *Store) StrandedJobs(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id FROM jobs WHERE state IN ('pausing','cancelling') ORDER BY rowid")
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
