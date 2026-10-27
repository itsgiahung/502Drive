package state

import (
	"context"
	"database/sql"
	"time"
)

// PruneOptions controls retention cleanup.
type PruneOptions struct {
	OlderThan time.Duration
	DryRun    bool
}

// PruneReport summarizes the outcome of a retention pass.
type PruneReport struct {
	OlderThan    string    `json:"older_than"`
	Cutoff       time.Time `json:"cutoff"`
	DryRun       bool      `json:"dry_run"`
	JobsPruned   int       `json:"jobs_pruned"`
	EventsPruned int       `json:"events_pruned"`
	// IntentsPruned stays 0: an item row is the live mapping and its intent_key
	// is the proof of ownership that audit and repair rely on, so retention
	// never clears it. The field remains in the report for compatibility.
	IntentsPruned        int `json:"intents_pruned"`
	TelegramInboxPruned  int `json:"telegram_inbox_pruned"`
	TelegramOutboxPruned int `json:"telegram_outbox_pruned"`
	PlansPruned          int `json:"plans_pruned"`
}

// Prune cleans up completed jobs, associated events, processed/failed telegram inbox,
// dead/sent outbox entries, and expired unconsumed plans older than the retention
// threshold. It preserves collections, active mappings (including their intent
// keys, the ownership proof used by audit and repair), and any job still
// referenced by items, plans, or sync occurrences.
func (s *Store) Prune(ctx context.Context, opt PruneOptions) (PruneReport, error) {
	if opt.OlderThan <= 0 {
		return PruneReport{
			OlderThan: "0s",
			Cutoff:    time.Now().UTC(),
			DryRun:    opt.DryRun,
		}, nil
	}

	cutoff := time.Now().Add(-opt.OlderThan).UTC()
	cutoffStr := cutoff.Format(time.RFC3339Nano)
	cutoffMs := cutoff.UnixMilli()

	rep := PruneReport{
		OlderThan: opt.OlderThan.String(),
		Cutoff:    cutoff,
		DryRun:    opt.DryRun,
	}

	// 1. Identify unreferenced completed jobs older than cutoff.
	candidateJobsQuery := `
		SELECT id FROM jobs
		WHERE state IN ('succeeded', 'failed', 'cancelled')
		  AND json_extract(body, '$.updated_at') < ?
		  AND id NOT IN (SELECT DISTINCT job_id FROM items)
		  AND id NOT IN (SELECT DISTINCT scan_job FROM items WHERE scan_job <> '')
		  AND id NOT IN (SELECT DISTINCT seen_job FROM items WHERE seen_job <> '')
		  AND id NOT IN (SELECT DISTINCT consumed_job FROM plans WHERE consumed_job IS NOT NULL AND consumed_job <> '')
		  AND id NOT IN (SELECT DISTINCT job_id FROM sync_occurrences WHERE job_id <> '')
		  AND id NOT IN (SELECT DISTINCT job_id FROM audits)
		  AND id NOT IN (SELECT DISTINCT job_id FROM notification_outbox WHERE state = 'pending' AND job_id <> '')
	`

	rows, err := s.db.QueryContext(ctx, candidateJobsQuery, cutoffStr)
	if err != nil {
		return rep, err
	}
	defer rows.Close()

	var jobIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return rep, err
		}
		jobIDs = append(jobIDs, id)
	}
	if err := rows.Err(); err != nil {
		return rep, err
	}
	rep.JobsPruned = len(jobIDs)

	// 2. Count events for candidate jobs.
	for _, jid := range jobIDs {
		var count int
		if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM events WHERE job_id = ?", jid).Scan(&count); err == nil {
			rep.EventsPruned += count
		}
	}

	// 3. Write intents are never pruned: item rows are live mappings and their
	// intent_key proves ownership to audit and repair, whatever the job's age.

	// 4. Count telegram inbox entries older than cutoff.
	_ = s.db.QueryRowContext(ctx, `
		SELECT count(*) FROM telegram_inbox
		WHERE state IN ('processed', 'failed') AND received_at < ?
	`, cutoffMs).Scan(&rep.TelegramInboxPruned)

	// 5. Count notification outbox entries older than cutoff.
	_ = s.db.QueryRowContext(ctx, `
		SELECT count(*) FROM notification_outbox
		WHERE state IN ('sent', 'dead') AND updated_at < ?
	`, cutoffMs).Scan(&rep.TelegramOutboxPruned)

	// 6. Count expired unconsumed plans.
	_ = s.db.QueryRowContext(ctx, `
		SELECT count(*) FROM plans
		WHERE (consumed_job IS NULL OR consumed_job = '')
		  AND id NOT IN (SELECT DISTINCT plan_id FROM collections)
		  AND json_extract(body, '$.expires_at') < ?
	`, cutoffStr).Scan(&rep.PlansPruned)

	if opt.DryRun {
		return rep, nil
	}

	// 7. Apply deletion in transaction.
	err = s.tx(ctx, func(tx *sql.Tx) error {
		// Prune events and jobs
		for _, jid := range jobIDs {
			if _, e := tx.ExecContext(ctx, "DELETE FROM events WHERE job_id = ?", jid); e != nil {
				return e
			}
			if _, e := tx.ExecContext(ctx, "DELETE FROM jobs WHERE id = ?", jid); e != nil {
				return e
			}
		}

		// Prune telegram inbox
		if _, e := tx.ExecContext(ctx, `
			DELETE FROM telegram_inbox
			WHERE state IN ('processed', 'failed') AND received_at < ?
		`, cutoffMs); e != nil {
			return e
		}

		// Prune notification outbox
		if _, e := tx.ExecContext(ctx, `
			DELETE FROM notification_outbox
			WHERE state IN ('sent', 'dead') AND updated_at < ?
		`, cutoffMs); e != nil {
			return e
		}

		// Prune expired/consumed callback states
		nowMs := time.Now().UnixMilli()
		_, _ = tx.ExecContext(ctx, `
			DELETE FROM callback_states
			WHERE (consumed = 1 OR expires_at < ?) AND created_at < ?
		`, nowMs, cutoffMs)

		// Prune expired unconsumed plans
		if _, e := tx.ExecContext(ctx, `
			DELETE FROM plans
			WHERE (consumed_job IS NULL OR consumed_job = '')
			  AND id NOT IN (SELECT DISTINCT plan_id FROM collections)
			  AND json_extract(body, '$.expires_at') < ?
		`, cutoffStr); e != nil {
			return e
		}

		return nil
	})

	return rep, err
}
