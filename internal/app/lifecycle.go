package app

import (
	"context"
	"fmt"
	"net/http"

	"github.com/itsgiahung/502Drive/internal/state"
)

type CleanupRequest struct {
	ExpectedRevision int    `json:"expected_revision"`
	DryRun           bool   `json:"dry_run"`
	ConfirmName      string `json:"confirm_name"`
}

func (s *Service) RetireCollection(ctx context.Context, p Principal, key, id string, expectedRevision int) (state.Collection, error) {
	if err := state.ValidateKey(key); err != nil {
		return state.Collection{}, Fail(http.StatusBadRequest, "invalid_request", "Idempotency-Key is required (1-128 printable characters).")
	}
	return s.Store.RetireCollection(ctx, p.ID, key, id, expectedRevision)
}

func (s *Service) ResumeCollection(ctx context.Context, p Principal, key, id string, expectedRevision int) (state.Collection, error) {
	if err := state.ValidateKey(key); err != nil {
		return state.Collection{}, Fail(http.StatusBadRequest, "invalid_request", "Idempotency-Key is required (1-128 printable characters).")
	}
	return s.Store.ResumeCollection(ctx, p.ID, key, id, expectedRevision)
}

func (s *Service) ForgetCollection(ctx context.Context, p Principal, key, id string, expectedRevision int) error {
	if err := state.ValidateKey(key); err != nil {
		return Fail(http.StatusBadRequest, "invalid_request", "Idempotency-Key is required (1-128 printable characters).")
	}
	return s.Store.ForgetCollection(ctx, p.ID, key, id, expectedRevision)
}

func (s *Service) CleanupCollection(ctx context.Context, p Principal, key, id string, req CleanupRequest) (state.Job, error) {
	if err := state.ValidateKey(key); err != nil {
		return state.Job{}, Fail(http.StatusBadRequest, "invalid_request", "Idempotency-Key is required (1-128 printable characters).")
	}
	col, err := s.Store.GetCollection(ctx, scope(p, true), id)
	if err != nil {
		return state.Job{}, err
	}
	if !req.DryRun {
		// The name to type is the one the folder has in Drive; the requested
		// name stays accepted for collections created before it was recorded.
		actual, err := s.Store.RootDestName(ctx, col.ID)
		if err != nil {
			return state.Job{}, err
		}
		if actual == "" {
			actual = col.RootName
		}
		if req.ConfirmName != actual && req.ConfirmName != col.RootName {
			return state.Job{}, Fail(http.StatusBadRequest, "name_confirmation_mismatch", fmt.Sprintf("Type the destination folder name exactly as it appears in Drive: %q.", actual))
		}
	}
	j, err := s.Store.AdmitCleanup(ctx, p.ID, key, id, req.ExpectedRevision, req.DryRun)
	if err != nil {
		return state.Job{}, err
	}
	s.wake()
	return j, nil
}
