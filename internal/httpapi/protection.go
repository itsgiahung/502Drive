package httpapi

import (
	"net/http"

	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/state"
)

func (s *Server) protectionRoutes(route func(string, handler)) {
	route("GET /v1/collections/{id}/protection", s.getProtection)
	route("PUT /v1/collections/{id}/protection", s.putProtection)
	route("GET /v1/collections/{id}/snapshots", s.listSnapshots)
	route("POST /v1/collections/{id}/snapshots", s.createSnapshot)
	route("GET /v1/snapshots/{id}", s.getSnapshot)
	route("GET /v1/snapshots/{id}/items", s.snapshotItems)
	route("POST /v1/snapshots/{id}/pin", s.pinSnapshot(true))
	route("DELETE /v1/snapshots/{id}/pin", s.pinSnapshot(false))
	route("POST /v1/snapshots/{id}/restore", s.restoreSnapshot)
	route("GET /v1/jobs/{id}/restore-plan", s.restorePlan)
}

func (s *Server) getProtection(w http.ResponseWriter, r *http.Request, p app.Principal) (int, any, error) {
	c, err := s.Svc.Collection(r.Context(), p, r.PathValue("id"))
	if err != nil {
		return 0, nil, err
	}
	return 200, map[string]any{"collection_id": c.ID, "revision": c.Revision, "options": c.Protection.Defaults()}, nil
}

func (s *Server) putProtection(w http.ResponseWriter, r *http.Request, p app.Principal) (int, any, error) {
	if err := s.refuseMaintenance(); err != nil {
		return 0, nil, err
	}
	var in struct {
		Expected *int                     `json:"expected_revision"`
		Options  *state.ProtectionOptions `json:"options"`
	}
	if err := decode(r, &in); err != nil {
		return 0, nil, err
	}
	if in.Expected == nil || in.Options == nil {
		return 0, nil, app.Fail(400, "invalid_request", "expected_revision and options are required.")
	}
	c, replay, err := s.Svc.SetProtection(r.Context(), p, idemKey(r), r.PathValue("id"), *in.Expected, *in.Options)
	if err != nil {
		return 0, nil, err
	}
	markReplay(w, replay)
	return 200, map[string]any{"collection_id": c.ID, "revision": c.Revision, "options": c.Protection}, nil
}

func (s *Server) listSnapshots(w http.ResponseWriter, r *http.Request, p app.Principal) (int, any, error) {
	limit, err := pageParams(r)
	if err != nil {
		return 0, nil, err
	}
	col := r.PathValue("id")
	cursor, err := decodeCursor(r.URL.Query().Get("cursor"), "snapshots", p.ID, col)
	if err != nil {
		return 0, nil, err
	}
	items, next, err := s.Svc.Snapshots(r.Context(), p, col, limit, cursor)
	if err != nil {
		return 0, nil, err
	}
	return 200, map[string]any{"items": items, "next_cursor": encodeCursor("snapshots", p.ID, col, next)}, nil
}

func (s *Server) createSnapshot(w http.ResponseWriter, r *http.Request, p app.Principal) (int, any, error) {
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
		return 0, nil, app.Fail(400, "invalid_request", "expected_revision is required.")
	}
	j, replay, err := s.Svc.RequestSnapshot(r.Context(), p, idemKey(r), r.PathValue("id"), *in.ExpectedRevision)
	if err != nil {
		return 0, nil, err
	}
	markReplay(w, replay)
	w.Header().Set("Location", "/v1/jobs/"+j.ID)
	return 202, jobDTO(j), nil
}

func (s *Server) getSnapshot(w http.ResponseWriter, r *http.Request, p app.Principal) (int, any, error) {
	v, err := s.Svc.Snapshot(r.Context(), p, r.PathValue("id"))
	return 200, v, err
}

func (s *Server) snapshotItems(w http.ResponseWriter, r *http.Request, p app.Principal) (int, any, error) {
	limit, err := pageParams(r)
	if err != nil {
		return 0, nil, err
	}
	// IDs are opaque stable keys, with scope bound by an authorized snapshot read.
	id := r.PathValue("id")
	after := r.URL.Query().Get("after")
	items, err := s.Svc.SnapshotItems(r.Context(), p, id, limit+1, after)
	if err != nil {
		return 0, nil, err
	}
	var next any
	if len(items) > limit {
		items = items[:limit]
		next = items[len(items)-1].SourceID
	}
	return 200, map[string]any{"items": items, "next_after": next}, nil
}

func (s *Server) pinSnapshot(pin bool) handler {
	return func(w http.ResponseWriter, r *http.Request, p app.Principal) (int, any, error) {
		if err := s.refuseMaintenance(); err != nil {
			return 0, nil, err
		}
		var in actionInput
		if err := decode(r, &in); err != nil {
			return 0, nil, err
		}
		if in.ExpectedRevision == nil {
			return 0, nil, app.Fail(400, "invalid_request", "expected_revision is required.")
		}
		v, replay, err := s.Svc.PinSnapshot(r.Context(), p, idemKey(r), r.PathValue("id"), *in.ExpectedRevision, pin)
		if err != nil {
			return 0, nil, err
		}
		markReplay(w, replay)
		return 200, v, nil
	}
}

func (s *Server) restoreSnapshot(w http.ResponseWriter, r *http.Request, p app.Principal) (int, any, error) {
	if err := s.refuseMaintenance(); err != nil {
		return 0, nil, err
	}
	if err := s.refuseSleeping(); err != nil {
		return 0, nil, err
	}
	var in app.RestoreRequest
	if err := decode(r, &in); err != nil {
		return 0, nil, err
	}
	if in.ExpectedRevision < 1 {
		return 0, nil, app.Fail(400, "invalid_request", "expected_revision is required.")
	}
	j, replay, err := s.Svc.RequestRestore(r.Context(), p, idemKey(r), r.PathValue("id"), in)
	if err != nil {
		return 0, nil, err
	}
	markReplay(w, replay)
	w.Header().Set("Location", "/v1/jobs/"+j.ID)
	return 202, jobDTO(j), nil
}

func (s *Server) restorePlan(w http.ResponseWriter, r *http.Request, p app.Principal) (int, any, error) {
	limit, err := pageParams(r)
	if err != nil {
		return 0, nil, err
	}
	id := r.PathValue("id")
	after, err := decodeCursor(r.URL.Query().Get("cursor"), "restore-plan", p.ID, id)
	if err != nil {
		return 0, nil, err
	}
	if after < 0 || after > int64(^uint(0)>>1) {
		return 0, nil, app.Fail(400, "invalid_request", "Invalid cursor.")
	}
	items, err := s.Svc.RestorePlan(r.Context(), p, id, int(after), limit+1)
	if err != nil {
		return 0, nil, err
	}
	var next int64
	if len(items) > limit {
		items = items[:limit]
		next = int64(items[len(items)-1].Ordinal + 1)
	}
	return 200, map[string]any{"items": items, "next_cursor": encodeCursor("restore-plan", p.ID, id, next)}, nil
}
