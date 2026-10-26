package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func driveApp(t *testing.T, h http.HandlerFunc) *App {
	t.Helper()
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	a := &App{io: IO{Out: &strings.Builder{}, Err: &strings.Builder{}}}
	a.client = &Client{Endpoint: ts.URL, Token: "tok"}
	return a
}

func reply(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{"data": data, "request_id": "req_1"})
}

func TestTUIBrowseMapsFoldersFilesAndMetadata(t *testing.T) {
	a := driveApp(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/drive/folders/root" || r.URL.Query().Get("page_token") != "tok2" {
			t.Errorf("unexpected request %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		reply(w, 200, map[string]any{
			"folder": map[string]any{"id": "root", "name": "My Drive", "kind": "folder", "can_add_children": true},
			"path":   []any{map[string]any{"id": "root", "name": "My Drive"}},
			"items": []any{map[string]any{"id": "f1", "name": "a.pdf", "kind": "binary", "mime_type": "application/pdf", "size_bytes": 2048,
				"modified_at": "2026-10-01T08:30:00Z", "owner": map[string]any{"name": "Ann", "email": "ann@example.test"}, "can_copy": true}},
			"next_page_token": "tok3",
		})
	})
	page, err := a.tuiBrowse(t.Context(), "root", "tok2")
	if err != nil {
		t.Fatal(err)
	}
	if page.Next != "tok3" || len(page.Path) != 1 || len(page.Items) != 1 || !page.Folder.CanWrite {
		t.Fatalf("page = %+v", page)
	}
	it := page.Items[0]
	if it.Size == nil || *it.Size != 2048 || it.Modified.IsZero() || it.Owner != "Ann <ann@example.test>" || !it.CanCopy {
		t.Fatalf("item = %+v", it)
	}
}

func TestTUIAddDestinationReusesAnExistingFolder(t *testing.T) {
	posts := 0
	a := driveApp(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/v1/destinations":
			reply(w, 200, map[string]any{"items": []any{map[string]any{"id": "dst_1", "name": "Archive", "folder_id": "FOLDER", "revision": 4, "is_default": true}}})
		case r.Method == "POST":
			posts++
			reply(w, 201, map[string]any{"id": "dst_2", "name": "New", "folder_id": "OTHER", "revision": 1})
		}
	})
	d, err := a.tuiAddDestination(t.Context(), "FOLDER")
	if err != nil || d.ID != "dst_1" || d.Revision != 4 || posts != 0 {
		t.Fatalf("an existing folder must be reused, not added again: %+v %v (posts=%d)", d, err, posts)
	}
	d, err = a.tuiAddDestination(t.Context(), "OTHER")
	if err != nil || d.ID != "dst_2" || posts != 1 {
		t.Fatalf("a new folder is added once: %+v %v (posts=%d)", d, err, posts)
	}
}
