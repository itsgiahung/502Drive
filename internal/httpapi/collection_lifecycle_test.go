package httpapi

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestCollectionLifecycleRoutes walks retire -> list -> resume -> retire ->
// forget over HTTP and checks each response against the OpenAPI contract.
func TestCollectionLifecycleRoutes(t *testing.T) {
	h := newHarness(t, true)
	h.setup()
	coll, rev := h.copiedFolder("Lifecycle")
	base := "/v1/collections/" + coll

	// Forgetting a copy that is not retired is refused.
	r := h.do("POST", base+"/forget", map[string]any{"expected_revision": rev}, "forget-early", true)
	if r.status != 409 {
		t.Fatalf("forget while ready = %d %s", r.status, r.body)
	}
	h.conform("POST", "/v1/collections/{collection_id}/forget", r)

	// A background run finishing right after the copy may bump the revision;
	// that is a legitimate stale conflict, so retry with the current one.
	for attempt := range 10 {
		rev = h.collectionRevision(coll)
		r = h.do("POST", base+"/retire", map[string]any{"expected_revision": rev}, fmt.Sprintf("retire-1-%d", attempt), true)
		if r.status != 409 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if r.status != 200 {
		t.Fatalf("retire = %d %s", r.status, r.body)
	}
	h.conform("POST", "/v1/collections/{collection_id}/retire", r)
	retired := h.data(r)
	if retired["state"] != "retired" || retired["retired_at"] == nil {
		t.Fatalf("retired collection = %v", retired)
	}
	rev = int(retired["revision"].(float64))

	// Default library hides retired copies; all=true shows them.
	list := h.do("GET", "/v1/collections", nil, "", true)
	if items, _ := h.data(list)["items"].([]any); len(items) != 0 {
		t.Fatalf("default list still shows retired copy: %v", items)
	}
	all := h.do("GET", "/v1/collections?all=true", nil, "", true)
	if items, _ := h.data(all)["items"].([]any); len(items) != 1 {
		t.Fatalf("all=true list = %v", items)
	}

	// Dry-run cleanup is admitted as a job; a real run needs the exact name.
	r = h.do("POST", base+"/cleanup", map[string]any{"expected_revision": rev, "dry_run": false, "confirm_name": "wrong"}, "clean-wrong", true)
	if r.status != 400 {
		t.Fatalf("cleanup with wrong name = %d %s", r.status, r.body)
	}
	r = h.do("POST", base+"/cleanup", map[string]any{"expected_revision": rev}, "clean-dry", true)
	if r.status != 202 {
		t.Fatalf("dry-run cleanup = %d %s", r.status, r.body)
	}
	h.conform("POST", "/v1/collections/{collection_id}/cleanup", r)
	if h.data(r)["kind"] != "cleanup" || h.data(r)["dry_run"] != true {
		t.Fatalf("cleanup job = %v", h.data(r))
	}
	// Let the dry-run finish so the retire below cannot meet an active job.
	h.waitState(h.data(r)["id"].(string), "succeeded", "failed", "blocked", "cancelled")

	r = h.do("POST", base+"/resume", map[string]any{"expected_revision": rev}, "resume-1", true)
	if r.status != 200 {
		t.Fatalf("resume = %d %s", r.status, r.body)
	}
	h.conform("POST", "/v1/collections/{collection_id}/resume", r)
	if h.data(r)["state"] != "ready" || h.data(r)["retired_at"] != nil {
		t.Fatalf("resumed collection = %v", h.data(r))
	}

	// Forget only after a retire, and only local records go.
	rev = int(h.data(r)["revision"].(float64))
	if r := h.do("POST", base+"/retire", map[string]any{"expected_revision": rev}, "retire-2", true); r.status != 200 {
		t.Fatalf("second retire = %d %s", r.status, r.body)
	}
	rev = h.collectionRevision(coll)
	r = h.do("POST", base+"/forget", map[string]any{"expected_revision": rev}, "forget-1", true)
	if r.status != 200 {
		t.Fatalf("forget = %d %s", r.status, r.body)
	}
	h.conform("POST", "/v1/collections/{collection_id}/forget", r)
	if g := h.do("GET", base, nil, "", true); g.status != 404 {
		t.Fatalf("forgotten collection still readable: %d", g.status)
	}
}

// When the destination already holds a folder with the source's name, the copy
// root becomes "Name (2)". The collection then carries that name, and cleanup
// asks for it (the requested name stays accepted for older collections).
func TestCleanupConfirmsTheNameTheRootHasInDrive(t *testing.T) {
	h := newHarness(t, true)
	dest := h.setup()
	h.fake.Folder("Taken", dest["folder_id"].(string))
	coll, _ := h.copiedFolder("Taken")
	base := "/v1/collections/" + coll

	got := h.data(h.do("GET", base, nil, "", true))
	if got["root_name"] != "Taken (2)" {
		t.Fatalf("root_name = %v, want the name the root has in Drive", got["root_name"])
	}
	if r := h.retireNow(coll, "retire-taken"); r.status != 200 {
		t.Fatalf("retire = %d %s", r.status, r.body)
	}
	rev := h.collectionRevision(coll)
	r := h.do("POST", base+"/cleanup", map[string]any{"expected_revision": rev, "dry_run": false, "confirm_name": "wrong"}, "clean-wrong", true)
	if r.status != 400 || !strings.Contains(string(r.body), "Taken (2)") {
		t.Fatalf("a wrong name must be refused and say which name to type: %d %s", r.status, r.body)
	}
	r = h.do("POST", base+"/cleanup", map[string]any{"expected_revision": rev, "dry_run": false, "confirm_name": "Taken (2)"}, "clean-ok", true)
	if r.status != 202 {
		t.Fatalf("cleanup with the Drive name = %d %s", r.status, r.body)
	}
}
