package httpapi

import "testing"

func TestSetupChecklistUsesDoctorAndRequiresAuth(t *testing.T) {
	h := newHarness(t, false)
	if r := h.do("GET", "/v1/setup", nil, "", false); r.status != 401 {
		t.Fatalf("unauthorized: %d", r.status)
	}
	r := h.do("GET", "/v1/setup", nil, "", true)
	if r.status != 200 {
		t.Fatalf("setup: %d %s", r.status, r.body)
	}
	data := r.data["data"].(map[string]any)
	if data["ready"] != false {
		t.Fatal(data)
	}
	checks := data["checks"].([]any)
	if len(checks) != 4 {
		t.Fatal(checks)
	}
	for i, name := range []string{"database", "oauth_client", "account", "destination"} {
		if checks[i].(map[string]any)["name"] != name {
			t.Fatal(checks)
		}
	}
}

func TestJobQueueFieldsAreFromScheduler(t *testing.T) {
	h := newHarness(t, false)
	h.setup()
	plan := h.batchPlan(1, "position-http")
	r := h.do("POST", "/v1/jobs", map[string]any{"plan_id": plan["id"], "expected_revision": plan["revision"]}, "position-job", true)
	id := h.data(r)["id"].(string)
	r = h.do("GET", "/v1/jobs/"+id, nil, "", true)
	data := h.data(r)
	if data["queue_position"] != float64(1) || data["queue_ahead"] != float64(0) {
		t.Fatal(data)
	}
	h.conform("GET", "/v1/jobs/{job_id}", r)
}
