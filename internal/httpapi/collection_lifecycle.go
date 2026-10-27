package httpapi

import (
	"net/http"

	"github.com/itsgiahung/502Drive/internal/app"
)

// lifecycleRoutes registers the collection lifecycle routes (L1): retire,
// resume, forget and cleanup. None of them touches Drive except cleanup, which
// only moves the 502Drive-owned destination root to the Drive trash.
func (s *Server) lifecycleRoutes(route func(string, handler)) {
	route("POST /v1/collections/{id}/retire", s.retireCollection)
	route("POST /v1/collections/{id}/resume", s.resumeCollection)
	route("POST /v1/collections/{id}/forget", s.forgetCollection)
	route("POST /v1/collections/{id}/cleanup", s.cleanupCollection)
}

func (s *Server) retireCollection(w http.ResponseWriter, r *http.Request, p app.Principal) (int, any, error) {
	in, err := expectedRevisionInput(r)
	if err != nil {
		return 0, nil, err
	}
	c, err := s.Svc.RetireCollection(r.Context(), p, idemKey(r), r.PathValue("id"), in)
	if err != nil {
		return 0, nil, err
	}
	dto, err := s.collectionDTO(r.Context(), c)
	if err != nil {
		return 0, nil, err
	}
	return http.StatusOK, dto, nil
}

func (s *Server) resumeCollection(w http.ResponseWriter, r *http.Request, p app.Principal) (int, any, error) {
	in, err := expectedRevisionInput(r)
	if err != nil {
		return 0, nil, err
	}
	c, err := s.Svc.ResumeCollection(r.Context(), p, idemKey(r), r.PathValue("id"), in)
	if err != nil {
		return 0, nil, err
	}
	dto, err := s.collectionDTO(r.Context(), c)
	if err != nil {
		return 0, nil, err
	}
	return http.StatusOK, dto, nil
}

func (s *Server) forgetCollection(w http.ResponseWriter, r *http.Request, p app.Principal) (int, any, error) {
	in, err := expectedRevisionInput(r)
	if err != nil {
		return 0, nil, err
	}
	id := r.PathValue("id")
	if err := s.Svc.ForgetCollection(r.Context(), p, idemKey(r), id, in); err != nil {
		return 0, nil, err
	}
	return http.StatusOK, map[string]any{"deleted": true, "id": id}, nil
}

// cleanupCollection admits a durable cleanup job; 202 because the job, not the
// trash move, is the committed result. Dry-run is the default in the CLI.
func (s *Server) cleanupCollection(w http.ResponseWriter, r *http.Request, p app.Principal) (int, any, error) {
	if err := s.refuseMaintenance(); err != nil {
		return 0, nil, err
	}
	var in struct {
		ExpectedRevision *int   `json:"expected_revision"`
		DryRun           *bool  `json:"dry_run"`
		ConfirmName      string `json:"confirm_name"`
	}
	if err := decode(r, &in); err != nil {
		return 0, nil, err
	}
	if in.ExpectedRevision == nil {
		return 0, nil, app.Fail(http.StatusBadRequest, "invalid_request", "expected_revision is required.")
	}
	dryRun := true
	if in.DryRun != nil {
		dryRun = *in.DryRun
	}
	job, err := s.Svc.CleanupCollection(r.Context(), p, idemKey(r), r.PathValue("id"), app.CleanupRequest{
		ExpectedRevision: *in.ExpectedRevision,
		DryRun:           dryRun,
		ConfirmName:      in.ConfirmName,
	})
	if err != nil {
		return 0, nil, err
	}
	w.Header().Set("Location", "/v1/jobs/"+job.ID)
	return http.StatusAccepted, jobDTO(job), nil
}

func expectedRevisionInput(r *http.Request) (int, error) {
	var in actionInput
	if err := decode(r, &in); err != nil {
		return 0, err
	}
	if in.ExpectedRevision == nil {
		return 0, app.Fail(http.StatusBadRequest, "invalid_request", "expected_revision is required.")
	}
	return *in.ExpectedRevision, nil
}
