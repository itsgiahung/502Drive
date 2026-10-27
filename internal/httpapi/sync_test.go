package httpapi

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/schedule"
	"github.com/itsgiahung/502Drive/internal/state"
)

// copiedFolder copies a small folder through the API and returns the
// collection id and revision once the baseline succeeded.
func (h *harness) copiedFolder(name string) (string, int) {
	h.t.Helper()
	key := strings.ReplaceAll(name, " ", "-")
	src := h.fake.Folder(name, "root")
	h.fake.File("a.txt", src.ID, 3)
	plan := h.data(h.do("POST", "/v1/plans", map[string]any{"source": src.ID}, "plan-"+key, true))
	r := h.do("POST", "/v1/jobs", map[string]any{"plan_id": plan["id"], "expected_revision": plan["revision"]}, "job-"+key, true)
	if r.status != 202 {
		h.t.Fatalf("submit: %d %s", r.status, r.body)
	}
	done := h.waitState(h.data(r)["id"].(string), "succeeded", "partial", "failed", "blocked")
	if done["state"] != "succeeded" {
		h.t.Fatalf("baseline %+v", done)
	}
	id := done["collection_id"].(string)
	get := h.do("GET", "/v1/collections/"+id, nil, "", true)
	if get.status != 200 {
		h.t.Fatalf("collection: %d %s", get.status, get.body)
	}
	h.conform("GET", "/v1/collections/{collection_id}", get)
	return id, int(h.data(get)["revision"].(float64))
}

// runNow retries with a fresh revision and key: a run finishing in between is
// a legitimate stale conflict and the request was not applied.
func (h *harness) runNow(collectionID, key string) resp {
	h.t.Helper()
	for attempt := range 10 {
		rev := h.collectionRevision(collectionID)
		r := h.do("POST", "/v1/collections/"+collectionID+"/runs", map[string]any{"expected_revision": rev},
			fmt.Sprintf("%s-%d", key, attempt), true)
		if r.status != 409 {
			return r
		}
		time.Sleep(5 * time.Millisecond)
	}
	h.t.Fatal("run now kept conflicting")
	return resp{}
}

// retireNow retires a collection, retrying with the current revision: a run or
// bookkeeping step finishing in between is a legitimate stale conflict and the
// request was not applied.
func (h *harness) retireNow(collectionID, key string) resp {
	h.t.Helper()
	for attempt := range 10 {
		rev := h.collectionRevision(collectionID)
		r := h.do("POST", "/v1/collections/"+collectionID+"/retire", map[string]any{"expected_revision": rev},
			fmt.Sprintf("%s-%d", key, attempt), true)
		if r.status != 409 {
			return r
		}
		time.Sleep(5 * time.Millisecond)
	}
	h.t.Fatal("retire kept conflicting")
	return resp{}
}

// collectionRevision reads the current revision; runs may have changed it.
func (h *harness) collectionRevision(id string) int {
	h.t.Helper()
	r := h.do("GET", "/v1/collections/"+id, nil, "", true)
	if r.status != 200 {
		h.t.Fatalf("collection: %d %s", r.status, r.body)
	}
	return int(h.data(r)["revision"].(float64))
}

