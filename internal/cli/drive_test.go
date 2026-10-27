package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// driveCLI runs one command against a fake daemon and returns its exit code
// and streams. The fake never needs a real config: endpoint and token come
// from the flag and the environment, and the config path only anchors the
// private pending file.
func driveCLI(t *testing.T, h http.Handler, args ...string) (code int, out, errOut string) {
	t.Helper()
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	cfg := filepath.Join(privateDir(t), "config.json")
	if err := os.WriteFile(cfg, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	var o, e strings.Builder
	in := IO{In: strings.NewReader(""), Out: &o, Err: &e, Getenv: func(k string) string {
		if k == "DRIVE502_TOKEN" {
			return "tok"
		}
		return ""
	}}
	argv := append([]string{"--config", cfg, "--endpoint", ts.URL}, args...)
	return Execute(t.Context(), argv, in), o.String(), e.String()
}

// folderPage writes one GET /v1/drive/folders response.
func folderPage(w http.ResponseWriter, items []any, next *string) {
	reply(w, 200, map[string]any{
		"folder":          map[string]any{"id": "root", "name": "My Drive", "kind": "folder", "can_add_children": true},
		"path":            []any{map[string]any{"id": "root", "name": "My Drive"}},
		"items":           items,
		"next_page_token": next,
	})
}

func TestLsListsFoldersFirstWithSizeAndDate(t *testing.T) {
	var hits int
	code, out, errOut := driveCLI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/preferences" {
			reply(w, 200, map[string]any{"language": "en", "timezone": "UTC"})
			return
		}
		hits++
		if r.URL.Path != "/v1/drive/folders/root" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.URL.Query().Get("resource_key") != "" || r.URL.Query().Get("page_token") != "" {
			t.Errorf("unexpected query %q", r.URL.RawQuery)
		}
		folderPage(w, []any{
			map[string]any{"id": "fldZ", "name": "zeta", "kind": "folder"},
			map[string]any{"id": "filB", "name": "b.txt", "kind": "binary", "size_bytes": 2048, "modified_at": "2026-10-01T08:30:00Z"},
			map[string]any{"id": "fldA", "name": "Alpha", "kind": "folder"},
			map[string]any{"id": "filA", "name": "a.txt", "kind": "binary", "size_bytes": 500, "modified_at": "2026-09-02T00:00:00Z"},
		}, nil)
	}), "ls")
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if hits != 1 {
		t.Fatalf("requests = %d, want 1", hits)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 5 {
		t.Fatalf("printed %d lines, want breadcrumb + 4 rows:\n%s", len(lines), out)
	}
	if lines[0] != "My Drive" {
		t.Errorf("breadcrumb = %q", lines[0])
	}
	order := []string{"Alpha/", "zeta/", "a.txt", "b.txt"}
	for i, want := range order {
		if !strings.Contains(lines[i+1], want) {
			t.Errorf("row %d = %q, want name %q", i, lines[i+1], want)
		}
	}
	if !strings.Contains(lines[3], "500 B") || !strings.Contains(lines[3], "2026-09-02") {
		t.Errorf("file row = %q", lines[3])
	}
	if !strings.Contains(lines[4], "2.0 KB") || !strings.Contains(lines[4], "2026-10-01") {
		t.Errorf("file row = %q", lines[4])
	}
	if !strings.HasPrefix(strings.TrimSpace(lines[1]), "-") {
		t.Errorf("folders should show a dash size: %q", lines[1])
	}
}

func TestLsAllFollowsNextPageToken(t *testing.T) {
	var tokens []string
	code, out, errOut := driveCLI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok := r.URL.Query().Get("page_token")
		tokens = append(tokens, tok)
		next := "p2"
		switch tok {
		case "":
			folderPage(w, []any{map[string]any{"id": "f1", "name": "first", "kind": "binary"}}, &next)
		case "p2":
			folderPage(w, []any{map[string]any{"id": "f2", "name": "second", "kind": "binary"}}, nil)
		default:
			t.Errorf("unexpected page token %q", tok)
		}
	}), "ls", "--all")
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if len(tokens) != 2 || tokens[0] != "" || tokens[1] != "p2" {
		t.Fatalf("page tokens = %q", tokens)
	}
	if !strings.Contains(out, "first") || !strings.Contains(out, "second") {
		t.Fatalf("a page was dropped:\n%s", out)
	}
}

