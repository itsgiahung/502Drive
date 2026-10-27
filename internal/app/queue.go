package app

import (
	"context"

	"github.com/itsgiahung/502Drive/internal/state"
)

// QueuePause pauses (or resumes) the whole queue. It is an owner action: it
// affects every principal's jobs.
func (s *Service) QueuePause(ctx context.Context, p Principal, paused bool) (state.QueueChange, error) {
	if err := requireOwner(p); err != nil {
		return state.QueueChange{}, err
	}
	out, err := s.Store.SetQueuePaused(ctx, paused)
	if err != nil {
		return state.QueueChange{}, err
	}
	if !paused {
		s.wake()
	}
	return out, nil
}

// MoveJob reorders or re-prioritises a queued job. Owners may move any job,
// others only their own (the same rule as JobAction).
func (s *Service) MoveJob(ctx context.Context, p Principal, id string, revision int, m state.Move) (state.Result, error) {
	owner := p.ID
	if p.Role == RoleOwner {
		j, err := s.Store.GetJob(ctx, "", id)
		if err != nil {
			return state.Result{}, err
		}
		owner = j.Principal
	}
	res, err := s.Store.MoveJob(ctx, owner, id, revision, m)
	if err != nil {
		return state.Result{}, err
	}
	s.wake()
	return res, nil
}

// Batch returns the jobs admitted under one batch id, scoped to the caller.
func (s *Service) Batch(ctx context.Context, p Principal, batchID string) ([]state.Job, error) {
	if !batchIDPattern.MatchString(batchID) {
		return nil, Fail(400, "invalid_request", "batch_id must be 8-64 letters, digits, '-' or '_'.")
	}
	jobs, err := s.Store.BatchJobs(ctx, scope(p, false), batchID, 500)
	if err != nil {
		return nil, err
	}
	if len(jobs) == 0 {
		return nil, state.ErrNotFound
	}
	return jobs, nil
}
