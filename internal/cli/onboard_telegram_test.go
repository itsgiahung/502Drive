package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

const (
	tgGoodToken = "123456:ABCDEFGHIJKLMNOP"
	tgBadToken  = "999999:WRONGWRONGWRONG"
)

// fakeTelegram answers getMe for the good token and getUpdates with one
// private message from user 4242 (or nothing when silent).
func fakeTelegram(t *testing.T, silent bool) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if !strings.HasPrefix(r.URL.Path, "/bot"+tgGoodToken+"/") {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"ok":false,"description":"Unauthorized"}`))
			return
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/getMe"):
			_, _ = w.Write([]byte(`{"ok":true,"result":{"username":"my_test_bot"}}`))
		case strings.HasSuffix(r.URL.Path, "/getUpdates"):
			if silent {
				_, _ = w.Write([]byte(`{"ok":true,"result":[]}`))
				return
			}
			_, _ = w.Write([]byte(`{"ok":true,"result":[
				{"update_id":1,"message":{"chat":{"type":"group"},"from":{"id":7,"is_bot":false,"first_name":"Group"}}},
				{"update_id":2,"message":{"chat":{"type":"private"},"from":{"id":4242,"is_bot":false,"first_name":"Lan"}}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(ts.Close)
	return ts
}

func telegramTestIO(in string, api string, out, errb *strings.Builder) IO {
	io := onboardTestIO(strings.NewReader(in), out, errb)
	base := io.Getenv
	io.Getenv = func(k string) string {
		if k == "DRIVE502_TELEGRAM_API" {
			return api
		}
		return base(k)
	}
	return io
}

func readTelegramConfig(t *testing.T, path string) (owner int64, tokenFile string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var c struct {
		Owner int64  `json:"telegram_owner_id"`
		File  string `json:"telegram_token_file"`
	}
	if err = json.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	return c.Owner, c.File
}

func TestOnboardTelegramFindsTheOwnerFromTheFirstPrivateMessage(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("HOME", t.TempDir())
	ts := fakeTelegram(t, false)
	cfg := onboardTestConfig(t, "client.json")
	openSetupPage = func(string) error { return nil }
	t.Cleanup(func() { openSetupPage = openBrowser })
	var out, errb strings.Builder
	// A wrong token first, then the right one, then Enter to confirm "is that you?".
	code := Execute(t.Context(), []string{"--config", cfg, "onboard", "--telegram"}, telegramTestIO(tgBadToken+"\n"+tgGoodToken+"\n\n", ts.URL, &out, &errb))
	if code != 0 {
		t.Fatalf("exit=%d err=%s", code, errb.String())
	}
	owner, file := readTelegramConfig(t, cfg)
	if owner != 4242 || file == "" {
		t.Fatalf("saved owner=%d token file=%q (the group message must be ignored)", owner, file)
	}
	st, err := os.Stat(file)
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("token file must be private: %v %v", st, err)
	}
	for _, text := range []string{out.String(), errb.String()} {
		if strings.Contains(text, tgGoodToken) || strings.Contains(text, tgBadToken) {
			t.Fatalf("a token was printed:\n%s", text)
		}
	}
	if !strings.Contains(errb.String(), "@my_test_bot") || !strings.Contains(errb.String(), "Lan") {
		t.Fatalf("the guide should name the bot and the person it found:\n%s", errb.String())
	}
}

func TestOnboardTelegramAsksForTheIDWhenNoMessageArrives(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("HOME", t.TempDir())
	ts := fakeTelegram(t, true)
	cfg := onboardTestConfig(t, "client.json")
	openSetupPage = func(string) error { return nil }
	old := onboardTelegramWait
	onboardTelegramWait = 100 * time.Millisecond
	t.Cleanup(func() { openSetupPage, onboardTelegramWait = openBrowser, old })
	var out, errb strings.Builder
	code := Execute(t.Context(), []string{"--config", cfg, "onboard", "--telegram"}, telegramTestIO(tgGoodToken+"\nabc\n555\n", ts.URL, &out, &errb))
	if code != 0 {
		t.Fatalf("exit=%d err=%s", code, errb.String())
	}
	if owner, _ := readTelegramConfig(t, cfg); owner != 555 {
		t.Fatalf("owner = %d, want the typed id after a bad one was refused", owner)
	}
}

func TestOnboardTelegramEnterSkipsAndSavesNothing(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("HOME", t.TempDir())
	ts := fakeTelegram(t, false)
	cfg := onboardTestConfig(t, "client.json")
	openSetupPage = func(string) error { return nil }
	t.Cleanup(func() { openSetupPage = openBrowser })
	var out, errb strings.Builder
	if code := Execute(t.Context(), []string{"--config", cfg, "onboard", "--telegram"}, telegramTestIO("\n", ts.URL, &out, &errb)); code != 0 {
		t.Fatalf("exit=%d err=%s", code, errb.String())
	}
	if owner, file := readTelegramConfig(t, cfg); owner != 0 || file != "" {
		t.Fatalf("skipping must save nothing: %d %q", owner, file)
	}
}

func TestOnboardTelegramNeedsATerminal(t *testing.T) {
	cfg := onboardTestConfig(t, "client.json")
	var out, errb strings.Builder
	code := Execute(t.Context(), []string{"--config", cfg, "--no-input", "onboard", "--telegram"}, IO{In: panicReader{}, Out: &out, Err: &errb})
	if code == 0 || !strings.Contains(errb.String(), "setup --telegram-token-file") {
		t.Fatalf("a script must be told to use setup: exit=%d %s", code, errb.String())
	}
}
