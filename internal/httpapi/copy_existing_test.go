package httpapi

import "testing"

// planFor inspects the source again and returns a fresh plan; each plan can
// be consumed only once, so every attempt below plans anew.
func (h *harness) planFor(source string, key string) map[string]any {
	h.t.Helper()
	r := h.do("POST", "/v1/plans", map[string]any{"source": source}, key, true)
	if r.status != 201 && r.status != 200 {
		h.t.Fatalf("plan: %d %s", r.status, r.body)
	}
	return h.data(r)
}

// TestCopyAgainAsksWhatToDo covers L2: a second copy of the same source to the
// same destination is refused unless the caller says update or new.
func TestCopyAgainAsksWhatToDo(t *testing.T) {
	h := newHarness(t, true)
	h.setup()
	coll, _ := h.copiedFolder("Again")
	get := h.do("GET", "/v1/collections/"+coll, nil, "", true)
	source := h.data(get)["source_id"].(string)

	plan := h.planFor(source, "plan-again-1")
	r := h.do("POST", "/v1/jobs", map[string]any{"plan_id": plan["id"], "expected_revision": plan["revision"]}, "job-again-1", true)
	if r.status != 409 || h.data2(r)["code"] != "already_copied" {
		t.Fatalf("second copy without a mode = %d %s", r.status, r.body)
	}

	plan = h.planFor(source, "plan-again-2")
	r = h.do("POST", "/v1/jobs", map[string]any{"plan_id": plan["id"], "expected_revision": plan["revision"], "mode": "update"}, "job-again-2", true)
	if r.status != 202 {
		t.Fatalf("update = %d %s", r.status, r.body)
	}
	h.conform("POST", "/v1/jobs", r)
	run := h.data(r)
	if run["collection_id"] != coll || run["kind"] == "cleanup" {
		t.Fatalf("update must refresh the same collection, got %v", run)
	}
	h.waitState(run["id"].(string), "succeeded", "partial", "failed", "blocked")

	plan = h.planFor(source, "plan-again-3")
	r = h.do("POST", "/v1/jobs", map[string]any{"plan_id": plan["id"], "expected_revision": plan["revision"], "mode": "new"}, "job-again-3", true)
	if r.status != 202 || h.data(r)["collection_id"] == coll {
		t.Fatalf("new must create another collection: %d %s", r.status, r.body)
	}
	h.waitState(h.data(r)["id"].(string), "succeeded", "partial", "failed", "blocked")

	// update targets the newest copy of this source and destination. Retire it:
	// a retired copy is not refreshed silently.
	newest := h.data(r)["collection_id"].(string)
	if r := h.retireNow(newest, "retire-again"); r.status != 200 {
		t.Fatalf("retire = %d %s", r.status, r.body)
	}
	plan = h.planFor(source, "plan-again-4")
	r = h.do("POST", "/v1/jobs", map[string]any{"plan_id": plan["id"], "expected_revision": plan["revision"], "mode": "update"}, "job-again-4", true)
	if r.status != 409 || h.data2(r)["code"] != "collection_retired" {
		t.Fatalf("update of a retired copy = %d %s", r.status, r.body)
	}
}
