package httpapi

import "testing"

func TestJobsMultipleStatesAndInvalidMember(t *testing.T) {
	h := newHarness(t, false)
	h.setup()
	plan := h.batchPlan(1, "multi-state")
	r := h.do("POST", "/v1/jobs", map[string]any{"plan_id": plan["id"], "expected_revision": plan["revision"]}, "multi-state-job", true)
	if r.status != 202 {
		t.Fatal(string(r.body))
	}
	r = h.do("GET", "/v1/jobs?state=queued,failed", nil, "", true)
	if r.status != 200 {
		t.Fatalf("multiple states: %d %s", r.status, r.body)
	}
	if len(h.data(r)["items"].([]any)) != 1 {
		t.Fatal(h.data(r))
	}
	for _, q := range []string{"queued,nope", "queued,", ",failed"} {
		r = h.do("GET", "/v1/jobs?state="+q, nil, "", true)
		if r.status != 400 || r.data["error"].(map[string]any)["code"] != "invalid_request" {
			t.Fatal(r)
		}
	}
}
