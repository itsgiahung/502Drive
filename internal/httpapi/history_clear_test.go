package httpapi

import "testing"

func TestClearHistoryPreviewsFirstAndValidates(t *testing.T) {
	h := newHarness(t, false)
	h.setup()
	plan := h.batchPlan(0, "")
	adm := h.do("POST", "/v1/jobs", map[string]any{"plan_id": plan["id"], "expected_revision": plan["revision"]}, "hist-1", true)
	if adm.status != 202 {
		t.Fatalf("admit = %d %s", adm.status, adm.body)
	}
	id := h.data(adm)["id"].(string)

	// Nothing says dry_run: it is a preview, and a queued job is never cleared.
	r := h.do("POST", "/v1/history/clear", map[string]any{"older_than_days": 0}, "", true)
	if r.status != 200 {
		t.Fatalf("preview = %d %s", r.status, r.body)
	}
	h.conform("POST", "/v1/history/clear", r)
	if d := h.data(r); d["dry_run"] != true || d["hidden"] != float64(0) {
		t.Fatalf("preview body = %v", d)
	}
	r = h.do("POST", "/v1/history/clear", map[string]any{"job_ids": []string{id, "job_nope"}, "dry_run": false}, "", true)
	if r.status != 200 || h.data(r)["hidden"] != float64(0) || h.data(r)["skipped"] != float64(2) {
		t.Fatalf("an unfinished or unknown job is skipped: %d %s", r.status, r.body)
	}
	if g := h.do("GET", "/v1/jobs/"+id, nil, "", true); g.status != 200 {
		t.Fatalf("job must still be there: %d", g.status)
	}
	for name, body := range map[string]any{
		"neither":  map[string]any{},
		"both":     map[string]any{"older_than_days": 3, "job_ids": []string{"x"}},
		"negative": map[string]any{"older_than_days": -1},
	} {
		if r := h.do("POST", "/v1/history/clear", body, "", true); r.status != 400 {
			t.Fatalf("%s = %d %s, want 400", name, r.status, r.body)
		}
	}
	if r := h.do("POST", "/v1/history/clear", map[string]any{"older_than_days": 1}, "", false); r.status != 401 {
		t.Fatalf("no token = %d", r.status)
	}
}
