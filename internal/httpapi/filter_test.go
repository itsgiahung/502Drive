package httpapi

import (
	"testing"
)

// The filter rule of a collection is read and replaced through the API, with
// the revision check that stops a stale client from overwriting a newer rule.
func TestCollectionFilterRoutesConformAndRefuseStaleRevisions(t *testing.T) {
	h := newHarness(t, true)
	h.setup()
	id, rev := h.copiedFolder("Filtered Lib")

	// A bad rule is refused with a message, not stored.
	bad := h.do("PUT", "/v1/collections/"+id+"/filter", map[string]any{
		"filter": map[string]any{"exclude": []string{"["}}, "expected_revision": rev}, "flt-bad", true)
	if bad.status != 400 {
		t.Fatalf("bad glob: %d %s", bad.status, bad.body)
	}

	ok := h.do("PUT", "/v1/collections/"+id+"/filter", map[string]any{
		"filter": map[string]any{"exclude": []string{"*.tmp"}, "max_size": 1 << 20}, "expected_revision": rev}, "flt-ok", true)
	if ok.status != 200 {
		t.Fatalf("set filter: %d %s", ok.status, ok.body)
	}
	h.conform("PUT", "/v1/collections/{collection_id}/filter", ok)
	got := h.data(ok)
	if int(got["revision"].(float64)) != rev+1 {
		t.Fatalf("revision = %v, want %d", got["revision"], rev+1)
	}

	// The old revision is stale now: nothing changes.
	stale := h.do("PUT", "/v1/collections/"+id+"/filter", map[string]any{
		"filter": map[string]any{}, "expected_revision": rev}, "flt-stale", true)
	if stale.status != 409 {
		t.Fatalf("stale revision: %d %s", stale.status, stale.body)
	}

	show := h.do("GET", "/v1/collections/"+id+"/filter", nil, "", true)
	if show.status != 200 {
		t.Fatalf("show filter: %d %s", show.status, show.body)
	}
	h.conform("GET", "/v1/collections/{collection_id}/filter", show)
	f := h.data(show)["filter"].(map[string]any)
	if ex := f["exclude"].([]any); len(ex) != 1 || ex[0] != "*.tmp" {
		t.Fatalf("stored filter = %v", f)
	}
}
