package httpapi

import (
	"net/http"

	"github.com/itsgiahung/502Drive/internal/app"
)

func (s *Server) historyClearRoutes(route func(string, handler)) {
	route("POST /v1/history/clear", s.clearHistory)
}

// clearHistory hides finished jobs from the lists. It previews unless the
// body says "dry_run": false, and never touches collections or Drive.
func (s *Server) clearHistory(w http.ResponseWriter, r *http.Request, p app.Principal) (int, any, error) {
	var in app.HistoryClearRequest
	if err := decode(r, &in); err != nil {
		return 0, nil, err
	}
	if in.DryRun != nil && !*in.DryRun {
		if err := s.refuseMaintenance(); err != nil {
			return 0, nil, err
		}
	}
	out, err := s.Svc.ClearHistory(r.Context(), p, in)
	if err != nil {
		return 0, nil, err
	}
	return 200, out, nil
}
