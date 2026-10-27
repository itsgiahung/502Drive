package httpapi

import (
	"net/http"

	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/filter"
)

// filterInput is the body of PUT /v1/collections/{id}/filter.
type filterInput struct {
	Filter           filter.Spec `json:"filter"`
	ExpectedRevision *int        `json:"expected_revision"`
}

// getCollectionFilter shows the rule later runs of the collection will use.
func (s *Server) getCollectionFilter(w http.ResponseWriter, r *http.Request, p app.Principal) (int, any, error) {
	c, err := s.Svc.Collection(r.Context(), p, r.PathValue("id"))
	if err != nil {
		return 0, nil, err
	}
	return 200, map[string]any{"collection_id": c.ID, "revision": c.Revision, "filter": c.Filter}, nil
}

// setCollectionFilter replaces the rule. expected_revision is the collection
// revision the caller read; a stale value is refused rather than overwritten.
func (s *Server) setCollectionFilter(w http.ResponseWriter, r *http.Request, p app.Principal) (int, any, error) {
	if err := s.refuseMaintenance(); err != nil {
		return 0, nil, err
	}
	var in filterInput
	if err := decode(r, &in); err != nil {
		return 0, nil, err
	}
	if in.ExpectedRevision == nil {
		return 0, nil, app.Fail(http.StatusBadRequest, "invalid_request", "expected_revision is required.")
	}
	c, err := s.Svc.SetCollectionFilter(r.Context(), p, r.PathValue("id"), in.Filter, *in.ExpectedRevision)
	if err != nil {
		return 0, nil, err
	}
	return 200, map[string]any{"collection_id": c.ID, "revision": c.Revision, "filter": c.Filter}, nil
}
