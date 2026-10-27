package httpapi

import (
	"fmt"
	"testing"
)

// batchPlan previews one source folder and returns its plan.
func (h *harness) batchPlan(i int, batchKey string) map[string]any {
	h.t.Helper()
	src := h.fake.Folder(fmt.Sprintf("Batch %d", i), "root")
	key := fmt.Sprintf("%s:%d-plan", batchKey, i)
	r := h.do("POST", "/v1/plans", map[string]any{"source": src.ID}, key, true)
	if r.status != 201 {
		h.t.Fatalf("plan %d = %d %s", i, r.status, r.body)
	}
	return h.data(r)
}

// TestBatchQueuePauseAndMoveOverHTTP walks a batch of three links through
// admission, replay, summary, move, and the global queue pause over HTTP. No
// engine runs, so every job stays queued and the assertions are deterministic.
func TestBatchQueuePauseAndMoveOverHTTP(t *testing.T) {
	h := newHarness(t, false)
	h.setup()
	const batch = "batch-http-0001"
	var jobs, plans []map[string]any
	for i := range 3 {
		plan := h.batchPlan(i, batch)
		plans = append(plans, plan)
		key := fmt.Sprintf("%s:%d", batch, i)
		r := h.do("POST", "/v1/jobs", map[string]any{"plan_id": plan["id"], "expected_revision": plan["revision"], "batch_id": batch}, key, true)
		if r.status != 202 {
			t.Fatalf("admit %d = %d %s", i, r.status, r.body)
		}
		h.conform("POST", "/v1/jobs", r)
		jobs = append(jobs, h.data(r))
	}
	if jobs[0]["batch_id"] != batch || jobs[0]["priority"] != float64(0) {
		t.Fatalf("job body = %v", jobs[0])
	}

	// A lost response is replayed with the same key: same job, no new one.
	plan := plans[0]
	again := h.do("POST", "/v1/jobs", map[string]any{"plan_id": plan["id"], "expected_revision": plan["revision"], "batch_id": batch}, batch+":0", true)
	if again.status != 202 || h.data(again)["id"] != jobs[0]["id"] {
		t.Fatalf("replay = %d %s, want job %v", again.status, again.body, jobs[0]["id"])
	}

	// A malformed batch id is refused before anything is admitted.
	bad := h.do("POST", "/v1/jobs", map[string]any{"plan_id": plan["id"], "expected_revision": plan["revision"], "batch_id": "x"}, "bad-batch", true)
	if bad.status != 400 {
		t.Fatalf("bad batch id = %d %s", bad.status, bad.body)
	}

	sum := h.do("GET", "/v1/batches/"+batch, nil, "", true)
	if sum.status != 200 {
		t.Fatalf("batch = %d %s", sum.status, sum.body)
	}
	h.conform("GET", "/v1/batches/{batch_id}", sum)
	if got := h.data(sum); got["total"] != float64(3) {
		t.Fatalf("batch summary = %v", got)
	}

	// Move the third link before the first; a stale revision is refused.
	third := jobs[2]
	id := third["id"].(string)
	rev := int(third["revision"].(float64))
	if r := h.do("POST", "/v1/jobs/"+id+"/move", map[string]any{"expected_revision": rev + 5, "before": jobs[0]["id"]}, "", true); r.status != 409 {
		t.Fatalf("stale move = %d %s", r.status, r.body)
	}
	both := map[string]any{"expected_revision": rev, "before": jobs[0]["id"], "after": jobs[1]["id"]}
	if r := h.do("POST", "/v1/jobs/"+id+"/move", both, "", true); r.status != 400 {
		t.Fatalf("before+after = %d %s", r.status, r.body)
	}
	r := h.do("POST", "/v1/jobs/"+id+"/move", map[string]any{"expected_revision": rev, "before": jobs[0]["id"]}, "", true)
	if r.status != 200 {
		t.Fatalf("move = %d %s", r.status, r.body)
	}
	h.conform("POST", "/v1/jobs/{job_id}/move", r)
	if int(h.data(r)["revision"].(float64)) != rev+1 {
		t.Fatalf("move must bump the revision: %v", h.data(r))
	}

	// Queue pause: owner only, durable, idempotent, visible in status.
	p := h.do("POST", "/v1/queue/pause", nil, "", true)
	if p.status != 200 {
		t.Fatalf("queue pause = %d %s", p.status, p.body)
	}
	h.conform("POST", "/v1/queue/pause", p)
	if pd := p.data["data"].(map[string]any); pd["paused"] != true || pd["changed"] != true {
		t.Fatalf("pause body = %v", pd)
	}
	st := h.do("GET", "/v1/status", nil, "", true)
	h.conform("GET", "/v1/status", st)
	if q := h.data(st)["queue"].(map[string]any); q["paused"] != true {
		t.Fatalf("status queue = %v", q)
	}
	if p2 := h.do("POST", "/v1/queue/pause", nil, "", true); p2.data["data"].(map[string]any)["changed"] != false {
		t.Fatalf("second pause must not change anything: %s", p2.body)
	}
	// A moved, queued job is still queued while the queue is paused.
	j := h.data(h.do("GET", "/v1/jobs/"+id, nil, "", true))
	if j["state"] != "queued" {
		t.Fatalf("job while queue paused = %v", j["state"])
	}

	res := h.do("POST", "/v1/queue/resume", nil, "", true)
	if res.status != 200 || res.data["data"].(map[string]any)["paused"] != false {
		t.Fatalf("queue resume = %d %s", res.status, res.body)
	}
	h.conform("POST", "/v1/queue/resume", res)

	if r := h.do("GET", "/v1/batches/no-such-batch-1", nil, "", true); r.status != 404 {
		t.Fatalf("unknown batch = %d %s", r.status, r.body)
	}
}
