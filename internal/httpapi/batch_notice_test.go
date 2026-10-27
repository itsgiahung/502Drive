package httpapi

import "testing"

func TestBatchSealIsScopedAndIdempotent(t *testing.T) {
	h := newHarness(t, false)
	h.setup()
	p := h.batchPlan(1, "seal")
	r := h.do("POST", "/v1/jobs", map[string]any{"plan_id": p["id"], "expected_revision": p["revision"], "batch_id": "batch-seal-test"}, "seal-job", true)
	if r.status != 202 {
		t.Fatal(r)
	}
	for range 2 {
		r = h.do("POST", "/v1/batches/batch-seal-test/seal", nil, "", true)
		if r.status != 200 {
			t.Fatalf("seal: %d %s", r.status, r.body)
		}
		h.conform("POST", "/v1/batches/{batch_id}/seal", r)
	}
	if r = h.do("POST", "/v1/batches/batch-seal-test/seal", nil, "", false); r.status != 401 {
		t.Fatal(r)
	}
}
