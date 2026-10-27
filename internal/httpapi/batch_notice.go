package httpapi

import (
	"github.com/itsgiahung/502Drive/internal/app"
	"net/http"
)

func (s *Server) sealBatch(w http.ResponseWriter, r *http.Request, p app.Principal) (int, any, error) {
	err := s.Svc.SealBatch(r.Context(), p, r.PathValue("batch_id"))
	return 200, map[string]bool{"sealed": true}, err
}
