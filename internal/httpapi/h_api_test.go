package httpapi

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/itsgiahung/502Drive/internal/drive"
	"github.com/itsgiahung/502Drive/internal/state"
)

// submitCopy creates one source tree and submits it, returning the job id.
func (h *harness) submitCopy(name, key string) (string, string) {
	h.t.Helper()
	src := h.fake.Folder(name, "root")
	h.fake.File(name+".txt", src.ID, 3)
	r := h.do("POST", "/v1/plans", map[string]any{"source": src.ID}, key+"-p", true)
	if r.status != 201 {
		h.t.Fatalf("plan %s: %d %s", name, r.status, r.body)
	}
	plan := h.data(r)
	r = h.do("POST", "/v1/jobs", map[string]any{"plan_id": plan["id"], "expected_revision": plan["revision"]}, key, true)
	if r.status != 202 {
		h.t.Fatalf("submit %s: %d %s", name, r.status, r.body)
	}
	j := h.data(r)
	return j["id"].(string), j["collection_id"].(string)
}

func TestJobListFiltersAndCursorBinding(t *testing.T) {
	h := newHarness(t, false)
	h.setup()
	first, firstCol := h.submitCopy("One", "j1")
	_, secondCol := h.submitCopy("Two", "j2")

	r := h.do("GET", "/v1/jobs?collection_id="+url.QueryEscape(firstCol), nil, "", true)
	h.conform("GET", "/v1/jobs", r)
	items := h.data(r)["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["id"] != first {
		t.Fatalf("collection filter = %s", r.body)
	}
	r = h.do("GET", "/v1/jobs?collection_id="+url.QueryEscape(secondCol)+"&state=queued", nil, "", true)
	if len(h.data(r)["items"].([]any)) != 1 {
		t.Fatalf("collection+state filter = %s", r.body)
	}
	// A cursor issued for one filter must not silently page another.
	r = h.do("GET", "/v1/jobs?limit=1", nil, "", true)
	next := h.data(r)["next_cursor"]
	if next == nil {
		t.Fatal("expected a next cursor")
	}
	r = h.do("GET", "/v1/jobs?collection_id="+url.QueryEscape(firstCol)+"&cursor="+url.QueryEscape(next.(string)), nil, "", true)
	if r.status != 400 {
		t.Fatalf("cursor reused with another filter = %d %s", r.status, r.body)
	}
	r = h.do("GET", "/v1/jobs?collection_id="+url.QueryEscape(strings.Repeat("x", 65)), nil, "", true)
	if r.status != 400 {
		t.Fatalf("oversize collection_id = %d", r.status)
	}
}

