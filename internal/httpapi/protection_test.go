package httpapi

import "testing"

func TestProtectionOptionsAreRevisionGuardedAndDefaultOff(t *testing.T) {
	h := newHarness(t, true)
	h.setup()
	id, _ := h.copiedFolder("Protected")
	path := "/v1/collections/" + id + "/protection"
	r := h.do("GET", path, nil, "", true)
	if r.status != 200 {
		t.Fatalf("get protection = %d %s", r.status, r.body)
	}
	d := h.data(r)
	opts := d["options"].(map[string]any)
	if opts["mirror"] != false || opts["snapshots"] != false {
		t.Fatalf("defaults = %v", opts)
	}
	body := map[string]any{"expected_revision": d["revision"], "options": map[string]any{"mirror": true, "snapshots": true, "keep_days": 30, "keep_count": 10}}
	r = h.do("PUT", path, body, "protect", true)
	if r.status != 200 {
		t.Fatalf("put protection = %d %s", r.status, r.body)
	}
	h.conform("PUT", "/v1/collections/{collection_id}/protection", r)
	scheduled := h.do("PUT", "/v1/collections/"+id+"/sync", map[string]any{
		"mode": "scheduled", "content_policy": "versioned_copy",
		"schedule":                     map[string]any{"kind": "daily", "timezone": "Asia/Ho_Chi_Minh", "local_time": "22:00"},
		"expected_collection_revision": h.data(r)["revision"], "expected_subscription_revision": nil,
	}, "sync-protected", true)
	if scheduled.status != 200 || h.data(scheduled)["deletion_policy"] != "mirror_owned" {
		t.Fatalf("mirror subscription = %d %s", scheduled.status, scheduled.body)
	}
	h.conform("PUT", "/v1/collections/{collection_id}/sync", scheduled)
	if r = h.do("PUT", path, body, "stale", true); r.status != 409 {
		t.Fatalf("stale = %d %s", r.status, r.body)
	}
	if r = h.do("GET", path, nil, "", false); r.status != 401 {
		t.Fatalf("no auth = %d", r.status)
	}
}

func TestSnapshotRoutesDefaultPreviewAndConform(t *testing.T) {
	h := newHarness(t, true)
	h.setup()
	col, rev := h.copiedFolder("Snapshot routes")
	create := h.do("POST", "/v1/collections/"+col+"/snapshots", map[string]any{"expected_revision": rev}, "snap", true)
	if create.status != 202 {
		t.Fatalf("capture %d %s", create.status, create.body)
	}
	h.conform("POST", "/v1/collections/{collection_id}/snapshots", create)
	job := h.data(create)["id"].(string)
	h.waitState(job, "succeeded")
	page := h.do("GET", "/v1/collections/"+col+"/snapshots?limit=1", nil, "", true)
	h.conform("GET", "/v1/collections/{collection_id}/snapshots", page)
	items := h.data(page)["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("snapshots %s", page.body)
	}
	snap := items[0].(map[string]any)
	id := snap["id"].(string)
	get := h.do("GET", "/v1/snapshots/"+id, nil, "", true)
	h.conform("GET", "/v1/snapshots/{snapshot_id}", get)
	mapping := h.do("GET", "/v1/snapshots/"+id+"/items?limit=1", nil, "", true)
	h.conform("GET", "/v1/snapshots/{snapshot_id}/items", mapping)
	pinned := h.do("POST", "/v1/snapshots/"+id+"/pin", map[string]any{"expected_revision": snap["revision"]}, "pin", true)
	h.conform("POST", "/v1/snapshots/{snapshot_id}/pin", pinned)
	unpinned := h.do("DELETE", "/v1/snapshots/"+id+"/pin", map[string]any{"expected_revision": h.data(pinned)["revision"]}, "unpin", true)
	h.conform("DELETE", "/v1/snapshots/{snapshot_id}/pin", unpinned)
	before := h.fake.Writes
	preview := h.do("POST", "/v1/snapshots/"+id+"/restore", map[string]any{"expected_revision": h.data(unpinned)["revision"]}, "preview", true)
	h.conform("POST", "/v1/snapshots/{snapshot_id}/restore", preview)
	previewID := h.data(preview)["id"].(string)
	h.waitState(previewID, "succeeded")
	final := h.data(h.do("GET", "/v1/jobs/"+previewID, nil, "", true))
	if final["counts"].(map[string]any)["total_items"] == nil {
		t.Fatalf("restore plan count lost: %v", final["counts"])
	}
	if h.fake.Writes != before {
		t.Fatal("default preview wrote Drive")
	}
	plan := h.do("GET", "/v1/jobs/"+previewID+"/restore-plan?limit=1", nil, "", true)
	h.conform("GET", "/v1/jobs/{job_id}/restore-plan", plan)
	next := h.data(plan)["next_cursor"]
	if next != nil {
		more := h.do("GET", "/v1/jobs/"+previewID+"/restore-plan?cursor="+next.(string), nil, "", true)
		if more.status != 200 {
			t.Fatalf("cursor %s", more.body)
		}
	}
}
