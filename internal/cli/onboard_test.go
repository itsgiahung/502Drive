package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
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

// onboardReadyServer answers the health probe and reports a finished setup so
// the tests see only the OAuth questions.
func onboardReadyServer(t *testing.T) *httptest.Server {
	t.Helper()
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health/live":
			w.WriteHeader(http.StatusOK)
		case "/v1/setup":
			reply(w, 200, map[string]any{"ready": true, "checks": []map[string]any{
				{"name": "oauth_client", "status": "passed", "message": "ok"},
				{"name": "account", "status": "passed", "message": "ok"},
				{"name": "destination", "status": "passed", "message": "ok"},
			}})
		default:
			reply(w, 404, map[string]any{})
		}
	})
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	return ts
}

func onboardTestIO(in io.Reader, out, errb *strings.Builder) IO {
	return IO{In: in, Out: out, Err: errb, StdinTTY: true, StdoutTTY: true, Width: 80,
		Getenv: func(k string) string {
			if k == "DRIVE502_TOKEN" {
				return "tok"
			}
			return ""
		}}
}

func onboardTestConfig(t *testing.T, oauthClient string) string {
	t.Helper()
	cfg := filepath.Join(privateDir(t), "config.json")
	body := `{"listen":"127.0.0.1:5020","data_dir":"data","token_file":"api-token"`
	if oauthClient != "" {
		body += `,"oauth_client_file":"` + oauthClient + `"`
	}
	body += "}"
	if err := os.WriteFile(cfg, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func writeClientJSON(t *testing.T, path, id, secret string) {
	t.Helper()
	body := `{"installed":{"client_id":"` + id + `","client_secret":"` + secret + `"}}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func noBrowser(t *testing.T) {
	t.Helper()
	openSetupPage = func(u string) error {
		t.Fatalf("the browser was opened at %s", u)
		return nil
	}
	t.Cleanup(func() { openSetupPage = openBrowser })
}

// A configured key is the shortest path: onboard must not ask anything.
func TestOnboardExistingKeyAsksNothing(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("HOME", t.TempDir())
	ts := onboardReadyServer(t)
	cfg := onboardTestConfig(t, "client.json")
	noBrowser(t)
	var out, errb strings.Builder
	code := Execute(t.Context(), []string{"--config", cfg, "--endpoint", ts.URL, "onboard"}, onboardTestIO(panicReader{}, &out, &errb))
	if code != 0 {
		t.Fatalf("exit=%d err=%s", code, errb.String())
	}
	if strings.Contains(errb.String(), "[Y/n]") || strings.Contains(errb.String(), "1/5") || errb.Len() != 0 {
		t.Fatalf("an existing key must ask nothing:\n%s", errb.String())
	}
}

// A fresh client_secret*.json in ~/Downloads is offered with one question.
func TestOnboardOffersRecentDownloadOnce(t *testing.T) {
	t.Chdir(t.TempDir())
	home := t.TempDir()
	t.Setenv("HOME", home)
	downloads := filepath.Join(home, "Downloads")
	if err := os.MkdirAll(downloads, 0o700); err != nil {
		t.Fatal(err)
	}
	writeClientJSON(t, filepath.Join(downloads, "client_secret_1.json"), "id.apps.googleusercontent.com", "synth-secret")
	ts := onboardReadyServer(t)
	cfg := onboardTestConfig(t, "")
	noBrowser(t)
	var out, errb strings.Builder
	code := Execute(t.Context(), []string{"--config", cfg, "--endpoint", ts.URL, "onboard"}, onboardTestIO(strings.NewReader("\n"), &out, &errb))
	if code != 0 {
		t.Fatalf("exit=%d out=%s err=%s", code, out.String(), errb.String())
	}
	if n := strings.Count(errb.String(), "[Y/n]"); n != 1 {
		t.Fatalf("want exactly one confirmation question, got %d:\n%s", n, errb.String())
	}
	if strings.Contains(errb.String(), "1/5") || strings.Contains(errb.String(), "Paste the JSON file path") {
		t.Fatalf("the download branch asked an extra question:\n%s", errb.String())
	}
	if !strings.Contains(out.String(), "OAuth client stored privately") {
		t.Fatalf("the downloaded client was not installed:\n%s", out.String())
	}
	saved, err := os.ReadFile(cfg)
	if err != nil || !strings.Contains(string(saved), `"oauth_client_file"`) {
		t.Fatalf("config was not updated: %v %s", err, saved)
	}
}

// A pasted path ends onboarding at the first question; the guide never starts.
func TestOnboardPastedPathSkipsGuide(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("HOME", t.TempDir())
	client := filepath.Join(t.TempDir(), "client_secret.json")
	writeClientJSON(t, client, "id.apps.googleusercontent.com", "synth-secret")
	ts := onboardReadyServer(t)
	cfg := onboardTestConfig(t, "")
	noBrowser(t)
	var out, errb strings.Builder
	code := Execute(t.Context(), []string{"--config", cfg, "--endpoint", ts.URL, "onboard"}, onboardTestIO(strings.NewReader(client+"\n"), &out, &errb))
	if code != 0 {
		t.Fatalf("exit=%d out=%s err=%s", code, out.String(), errb.String())
	}
	if strings.Contains(errb.String(), "1/5") || strings.Contains(errb.String(), "[Y/n]") {
		t.Fatalf("a pasted path must not start the guide:\n%s", errb.String())
	}
	if !strings.Contains(out.String(), "OAuth client stored privately") {
		t.Fatalf("the pasted client was not installed:\n%s", out.String())
	}
}

// The guide opens each Google page in order and accepts the file afterwards.
func TestOnboardGuideOpensEveryPage(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("HOME", t.TempDir())
	client := filepath.Join(t.TempDir(), "client_secret.json")
	writeClientJSON(t, client, "id.apps.googleusercontent.com", "synth-secret")
	ts := onboardReadyServer(t)
	cfg := onboardTestConfig(t, "")
	var opened []string
	openSetupPage = func(u string) error {
		opened = append(opened, u)
		return nil
	}
	t.Cleanup(func() { openSetupPage = openBrowser })
	input := "g\n" + strings.Repeat("\n", 5) + client + "\n"
	var out, errb strings.Builder
	code := Execute(t.Context(), []string{"--config", cfg, "--endpoint", ts.URL, "onboard"}, onboardTestIO(strings.NewReader(input), &out, &errb))
	if code != 0 {
		t.Fatalf("exit=%d out=%s err=%s", code, out.String(), errb.String())
	}
	want := []string{
		"https://console.cloud.google.com/projectcreate",
		"https://console.cloud.google.com/apis/library/drive.googleapis.com",
		"https://console.cloud.google.com/auth/branding",
		"https://console.cloud.google.com/auth/clients/create",
	}
	if !slices.Equal(opened, want) {
		t.Fatalf("opened %v, want %v", opened, want)
	}
	if !strings.Contains(errb.String(), "1/5") || !strings.Contains(errb.String(), "5/5") {
		t.Fatalf("the guide did not print all steps:\n%s", errb.String())
	}
	if !strings.Contains(out.String(), "OAuth client stored privately") {
		t.Fatalf("the guide did not install the final client:\n%s", out.String())
	}
}

// A Web client pasted in the guide is explained and retried, not fatal at once.
func TestOnboardGuideRetriesWebClientThreeTimes(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("HOME", t.TempDir())
	web := filepath.Join(t.TempDir(), "client_secret_web.json")
	if err := os.WriteFile(web, []byte(`{"web":{"client_id":"a","client_secret":"b"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	ts := onboardReadyServer(t)
	cfg := onboardTestConfig(t, "")
	openSetupPage = func(string) error { return nil }
	t.Cleanup(func() { openSetupPage = openBrowser })
	input := "g\n" + web + "\n" + web + "\n" + web + "\n"
	var out, errb strings.Builder
	code := Execute(t.Context(), []string{"--config", cfg, "--endpoint", ts.URL, "onboard"}, onboardTestIO(strings.NewReader(input), &out, &errb))
	if code != ExitUsage {
		t.Fatalf("exit=%d, want %d; err=%s", code, ExitUsage, errb.String())
	}
	if n := strings.Count(errb.String(), "Web client"); n != 3 {
		t.Fatalf("want three explanations of the Web client, got %d:\n%s", n, errb.String())
	}
	if !strings.Contains(errb.String(), "Desktop app") {
		t.Fatalf("the error should say to create a Desktop app:\n%s", errb.String())
	}
}