func TestCollectionListIncludeRetired(t *testing.T) {
	h := newHarness(t, false)
	h.setup()
	jobID, colID := h.submitCopy("Retire me", "j1")
	// Cancel the baseline so retirement is allowed, then retire through store.
	j, err := h.store.GetJob(context.Background(), "owner", jobID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = h.store.Action(context.Background(), "owner", "cancel", j.ID, "cancel", j.Revision); err != nil {
		t.Fatal(err)
	}
	col, err := h.store.GetCollection(context.Background(), "owner", colID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = h.store.RetireCollection(context.Background(), "owner", "retire", colID, col.Revision); err != nil {
		t.Fatal(err)
	}

	r := h.do("GET", "/v1/collections", nil, "", true)
	h.conform("GET", "/v1/collections", r)
	for _, it := range h.data(r)["items"].([]any) {
		if it.(map[string]any)["id"] == colID {
			t.Fatal("retired collection listed by default")
		}
	}
	r = h.do("GET", "/v1/collections?include_retired=true", nil, "", true)
	h.conform("GET", "/v1/collections", r)
	found := false
	for _, it := range h.data(r)["items"].([]any) {
		if it.(map[string]any)["id"] == colID {
			found = true
		}
	}
	if !found {
		t.Fatalf("include_retired = %s", r.body)
	}
}

func TestLatestAuditWithRepairableCount(t *testing.T) {
	h := newHarness(t, false)
	dest := h.setup()
	_ = dest
	jobID, colID := h.submitCopy("Audited", "j1")
	ctx := context.Background()
	j, err := h.store.GetJob(ctx, "owner", jobID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = h.store.Action(ctx, "owner", "cancel", j.ID, "cancel", j.Revision); err != nil {
		t.Fatal(err)
	}
	r := h.do("GET", "/v1/collections/"+url.PathEscape(colID)+"/audits/latest", nil, "", true)
	if r.status != 404 {
		t.Fatalf("no audit yet = %d %s", r.status, r.body)
	}
	a, _, _, err := h.store.AdmitAudit(ctx, "owner", "owner", "audit-key", colID)
	if err != nil {
		t.Fatal(err)
	}
	if err = h.store.RecordFindings(ctx, a.ID, []state.Finding{
		{Kind: state.FindingMissing, SourceID: "s1", Repairable: true},
		{Kind: state.FindingExtra, DestID: "d1"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = h.store.CompleteAudit(ctx, a.ID, state.AuditCompleted, map[string]int{"missing_at_destination": 1}, nil); err != nil {
		t.Fatal(err)
	}
	r = h.do("GET", "/v1/collections/"+url.PathEscape(colID)+"/audits/latest", nil, "", true)
	if r.status != 200 {
		t.Fatalf("latest audit = %d %s", r.status, r.body)
	}
	h.conform("GET", "/v1/collections/{collection_id}/audits/latest", r)
	if r.data["data"].(map[string]any)["repairable_findings"] != float64(1) {
		t.Fatalf("repairable_findings = %s", r.body)
	}
}

func TestDriveSearchIsBoundedAndExcludesTrash(t *testing.T) {
	h := newHarness(t, false)
	for i := range 25 {
		h.fake.File("report-"+string(rune('a'+i))+".txt", "root", 1)
	}
	trashed := h.fake.File("report-trashed.txt", "root", 1)
	h.fake.Mutate(trashed.ID, func(f *drive.File) { f.Trashed = true })

	r := h.do("GET", "/v1/drive/search?q=report", nil, "", true)
	if r.status != 200 {
		t.Fatalf("search = %d %s", r.status, r.body)
	}
	h.conform("GET", "/v1/drive/search", r)
	items := h.data(r)["items"].([]any)
	if len(items) != 20 {
		t.Fatalf("search returned %d, want 20", len(items))
	}
	for _, it := range items {
		if it.(map[string]any)["name"] == "report-trashed.txt" {
			t.Fatal("search returned a trashed file")
		}
	}
	if r = h.do("GET", "/v1/drive/search?q=", nil, "", true); r.status != 400 {
		t.Fatalf("empty q = %d", r.status)
	}
	// A folded query still matches.
	r = h.do("GET", "/v1/drive/search?q=REPORT-A", nil, "", true)
	if r.status != 200 || len(h.data(r)["items"].([]any)) == 0 {
		t.Fatalf("case-insensitive search = %d %s", r.status, r.body)
	}
}

func TestJobAcceptsAfterCopySyncMode(t *testing.T) {
	h := newHarness(t, false)
	h.setup()
	src := h.fake.Folder("Sync", "root")
	r := h.do("POST", "/v1/plans", map[string]any{"source": src.ID}, "p-sync", true)
	if r.status != 201 {
		t.Fatalf("plan %d %s", r.status, r.body)
	}
	plan := h.data(r)
	r = h.do("POST", "/v1/jobs", map[string]any{"plan_id": plan["id"], "expected_revision": plan["revision"], "after_copy_sync_mode": "daily"}, "j-sync", true)
	if r.status != 202 {
		t.Fatalf("submit %d %s", r.status, r.body)
	}
	h.conform("POST", "/v1/jobs", r)
	if h.data(r)["after_copy_sync_mode"] != "daily" {
		t.Fatalf("job = %s", r.body)
	}
	// Invalid values are refused before admission.
	src2 := h.fake.Folder("Sync 2", "root")
	r = h.do("POST", "/v1/plans", map[string]any{"source": src2.ID}, "p-sync2", true)
	plan = h.data(r)
	r = h.do("POST", "/v1/jobs", map[string]any{"plan_id": plan["id"], "expected_revision": plan["revision"], "after_copy_sync_mode": "weekly"}, "j-sync2", true)
	if r.status != 400 {
		t.Fatalf("invalid mode = %d %s", r.status, r.body)
	}
}

func TestHistoryPagesAndBindsCursorToWindow(t *testing.T) {
	h := newHarness(t, false)
	h.setup()
	ids := map[string]bool{}
	for i, name := range []string{"One", "Two", "Three"} {
		id, _ := h.submitCopy(name, "hist"+string(rune('a'+i)))
		ids[id] = true
	}
	// Without a window every submitted job is visible, newest first.
	seen := map[string]bool{}
	path := "/v1/history?limit=1"
	for page := 0; page < 10; page++ {
		r := h.do("GET", path, nil, "", true)
		if r.status != 200 {
			t.Fatalf("history page %d = %d %s", page, r.status, r.body)
		}
		h.conform("GET", "/v1/history", r)
		items := h.data(r)["items"].([]any)
		if len(items) != 1 {
			t.Fatalf("page %d items = %d", page, len(items))
		}
		id := items[0].(map[string]any)["id"].(string)
		if !ids[id] {
			t.Fatalf("history returned an unknown job %q", id)
		}
		if seen[id] {
			t.Fatalf("history repeated job %q across pages", id)
		}
		seen[id] = true
		next, _ := h.data(r)["next_cursor"].(string)
		if next == "" {
			break
		}
		path = "/v1/history?limit=1&cursor=" + url.QueryEscape(next)
	}
	if len(seen) != len(ids) {
		t.Fatalf("history pages covered %d of %d jobs", len(seen), len(ids))
	}

	// A cursor is bound to its window: changing since must be a 400, not a
	// silently different page.
	r := h.do("GET", "/v1/history?limit=1", nil, "", true)
	next := h.data(r)["next_cursor"].(string)
	r = h.do("GET", "/v1/history?limit=1&since=2000-01-01&cursor="+url.QueryEscape(next), nil, "", true)
	if r.status != 400 {
		t.Fatalf("cursor reused with another window = %d %s", r.status, r.body)
	}
	// The future window is empty.
	r = h.do("GET", "/v1/history?since=2999-01-01T00:00:00Z", nil, "", true)
	if r.status != 200 || len(h.data(r)["items"].([]any)) != 0 || h.data(r)["next_cursor"] != nil {
		t.Fatalf("future window = %d %s", r.status, r.body)
	}
	r = h.do("GET", "/v1/history?since=yesterday", nil, "", true)
	if r.status != 400 {
		t.Fatalf("bad since = %d %s", r.status, r.body)
	}
}
