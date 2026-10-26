package cli

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GiaHung07/502Drive/internal/config"
)

func TestSetupWritesTelegramAndCallbackOptions(t *testing.T) {
	home := privateDir(t)
	cfgPath := filepath.Join(home, "config.json")
	tokenFile := filepath.Join(home, "tg-token")
	if err := os.WriteFile(tokenFile, []byte("123456:synthetic-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errb strings.Builder
	code := Execute(t.Context(), []string{"--config", cfgPath, "setup",
		"--telegram-token-file", tokenFile, "--telegram-owner-id", "4242", "--oauth-callback-port", "50200"},
		IO{In: strings.NewReader(""), Out: &out, Err: &errb})
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	if strings.Contains(out.String(), "synthetic-token") {
		t.Fatal("the bot token was printed")
	}
	if !strings.Contains(out.String(), "4242") || !strings.Contains(out.String(), "50200") {
		t.Fatalf("summary = %q", out.String())
	}
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	var cfg config.Config
	if err = json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.TelegramTokenFile != tokenFile || cfg.TelegramOwnerID != 4242 || cfg.OAuthCallbackPort != 50200 {
		t.Fatalf("config = %+v", cfg)
	}
	if info, err := os.Stat(cfgPath); err != nil || info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("config permissions = %v", info.Mode())
	}
}

func TestSetupRejectsUnsafeOptions(t *testing.T) {
	home := privateDir(t)
	cfgPath := filepath.Join(home, "config.json")
	shared := filepath.Join(home, "shared-token")
	if err := os.WriteFile(shared, []byte("token"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"group-readable token", []string{"setup", "--telegram-token-file", shared}},
		{"missing token", []string{"setup", "--telegram-token-file", filepath.Join(home, "nope")}},
		{"zero owner", []string{"setup", "--telegram-owner-id", "0"}},
		{"negative owner", []string{"setup", "--telegram-owner-id", "-5"}},
		{"port too high", []string{"setup", "--oauth-callback-port", "70000"}},
		{"port far too high", []string{"setup", "--oauth-callback-port", "99999"}},
		{"port negative", []string{"setup", "--oauth-callback-port", "-1"}},
	} {
		args := append([]string{"--config", cfgPath}, tc.args...)
		code := Execute(t.Context(), args, IO{In: strings.NewReader(""), Out: &outBuffer{}, Err: &outBuffer{}})
		if code != ExitUsage {
			t.Errorf("%s: exit %d", tc.name, code)
		}
	}
	if _, err := os.Stat(cfgPath); !os.IsNotExist(err) {
		t.Fatal("an invalid setup created a config")
	}
}

// privateDir creates a 0700 directory for config tests.
func privateDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "cfg")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	return dir
}

type outBuffer struct{ b strings.Builder }

func (o *outBuffer) Write(p []byte) (int, error) { return o.b.Write(p) }

func TestSSHForwardHintUsesTheSessionPort(t *testing.T) {
	authURL := "https://accounts.google.com/o/oauth2/auth?client_id=x&redirect_uri=" +
		url.QueryEscape("http://127.0.0.1:50200/oauth/callback") + "&scope=y"
	hint := sshForwardHint(authURL)
	if !strings.Contains(hint, "ssh -L 50200:127.0.0.1:50200") {
		t.Fatalf("hint = %q", hint)
	}
	if hint := sshForwardHint("https://accounts.google.com/o/oauth2/auth?client_id=x"); hint != "" {
		t.Fatalf("a URL without a port produced a guess: %q", hint)
	}
	if hint := sshForwardHint("::not a url::"); hint != "" {
		t.Fatalf("an unparsable URL produced a hint: %q", hint)
	}
}
