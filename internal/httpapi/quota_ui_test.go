package httpapi

import "testing"

func TestStatusQuotaDoesNotInventALimit(t *testing.T) {
	h := newHarness(t, false)
	h.svc.DailyCopyLimit = 750000000000
	r := h.do("GET", "/v1/status", nil, "", true)
	if r.status != 200 {
		t.Fatal(r)
	}
	q, ok := h.data(r)["quota"].(map[string]any)
	if !ok {
		t.Fatal(h.data(r))
	}
	if q["limit_known"] != false || q["used_24h_bytes"] != float64(0) || q["resume_after"] != nil {
		t.Fatal(q)
	}
	h.conform("GET", "/v1/status", r)
}