func TestSyncRoutesConformToOpenAPI(t *testing.T) {
	h := newHarness(t, true)
	h.setup()
	coll, revision := h.copiedFolder("Syncable")
	path := "/v1/collections/" + coll + "/sync"

	before := h.do("GET", "/v1/collections/"+coll, nil, "", true)
	if h.data(before)["sync"] != nil {
		t.Fatalf("a fresh collection has sync %v", h.data(before)["sync"])
	}

	scheduled := h.do("PUT", path, map[string]any{
		"mode": "scheduled", "content_policy": "versioned_copy",
		"schedule":                       map[string]any{"kind": "daily", "timezone": "Asia/Ho_Chi_Minh", "local_time": "22:00"},
		"expected_collection_revision":   revision,
		"expected_subscription_revision": nil,
	}, "sync-1", true)
	if scheduled.status != 200 {
		t.Fatalf("configure: %d %s", scheduled.status, scheduled.body)
	}
	h.conform("PUT", "/v1/collections/{collection_id}/sync", scheduled)
	sub := h.data(scheduled)
	if sub["mode"] != "scheduled" || sub["state"] != "active" || sub["deletion_policy"] != "preserve_destination" {
		t.Fatalf("subscription = %+v", sub)
	}
	if sub["next_run_at"] == nil || sub["active_job_id"] != nil || sub["content_policy"] != "versioned_copy" {
		t.Fatalf("subscription = %+v", sub)
	}
	schedule := sub["schedule"].(map[string]any)
	if schedule["kind"] != "daily" || schedule["local_time"] != "22:00" || schedule["timezone"] != "Asia/Ho_Chi_Minh" {
		t.Fatalf("schedule = %+v", schedule)
	}

	// Creating again while a subscription exists is a revision conflict.
	again := h.do("PUT", path, map[string]any{
		"mode": "scheduled", "content_policy": "versioned_copy",
		"schedule":                       map[string]any{"kind": "daily", "timezone": "UTC", "local_time": "03:00"},
		"expected_collection_revision":   revision,
		"expected_subscription_revision": nil,
	}, "sync-2", true)
	if again.status != 409 {
		t.Fatalf("re-create: %d %s", again.status, again.body)
	}
	h.conform("PUT", "/v1/collections/{collection_id}/sync", again)

	subRevision := int(sub["revision"].(float64))
	realtime := h.do("PUT", path, map[string]any{
		"mode": "realtime", "content_policy": "replace_copy",
		"expected_collection_revision":   revision,
		"expected_subscription_revision": subRevision,
	}, "sync-3", true)
	if realtime.status != 200 {
		t.Fatalf("realtime: %d %s", realtime.status, realtime.body)
	}
	h.conform("PUT", "/v1/collections/{collection_id}/sync", realtime)
	rt := h.data(realtime)
	if rt["mode"] != "realtime" || rt["schedule"] != nil || rt["content_policy"] != "replace_copy" {
		t.Fatalf("realtime subscription = %+v", rt)
	}
	if s := rt["state"]; s != "bootstrapping" && s != "active" && s != "needs_reconcile" {
		t.Fatalf("realtime state = %v", s)
	}

	// A realtime schedule payload violates the input schema.
	badSchedule := h.do("PUT", path, map[string]any{
		"mode": "realtime", "content_policy": "versioned_copy",
		"schedule":                       map[string]any{"kind": "daily", "timezone": "UTC", "local_time": "09:00"},
		"expected_collection_revision":   revision,
		"expected_subscription_revision": int(rt["revision"].(float64)),
	}, "sync-4", true)
	if badSchedule.status != 400 {
		t.Fatalf("realtime with schedule: %d %s", badSchedule.status, badSchedule.body)
	}

	list := h.do("GET", "/v1/collections?sync_mode=realtime", nil, "", true)
	if list.status != 200 {
		t.Fatalf("list: %d %s", list.status, list.body)
	}
	h.conform("GET", "/v1/collections", list)
	if items := h.data(list)["items"].([]any); len(items) != 1 {
		t.Fatalf("realtime filter returned %d collections", len(items))
	}
	off := h.do("GET", "/v1/collections?sync_mode=off", nil, "", true)
	if items := h.data(off)["items"].([]any); len(items) != 0 {
		t.Fatalf("off filter returned %d collections", len(items))
	}
	scheduledOnly := h.do("GET", "/v1/collections?sync_mode=scheduled", nil, "", true)
	if items := h.data(scheduledOnly)["items"].([]any); len(items) != 0 {
		t.Fatalf("scheduled filter returned %d collections", len(items))
	}

	rev := int(rt["revision"].(float64))
	paused := h.do("POST", path+"/pause", map[string]any{"expected_revision": rev}, "sp-1", true)
	if paused.status != 202 {
		t.Fatalf("pause: %d %s", paused.status, paused.body)
	}
	h.conform("POST", "/v1/collections/{collection_id}/sync/pause", paused)
	if state := h.data(paused)["state"]; state != "paused" {
		t.Fatalf("paused state = %v", state)
	}
	rev = int(h.data(paused)["revision"].(float64))

	resumed := h.do("POST", path+"/resume", map[string]any{"expected_revision": rev}, "sr-1", true)
	if resumed.status != 202 {
		t.Fatalf("resume: %d %s", resumed.status, resumed.body)
	}
	h.conform("POST", "/v1/collections/{collection_id}/sync/resume", resumed)
	if state := h.data(resumed)["state"]; state != "active" {
		t.Fatalf("resumed state = %v", state)
	}
	rev = int(h.data(resumed)["revision"].(float64))

	stopped := h.do("POST", path+"/stop", map[string]any{"expected_revision": rev}, "ss-1", true)
	if stopped.status != 202 {
		t.Fatalf("stop: %d %s", stopped.status, stopped.body)
	}
	h.conform("POST", "/v1/collections/{collection_id}/sync/stop", stopped)
	if s := h.data(stopped)["state"]; s != "stopped" && s != "stopping" {
		t.Fatalf("stopped state = %v", s)
	}
	rev = int(h.data(stopped)["revision"].(float64))

	// A stale revision is refused, so a repeated click cannot stop the next run.
	staleStop := h.do("POST", path+"/stop", map[string]any{"expected_revision": rev - 1}, "ss-2", true)
	if staleStop.status != 409 {
		t.Fatalf("stale stop: %d %s", staleStop.status, staleStop.body)
	}
	h.conform("POST", "/v1/collections/{collection_id}/sync/stop", staleStop)

	// Run now works with sync off (no subscription), and a second request joins
	// the active run instead of queueing another.
	plain, _ := h.copiedFolder("Runs without sync")
	first := h.runNow(plain, "run-1")
	if first.status != 202 || first.header.Get("Location") == "" {
		t.Fatalf("run now: %d %s", first.status, first.body)
	}
	h.conform("POST", "/v1/collections/{collection_id}/runs", first)
	if h.data(first)["collection_id"] != plain {
		t.Fatalf("run belongs to %v", h.data(first)["collection_id"])
	}
	second := h.runNow(plain, "run-2")
	if second.status != 202 {
		t.Fatalf("second run: %d %s", second.status, second.body)
	}
	h.conform("POST", "/v1/collections/{collection_id}/runs", second)
	if h.data(second)["id"] != h.data(first)["id"] {
		t.Fatal("run now admitted a second parallel run")
	}

	// The collection carries the subscription and stays schema-valid.
	withSync := h.do("GET", "/v1/collections/"+coll, nil, "", true)
	if withSync.status != 200 {
		t.Fatalf("collection: %d %s", withSync.status, withSync.body)
	}
	h.conform("GET", "/v1/collections/{collection_id}", withSync)
	if h.data(withSync)["sync"] == nil {
		t.Fatal("collection sync is null after configuring")
	}
}

