package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// RetireCollection disables any active sync subscription, cancels pending automatic
// runs, and marks the collection as retired with a retired_at timestamp.
// If an active run is currently executing or queued, it returns ErrCollectionBusy.
func (s *Store) RetireCollection(ctx context.Context, principal, key, collectionID string, expectedRevision int) (Collection, error) {
	var out Collection
	body, _, err := s.idempotent(ctx, principal, "POST /v1/collections/retire", key, []any{collectionID, expectedRevision}, func(tx *sql.Tx) (any, error) {
		var cb []byte
		var owner string
		err := tx.QueryRowContext(ctx, "SELECT principal, body FROM collections WHERE id=?", collectionID).Scan(&owner, &cb)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		if err != nil {
			return nil, err
		}
		if principal != "" && principal != "owner" && owner != principal {
			return nil, ErrNotFound
		}
		c, err := decodeCollection(cb)
		if err != nil {
			return nil, err
		}
		if c.Revision != expectedRevision {
			return nil, ErrStale
		}
		if c.State == CollectionRetired {
			return c, nil
		}
		activeID, err := activeRun(ctx, tx, collectionID)
		if err != nil {
			return nil, err
		}
		if activeID != "" {
			return nil, ErrCollectionBusy{ActiveJobID: activeID}
		}

		// Stop any sync subscription and cancel queued/paused automatic jobs
		sub, err := getSub(ctx, tx, collectionID)
		if err == nil {
			sub.NextDue, sub.NextKey, sub.RerunRequested = nil, "", false
			sub.State = SyncStopped
			sub.Revision++
			_, _ = tx.ExecContext(ctx, `UPDATE jobs SET state='cancelled', body=json_set(body,'$.state','cancelled','$.desired_state','cancel','$.revision',json_extract(body,'$.revision')+1)
				WHERE collection_id=? AND state IN ('queued','paused')`, collectionID)
			if err = putSub(ctx, tx, sub); err != nil {
				return nil, err
			}
		}

		now := time.Now().UTC()
		c.State = CollectionRetired
		c.RetiredAt = &now
		c.Revision++
		nb, err := encode(c)
		if err != nil {
			return nil, err
		}
		if _, err = tx.ExecContext(ctx, "UPDATE collections SET body=?, retired_at=? WHERE id=?", nb, now.UnixMilli(), collectionID); err != nil {
			return nil, err
		}
		return c, nil
	})
	if err != nil {
		return out, err
	}
	err = json.Unmarshal(body, &out)
	return out, err
}

// ResumeCollection restores a retired collection back to ready state.
func (s *Store) ResumeCollection(ctx context.Context, principal, key, collectionID string, expectedRevision int) (Collection, error) {
	var out Collection
	body, _, err := s.idempotent(ctx, principal, "POST /v1/collections/resume", key, []any{collectionID, expectedRevision}, func(tx *sql.Tx) (any, error) {
		var cb []byte
		var owner string
		err := tx.QueryRowContext(ctx, "SELECT principal, body FROM collections WHERE id=?", collectionID).Scan(&owner, &cb)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		if err != nil {
			return nil, err
		}
		if principal != "" && principal != "owner" && owner != principal {
			return nil, ErrNotFound
		}
		c, err := decodeCollection(cb)
		if err != nil {
			return nil, err
		}
		if c.Revision != expectedRevision {
			return nil, ErrStale
		}
		if c.State != CollectionRetired {
			return nil, ErrTransition
		}

		c.State = CollectionReady
		c.RetiredAt = nil
		c.Revision++
		nb, err := encode(c)
		if err != nil {
			return nil, err
		}
		if _, err = tx.ExecContext(ctx, "UPDATE collections SET body=?, retired_at=NULL WHERE id=?", nb, collectionID); err != nil {
			return nil, err
		}
		return c, nil
	})
	if err != nil {
		return out, err
	}
	err = json.Unmarshal(body, &out)
	return out, err
}

