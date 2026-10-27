package cli

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// bulkCLI is driveCLI with a real terminal on both ends and scripted input.
func bulkCLI(t *testing.T, h http.Handler, stdin string, args ...string) (code int, out, errOut string) {
	t.Helper()
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	cfg := filepath.Join(privateDir(t), "config.json")
	if err := os.WriteFile(cfg, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	var o, e strings.Builder
	in := IO{In: strings.NewReader(stdin), Out: &o, Err: &e, StdinTTY: true, StdoutTTY: true, Width: 100,
		Getenv: func(k string) string {
			if k == "DRIVE502_TOKEN" {
				return "tok"
			}
			return ""
		}}
	argv := append([]string{"--config", cfg, "--endpoint", ts.URL}, args...)
	return Execute(t.Context(), argv, in), o.String(), e.String()
}

func jobRow(id, st string, rev int) map[string]any {
	return map[string]any{"id": id, "state": st, "revision": rev, "collection_id": "col_1", "source_name": "src",
		"root_name": "root", "counts": map[string]any{}, "updated_at": "2026-10-27T00:00:00Z"}
}

func TestJobsCancelAllCancelsEveryActiveJobOnce(t *testing.T) {
	var mu sync.Mutex
	posted := []string{}
	revisions := map[string]int{}
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.Method == "GET" && r.URL.Path == "/v1/jobs":
			if r.URL.Query().Get("all") != "true" || r.URL.Query().Get("limit") != "100" {
				t.Errorf("list query = %q", r.URL.RawQuery)
			}
			if r.URL.Query().Get("cursor") == "" {
				next := "5"
				reply(w, 200, map[string]any{"items": []any{
					jobRow("j1", "queued", 1), jobRow("j2", "succeeded", 2),
					jobRow("j3", "blocked", 4), jobRow("j4", "running", 1),
				}, "next_cursor": &next})
				return
			}
			reply(w, 200, map[string]any{"items": []any{jobRow("j5", "paused", 3)}, "next_cursor": nil})
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/v1/jobs/"):
			id := strings.TrimPrefix(r.URL.Path, "/v1/jobs/")
			reply(w, 200, jobRow(id, "running", revisions[id]))
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/cancel"):
			id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v1/jobs/"), "/cancel")
			var body struct {
				Revision int `json:"expected_revision"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			if body.Revision != revisions[id] {
				t.Errorf("%s expected_revision = %d, want %d", id, body.Revision, revisions[id])
			}
			posted = append(posted, id)
			reply(w, 200, jobRow(id, "cancelled", revisions[id]+1))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	})
	revisions = map[string]int{"j1": 1, "j2": 2, "j3": 4, "j4": 1, "j5": 3}
	code, out, errOut := 0, "", ""
	// driveCLI runs with a non-TTY; --yes is the documented path there.
	code, out, errOut = driveCLI(t, h, "jobs", "cancel", "--all", "--yes")
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if got := strings.Join(posted, ","); got != "j1,j3,j4,j5" {
		t.Fatalf("posted = %s", got)
	}
	if !strings.Contains(out, "canceled 4, skipped 0, failed 0") {
		t.Fatalf("out = %q", out)
	}
}

func TestJobsCancelAllRequiresYesWithoutATerminal(t *testing.T) {
	mutations := 0
	code, _, errOut := driveCLI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			mutations++
			t.Errorf("a mutating request was sent without --yes: %s %s", r.Method, r.URL.Path)
		}
		reply(w, 200, map[string]any{"items": []any{jobRow("j1", "running", 1)}, "next_cursor": nil})
	}), "jobs", "cancel", "--all")
	if code != ExitUsage {
		t.Fatalf("exit %d, want %d (%s)", code, ExitUsage, errOut)
	}
	if mutations != 0 {
		t.Fatal("a job was cancelled without consent")
	}
	if !strings.Contains(errOut, "--yes") {
		t.Fatalf("stderr = %q", errOut)
	}
}

func TestJobsCancelAllAsksInATerminal(t *testing.T) {
	for _, tc := range []struct {
		answer     string
		wantCancel bool
	}{{"n\n", false}, {"y\n", true}} {
		posted := 0
		var mu sync.Mutex
		h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == "GET" && r.URL.Path == "/v1/jobs":
				reply(w, 200, map[string]any{"items": []any{jobRow("j1", "running", 1)}, "next_cursor": nil})
			case r.Method == "GET" && r.URL.Path == "/v1/jobs/j1":
				reply(w, 200, jobRow("j1", "running", 1))
			case r.Method == "POST":
				mu.Lock()
				posted++
				mu.Unlock()
				reply(w, 200, jobRow("j1", "cancelled", 2))
			}
		})
		code, out, errOut := bulkCLI(t, h, tc.answer, "jobs", "cancel", "--all")
		if code != ExitOK {
			t.Fatalf("%q: exit %d: %s", tc.answer, code, errOut)
		}
		if tc.wantCancel && posted != 1 {
			t.Fatalf("%q: posted = %d", tc.answer, posted)
		}
		if !tc.wantCancel && posted != 0 {
			t.Fatalf("%q: cancellation happened without consent", tc.answer)
		}
		if !tc.wantCancel && !strings.Contains(out, "aborted") {
			t.Fatalf("%q: out = %q", tc.answer, out)
		}
	}
}

func TestJobsCancelAllCountsSkippedAndFailed(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/v1/jobs":
			reply(w, 200, map[string]any{"items": []any{jobRow("j1", "running", 1), jobRow("j2", "paused", 1)}, "next_cursor": nil})
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/v1/jobs/"):
			id := strings.TrimPrefix(r.URL.Path, "/v1/jobs/")
			reply(w, 200, jobRow(id, "running", 1))
		case strings.HasSuffix(r.URL.Path, "/cancel") && strings.Contains(r.URL.Path, "j1"):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(409)
			json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "stale_revision", "message": "moved"}})
		case strings.HasSuffix(r.URL.Path, "/cancel"):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(500)
			json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "internal_error", "message": "boom"}})
		}
	})
	code, out, _ := driveCLI(t, h, "jobs", "cancel", "--all", "--yes")
	if code != ExitInternal {
		t.Fatalf("exit %d, want %d", code, ExitInternal)
	}
	if !strings.Contains(out, "canceled 0, skipped 1, failed 1") {
		t.Fatalf("out = %q", out)
	}
}

func TestJobsCancelAllFiltersByState(t *testing.T) {
	var listed string
	posted := []string{}
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/v1/jobs":
			listed = r.URL.Query().Get("state")
			reply(w, 200, map[string]any{"items": []any{jobRow("j1", "paused", 1), jobRow("j2", "running", 1)}, "next_cursor": nil})
		case r.Method == "GET":
			reply(w, 200, jobRow("j1", "paused", 1))
		case r.Method == "POST":
			posted = append(posted, r.URL.Path)
			reply(w, 200, jobRow("j1", "cancelled", 2))
		}
	})
	code, _, errOut := driveCLI(t, h, "jobs", "cancel", "--all", "--yes", "--state", "paused")
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if listed != "paused" {
		t.Fatalf("state filter = %q", listed)
	}
	if len(posted) != 1 || !strings.Contains(posted[0], "j1") {
		t.Fatalf("posted = %v", posted)
	}
	// An unknown state is a local usage error, never a daemon call.
	if code, _, _ := driveCLI(t, h, "jobs", "cancel", "--all", "--yes", "--state", "bogus"); code != ExitUsage {
		t.Fatalf("bogus state exit = %d", code)
	}
}

func TestJobsCancelAllPrintsAJSONEnvelope(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/v1/jobs":
			reply(w, 200, map[string]any{"items": []any{jobRow("j1", "queued", 1)}, "next_cursor": nil})
		case r.Method == "GET":
			reply(w, 200, jobRow("j1", "queued", 1))
		default:
			reply(w, 200, jobRow("j1", "cancelled", 2))
		}
	})
	code, out, errOut := driveCLI(t, h, "jobs", "cancel", "--all", "--yes", "--json")
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	var env struct {
		Data struct {
			Canceled []string `json:"canceled"`
			Skipped  []string `json:"skipped"`
			Failed   []any    `json:"failed"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("not an envelope: %v (%q)", err, out)
	}
	if len(env.Data.Canceled) != 1 || env.Data.Canceled[0] != "j1" || env.Data.Skipped == nil {
		t.Fatalf("envelope = %+v", env.Data)
	}
	if strings.Contains(out, "canceled 1") {
		t.Fatal("human output leaked into --json")
	}
}

func TestJobsCancelSingleStillWorks(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			reply(w, 200, jobRow("j9", "running", 7))
			return
		}
		var body struct {
			Revision int `json:"expected_revision"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.Revision != 7 {
			t.Errorf("expected_revision = %d", body.Revision)
		}
		reply(w, 200, jobRow("j9", "cancelled", 8))
	})
	code, out, errOut := driveCLI(t, h, "jobs", "cancel", "j9")
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(out, "cancel requested") || !strings.Contains(out, "j9") {
		t.Fatalf("out = %q", out)
	}
	if code, _, _ := driveCLI(t, h, "jobs", "cancel"); code != ExitUsage {
		t.Fatalf("no-arg cancel exit = %d", code)
	}
	_ = strconv.Itoa
}

// TestJobsCancelAllFollowsOpaqueCursors is finding #20: the router hands out
// base64 cursors bound to the filters, so the CLI must send them back verbatim
// instead of parsing them as numbers and mistaking them for the last page.
func TestJobsCancelAllFollowsOpaqueCursors(t *testing.T) {
	cursor := base64.RawURLEncoding.EncodeToString([]byte("jobs\x1fowner\x1f\x1f100"))
	var mu sync.Mutex
	var cursors []string
	var posted []string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/v1/jobs":
			got := r.URL.Query().Get("cursor")
			mu.Lock()
			cursors = append(cursors, got)
			mu.Unlock()
			if got == "" {
				next := cursor
				reply(w, 200, map[string]any{"items": []any{jobRow("j1", "succeeded", 1)}, "next_cursor": &next})
				return
			}
			reply(w, 200, map[string]any{"items": []any{jobRow("j2", "paused", 2)}, "next_cursor": nil})
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/v1/jobs/"):
			id := strings.TrimPrefix(r.URL.Path, "/v1/jobs/")
			reply(w, 200, jobRow(id, "paused", 2))
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/cancel"):
			id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v1/jobs/"), "/cancel")
			mu.Lock()
			posted = append(posted, id)
			mu.Unlock()
			reply(w, 200, jobRow(id, "cancelled", 3))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	})
	code, out, errOut := driveCLI(t, h, "jobs", "cancel", "--all", "--yes")
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if len(cursors) != 2 || cursors[1] != cursor {
		t.Fatalf("cursors = %v, want the opaque %q sent back unchanged", cursors, cursor)
	}
	if len(posted) != 1 || posted[0] != "j2" {
		t.Fatalf("posted = %v, want only the job on the second page", posted)
	}
	if !strings.Contains(out, "canceled 1, skipped 0, failed 0") {
		t.Fatalf("out = %q", out)
	}
}

// TestJobActionsResolveAliasesAndPrefixes is finding #21: the action must be
// posted to the ID the alias or prefix resolved to, not to the raw reference.
func TestJobActionsResolveAliasesAndPrefixes(t *testing.T) {
	full := "job_0123456789abcdef0123456789abcdef"
	var mu sync.Mutex
	var posted []string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/v1/preferences":
			reply(w, 200, map[string]any{})
		case r.Method == "GET" && r.URL.Path == "/v1/jobs":
			reply(w, 200, map[string]any{"items": []any{jobRow(full, "running", 3)}, "next_cursor": nil})
		case r.Method == "GET" && r.URL.Path == "/v1/jobs/"+full:
			reply(w, 200, jobRow(full, "running", 3))
		case r.Method == "POST" && r.URL.Path == "/v1/jobs/"+full+"/pause":
			mu.Lock()
			posted = append(posted, r.URL.Path)
			mu.Unlock()
			reply(w, 200, jobRow(full, "paused", 4))
		default:
			mu.Lock()
			posted = append(posted, r.Method+" "+r.URL.Path)
			mu.Unlock()
			replyError(w, 404, "not_found", "Not found.")
		}
	})
	for _, ref := range []string{"@last", "job_0123", full} {
		code, _, errOut := driveCLI(t, h, "jobs", "pause", ref)
		if code != ExitOK {
			t.Fatalf("%s: exit %d: %s", ref, code, errOut)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(posted) != 3 {
		t.Fatalf("posts = %v", posted)
	}
	for _, path := range posted {
		if path != "/v1/jobs/"+full+"/pause" {
			t.Fatalf("an action was posted to the unresolved reference: %v", posted)
		}
	}
}

// TestJobsCancelAllRefusesAnEndlessList keeps the other half of finding #20:
// a list that never ends must fail loudly instead of reporting a partial
// "canceled 0, skipped 0, failed 0" success.
func TestJobsCancelAllRefusesAnEndlessList(t *testing.T) {
	pages := 0
	mutations := 0
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/v1/jobs":
			pages++
			next := base64.RawURLEncoding.EncodeToString([]byte("jobs\x1fowner\x1f\x1f" + strconv.Itoa(pages)))
			reply(w, 200, map[string]any{"items": []any{jobRow("j1", "queued", 1)}, "next_cursor": &next})
		case strings.HasSuffix(r.URL.Path, "/cancel"):
			mutations++
			reply(w, 200, jobRow("j1", "cancelled", 2))
		default:
			reply(w, 200, jobRow("j1", "queued", 1))
		}
	})
	code, _, errOut := driveCLI(t, h, "jobs", "cancel", "--all", "--yes")
	if code == ExitOK {
		t.Fatalf("an endless list was reported as success: %q", errOut)
	}
	if mutations != 0 {
		t.Fatalf("%d job(s) were canceled from a partial list", mutations)
	}
	if !strings.Contains(errOut, "did not end") {
		t.Fatalf("stderr = %q", errOut)
	}
}
