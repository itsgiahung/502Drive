package httpapi

import (
	"testing"
	"time"

	"github.com/itsgiahung/502Drive/internal/drive"
)

func TestBrowseShowsBreadcrumbChildrenAndMetadata(t *testing.T) {
	h := newHarness(t, false)
	a := h.fake.Folder("Projects", "root")
	b := h.fake.Folder("2026", a.ID)
	f := h.fake.File("report.pdf", b.ID, 1234)
	h.fake.Mutate(f.ID, func(x *drive.File) {
		x.Modified = time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
		x.OwnerName, x.OwnerEmail = "Owner Name", "owner@example.test"
		x.WebLink = "https://drive.google.com/file/d/" + f.ID + "/view"
	})
	h.fake.Folder("Zeta", b.ID)

	r := h.do("GET", "/v1/drive/folders/"+b.ID, nil, "", true)
	if r.status != 200 {
		t.Fatalf("browse = %d %s", r.status, r.body)
	}
	h.conform("GET", "/v1/drive/folders/{id}", r)
	d := h.data(r)
	path := d["path"].([]any)
	if len(path) != 3 || path[0].(map[string]any)["id"] != "root" || path[2].(map[string]any)["name"] != "2026" {
		t.Fatalf("breadcrumb = %v", path)
	}
	items := d["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("items = %d", len(items))
	}
	var file map[string]any
	for _, it := range items {
		if m := it.(map[string]any); m["kind"] == "binary" {
			file = m
		}
	}
	if file == nil || file["size_bytes"].(float64) != 1234 || file["modified_at"] == nil ||
		file["owner"].(map[string]any)["email"] != "owner@example.test" {
		t.Fatalf("file metadata = %v", file)
	}
	// "root" is the alias for My Drive and has no parents.
	if r = h.do("GET", "/v1/drive/folders/root", nil, "", true); r.status != 200 || len(h.data(r)["path"].([]any)) != 1 {
		t.Fatalf("root = %d %s", r.status, r.body)
	}
}

func TestBrowseRefusesFilesAndNeedsAToken(t *testing.T) {
	h := newHarness(t, false)
	f := h.fake.File("a.txt", "root", 1)
	if r := h.do("GET", "/v1/drive/folders/"+f.ID, nil, "", true); r.status != 400 {
		t.Fatalf("a file is not a folder: %d", r.status)
	}
	if r := h.do("GET", "/v1/drive/folders/root", nil, "", false); r.status != 401 {
		t.Fatalf("no token = %d", r.status)
	}
	if r := h.do("GET", "/v1/drive/folders/nope", nil, "", true); r.status != 404 {
		t.Fatalf("missing folder = %d", r.status)
	}
}

func TestBrowsePagesThroughALargeFolder(t *testing.T) {
	h := newHarness(t, false)
	h.fake.PageSize = 3
	for i := 0; i < 7; i++ {
		h.fake.File("f"+string(rune('a'+i)), "root", 1)
	}
	seen, token := 0, ""
	for page := 0; page < 5; page++ {
		path := "/v1/drive/folders/root"
		if token != "" {
			path += "?page_token=" + token
		}
		r := h.do("GET", path, nil, "", true)
		if r.status != 200 {
			t.Fatalf("page %d = %d %s", page, r.status, r.body)
		}
		d := h.data(r)
		seen += len(d["items"].([]any))
		if page > 0 && len(d["path"].([]any)) != 0 {
			t.Fatal("the breadcrumb must only be built for the first page")
		}
		next, _ := d["next_page_token"].(string)
		if next == "" {
			break
		}
		token = next
	}
	if seen != 7 {
		t.Fatalf("saw %d of 7 files across pages", seen)
	}
}

func TestMkdirIsIdempotentAndChecksTheParent(t *testing.T) {
	h := newHarness(t, false)
	parent := h.fake.Folder("Archive", "root")
	body := map[string]any{"parent_id": parent.ID, "name": "New folder"}
	r1 := h.do("POST", "/v1/drive/folders", body, "mk-1", true)
	if r1.status != 201 {
		t.Fatalf("mkdir = %d %s", r1.status, r1.body)
	}
	h.conform("POST", "/v1/drive/folders", r1)
	r2 := h.do("POST", "/v1/drive/folders", body, "mk-1", true)
	if r2.status != 201 || h.data(r2)["id"] != h.data(r1)["id"] || r2.header.Get("Idempotency-Replayed") != "true" {
		t.Fatalf("a retry must return the same folder: %d %s", r2.status, r2.body)
	}
	if n := len(h.fake.Children(parent.ID)); n != 1 {
		t.Fatalf("the parent has %d children, want exactly 1", n)
	}
	for name, tc := range map[string]struct {
		body map[string]any
		key  string
		want int
	}{
		"no key":         {map[string]any{"parent_id": parent.ID, "name": "x"}, "", 400},
		"empty name":     {map[string]any{"parent_id": parent.ID, "name": ""}, "k", 400},
		"padded name":    {map[string]any{"parent_id": parent.ID, "name": " x "}, "k", 400},
		"parent missing": {map[string]any{"parent_id": "nope", "name": "x"}, "k", 404},
		"unknown field":  {map[string]any{"parent_id": parent.ID, "name": "x", "extra": 1}, "k", 400},
	} {
		if r := h.do("POST", "/v1/drive/folders", tc.body, tc.key, true); r.status != tc.want {
			t.Errorf("%s: status %d, want %d (%s)", name, r.status, tc.want, r.body)
		}
	}
	ro := h.fake.Folder("Read only", "root")
	h.fake.Mutate(ro.ID, func(x *drive.File) { x.CanAddChildren = false })
	if r := h.do("POST", "/v1/drive/folders", map[string]any{"parent_id": ro.ID, "name": "x"}, "k2", true); r.status != 409 {
		t.Fatalf("a read-only parent = %d", r.status)
	}
}

func TestFileInfoReturnsOneItem(t *testing.T) {
	h := newHarness(t, false)
	f := h.fake.File("doc.txt", "root", 42)
	r := h.do("GET", "/v1/drive/files/"+f.ID, nil, "", true)
	if r.status != 200 || h.data(r)["size_bytes"].(float64) != 42 {
		t.Fatalf("file info = %d %s", r.status, r.body)
	}
	h.conform("GET", "/v1/drive/files/{id}", r)
}
