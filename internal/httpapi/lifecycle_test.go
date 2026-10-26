package httpapi

import (
	"encoding/json"
	"sync/atomic"
	"testing"

	"github.com/itsgiahung/502Drive/internal/app"
)

func TestStatusReportsTheLifecycle(t *testing.T) {
	h := newHarness(t, false)
	h.svc.Lifecycle = func() app.Lifecycle {
		return app.Lifecycle{State: "sleeping", Online: false, LastGapSeconds: 3600}
	}
	r := h.do("GET", "/v1/status", nil, "", true)
	if r.status != 200 {
		t.Fatalf("status = %d (%s)", r.status, r.body)
	}
	h.conform("GET", "/v1/status", r)
	var body struct {
		Data struct {
			Lifecycle struct {
				State          string `json:"state"`
				Online         bool   `json:"online"`
				LastGapSeconds int    `json:"last_gap_seconds"`
			} `json:"lifecycle"`
		} `json:"data"`
	}
	if err := json.Unmarshal(r.body, &body); err != nil {
		t.Fatal(err)
	}
	got := body.Data.Lifecycle
	if got.State != "sleeping" || got.Online || got.LastGapSeconds != 3600 {
		t.Fatalf("lifecycle = %+v", got)
	}
}

func TestSleepingRefusesNewWorkButKeepsReads(t *testing.T) {
	h := newHarness(t, false)
	var sleeping atomic.Bool
	sleeping.Store(true)
	h.srv.Sleeping = sleeping.Load

	for _, c := range []struct {
		method, path string
		body         any
	}{
		{"POST", "/v1/plans", map[string]any{"source": "https://drive.google.com/drive/folders/" + h.fake.Folder("S", "root").ID}},
		{"POST", "/v1/jobs", map[string]any{"plan_id": "plan_x", "expected_revision": 1}},
	} {
		r := h.do(c.method, c.path, c.body, "sleep-key", true)
		errObj, _ := r.data["error"].(map[string]any)
		if r.status != 503 || errObj["code"] != "suspending" {
			t.Fatalf("%s %s while sleeping = %d %v (%s)", c.method, c.path, r.status, errObj, r.body)
		}
	}
	// Reads keep working: the tray and the CLI still need the truth.
	if r := h.do("GET", "/v1/status", nil, "", true); r.status != 200 {
		t.Fatalf("status while sleeping = %d", r.status)
	}
	sleeping.Store(false)
	src := h.fake.Folder("S2", "root")
	if r := h.do("POST", "/v1/plans", map[string]any{"source": src.ID}, "awake-key", true); r.status != 201 {
		t.Fatalf("plan after waking = %d (%s)", r.status, r.body)
	}
}
