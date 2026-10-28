package state

import (
	"context"
	"strings"
	"time"
)

// HistoryClear is what a history clear did, or would do on a dry run.
type HistoryClear struct {
	DryRun bool `json:"dry_run"`
	// Hidden counts finished jobs removed from the lists. Skipped counts the
	// requested ones that were not finished, unknown or not the caller's.
	Hidden  int `json:"hidden"`
	Skipped int `json:"skipped"`
	// Deleted is what retention could also remove for real: jobs no item
	// points at, their events, old plans and Telegram records.
	Deleted PruneReport `json:"deleted"`
}

const finishedStates = "'succeeded','partial','failed','cancelled'"

// ClearHistory hides finished jobs from the job lists and then lets retention
// delete whatever nothing refers to. It never touches collections, mappings or
// Drive. before zero means every finished job; ids, when given, restrict it to
// those jobs. principal "" is the owner and reaches every principal's jobs.
func (s *Store) ClearHistory(ctx context.Context, principal string, before time.Time, ids []string, dryRun bool) (HistoryClear, error) {
	out := HistoryClear{DryRun: dryRun}
	where := "state IN (" + finishedStates + ") AND json_extract(body,'$.hidden_at') IS NULL AND (?='' OR principal=?)"
	args := []any{principal, principal}
	if !before.IsZero() {
		// julianday, not a text compare: "…00.500Z" sorts before "…00Z" as text.
		where += " AND julianday(json_extract(body,'$.updated_at')) < julianday(?)"
		args = append(args, before.UTC().Format(time.RFC3339Nano))
	}
	if len(ids) > 0 {
		where += " AND id IN (" + strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",") + ")"
		for _, id := range ids {
			args = append(args, id)
		}
	}
	var n int
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM jobs WHERE "+where, args...).Scan(&n); err != nil {
		return out, err
	}
	out.Hidden = n
	if len(ids) > 0 {
		out.Skipped = len(ids) - n
	}
	if !dryRun && n > 0 {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if _, err := s.db.ExecContext(ctx, "UPDATE jobs SET body=json_set(body,'$.hidden_at',?) WHERE "+where,
			append([]any{now}, args...)...); err != nil {
			return out, err
		}
	}
	// Real deletion is retention's, owner-wide work: only for a whole-history
	// clear by the owner, never for a handful of ids or an operator.
	if principal == "" && len(ids) == 0 {
		age := time.Duration(0)
		if !before.IsZero() {
			age = time.Since(before)
		}
		opt := PruneOptions{OlderThan: age, DryRun: dryRun, All: before.IsZero()}
		rep, err := s.Prune(ctx, opt)
		if err != nil {
			return out, err
		}
		out.Deleted = rep
	}
	return out, nil
}