// maintenanceCollection creates a ready collection directly in the store so
// the maintenance gate can be tested without running the engine.
func (h *harness) maintenanceCollection() (string, int) {
	h.t.Helper()
	ctx := context.Background()
	src := h.fake.Folder("Maintenance", "root")
	plan := state.Plan{ID: state.ID("plan"), Principal: "owner", SourceID: src.ID, SourceName: src.Name, SourceKind: state.KindFolder,
		DestinationID: "dst", DestinationFolder: h.fake.Folder("Archive2", "root").ID, RootName: src.Name,
		ExpiresAt: time.Now().Add(time.Hour), Revision: 1}
	if err := h.store.PutPlan(ctx, plan); err != nil {
		h.t.Fatal(err)
	}
	res, err := h.store.Admit(ctx, "owner", state.ID("admit"), plan.ID, 1)
	if err != nil {
		h.t.Fatal(err)
	}
	if _, err = h.store.UpdateJob(ctx, res.Job.ID, func(j *state.Job) (string, bool) {
		j.State = state.JobSucceeded
		return "finished", true
	}); err != nil {
		h.t.Fatal(err)
	}
	if err = h.store.SetCollectionState(ctx, res.Job.CollectionID, "ready"); err != nil {
		h.t.Fatal(err)
	}
	c, err := h.store.GetCollection(ctx, "", res.Job.CollectionID)
	if err != nil {
		h.t.Fatal(err)
	}
	return c.ID, c.Revision
}

// addCollection inserts a ready collection directly, without running a copy.
func (h *harness) addCollection(name string) (string, int) {
	h.t.Helper()
	ctx := context.Background()
	src := h.fake.Folder(name, "root")
	plan := state.Plan{ID: state.ID("plan"), Principal: "owner", SourceID: src.ID, SourceName: name, SourceKind: state.KindFolder,
		DestinationID: "dst", DestinationFolder: h.fake.Folder("Archive-"+name, "root").ID, RootName: name,
		ExpiresAt: time.Now().Add(time.Hour), Revision: 1}
	if err := h.store.PutPlan(ctx, plan); err != nil {
		h.t.Fatal(err)
	}
	res, err := h.store.Admit(ctx, "owner", state.ID("admit"), plan.ID, 1)
	if err != nil {
		h.t.Fatal(err)
	}
	if _, err = h.store.UpdateJob(ctx, res.Job.ID, func(j *state.Job) (string, bool) {
		j.State = state.JobSucceeded
		return "finished", true
	}); err != nil {
		h.t.Fatal(err)
	}
	if err = h.store.SetCollectionState(ctx, res.Job.CollectionID, "ready"); err != nil {
		h.t.Fatal(err)
	}
	c, err := h.store.GetCollection(ctx, "", res.Job.CollectionID)
	if err != nil {
		h.t.Fatal(err)
	}
	return c.ID, c.Revision
}

