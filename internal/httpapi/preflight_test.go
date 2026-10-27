package httpapi

import (
	"testing"
	"time"
)

// TestDeepPlanOverHTTP asks for a deep plan, polls it to completion through the
// contract, and checks the deep block and the cancel endpoint on a finished scan.
func TestDeepPlanOverHTTP(t *testing.T) {
	h := newHarness(t, false)
	h.setup()
	src := h.fake.Folder("Deep HTTP", "root")
	h.fake.File("a.txt", src.ID, 10)
	sub := h.fake.Folder("sub", src.ID)
	h.fake.File("b.txt", sub.ID, 20)

	r := h.do("POST", "/v1/plans", map[string]any{"source": src.ID, "deep": true}, "deep-plan-1", true)
	if r.status != 201 {
		t.Fatalf("deep plan = %d %s", r.status, r.body)
	}
	h.conform("POST", "/v1/plans", r)
	id := h.data(r)["id"].(string)

	var deep map[string]any
	for range 2000 {
		g := h.do("GET", "/v1/plans/"+id, nil, "", true)
		if g.status != 200 {
			t.Fatalf("get plan = %d %s", g.status, g.body)
		}
		h.conform("GET", "/v1/plans/{plan_id}", g)
		deep, _ = h.data(g)["preview"].(map[string]any)["deep"].(map[string]any)
		if deep != nil && deep["state"] != "running" {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if deep == nil || deep["state"] != "complete" || deep["complete"] != true {
		t.Fatalf("deep = %v", deep)
	}
	if deep["folders"] != float64(1) || deep["files"] != float64(2) || deep["bytes"] != float64(30) {
		t.Fatalf("counts = %v", deep)
	}

	c := h.do("POST", "/v1/plans/"+id+"/preflight/cancel", nil, "", true)
	if c.status != 200 {
		t.Fatalf("cancel = %d %s", c.status, c.body)
	}
	h.conform("POST", "/v1/plans/{plan_id}/preflight/cancel", c)
	if d := h.data(c)["preview"].(map[string]any)["deep"].(map[string]any); d["state"] != "complete" {
		t.Fatalf("cancel of a finished scan changed it: %v", d)
	}
	if nf := h.do("GET", "/v1/plans/no-such-plan", nil, "", true); nf.status != 404 {
		t.Fatalf("unknown plan = %d %s", nf.status, nf.body)
	}
}
