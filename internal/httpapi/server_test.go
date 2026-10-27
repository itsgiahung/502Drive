package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/drive"
	"github.com/itsgiahung/502Drive/internal/drive/drivetest"
	"github.com/itsgiahung/502Drive/internal/engine"
	"github.com/itsgiahung/502Drive/internal/state"
)

const testToken = "test-token-synthetic"

type harness struct {
	t     *testing.T
	ts    *httptest.Server
	store *state.Store
	fake  *drivetest.Fake
	spec  *spec
	svc   *app.Service
	eng   *engine.Engine
	srv   *Server
}

func newHarness(t *testing.T, startEngine bool) *harness {
	t.Helper()
	store, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	fake := drivetest.New()
	eng := &engine.Engine{Store: store, Provider: func(context.Context) (drive.Provider, error) { return fake, nil },
		BackoffBase: time.Millisecond, Settle: time.Millisecond, Poll: 5 * time.Millisecond}
	svc := &app.Service{Store: store, Version: "test", Provider: func(context.Context) (drive.Provider, error) { return fake, nil }, Wake: func() {
		if startEngine {
			eng.Wake()
		}
	}}
	srv := &Server{Svc: svc, TokenHash: HashToken(testToken), WorkerReady: func() bool { return true }}
	// Mirror the daemon: sync bookkeeping runs after every finished run.
	eng.AfterRun = func(id string) {
		next, err := store.RunFinished(context.Background(), id)
		if err == nil && next != nil {
			eng.Wake()
		}
	}
	h := &harness{t: t, ts: httptest.NewServer(srv.Handler()), store: store, fake: fake, spec: loadSpec(t), svc: svc, eng: eng, srv: srv}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	if startEngine {
		go func() { eng.Run(ctx); close(done) }()
	} else {
		close(done)
	}
	t.Cleanup(func() { cancel(); <-done; h.ts.Close(); store.Close() })
	return h
}

type resp struct {
	status int
	header http.Header
	body   []byte
	data   map[string]any
}