func TestLsWithoutAllReadsOnePageAndKeepsJSONEnvelope(t *testing.T) {
	next := "p2"
	code, out, errOut := driveCLI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		folderPage(w, []any{map[string]any{"id": "f1", "name": "first", "kind": "binary"}}, &next)
	}), "ls", "--json")
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	var env struct {
		Data struct {
			Items []struct {
				Name string `json:"name"`
			} `json:"items"`
			Next *string `json:"next_page_token"`
		} `json:"data"`
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("not an envelope: %v\n%s", err, out)
	}
	if len(env.Data.Items) != 1 || env.Data.Items[0].Name != "first" || env.Data.Next == nil || *env.Data.Next != "p2" {
		t.Fatalf("envelope = %+v", env)
	}
	if env.RequestID != "req_1" {
		t.Errorf("request_id = %q", env.RequestID)
	}
}

func TestLsAllMergesPagesIntoOneJSONEnvelope(t *testing.T) {
	code, out, errOut := driveCLI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page_token") == "" {
			next := "p2"
			folderPage(w, []any{map[string]any{"id": "f1", "name": "first", "kind": "binary"}}, &next)
			return
		}
		folderPage(w, []any{map[string]any{"id": "f2", "name": "second", "kind": "binary"}}, nil)
	}), "ls", "--all", "--json")
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	var env struct {
		Data struct {
			Items []struct {
				Name string `json:"name"`
			} `json:"items"`
			Path []struct {
				Name string `json:"name"`
			} `json:"path"`
			Next *string `json:"next_page_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("not an envelope: %v\n%s", err, out)
	}
	if len(env.Data.Items) != 2 || env.Data.Items[1].Name != "second" || env.Data.Next != nil {
		t.Fatalf("envelope = %+v", env)
	}
	if len(env.Data.Path) != 1 || env.Data.Path[0].Name != "My Drive" {
		t.Fatalf("path = %+v", env.Data.Path)
	}
}

func TestLsEscapesTerminalSequencesInNames(t *testing.T) {
	code, out, errOut := driveCLI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		folderPage(w, []any{map[string]any{"id": "f1", "name": "evil\x1b[31m.red", "kind": "binary"}}, nil)
	}), "ls")
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if strings.Contains(out, "\x1b") {
		t.Fatalf("a terminal escape reached the output: %q", out)
	}
	if !strings.Contains(out, `evil\x1b[31m.red`) {
		t.Fatalf("name was not shown as text: %q", out)
	}
}

func TestInfoPrintsMetadataAndUsesTheParsedID(t *testing.T) {
	var gotKey string
	code, out, errOut := driveCLI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/preferences" {
			reply(w, 200, map[string]any{"language": "en", "timezone": "UTC"})
			return
		}
		if r.URL.Path != "/v1/drive/files/1AbCdEfGhIjKlMnOp" {
			t.Errorf("path = %q", r.URL.Path)
		}
		gotKey = r.URL.Query().Get("resource_key")
		reply(w, 200, map[string]any{
			"id": "1AbCdEfGhIjKlMnOp", "name": "Report.pdf", "kind": "binary", "mime_type": "application/pdf",
			"size_bytes": 1536, "created_at": "2026-01-02T03:04:05Z", "modified_at": "2026-10-01T08:30:00Z",
			"owner": map[string]any{"name": "Ann", "email": "ann@example.test"}, "can_copy": true, "can_add_children": false,
			"web_url": "https://drive.google.com/file/d/1AbCdEfGhIjKlMnOp/view",
		})
	}), "info", "https://drive.google.com/file/d/1AbCdEfGhIjKlMnOp/view?usp=sharing&resourcekey=k3y")
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if gotKey != "k3y" {
		t.Errorf("resource_key = %q", gotKey)
	}
	for _, want := range []string{
		"KIND", "binary",
		"1.5 KB",
		"2026-10-01", "2026-01-02",
		"Ann <ann@example.test>",
		"yes", "no",
		"https://drive.google.com/file/d/1AbCdEfGhIjKlMnOp/view",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output misses %q:\n%s", want, out)
		}
	}
}

func TestInfoJSONIsTheRawEnvelope(t *testing.T) {
	code, out, errOut := driveCLI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reply(w, 200, map[string]any{"id": "FILE_0123456789", "name": "x", "kind": "binary"})
	}), "info", "FILE_0123456789", "--json")
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(out, `"request_id":"req_1"`) || !strings.Contains(out, `"kind":"binary"`) {
		t.Fatalf("out = %q", out)
	}
	if strings.Contains(out, "KIND") {
		t.Fatalf("human output leaked into --json: %q", out)
	}
}

func TestMkdirSendsIdempotencyKeyAndPrintsLink(t *testing.T) {
	var gotKey, parent, name string
	code, out, errOut := driveCLI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/v1/drive/folders" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		gotKey = r.Header.Get("Idempotency-Key")
		var body struct {
			ParentID string `json:"parent_id"`
			Name     string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		parent, name = body.ParentID, body.Name
		reply(w, 201, map[string]any{"id": "FOLDER_0123456789", "name": "New", "kind": "folder",
			"web_url": "https://drive.google.com/drive/folders/FOLDER_0123456789"})
	}), "mkdir", "New", "--in", "root")
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if gotKey == "" {
		t.Error("no Idempotency-Key header")
	}
	if parent != "root" || name != "New" {
		t.Errorf("body parent=%q name=%q", parent, name)
	}
	if !strings.Contains(out, "FOLDER_0123456789") || !strings.Contains(out, "https://drive.google.com/drive/folders/FOLDER_0123456789") {
		t.Fatalf("out = %q", out)
	}
}

func TestMkdirUsesAnExplicitKeyAndPassesJSONThrough(t *testing.T) {
	var header string
	code, out, errOut := driveCLI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header = r.Header.Get("Idempotency-Key")
		reply(w, 201, map[string]any{"id": "FOLDER_0123456789", "kind": "folder", "web_url": "https://drive.google.com/drive/folders/FOLDER_0123456789"})
	}), "mkdir", "New", "--idempotency-key", "fixed-key", "--json")
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if header != "fixed-key" {
		t.Errorf("Idempotency-Key = %q", header)
	}
	if !strings.Contains(out, `"request_id":"req_1"`) || !strings.Contains(out, `"id":"FOLDER_0123456789"`) {
		t.Fatalf("out = %q", out)
	}
}

func TestMkdirRejectsABadParentBeforeAnyRequest(t *testing.T) {
	code, _, _ := driveCLI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request %s", r.URL.Path)
	}), "mkdir", "New", "--in", "not a link")
	if code != ExitUsage {
		t.Fatalf("exit %d, want %d", code, ExitUsage)
	}
}

func TestDriveCommandsMapAPIFailuresToExitCodes(t *testing.T) {
	cases := []struct {
		name   string
		args   []string
		status int
		want   int
	}{
		{"not found", []string{"info", "FILE_0123456789"}, 404, ExitNotFound},
		{"conflict", []string{"mkdir", "New"}, 409, ExitConflict},
		{"auth", []string{"ls"}, 401, ExitAuth},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, _, errOut := driveCLI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "api_problem", "message": "the daemon said no"}})
			}), tc.args...)
			if code != tc.want {
				t.Fatalf("exit %d, want %d: %s", code, tc.want, errOut)
			}
			if strings.TrimSpace(errOut) == "" {
				t.Error("no error message on stderr")
			}
		})
	}
}

func TestCopyAutoNameUsesTheSourceNameAndDate(t *testing.T) {
	old := todayFunc
	todayFunc = func() time.Time { return time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { todayFunc = old })

	var plans []map[string]any
	code, out, errOut := driveCLI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/v1/plans":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			plans = append(plans, body)
			reply(w, 200, map[string]any{"id": "plan_1", "revision": 3, "source_id": "SRC_0123456789",
				"source_name": "Project Alpha", "source_kind": "folder", "destination_id": "dst_1",
				"root_name": body["root_name"], "expires_at": "2026-10-27T13:00:00Z"})
		case r.Method == "POST" && r.URL.Path == "/v1/jobs":
			reply(w, 202, map[string]any{"id": "job_1", "state": "queued", "source_name": "Project Alpha",
				"root_name": "Project Alpha (copy 2026-10-27)", "desired_state": "running", "revision": 1,
				"counts":     map[string]any{"completed": 0, "failed": 0, "blocked": 0, "skipped": 0},
				"updated_at": "2026-10-27T12:00:00Z"})
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}), "copy", "SRC_0123456789", "--auto-name", "--to", "dst_1")
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if len(plans) != 2 {
		t.Fatalf("plans = %d, want 2 (name lookup + real plan)", len(plans))
	}
	if _, named := plans[0]["root_name"]; named {
		t.Errorf("the lookup plan must not name the root: %+v", plans[0])
	}
	if got := plans[1]["root_name"]; got != "Project Alpha (copy 2026-10-27)" {
		t.Errorf("root_name = %v", got)
	}
	if !strings.Contains(out, "job_1") {
		t.Errorf("out = %q", out)
	}
}

func TestCopyAutoNameAndNameAreExclusive(t *testing.T) {
	code, _, _ := driveCLI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request %s", r.URL.Path)
	}), "copy", "SRC_0123456789", "--auto-name", "--name", "x")
	if code != ExitUsage {
		t.Fatalf("exit %d, want %d", code, ExitUsage)
	}
}

func TestCopyAutoNameDryRunPrintsTheDerivedName(t *testing.T) {
	old := todayFunc
	todayFunc = func() time.Time { return time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { todayFunc = old })

	code, out, errOut := driveCLI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		name, _ := body["root_name"].(string)
		source := "Project Alpha"
		if name == "" {
			name = source
		}
		reply(w, 200, map[string]any{"id": "plan_1", "revision": 3, "source_id": "SRC_0123456789",
			"source_name": source, "source_kind": "folder", "destination_id": "dst_1",
			"root_name": name, "expires_at": "2026-10-27T13:00:00Z"})
	}), "copy", "SRC_0123456789", "--auto-name", "--dry-run")
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(out, "Project Alpha (copy 2026-10-27)") {
		t.Fatalf("out = %q", out)
	}
}

// Each `audit` invocation starts a new audit: two runs must not share an
// idempotency key, or the server replays the first report forever.
func TestAuditStartsANewAuditEachTime(t *testing.T) {
	var keys []string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/audits") {
			keys = append(keys, r.Header.Get("Idempotency-Key"))
		}
		if strings.HasPrefix(r.URL.Path, "/v1/jobs/") {
			reply(w, 200, map[string]any{"id": "job_1", "state": "succeeded", "revision": 1, "counts": map[string]any{}})
			return
		}
		reply(w, 200, map[string]any{"id": "aud_1", "collection_id": "col_1", "job_id": "job_1", "state": "completed", "revision": 1})
	})
	for range 2 {
		driveCLI(t, h, "audit", "col_1", "--json")
	}
	if len(keys) != 2 || keys[0] == "" || keys[0] == keys[1] {
		t.Fatalf("audit idempotency keys = %q", keys)
	}
}

// A dry-run cleanup prints what it would move instead of only a job id.
func TestCleanupDryRunPrintsItsPlan(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/v1/collections/"):
			reply(w, 200, map[string]any{"id": "col_1", "revision": 4, "state": "retired"})
		case r.Method == "POST":
			reply(w, 202, map[string]any{"id": "job_c", "state": "queued", "dry_run": true, "root_name": "Photos (2)"})
		default:
			reply(w, 200, map[string]any{"id": "job_c", "state": "succeeded", "dry_run": true, "root_name": "Photos (2)",
				"counts": map[string]any{"total_items": 8, "logical_bytes": 150}})
		}
	})
	code, out, errOut := driveCLI(t, h, "collections", "cleanup", "col_1")
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(out, `Would move "Photos (2)" to the Drive trash: 8 copied items`) || !strings.Contains(out, "--confirm-name 'Photos (2)'") {
		t.Fatalf("out = %q", out)
	}
}

// audit --wait is a switch like copy --wait; the old duration form explains
// the change, and --wait=false only starts the audit.
func TestAuditWaitFlagMatchesCopy(t *testing.T) {
	posts := 0
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			posts++
		}
		reply(w, 202, map[string]any{"id": "aud_1", "collection_id": "col_1", "job_id": "job_1", "state": "running", "revision": 1})
	})
	code, _, errOut := driveCLI(t, h, "audit", "col_1", "--wait", "5m")
	if code != ExitUsage || !strings.Contains(errOut, "--wait-timeout 5m") || posts != 0 {
		t.Fatalf("old --wait form: exit %d posts %d err %q", code, posts, errOut)
	}
	code, out, errOut := driveCLI(t, h, "audit", "col_1", "--wait=false")
	if code != ExitOK || !strings.Contains(out, "aud_1") || posts != 1 {
		t.Fatalf("--wait=false: exit %d out %q err %q", code, out, errOut)
	}
	if code, _, _ = driveCLI(t, h, "audit", "col_1", "--wait=false", "--repair"); code != ExitUsage {
		t.Fatalf("--repair without waiting must be refused, exit %d", code)
	}
}