func TestCollectionsPagination(t *testing.T) {
	h := newHarness(t, false)
	h.setup()
	names := []string{"Lib One", "Lib Two", "Lib Three", "Lib Four", "Lib Five"}
	ids := map[string]string{}
	revs := map[string]int{}
	for _, name := range names {
		id, rev := h.addCollection(name)
		ids[name], revs[name] = id, rev
	}

	var got []string
	path := "/v1/collections?limit=2"
	for page := 0; page < 5; page++ {
		r := h.do("GET", path, nil, "", true)
		if r.status != 200 {
			t.Fatalf("page %d: %d %s", page, r.status, r.body)
		}
		h.conform("GET", "/v1/collections", r)
		items := h.data(r)["items"].([]any)
		for _, it := range items {
			got = append(got, it.(map[string]any)["source_name"].(string))
		}
		next, _ := h.data(r)["next_cursor"].(string)
		if next == "" {
			break
		}
		path = "/v1/collections?limit=2&cursor=" + url.QueryEscape(next)
	}
	want := []string{"Lib Five", "Lib Four", "Lib Three", "Lib Two", "Lib One"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("order = %v, want %v", got, want)
	}

	// Filters are part of the page and of the cursor binding.
	single := h.do("GET", "/v1/collections?limit=10&query=lib%20three", nil, "", true)
	if items := h.data(single)["items"].([]any); len(items) != 1 || items[0].(map[string]any)["source_name"] != "Lib Three" {
		t.Fatalf("query filter = %v", h.data(single)["items"])
	}
	if h.data(single)["next_cursor"] != nil {
		t.Fatal("a single match returned a cursor")
	}

	if _, _, err := h.svc.ConfigureSync(context.Background(), app.Owner, "pg-sync", ids["Lib Two"], app.SyncRequest{
		Mode: state.ModeScheduled, ContentPolicy: state.PolicyVersioned,
		Schedule:           &schedule.Spec{Kind: schedule.KindDaily, Timezone: "UTC", LocalTime: "22:00"},
		ExpectedCollection: revs["Lib Two"],
	}); err != nil {
		t.Fatalf("configure: %v", err)
	}
	scheduled := h.do("GET", "/v1/collections?sync_mode=scheduled&limit=10", nil, "", true)
	items := h.data(scheduled)["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["id"] != ids["Lib Two"] {
		t.Fatalf("scheduled filter = %v", items)
	}
	off := h.do("GET", "/v1/collections?sync_mode=off&limit=2", nil, "", true)
	if items = h.data(off)["items"].([]any); len(items) != 2 {
		t.Fatalf("off filter page = %d", len(items))
	}
	tail, _ := h.data(off)["next_cursor"].(string)
	if tail == "" {
		t.Fatal("off filter did not paginate")
	}
	// A cursor is bound to its filters: reusing it with another mode fails.
	if r := h.do("GET", "/v1/collections?sync_mode=scheduled&limit=2&cursor="+url.QueryEscape(tail), nil, "", true); r.status != 400 {
		t.Fatalf("cursor reuse across filters = %d %s", r.status, r.body)
	}
	if r := h.do("GET", "/v1/collections?sync_mode=hourly", nil, "", true); r.status != 400 {
		t.Fatalf("invalid sync_mode = %d", r.status)
	}
}

func TestMaintenanceRefusesNewWork(t *testing.T) {
	h := newHarness(t, false)
	h.setup()
	coll, revision := h.maintenanceCollection()
	h.srv.Maintenance = true

	src := h.fake.Folder("Blocked", "root")
	plan := h.data(h.do("POST", "/v1/plans", map[string]any{"source": src.ID}, "mp-1", true))
	submit := h.do("POST", "/v1/jobs", map[string]any{"plan_id": plan["id"], "expected_revision": plan["revision"]}, "mj-1", true)
	if submit.status != 503 {
		t.Fatalf("submit in maintenance: %d %s", submit.status, submit.body)
	}
	h.conform("POST", "/v1/jobs", submit)

	sync := h.do("PUT", "/v1/collections/"+coll+"/sync", map[string]any{
		"mode": "scheduled", "content_policy": "versioned_copy",
		"schedule":                       map[string]any{"kind": "daily", "timezone": "UTC", "local_time": "22:00"},
		"expected_collection_revision":   revision,
		"expected_subscription_revision": nil,
	}, "ms-1", true)
	if sync.status != 503 {
		t.Fatalf("configure in maintenance: %d %s", sync.status, sync.body)
	}
	h.conform("PUT", "/v1/collections/{collection_id}/sync", sync)

	run := h.do("POST", "/v1/collections/"+coll+"/runs", map[string]any{"expected_revision": revision}, "mr-1", true)
	if run.status != 503 {
		t.Fatalf("run now in maintenance: %d %s", run.status, run.body)
	}
	h.conform("POST", "/v1/collections/{collection_id}/runs", run)

	// Read-only endpoints keep working.
	if r := h.do("GET", "/v1/collections/"+coll, nil, "", true); r.status != 200 {
		t.Fatalf("collection read in maintenance: %d %s", r.status, r.body)
	}
	if _, configured, err := h.svc.SyncStatus(context.Background(), coll); err != nil || configured {
		t.Fatalf("maintenance stored a subscription: %v %v", configured, err)
	}
}
