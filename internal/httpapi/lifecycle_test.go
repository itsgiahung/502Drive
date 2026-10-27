package httpapi

import (
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

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

func TestStatusReportsTheThrottleState(t *testing.T) {
	h := newHarness(t, false)
	next := time.Date(2026, 10, 27, 12, 0, 30, 0, time.UTC)
	h.svc.Throttle = func() app.ThrottleStatus {
		return app.ThrottleStatus{WorkersNow: 6, ThrottledLast5m: 3, NextRetryAt: &next}
	}
	r := h.do("GET", "/v1/status", nil, "", true)
	if r.status != 200 {
		t.Fatalf("status = %d (%s)", r.status, r.body)
	}
	h.conform("GET", "/v1/status", r)
	thr, ok := r.data["data"].(map[string]any)["throttle"].(map[string]any)
	if !ok {
		t.Fatalf("throttle missing: %s", r.body)
	}
	if thr["workers_now"] != float64(6) || thr["throttled_last_5m"] != float64(3) {
		t.Fatalf("throttle = %v", thr)
	}
	if got, _ := thr["next_retry_at"].(string); got != "2026-10-27T12:00:30Z" {
		t.Fatalf("next_retry_at = %v", thr["next_retry_at"])
	}

	// Not wired (a bare Service) still reports a truthful shape with no hint.
	h.svc.Throttle = nil
	r = h.do("GET", "/v1/status", nil, "", true)
	h.conform("GET", "/v1/status", r)
	thr = r.data["data"].(map[string]any)["throttle"].(map[string]any)
	if thr["workers_now"] != float64(4) || thr["next_retry_at"] != nil {
		t.Fatalf("unwired throttle = %v", thr)
	}
}
