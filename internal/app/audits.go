package app

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/itsgiahung/502Drive/internal/state"
)

// RequestAudit starts a read-only integrity audit of a collection. The audit
// runs as a durable job, so the usual admission rules apply (idempotency key,
// one active job per collection, capacity).
func (s *Service) RequestAudit(ctx context.Context, p Principal, key, collectionID string) (state.Audit, state.Job, bool, error) {
	if err := state.ValidateKey(key); err != nil {
		return state.Audit{}, state.Job{}, false, Fail(http.StatusBadRequest, "invalid_request", "Idempotency-Key is required (1-128 printable characters).")
	}
	a, j, replayed, err := s.Store.AdmitAudit(ctx, p.ID, scope(p, true), key, collectionID)
	if err != nil {
		return state.Audit{}, state.Job{}, false, err
	}
	if !replayed {
		s.wake()
	}
	return a, j, replayed, nil
}

// Audit reads one audit with its finding counts.
func (s *Service) Audit(ctx context.Context, p Principal, id string) (state.Audit, error) {
	return s.Store.GetAudit(ctx, scope(p, true), id)
}

// AuditFindings pages the findings of an audit, optionally filtered by kind.
func (s *Service) AuditFindings(ctx context.Context, p Principal, id, kind string, limit int, cursor int64) ([]state.Finding, int64, error) {
	a, err := s.Store.GetAudit(ctx, scope(p, true), id)
	if err != nil {
		return nil, 0, err
	}
	findings, next, err := s.Store.ListFindings(ctx, a.ID, kind, limit, cursor)
	if err == nil {
		err = s.Store.ResolveFindingPaths(ctx, a.CollectionID, findings)
	}
	return findings, next, err
}

// LastAudit returns the newest audit of a collection, for the collection card.
func (s *Service) LastAudit(ctx context.Context, p Principal, collectionID string) (state.Audit, bool, error) {
	a, err := s.Store.LastAudit(ctx, scope(p, true), collectionID)
	if err != nil {
		return state.Audit{}, false, err
	}
	return a, true, nil
}

// RepairRequest is the body of POST /v1/audits/{id}/repair. A dry run plans
// the fixes and writes nothing; the default for the CLI is a dry run.
type RepairRequest struct {
	ExpectedRevision *int `json:"expected_revision"`
	DryRun           bool `json:"dry_run"`
}

// RequestRepair admits a repair job for a completed audit.
func (s *Service) RequestRepair(ctx context.Context, p Principal, key, auditID string, in RepairRequest) (state.Job, bool, error) {
	if err := state.ValidateKey(key); err != nil {
		return state.Job{}, false, Fail(http.StatusBadRequest, "invalid_request", "Idempotency-Key is required (1-128 printable characters).")
	}
	if in.ExpectedRevision == nil {
		return state.Job{}, false, Fail(http.StatusBadRequest, "invalid_request", "expected_revision is required.")
	}
	j, replayed, err := s.Store.AdmitRepair(ctx, p.ID, scope(p, true), key, auditID, *in.ExpectedRevision, in.DryRun)
	if err != nil {
		return state.Job{}, false, err
	}
	if !replayed {
		s.wake()
	}
	return j, replayed, nil
}

// RepairPlanJSON is a small helper for adapters that show the stored plan.
func RepairPlanJSON(a state.Audit) json.RawMessage { return a.Plan }
