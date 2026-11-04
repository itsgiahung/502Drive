package cli

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/itsgiahung/502Drive/internal/tui"
)

func TestLibraryReadsRequestedPagesAndKeepsCursor(t *testing.T) {
	var cursors []string
	a := driveApp(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/collections" {
			t.Errorf("unexpected request: %s", r.URL.Path)
		}
		cursor := r.URL.Query().Get("cursor")
		cursors = append(cursors, cursor)
		if r.URL.Query().Get("include_retired") != "true" {
			t.Error("pagination lost retired filter")
		}
		if cursor == "page2" {
			reply(w, 200, map[string]any{"items": []any{map[string]any{"id": "col_21", "source_name": "last"}}, "next_cursor": nil})
			return
		}
		var items []any
		for i := range 20 {
			items = append(items, map[string]any{"id": fmt.Sprintf("col_%02d", i), "source_name": "source"})
		}
		reply(w, 200, map[string]any{"items": items, "next_cursor": "page2"})
	})
	a.showRetired.Store(true)
	var snap tui.Snapshot
	a.tuiCollections(t.Context(), &snap)
	if len(snap.Collections) != 20 || snap.CollectionsNext != "page2" || len(cursors) != 1 {
		t.Fatalf("first page must expose cursor without eagerly loading library: rows=%d next=%q calls=%v", len(snap.Collections), snap.CollectionsNext, cursors)
	}
	a.slow.collectionPages = 2
	snap = tui.Snapshot{}
	a.tuiCollections(t.Context(), &snap)
	if len(snap.Collections) != 21 || snap.CollectionsNext != "" || snap.Collections[20].ID != "col_21" {
		t.Fatalf("second page missing: rows=%d next=%q", len(snap.Collections), snap.CollectionsNext)
	}
}

func TestTrayHistoryCarriesScopeWindowAndCursor(t *testing.T) {
	since := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	a := driveApp(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.URL.Path != "/v1/history" || q.Get("limit") != "100" || q.Get("all") != "true" || q.Get("since") != "2026-11-05T00:00:00Z" || q.Get("cursor") != "page2" {
			t.Errorf("notification read lost history window: %s", r.URL)
		}
		reply(w, 200, map[string]any{"items": []any{map[string]any{
			"id": "old-job", "state": "failed", "source_name": "Older source", "updated_at": "2026-11-05T00:00:01Z",
			"last_error": map[string]any{"code": "permission_denied"},
		}}, "next_cursor": "page3"})
	})
	jobs, next, err := a.trayJobChanges(t.Context(), since, "page2")
	if err != nil || next != "page3" || len(jobs) != 1 || jobs[0].ID != "old-job" || jobs[0].LastErrorCode != "permission_denied" || !jobs[0].UpdatedAt.Equal(since.Add(time.Second)) {
		t.Fatalf("history page lost job state/cursor: %+v %q %v", jobs, next, err)
	}
}
