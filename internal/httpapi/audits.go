package httpapi

import (
	"net/http"
	"strconv"

	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/present"
	"github.com/itsgiahung/502Drive/internal/state"
)

// auditDTO is the wire shape of one integrity audit.
func auditDTO(a state.Audit) map[string]any {
	counts := a.Counts
	if counts == nil {
		counts = map[string]int{}
	}
	out := map[string]any{
		"id": a.ID, "collection_id": a.CollectionID, "job_id": a.JobID, "state": a.State,
		"counts": counts, "repairable_findings": a.Repairable, "scanned_folders": a.Scanned, "revision": a.Revision,
		"started_at": ts(a.StartedAt),
	}
	if !a.FinishedAt.IsZero() {
		out["finished_at"] = ts(a.FinishedAt)
	}
	if len(a.Plan) > 0 {
		out["plan"] = a.Plan
	}
	return out
}

func findingDTO(f state.Finding) map[string]any {
	return map[string]any{
		"path": f.Path, "explanation": present.Explain(f.Kind, "en"), "audit_id": f.AuditID, "kind": f.Kind, "source_id": f.SourceID, "dest_id": f.DestID,
		"repairable": f.Repairable, "repaired": f.Repaired, "detail": f.Detail,
	}
}

// startAudit admits a read-only integrity audit for a collection.
func (s *Server) startAudit(w http.ResponseWriter, r *http.Request, p app.Principal) (int, any, error) {
	if err := s.refuseMaintenance(); err != nil {
		return 0, nil, err
	}
	if err := s.refuseSleeping(); err != nil {
		return 0, nil, err
	}
	id := r.PathValue("id")
	a, _, replayed, err := s.Svc.RequestAudit(r.Context(), p, idemKey(r), id)
	if err != nil {
		return 0, nil, err
	}
	markReplay(w, replayed)
	w.Header().Set("Location", "/v1/audits/"+a.ID)
	return http.StatusAccepted, auditDTO(a), nil
}

// latestAudit returns the newest audit of a collection, or 404.
func (s *Server) latestAudit(w http.ResponseWriter, r *http.Request, p app.Principal) (int, any, error) {
	a, _, err := s.Svc.LastAudit(r.Context(), p, r.PathValue("id"))
	if err != nil {
		return 0, nil, err
	}
	return http.StatusOK, auditDTO(a), nil
}

// getAudit returns one audit.
func (s *Server) getAudit(w http.ResponseWriter, r *http.Request, p app.Principal) (int, any, error) {
	a, err := s.Svc.Audit(r.Context(), p, r.PathValue("id"))
	if err != nil {
		return 0, nil, err
	}
	return http.StatusOK, auditDTO(a), nil
}

// auditFindings pages the findings of an audit.
func (s *Server) auditFindings(w http.ResponseWriter, r *http.Request, p app.Principal) (int, any, error) {
	q := r.URL.Query()
	limit := 50
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 500 {
			return 0, nil, app.Fail(http.StatusBadRequest, "invalid_request", "limit must be between 1 and 500.")
		}
		limit = n
	}
	var cursor int64
	if v := q.Get("cursor"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 0 {
			return 0, nil, app.Fail(http.StatusBadRequest, "invalid_request", "bad cursor.")
		}
		cursor = n
	}
	items, next, err := s.Svc.AuditFindings(r.Context(), p, r.PathValue("id"), q.Get("kind"), limit, cursor)
	if err != nil {
		return 0, nil, err
	}
	out := make([]map[string]any, 0, len(items))
	for _, f := range items {
		out = append(out, findingDTO(f))
	}
	body := map[string]any{"items": out, "next_cursor": strconv.FormatInt(next, 10)}
	if next == 0 {
		body["next_cursor"] = nil
	}
	return http.StatusOK, body, nil
}

// repairAudit admits a repair job for a completed audit.
func (s *Server) repairAudit(w http.ResponseWriter, r *http.Request, p app.Principal) (int, any, error) {
	if err := s.refuseMaintenance(); err != nil {
		return 0, nil, err
	}
	if err := s.refuseSleeping(); err != nil {
		return 0, nil, err
	}
	var in app.RepairRequest
	if err := decode(r, &in); err != nil {
		return 0, nil, err
	}
	j, replayed, err := s.Svc.RequestRepair(r.Context(), p, idemKey(r), r.PathValue("id"), in)
	if err != nil {
		return 0, nil, err
	}
	markReplay(w, replayed)
	w.Header().Set("Location", "/v1/jobs/"+j.ID)
	return http.StatusAccepted, jobDTO(j), nil
}
