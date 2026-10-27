package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOnboardNonTTYDoesNotReadInput(t *testing.T) {
	var out, errb strings.Builder
	code := Execute(t.Context(), []string{"--config", filepath.Join(t.TempDir(), "missing.json"), "--no-input", "onboard"}, IO{In: panicReader{}, Out: &out, Err: &errb})
	if code != 9 || !strings.Contains(out.String(), `"checks"`) || !strings.Contains(out.String(), `"daemon"`) {
		t.Fatalf("exit=%d out=%s err=%s", code, out.String(), errb.String())
	}
}

type panicReader struct{}

func (panicReader) Read([]byte) (int, error) { panic("unattended command read stdin") }

func TestCleanPromptPathExpandsHomeAndDropsQuotes(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	for in, want := range map[string]string{
		"  ~/Downloads/client.json ": filepath.Join(home, "Downloads/client.json"),
		"'/tmp/with space/c.json'":   "/tmp/with space/c.json",
		`"/tmp/c.json"`:              "/tmp/c.json",
		"/abs/c.json":                "/abs/c.json",
		"~":                          home,
	} {
		if got := cleanPromptPath(in); got != want {
			t.Fatalf("cleanPromptPath(%q) = %q, want %q", in, got, want)
		}
	}
}

// In a terminal, onboard prints a readable checklist (not JSON), and pressing
// Enter at the destination step uses My Drive.
func TestOnboardInteractiveIsReadableAndEnterMeansMyDrive(t *testing.T) {
	var added string
	destDone := false
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/health/live":
			w.WriteHeader(200)
		case r.URL.Path == "/v1/setup":
			status := "failed"
			if destDone {
				status = "passed"
			}
			reply(w, 200, map[string]any{"ready": destDone, "checks": []map[string]any{
				{"name": "account", "status": "passed", "message": "ok"},
				{"name": "destination", "status": status, "message": "x"}}})
		case r.URL.Path == "/v1/drive/folders/root":
			reply(w, 200, map[string]any{"folder": map[string]any{"id": "MYDRIVE_ROOT_0123"}, "items": []any{}})
		case r.Method == "GET" && r.URL.Path == "/v1/destinations":
			reply(w, 200, map[string]any{"items": []any{}})
		case r.Method == "POST" && r.URL.Path == "/v1/destinations":
			var body struct {
				Source string `json:"source"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			added = body.Source
			reply(w, 201, map[string]any{"id": "dst_1", "name": "My Drive", "revision": 1})
		case r.URL.Path == "/v1/destinations/default":
			destDone = true
			reply(w, 200, map[string]any{"id": "dst_1", "revision": 2, "is_default": true})
		default:
			reply(w, 404, map[string]any{})
		}
	})
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	dir := privateDir(t)
	cfg := filepath.Join(dir, "config.json")
	conf := `{"listen":"127.0.0.1:5020","data_dir":"data","token_file":"api-token","oauth_client_file":"client.json"}`
	if err := os.WriteFile(cfg, []byte(conf), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errb strings.Builder
	in := IO{In: strings.NewReader("\n"), Out: &out, Err: &errb, StdinTTY: true, StdoutTTY: true, Width: 80,
		Getenv: func(k string) string {
			if k == "DRIVE502_TOKEN" {
				return "tok"
			}
			return ""
		}}
	code := Execute(t.Context(), []string{"--config", cfg, "--endpoint", ts.URL, "onboard"}, in)
	if code != 0 || added != "MYDRIVE_ROOT_0123" {
		t.Fatalf("exit=%d added=%q err=%s", code, added, errb.String())
	}
	if strings.Contains(out.String(), "{") || !strings.Contains(out.String(), "✓") {
		t.Fatalf("interactive output should be a readable checklist:\n%s", out.String())
	}
}
