package cli

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLogsPrintsTheTailAndSanitizes(t *testing.T) {
	home := privateDir(t)
	cfg := filepath.Join(home, "config.json")
	if err := os.MkdirAll(filepath.Join(home, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	for i := 1; i <= 200; i++ {
		sb.WriteString("line " + strings.Repeat("x", i%3) + "\n")
	}
	sb.WriteString("evil \x1b[31mred\x1b[0m\n")
	if err := os.WriteFile(filepath.Join(home, "logs", "daemon.log"), []byte(sb.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errb strings.Builder
	code := Execute(t.Context(), []string{"--config", cfg, "logs", "-n", "3"}, IO{In: strings.NewReader(""), Out: &out, Err: &errb})
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	got := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(got) != 3 {
		t.Fatalf("printed %d lines, want 3: %q", len(got), got)
	}
	if strings.Contains(out.String(), "\x1b") {
		t.Fatalf("a terminal escape reached the output: %q", out.String())
	}
}

func TestLogsWithoutAFileExplainsWhere(t *testing.T) {
	home := privateDir(t)
	var out, errb strings.Builder
	code := Execute(t.Context(), []string{"--config", filepath.Join(home, "config.json"), "logs"}, IO{In: strings.NewReader(""), Out: &out, Err: &errb})
	if code != ExitNotFound || !strings.Contains(errb.String(), "journalctl") {
		t.Fatalf("exit %d, stderr %q", code, errb.String())
	}
}

func TestAutostartEnableDisableOnLinux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the XDG autostart file is Linux-only")
	}
	home := privateDir(t)
	env := func(k string) string {
		if k == "XDG_CONFIG_HOME" {
			return home
		}
		return ""
	}
	run := func(args ...string) (int, string) {
		var out, errb strings.Builder
		code := Execute(t.Context(), args, IO{In: strings.NewReader(""), Out: &out, Err: &errb, Getenv: env})
		return code, out.String() + errb.String()
	}
	file := filepath.Join(home, "autostart", "502drive.desktop")
	if code, out := run("autostart", "enable"); code != ExitOK {
		t.Fatalf("enable: %d %s", code, out)
	}
	raw, err := os.ReadFile(file)
	if err != nil || !strings.Contains(string(raw), " up --quiet") {
		t.Fatalf("entry = %q, %v", raw, err)
	}
	if _, out := run("autostart", "status"); !strings.Contains(out, "autostart  on") {
		t.Fatalf("status = %q", out)
	}
	if code, out := run("autostart", "disable"); code != ExitOK {
		t.Fatalf("disable: %d %s", code, out)
	}
	if _, err = os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("disable left the autostart file behind")
	}
	if code, _ := run("autostart", "disable"); code != ExitOK {
		t.Fatal("disabling twice must not fail")
	}
}

func TestDesktopExecQuotingIsSafe(t *testing.T) {
	got := desktopExec(`/opt/my "app"/$HOME/502drive`)
	if got != `"/opt/my \"app\"/\$HOME/502drive"` {
		t.Fatalf("quoted = %s", got)
	}
}

func TestUpdateCheckReportsNewerAndNeverInstalls(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"tag_name":"v9.0.0","html_url":"https://example.test/release"}`))
	}))
	defer ts.Close()
	old := Version
	Version = "1.0.0"
	defer func() { Version = old }()
	env := func(k string) string {
		if k == "DRIVE502_UPDATE_API" {
			return ts.URL
		}
		return ""
	}
	var out, errb strings.Builder
	code := Execute(t.Context(), []string{"update"}, IO{In: strings.NewReader(""), Out: &out, Err: &errb, Getenv: env})
	if code != ExitOK || !strings.Contains(out.String(), "v9.0.0") || !strings.Contains(out.String(), "never updates itself") {
		t.Fatalf("exit %d out %q err %q", code, out.String(), errb.String())
	}
}