// ForgetCollection removes a retired collection and all its associated database records.
// The collection must already be in retired state and have no active jobs.
func (s *Store) ForgetCollection(ctx context.Context, principal, key, collectionID string, expectedRevision int) error {
	_, _, err := s.idempotent(ctx, principal, "POST /v1/collections/forget", key, []any{collectionID, expectedRevision}, func(tx *sql.Tx) (any, error) {
		var cb []byte
		var owner string
		err := tx.QueryRowContext(ctx, "SELECT principal, body FROM collections WHERE id=?", collectionID).Scan(&owner, &cb)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		if err != nil {
			return nil, err
		}
		if principal != "" && principal != "owner" && owner != principal {
			return nil, ErrNotFound
		}
		c, err := decodeCollection(cb)
		if err != nil {
			return nil, err
		}
		if c.Revision != expectedRevision {
			return nil, ErrStale
		}
		if c.State != CollectionRetired {
			return nil, ErrCollectionNotRetired
		}
		activeID, err := activeRun(ctx, tx, collectionID)
		if err != nil {
			return nil, err
		}
		if activeID != "" {
			return nil, ErrCollectionBusy{ActiveJobID: activeID}
		}

		// Cascade delete in foreign key safe order
		queries := []string{
			"DELETE FROM snapshots WHERE collection_id=?",
			"DELETE FROM audit_findings WHERE audit_id IN (SELECT id FROM audits WHERE collection_id=?)",
			"DELETE FROM audits WHERE collection_id=?",
			"DELETE FROM sync_occurrences WHERE collection_id=?",
			"DELETE FROM sync_pending_changes WHERE collection_id=?",
			"DELETE FROM sync_subscriptions WHERE collection_id=?",
			"DELETE FROM item_versions WHERE collection_id=?",
			"DELETE FROM items WHERE collection_id=?",
			"DELETE FROM events WHERE job_id IN (SELECT id FROM jobs WHERE collection_id=?)",
			"DELETE FROM notification_outbox WHERE job_id IN (SELECT id FROM jobs WHERE collection_id=?)",
			"DELETE FROM jobs WHERE collection_id=?",
			"DELETE FROM notify_markers WHERE subject=?",
			"DELETE FROM collections WHERE id=?",
		}
		for _, q := range queries {
			if _, err = tx.ExecContext(ctx, q, collectionID); err != nil {
				return nil, fmt.Errorf("forget cascade %s: %w", q, err)
			}
		}

		// Verify foreign key integrity
		fk, err := tx.QueryContext(ctx, "PRAGMA foreign_key_check")
		if err != nil {
			return nil, err
		}
		defer fk.Close()
		if fk.Next() {
			return nil, errors.New("foreign key violation during forget")
		}

		return true, nil
	})
	return err
}

// AdmitCleanup admits a durable cleanup job on a retired collection.
func (s *Store) AdmitCleanup(ctx context.Context, principal, key, collectionID string, expectedRevision int, dryRun bool) (Job, error) {
	var j Job
	body, _, err := s.idempotent(ctx, principal, "POST /v1/collections/cleanup", key, []any{collectionID, expectedRevision, dryRun}, func(tx *sql.Tx) (any, error) {
		var cb []byte
		var owner string
		err := tx.QueryRowContext(ctx, "SELECT principal, body FROM collections WHERE id=?", collectionID).Scan(&owner, &cb)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		if err != nil {
			return nil, err
		}
		if principal != "" && principal != "owner" && owner != principal {
			return nil, ErrNotFound
		}
		c, err := decodeCollection(cb)
		if err != nil {
			return nil, err
		}
		if c.Revision != expectedRevision {
			return nil, ErrStale
		}
		if c.State != CollectionRetired {
			return nil, ErrCollectionNotRetired
		}
		activeID, err := activeRun(ctx, tx, collectionID)
		if err != nil {
			return nil, err
		}
		if activeID != "" {
			return nil, ErrCollectionBusy{ActiveJobID: activeID}
		}

		now := time.Now().UTC()
		job := Job{
			ID:            ID("job"),
			Principal:     owner,
			CollectionID:  collectionID,
			Kind:          JobKindCleanup,
			Trigger:       "cleanup",
			SourceName:    c.SourceName,
			DestinationID: c.DestinationID,
			RootName:      c.RootName,
			State:         JobQueued,
			DesiredState:  "run",
			DryRun:        dryRun,
			Revision:      1,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		jb, err := encode(job)
		if err != nil {
			return nil, err
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO jobs VALUES(?,?,?,?,?)", job.ID, owner, collectionID, job.State, jb); err != nil {
			return nil, err
		}
		eb, _ := encode(map[string]any{"state": job.State})
		if _, err = tx.ExecContext(ctx, "INSERT INTO events(job_id,body) VALUES(?,?)", job.ID, eb); err != nil {
			return nil, err
		}
		return job, nil
	})
	if err != nil {
		return j, err
	}
	err = json.Unmarshal(body, &j)
	return j, err
}
