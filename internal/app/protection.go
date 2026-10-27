package app

import (
	"context"
	"net/http"

	"github.com/itsgiahung/502Drive/internal/state"
)

func (s *Service) SetProtection(ctx context.Context, p Principal, key, id string, expected int, opts state.ProtectionOptions) (state.Collection, bool, error) {
	if err := state.ValidateKey(key); err != nil {
		return state.Collection{}, false, Fail(400, "invalid_request", "Idempotency-Key is required.")
	}
	opts = opts.Defaults()
	if err := opts.Validate(); err != nil {
		return state.Collection{}, false, Fail(400, "invalid_request", err.Error())
	}
	return s.Store.SetProtection(ctx, p.ID, scope(p, true), key, id, expected, opts)
}

func (s *Service) Snapshots(ctx context.Context, p Principal, id string, limit int, cursor int64) ([]state.Snapshot, int64, error) {
	return s.Store.Snapshots(ctx, scope(p, true), id, limit, cursor)
}
func (s *Service) Snapshot(ctx context.Context, p Principal, id string) (state.Snapshot, error) {
	return s.Store.Snapshot(ctx, scope(p, true), id)
}

func (s *Service) SnapshotItems(ctx context.Context, p Principal, id string, limit int, after string) ([]state.SnapshotItem, error) {
	if _, err := s.Snapshot(ctx, p, id); err != nil {
		return nil, err
	}
	return s.Store.SnapshotItems(ctx, id, limit, after)
}

func (s *Service) RequestSnapshot(ctx context.Context, p Principal, key, id string, expected int) (state.Job, bool, error) {
	if err := state.ValidateKey(key); err != nil {
		return state.Job{}, false, Fail(400, "invalid_request", "Idempotency-Key is required.")
	}
	j, replay, err := s.Store.AdmitProtectionJob(ctx, p.ID, scope(p, true), key, id, "", expected, false)
	if err == nil {
		s.wake()
	}
	return j, replay, err
}

type RestoreRequest struct {
	ExpectedRevision int   `json:"expected_revision"`
	DryRun           *bool `json:"dry_run"`
}

func (s *Service) RequestRestore(ctx context.Context, p Principal, key, id string, req RestoreRequest) (state.Job, bool, error) {
	if err := state.ValidateKey(key); err != nil {
		return state.Job{}, false, Fail(http.StatusBadRequest, "invalid_request", "Idempotency-Key is required.")
	}
	v, err := s.Snapshot(ctx, p, id)
	if err != nil {
		return state.Job{}, false, err
	}
	dry := req.DryRun == nil || *req.DryRun
	j, replay, err := s.Store.AdmitProtectionJob(ctx, p.ID, scope(p, true), key, v.CollectionID, id, req.ExpectedRevision, dry)
	if err == nil {
		s.wake()
	}
	return j, replay, err
}

func (s *Service) PinSnapshot(ctx context.Context, p Principal, key, id string, expected int, pin bool) (state.Snapshot, bool, error) {
	if err := state.ValidateKey(key); err != nil {
		return state.Snapshot{}, false, Fail(400, "invalid_request", "Idempotency-Key is required.")
	}
	return s.Store.PinSnapshot(ctx, p.ID, scope(p, true), key, id, expected, pin)
}

func (s *Service) RestorePlan(ctx context.Context, p Principal, jobID string, after, limit int) ([]state.DestinationAction, error) {
	j, err := s.Store.GetJob(ctx, scope(p, true), jobID)
	if err != nil {
		return nil, err
	}
	if j.Kind != state.JobKindRestore {
		return nil, state.ErrNotFound
	}
	return s.Store.DestinationActionPage(ctx, jobID, after, limit)
}
