package httpapi

import (
	"github.com/itsgiahung/502Drive/internal/app"
	"net/http"
)

func (s *Server) setup(w http.ResponseWriter, r *http.Request, p app.Principal) (int, any, error) {
	out, err := s.Svc.Setup(r.Context(), p)
	return http.StatusOK, out, err
}
