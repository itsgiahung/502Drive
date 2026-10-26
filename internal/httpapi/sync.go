package httpapi

import (
	"net/http"

	"github.com/itsgiahung/502Drive/internal/app"
)

// syncRoutes registers the collection sync routes of the HTTP contract.
func (s *Server) syncRoutes(route func(string, handler)) {
	route("PUT /v1/collections/{id}/sync", s.configureSync)
	route("POST /v1/collections/{id}/sync/pause", s.syncAction("pause"))
	route("POST /v1/collections/{id}/sync/resume", s.syncAction("resume"))
	route("POST /v1/collections/{id}/sync/stop", s.syncAction("stop"))
	route("POST /v1/collections/{id}/runs", s.runNow)
}

// configureSync creates or replaces the subscription; 200 because the
// subscription itself (not a job) is the committed result.
func (s *Server) configureSync(w http.ResponseWriter, r *http.Request, p app.Principal) (int, any, error) {
	if err := s.refuseMaintenance(); err != nil {
		return 0, nil, err
	}
	if err := s.refuseSleeping(); err != nil {
		return 0, nil, err
	}
	var in app.SyncRequest
	if err := decode(r, &in); err != nil {
		return 0, nil, err
	}
	view, replayed, err := s.Svc.ConfigureSync(r.Context(), p, idemKey(r), r.PathValue("id"), in)
	if err != nil {
		return 0, nil, err
	}
	markReplay(w, replayed)
	return http.StatusOK, syncDTO(view), nil
}

func (s *Server) syncAction(action string) handler {
	return func(w http.ResponseWriter, r *http.Request, p app.Principal) (int, any, error) {
		var in actionInput
		if err := decode(r, &in); err != nil {
			return 0, nil, err
		}
		if in.ExpectedRevision == nil {
			return 0, nil, app.Fail(http.StatusBadRequest, "invalid_request", "expected_revision is required.")
		}
		view, replayed, err := s.Svc.SyncAction(r.Context(), p, idemKey(r), r.PathValue("id"), action, *in.ExpectedRevision)
		if err != nil {
			return 0, nil, err
		}
		markReplay(w, replayed)
		return http.StatusAccepted, syncDTO(view), nil
	}
}

// runNow admits a delta run; the response is the job (or the active job when
// the request was coalesced) plus its Location.
func (s *Server) runNow(w http.ResponseWriter, r *http.Request, p app.Principal) (int, any, error) {
	if err := s.refuseMaintenance(); err != nil {
		return 0, nil, err
	}
	if err := s.refuseSleeping(); err != nil {
		return 0, nil, err
	}
	var in actionInput
	if err := decode(r, &in); err != nil {
		return 0, nil, err
	}
	if in.ExpectedRevision == nil {
		return 0, nil, app.Fail(http.StatusBadRequest, "invalid_request", "expected_revision is required.")
	}
	res, replayed, err := s.Svc.RunNow(r.Context(), p, idemKey(r), r.PathValue("id"), *in.ExpectedRevision)
	if err != nil {
		return 0, nil, err
	}
	markReplay(w, replayed)
	w.Header().Set("Location", "/v1/jobs/"+res.Job.ID)
	return http.StatusAccepted, jobDTO(res.Job), nil
}
