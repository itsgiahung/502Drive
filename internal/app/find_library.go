package app

import (
	"context"
	"github.com/itsgiahung/502Drive/internal/state"
	"strings"
	"unicode/utf8"
)

func (s *Service) FindLibrary(ctx context.Context, p Principal, query string, limit int, cursor int64) ([]state.Collection, int64, error) {
	if p.ID == "" || (p.Role != RoleOwner && p.Role != RoleOperator) {
		return nil, 0, Fail(403, "forbidden", "A valid principal is required.")
	}
	query = strings.TrimSpace(query)
	if utf8.RuneCountInString(query) > 80 || cursor < 0 {
		return nil, 0, Fail(400, "invalid_request", "Invalid library search.")
	}
	return s.CollectionPage(ctx, p, p.Role == RoleOwner, true, query, "", limit, cursor)
}