func (h *harness) do(method, path string, body any, key string, auth bool) resp {
	h.t.Helper()
	var rd io.Reader
	if body != nil {
		if s, ok := body.(string); ok {
			rd = strings.NewReader(s)
		} else {
			b, _ := json.Marshal(body)
			rd = bytes.NewReader(b)
		}
	}
	req, _ := http.NewRequest(method, h.ts.URL+path, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	if auth {
		req.Header.Set("Authorization", "Bearer "+testToken)
	}
	r, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	out := resp{status: r.StatusCode, header: r.Header, body: b}
	json.Unmarshal(b, &out.data)
	return out
}

// conform fails the test if the response does not match the OpenAPI document.
func (h *harness) conform(method, tmpl string, r resp) {
	h.t.Helper()
	if errs := h.spec.check(method, tmpl, r.status, r.body); len(errs) > 0 {
		h.t.Fatalf("%s %s -> %d violates OpenAPI: %v\n%s", method, tmpl, r.status, errs, r.body)
	}
}

func (h *harness) data(r resp) map[string]any { return r.data["data"].(map[string]any) }

func (h *harness) setup() (dest map[string]any) {
	h.t.Helper()
	folder := h.fake.Folder("Archive", "root")
	r := h.do("POST", "/v1/destinations", map[string]any{"source": folder.ID}, "d1", true)
	if r.status != 201 {
		h.t.Fatalf("destination: %d %s", r.status, r.body)
	}
	h.conform("POST", "/v1/destinations", r)
	d := h.data(r)
	r = h.do("PUT", "/v1/destinations/default", map[string]any{"destination_id": d["id"], "expected_revision": d["revision"]}, "d2", true)
	if r.status != 200 {
		h.t.Fatalf("default: %d %s", r.status, r.body)
	}
	h.conform("PUT", "/v1/destinations/default", r)
	return h.data(r)
}

func (h *harness) waitState(id string, want ...string) map[string]any {
	h.t.Helper()
	for range 800 {
		r := h.do("GET", "/v1/jobs/"+id, nil, "", true)
		h.conform("GET", "/v1/jobs/{job_id}", r)
		j := h.data(r)
		for _, w := range want {
			if j["state"] == w {
				return j
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	h.t.Fatalf("job %s never reached %v", id, want)
	return nil
}

func TestAuthAndInputLimits(t *testing.T) {
	h := newHarness(t, false)
	if r := h.do("GET", "/health/live", nil, "", false); r.status != 200 {
		t.Fatalf("live %d", r.status)
	} else {
		h.conform("GET", "/health/live", r)
	}
	for _, p := range []string{"/v1/status", "/v1/jobs", "/v1/account", "/health/ready"} {
		if r := h.do("GET", p, nil, "", false); r.status != 401 {
			t.Fatalf("%s without token = %d", p, r.status)
		}
	}
	req, _ := http.NewRequest("GET", h.ts.URL+"/v1/status", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	if r, _ := http.DefaultClient.Do(req); r.StatusCode != 401 {
		t.Fatal("wrong token accepted")
	}
	for _, c := range []struct {
		name, body string
		want       int
	}{
		{"unknown field", `{"source":"abcdefghijk","actor_id":"owner"}`, 400},
		{"trailing data", `{"source":"abcdefghijk"} {}`, 400},
		{"oversize", `{"source":"` + strings.Repeat("a", 70000) + `"}`, 413},
		{"bad json", `{`, 400},
	} {
		r := h.do("POST", "/v1/plans", c.body, "k-"+c.name[:3], true)
		if r.status != c.want {
			t.Errorf("%s: got %d want %d", c.name, r.status, c.want)
		}
		h.conform("POST", "/v1/plans", r)
	}
	if r := h.do("GET", "/v1/jobs?cursor=not-a-cursor", nil, "", true); r.status != 400 {
		t.Errorf("bad cursor = %d", r.status)
	}
	if r := h.do("GET", "/v1/jobs?limit=500", nil, "", true); r.status != 400 {
		t.Errorf("limit = %d", r.status)
	}
	r := h.do("POST", "/v1/plans", map[string]any{"source": "abcdefghijk"}, "", true)
	if r.status != 400 {
		t.Errorf("missing idempotency key = %d", r.status)
	}
}

func TestEndToEndCopyConformsToOpenAPI(t *testing.T) {
	h := newHarness(t, true)
	dest := h.setup()
	src := h.fake.Folder("Project", "root")
	h.fake.File("a.txt", src.ID, 4)
	sub := h.fake.Folder("sub", src.ID)
	h.fake.File("b.txt", sub.ID, 6)

	r := h.do("POST", "/v1/plans", map[string]any{"source": "https://drive.google.com/drive/folders/" + src.ID}, "p1", true)
	if r.status != 201 {
		t.Fatalf("plan %d %s", r.status, r.body)
	}
	h.conform("POST", "/v1/plans", r)
	plan := h.data(r)
	if plan["destination_id"] != dest["id"] || plan["root_name"] != "Project" {
		t.Fatalf("plan %+v", plan)
	}
	// replay of the plan request returns the same plan
	r2 := h.do("POST", "/v1/plans", map[string]any{"source": "https://drive.google.com/drive/folders/" + src.ID}, "p1", true)
	if h.data(r2)["id"] != plan["id"] || r2.header.Get("Idempotency-Replayed") != "true" {
		t.Fatal("plan replay")
	}

	in := map[string]any{"plan_id": plan["id"], "expected_revision": plan["revision"]}
	r = h.do("POST", "/v1/jobs", in, "j1", true)
	if r.status != 202 || r.header.Get("Location") == "" {
		t.Fatalf("submit %d %s", r.status, r.body)
	}
	h.conform("POST", "/v1/jobs", r)
	job := h.data(r)
	// a second key on the same consumed plan yields the same job
	r = h.do("POST", "/v1/jobs", in, "j2", true)
	if h.data(r)["id"] != job["id"] {
		t.Fatal("consumed plan produced a second job")
	}
	// same key, different payload
	r = h.do("POST", "/v1/jobs", map[string]any{"plan_id": plan["id"], "expected_revision": 9}, "j1", true)
	if r.status != 409 {
		t.Fatalf("mismatch = %d", r.status)
	}
	h.conform("POST", "/v1/jobs", r)

	done := h.waitState(job["id"].(string), "succeeded", "partial", "failed", "blocked")
	if done["state"] != "succeeded" {
		t.Fatalf("job %+v", done)
	}
	if done["result_url"] == nil {
		t.Fatal("no result link")
	}
	root := h.fake.Children(h.fake.Children("root")[0].ID)
	_ = root

	r = h.do("GET", "/v1/jobs/"+job["id"].(string)+"/events?after_sequence=0&limit=100", nil, "", true)
	h.conform("GET", "/v1/jobs/{job_id}/events", r)
	items := h.data(r)["items"].([]any)
	if len(items) < 3 || items[0].(map[string]any)["kind"] != "admitted" {
		t.Fatalf("events %+v", items)
	}
	last := h.data(r)["next_sequence"]
	r = h.do("GET", fmt.Sprintf("/v1/jobs/%s/events?after_sequence=%v", job["id"], last), nil, "", true)
	if len(h.data(r)["items"].([]any)) != 0 || h.data(r)["next_sequence"] != last {
		t.Fatal("empty poll must keep the cursor")
	}

	for _, c := range []struct{ method, path, tmpl string }{
		{"GET", "/v1/status", "/v1/status"}, {"GET", "/v1/jobs?state=succeeded", "/v1/jobs"}, {"GET", "/v1/destinations", "/v1/destinations"},
		{"GET", "/v1/account", "/v1/account"}, {"GET", "/v1/preferences", "/v1/preferences"}, {"GET", "/v1/operators", "/v1/operators"},
		{"GET", "/v1/doctor", "/v1/doctor"}, {"GET", "/v1/collections", "/v1/collections"}, {"GET", "/health/ready", "/health/ready"},
		{"GET", "/v1/collections/" + done["collection_id"].(string), "/v1/collections/{collection_id}"},
	} {
		r := h.do(c.method, c.path, nil, "", true)
		if r.status != 200 {
			t.Fatalf("%s -> %d %s", c.path, r.status, r.body)
		}
		h.conform(c.method, c.tmpl, r)
	}
}

func TestConcurrentSubmitSameKeyAdmitsOnce(t *testing.T) {
	h := newHarness(t, false)
	h.setup()
	src := h.fake.Folder("S", "root")
	plan := h.data(h.do("POST", "/v1/plans", map[string]any{"source": src.ID}, "p", true))
	var wg sync.WaitGroup
	ids := make(chan string, 10)
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := h.do("POST", "/v1/jobs", map[string]any{"plan_id": plan["id"], "expected_revision": plan["revision"]}, "same", true)
			if r.status != 202 {
				t.Errorf("status %d %s", r.status, r.body)
				return
			}
			ids <- h.data(r)["id"].(string)
		}()
	}
	wg.Wait()
	close(ids)
	seen := map[string]bool{}
	for id := range ids {
		seen[id] = true
	}
	if len(seen) != 1 {
		t.Fatalf("admitted %d jobs", len(seen))
	}
}

func TestPlanRejectsDestinationInsideSourceAndBadInput(t *testing.T) {
	h := newHarness(t, false)
	h.setup()
	// destination nested inside the source
	src := h.fake.Folder("Top", "root")
	inner := h.fake.Folder("Inner", src.ID)
	r := h.do("POST", "/v1/destinations", map[string]any{"source": inner.ID, "name": "Inner dest"}, "d3", true)
	d := h.data(r)
	r = h.do("POST", "/v1/plans", map[string]any{"source": src.ID, "destination_id": d["id"]}, "p", true)
	if r.status != 400 || h.data2(r)["code"] != "destination_inside_source" {
		t.Fatalf("got %d %s", r.status, r.body)
	}
	h.conform("POST", "/v1/plans", r)
	for _, bad := range []map[string]any{{"source": "https://evil.example/file/d/abcdefghij"}, {"source": "abc"}, {"source": src.ID, "root_name": "bad\nname"}} {
		if r := h.do("POST", "/v1/plans", bad, "x"+fmt.Sprint(len(fmt.Sprint(bad))), true); r.status != 400 {
			t.Errorf("%v -> %d", bad, r.status)
		}
	}
	if r := h.do("POST", "/v1/plans", map[string]any{"source": "missingfile123"}, "m", true); r.status != 404 {
		t.Errorf("missing source = %d", r.status)
	}
}

func (h *harness) data2(r resp) map[string]any { return r.data["error"].(map[string]any) }

func TestStaleAndExpiredPlansAreRefused(t *testing.T) {
	h := newHarness(t, false)
	dest := h.setup()
	src := h.fake.Folder("S", "root")
	plan := h.data(h.do("POST", "/v1/plans", map[string]any{"source": src.ID}, "p", true))
	// re-pointing the default bumps the destination revision: the old plan is stale
	h.do("PUT", "/v1/destinations/default", map[string]any{"destination_id": dest["id"], "expected_revision": dest["revision"]}, "bump", true)
	r := h.do("POST", "/v1/jobs", map[string]any{"plan_id": plan["id"], "expected_revision": plan["revision"]}, "j", true)
	if r.status != 409 || h.data2(r)["code"] != "plan_changed" {
		t.Fatalf("stale destination: %d %s", r.status, r.body)
	}
	h.conform("POST", "/v1/jobs", r)
	if r := h.do("POST", "/v1/jobs", map[string]any{"plan_id": "plan_nope", "expected_revision": 1}, "j9", true); r.status != 404 {
		t.Fatalf("unknown plan = %d", r.status)
	}
}

func TestPauseAndCancelThroughAPI(t *testing.T) {
	h := newHarness(t, false) // no worker: the job stays queued
	h.setup()
	src := h.fake.Folder("S", "root")
	plan := h.data(h.do("POST", "/v1/plans", map[string]any{"source": src.ID}, "p", true))
	job := h.data(h.do("POST", "/v1/jobs", map[string]any{"plan_id": plan["id"], "expected_revision": plan["revision"]}, "j", true))
	base := "/v1/jobs/" + job["id"].(string)
	if r := h.do("POST", base+"/pause", map[string]any{"expected_revision": 99}, "a1", true); r.status != 409 {
		t.Fatalf("stale pause = %d", r.status)
	}
	r := h.do("POST", base+"/pause", map[string]any{"expected_revision": job["revision"]}, "a2", true)
	if r.status != 202 || h.data(r)["state"] != "paused" {
		t.Fatalf("pause %d %s", r.status, r.body)
	}
	h.conform("POST", "/v1/jobs/{job_id}/pause", r)
	r = h.do("POST", base+"/pause", map[string]any{"expected_revision": job["revision"]}, "a2", true)
	if r.header.Get("Idempotency-Replayed") != "true" {
		t.Fatal("action replay")
	}
	cur := h.data(h.do("GET", base, nil, "", true))
	r = h.do("POST", base+"/cancel", map[string]any{"expected_revision": cur["revision"]}, "a3", true)
	if h.data(r)["state"] != "cancelled" {
		t.Fatalf("cancel %s", r.body)
	}
	if r := h.do("POST", base+"/resume", map[string]any{"expected_revision": h.data(r)["revision"]}, "a4", true); r.status != 409 {
		t.Fatalf("resume of cancelled = %d", r.status)
	}
}

func TestCursorPaginationBoundToFilters(t *testing.T) {
	h := newHarness(t, false)
	h.setup()
	for i := range 3 {
		src := h.fake.Folder(fmt.Sprintf("S%d", i), "root")
		plan := h.data(h.do("POST", "/v1/plans", map[string]any{"source": src.ID}, fmt.Sprintf("p%d", i), true))
		h.do("POST", "/v1/jobs", map[string]any{"plan_id": plan["id"], "expected_revision": plan["revision"]}, fmt.Sprintf("j%d", i), true)
	}
	r := h.do("GET", "/v1/jobs?limit=2", nil, "", true)
	h.conform("GET", "/v1/jobs", r)
	page := h.data(r)
	if len(page["items"].([]any)) != 2 || page["next_cursor"] == nil {
		t.Fatalf("page %+v", page)
	}
	r = h.do("GET", "/v1/jobs?limit=2&cursor="+url.QueryEscape(page["next_cursor"].(string)), nil, "", true)
	if p2 := h.data(r); len(p2["items"].([]any)) != 1 || p2["next_cursor"] != nil {
		t.Fatalf("page2 %+v", p2)
	}
	r = h.do("GET", "/v1/jobs?limit=2&state=queued&cursor="+url.QueryEscape(page["next_cursor"].(string)), nil, "", true)
	if r.status != 400 {
		t.Fatalf("cursor reused with other filter = %d", r.status)
	}
}

func TestLoginThroughAPIDoesNotReportConnectedBeforeValidation(t *testing.T) {
	h := newHarness(t, false)
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token":"a","refresh_token":"r","token_type":"Bearer","expires_in":3600,"scope":"https://www.googleapis.com/auth/drive"}`))
	}))
	defer tokenSrv.Close()
	cfg := oauth2.Config{ClientID: "synthetic", Scopes: []string{drive.Scope}, Endpoint: oauth2.Endpoint{AuthURL: "https://accounts.google.com/o/oauth2/v2/auth", TokenURL: tokenSrv.URL}}
	reject := true
	auth, err := drive.NewAuth(t.TempDir(), cfg, func(context.Context, string) error {
		if reject {
			return fmt.Errorf("no drive access")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer auth.Close()
	h.svc.Auth = auth

	if st := h.data(h.do("GET", "/v1/account", nil, "", true)); st["status"] != "disconnected" {
		t.Fatalf("account %+v", st)
	}
	login := func() (string, map[string]any) {
		r := h.do("POST", "/v1/account/auth-sessions", map[string]any{}, fmt.Sprint("login", time.Now().UnixNano()), true)
		if r.status != 201 {
			t.Fatalf("start %d %s", r.status, r.body)
		}
		h.conform("POST", "/v1/account/auth-sessions", r)
		s := h.data(r)
		u, _ := url.Parse(s["authorization_url"].(string))
		cb := u.Query().Get("redirect_uri") + "?code=c&state=" + url.QueryEscape(u.Query().Get("state"))
		return cb, s
	}
	cb, s := login()
	resp, _ := http.Get(cb)
	resp.Body.Close()
	if resp.StatusCode == 200 {
		t.Fatal("callback reported success although validation failed")
	}
	r := h.do("GET", "/v1/account/auth-sessions/"+s["id"].(string), nil, "", true)
	h.conform("GET", "/v1/account/auth-sessions/{session_id}", r)
	if h.data(r)["state"] != "failed" {
		t.Fatalf("session %s", r.body)
	}
	if st := h.data(h.do("GET", "/v1/account", nil, "", true)); st["status"] != "disconnected" {
		t.Fatalf("account after failed login %+v", st)
	}
	reject = false
	cb, _ = login()
	resp, _ = http.Get(cb)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("valid callback = %d", resp.StatusCode)
	}
	if st := h.data(h.do("GET", "/v1/account", nil, "", true)); st["status"] != "connected" {
		t.Fatalf("account %+v", st)
	}
	// revoke: Google is unreachable, so the status must say so
	h2 := h.do("POST", "/v1/account/revoke", map[string]any{}, "rev", true)
	h.conform("POST", "/v1/account/revoke", h2)
	if h.data(h2)["status"] != "revoking" && h.data(h2)["status"] != "disconnected" {
		t.Fatalf("revoke %s", h2.body)
	}
}

func TestOperatorsAreManagedByOwnerAndNeverOwners(t *testing.T) {
	h := newHarness(t, false)
	r := h.do("POST", "/v1/operators", map[string]any{"telegram_user_id": 12345}, "o1", true)
	if r.status != 201 {
		t.Fatalf("%d %s", r.status, r.body)
	}
	h.conform("POST", "/v1/operators", r)
	op := h.data(r)
	r2 := h.do("POST", "/v1/operators", map[string]any{"telegram_user_id": 12345}, "o2", true)
	if h.data(r2)["id"] != op["id"] {
		t.Fatal("same telegram user got two principals")
	}
	if r := h.do("POST", "/v1/operators", map[string]any{"telegram_user_id": 0}, "o3", true); r.status != 400 {
		t.Fatalf("zero id = %d", r.status)
	}
	r = h.do("DELETE", "/v1/operators/"+op["id"].(string), nil, "o4", true)
	if r.status != 204 {
		t.Fatalf("delete %d", r.status)
	}
	h.conform("DELETE", "/v1/operators/{principal_id}", r)
	if r := h.do("DELETE", "/v1/operators/owner", nil, "o5", true); r.status != 404 {
		t.Fatalf("owner must not be removable: %d", r.status)
	}
}
