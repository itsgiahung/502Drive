package app

import (
	"context"
	"time"

	"github.com/itsgiahung/502Drive/internal/state"
)

// HistoryClearRequest picks what to clear: finished jobs older than N days
// (0 = every finished job; owner only) or the listed job ids (an operator may
// do this for their own). DryRun nil means true: preview first.
type HistoryClearRequest struct {
	OlderThanDays *int     `json:"older_than_days"`
	JobIDs        []string `json:"job_ids"`
	DryRun        *bool    `json:"dry_run"`
}

// ClearHistory removes finished jobs from the job lists and lets retention
// delete what nothing refers to. Collections, mappings and Drive are untouched.
func (s *Service) ClearHistory(ctx context.Context, p Principal, req HistoryClearRequest) (state.HistoryClear, error) {
	byAge, byIDs := req.OlderThanDays != nil, len(req.JobIDs) > 0
	if byAge == byIDs {
		return state.HistoryClear{}, Fail(400, "invalid_request", "Give either older_than_days or job_ids.")
	}
	if len(req.JobIDs) > 500 {
		return state.HistoryClear{}, Fail(400, "invalid_request", "At most 500 job_ids at a time.")
	}
	dry := req.DryRun == nil || *req.DryRun
	var before time.Time
	if byAge {
		if err := requireOwner(p); err != nil {
			return state.HistoryClear{}, err
		}
		if *req.OlderThanDays < 0 || *req.OlderThanDays > 36500 {
			return state.HistoryClear{}, Fail(400, "invalid_request", "older_than_days must be between 0 and 36500.")
		}
		if *req.OlderThanDays > 0 {
			before = time.Now().Add(-time.Duration(*req.OlderThanDays) * 24 * time.Hour)
		}
	}
	return s.Store.ClearHistory(ctx, scope(p, true), before, req.JobIDs, dry)
}
