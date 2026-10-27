package httpapi

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/state"
)

// encodeHistoryCursor packs the (updated_ms,rowid) keyset into the same opaque
// shape as the other cursors. It is bound to the scope and the since value, so
// changing either one is an error rather than a silently wrong page.
func encodeHistoryCursor(c state.HistoryCursor, scopeKey, since string) *string {
	if c.RowID == 0 {
		return nil
	}
	raw := fmt.Sprintf("%s\x1f%s\x1f%d:%d", scopeKey, since, c.UpdatedMS, c.RowID)
	v := base64.RawURLEncoding.EncodeToString([]byte(raw))
	return &v
}

func decodeHistoryCursor(raw, scopeKey, since string) (state.HistoryCursor, error) {
	if raw == "" {
		return state.HistoryCursor{}, nil
	}
	bad := app.Fail(http.StatusBadRequest, "invalid_request", "The cursor is not valid for this request.")
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return state.HistoryCursor{}, bad
	}
	parts := strings.Split(string(b), "\x1f")
	if len(parts) != 3 || parts[0] != scopeKey || parts[1] != since {
		return state.HistoryCursor{}, bad
	}
	pair := strings.Split(parts[2], ":")
	if len(pair) != 2 {
		return state.HistoryCursor{}, bad
	}
	ms, err1 := strconv.ParseInt(pair[0], 10, 64)
	row, err2 := strconv.ParseInt(pair[1], 10, 64)
	if err1 != nil || err2 != nil || row < 1 {
		return state.HistoryCursor{}, bad
	}
	return state.HistoryCursor{UpdatedMS: ms, RowID: row}, nil
}

// historySince parses the export window. RFC3339 and a bare date are accepted;
// the CLI turns "30d" into a timestamp before calling.
func historySince(raw string) (time.Time, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, "", nil
	}
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return t.UTC(), t.UTC().Format(time.RFC3339Nano), nil
	}
	if t, err := time.Parse("2006-01-02", raw); err == nil {
		return t.UTC(), t.UTC().Format(time.RFC3339Nano), nil
	}
	return time.Time{}, "", app.Fail(http.StatusBadRequest, "invalid_request", "since must be RFC3339 or YYYY-MM-DD.")
}

func (s *Server) history(w http.ResponseWriter, r *http.Request, p app.Principal) (int, any, error) {
	limit, err := pageParams(r)
	if err != nil {
		return 0, nil, err
	}
	q := r.URL.Query()
	all := q.Get("all") == "true"
	since, sinceKey, err := historySince(q.Get("since"))
	if err != nil {
		return 0, nil, err
	}
	scopeKey := p.ID
	if all && p.Role == app.RoleOwner {
		scopeKey = "*"
	}
	cur, err := decodeHistoryCursor(q.Get("cursor"), scopeKey, sinceKey)
	if err != nil {
		return 0, nil, err
	}
	jobs, next, err := s.Svc.History(r.Context(), p, all, since, limit, cur)
	if err != nil {
		return 0, nil, err
	}
	items := make([]any, 0, len(jobs))
	for _, j := range jobs {
		items = append(items, jobDTO(j))
	}
	return 200, map[string]any{"items": items, "next_cursor": encodeHistoryCursor(next, scopeKey, sinceKey)}, nil
}
