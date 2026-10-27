package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/itsgiahung/502Drive/internal/config"
	"github.com/itsgiahung/502Drive/internal/daemon"
	"github.com/itsgiahung/502Drive/internal/drive"
	"github.com/itsgiahung/502Drive/internal/drive/drivetest"
)

type rig struct {
	t      *testing.T
	cfg    string
	addr   string
	fake   *drivetest.Fake
	home   string
	cancel context.CancelFunc
	done   chan error
}

func newRig(t *testing.T) *rig {
	t.Helper()
	pollInterval = 10 * time.Millisecond
	home := filepath.Join(t.TempDir(), "cfg")
	if err := os.Mkdir(home, 0o700); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(home, "config.json")
	cfg, err := config.Setup(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	fake := drivetest.New()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	r := &rig{t: t, cfg: cfgPath, addr: "http://" + ln.Addr().String(), fake: fake, home: home, cancel: cancel, done: make(chan error, 1)}
	started := make(chan struct{})
	go func() {
		r.done <- daemon.Run(ctx, cfg, daemon.Options{Version: "test", Listener: ln, Provider: fake, Started: func(string) { close(started) }})
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("daemon did not start")
	}
	t.Cleanup(func() { cancel(); <-r.done })
	return r
}

type result struct {
	code        int
	out, errOut string
}

func (r *rig) run(args ...string) result { return r.runWith("", nil, args...) }

func (r *rig) runWith(stdin string, env map[string]string, args ...string) result {
	r.t.Helper()
	var out, errb bytes.Buffer
	full := append([]string{"--config", r.cfg, "--endpoint", r.addr}, args...)
	code := Execute(context.Background(), full, IO{In: strings.NewReader(stdin), Out: &out, Err: &errb, Width: 80,
		Getenv: func(k string) string { return env[k] }})
	return result{code, out.String(), errb.String()}
}

func decodeData(t *testing.T, s string) map[string]any {
	t.Helper()
	var env struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(s)), &env); err != nil {
		t.Fatalf("not a JSON envelope: %q: %v", s, err)
	}
	return env.Data
}

func (r *rig) setupDest() {
	r.t.Helper()
	f := r.fake.Folder("Archive", "root")
	res := r.run("destinations", "add", f.ID, "--json")
	if res.code != 0 {
		r.t.Fatalf("add: %+v", res)
	}
	id := decodeData(r.t, res.out)["id"].(string)
	if res = r.run("destinations", "default", id); res.code != 0 {
		r.t.Fatalf("default: %+v", res)
	}
}

func TestHelpVersionAndBareCommandNeedNoDaemonOrConfig(t *testing.T) {
	for _, args := range [][]string{{"version"}, {"--help"}, {}, {"copy", "--help"}} {
		var out, errb bytes.Buffer
		code := Execute(context.Background(), append([]string{"--config", filepath.Join(t.TempDir(), "none.json")}, args...), IO{In: strings.NewReader(""), Out: &out, Err: &errb})
		if code != 0 || out.Len() == 0 {
			t.Errorf("%v -> %d %q %q", args, code, out.String(), errb.String())
		}
	}
	var out bytes.Buffer
	Execute(context.Background(), []string{"--json", "version"}, IO{In: strings.NewReader(""), Out: &out, Err: io.Discard})
	if decodeData(t, out.String())["version"] == nil {
		t.Fatal("version json")
	}
}

func TestExitCodes(t *testing.T) {
	r := newRig(t)
	for _, c := range []struct {
		name string
		args []string
		want int
	}{
		{"unknown flag", []string{"status", "--bogus"}, ExitUsage},
		{"unknown command", []string{"frobnicate"}, ExitUsage},
		{"missing arg", []string{"jobs", "get"}, ExitUsage},
		{"bad color", []string{"--color", "rainbow", "status"}, ExitUsage},
		{"not found", []string{"jobs", "get", "job_nope"}, ExitNotFound},
		{"bad source", []string{"inspect", "https://evil.example/x"}, ExitUsage},
		{"unknown source", []string{"inspect", "abcdefghijk"}, ExitNotFound},
	} {
		if res := r.run(c.args...); res.code != c.want {
			t.Errorf("%s: exit %d want %d (%s)", c.name, res.code, c.want, res.errOut)
		}
	}
	// wrong token
	tokFile := filepath.Join(r.home, "bad-token")
	os.WriteFile(tokFile, []byte("wrong"), 0o600)
	if res := r.run("--token-file", tokFile, "status"); res.code != ExitAuth {
		t.Errorf("bad token exit %d", res.code)
	}
	// daemon down
	var errb bytes.Buffer
	code := Execute(context.Background(), []string{"--config", r.cfg, "--endpoint", "http://127.0.0.1:1", "--timeout", "1s", "status"}, IO{Out: io.Discard, Err: &errb})
	if code != ExitUnavailable || !strings.Contains(errb.String(), "502drive serve") {
		t.Errorf("daemon down: %d %s", code, errb.String())
	}
}

func TestJSONOutputIsCleanAndErrorsGoToStderr(t *testing.T) {
	r := newRig(t)
	res := r.run("status", "--json")
	if res.code != 0 || res.errOut != "" || strings.Contains(res.out, "\x1b") {
		t.Fatalf("%+v", res)
	}
	if decodeData(t, res.out)["ready"] != true {
		t.Fatal("not ready")
	}
	res = r.run("jobs", "get", "nope", "--json")
	if res.out != "" || res.code != ExitNotFound {
		t.Fatalf("error leaked to stdout: %+v", res)
	}
	var e struct {
		Error struct{ Code string } `json:"error"`
	}
	if err := json.Unmarshal([]byte(res.errOut), &e); err != nil || e.Error.Code != "not_found" {
		t.Fatalf("stderr %q", res.errOut)
	}
}

func TestCopyWaitSucceedsAndSubmitAloneReturnsAdmitted(t *testing.T) {
	r := newRig(t)
	r.setupDest()
	src := r.fake.Folder("Project", "root")
	r.fake.File("a.txt", src.ID, 3)
	res := r.run("copy", "https://drive.google.com/drive/folders/"+src.ID, "--wait", "--json")
	if res.code != 0 {
		t.Fatalf("%+v", res)
	}
	if st := decodeData(t, res.out); st["state"] != "succeeded" || st["result_url"] == nil {
		t.Fatalf("%v", st)
	}
	// submit-only returns right away with the job queued/running and exit 0
	// --new asks for a second copy; a plain copy would refresh the first (L2)
	res = r.run("copy", src.ID, "--quiet", "--new")
	if res.code != 0 || !strings.HasPrefix(strings.TrimSpace(res.out), "job_") {
		t.Fatalf("%+v", res)
	}
	// two intentional copies are two collections
	if res = r.run("collections", "list", "--json"); len(decodeData(t, res.out)["items"].([]any)) != 2 {
		t.Fatalf("collections: %s", res.out)
	}
}

func TestWaitExitCodesForPartialAndCancelled(t *testing.T) {
	r := newRig(t)
	r.setupDest()
	src := r.fake.Folder("P", "root")
	r.fake.Folder("sub", src.ID)
	r.fake.Before = func(op string, f drive.File) error {
		if op == "folder" && f.Name == "sub" {
			return &drive.Error{Kind: drive.KindPermission, Status: 403}
		}
		return nil
	}
	res := r.run("copy", src.ID, "--wait")
	if res.code != ExitJobFailed {
		t.Fatalf("partial exit %d: %s", res.code, res.errOut)
	}
	if !strings.Contains(res.out, "PARTIAL") {
		t.Fatalf("out %q", res.out)
	}
}

func TestPipedOutputHasNoColourUnlessExplicit(t *testing.T) {
	r := newRig(t)
	if res := r.run("status"); strings.Contains(res.out, "\x1b") {
		t.Fatal("colour in a pipe")
	}
	if res := r.run("--color", "always", "status"); !strings.Contains(res.out, "\x1b[") {
		t.Fatal("--color always ignored")
	}
	if res := r.run("--color", "always", "--json", "status"); strings.Contains(res.out, "\x1b") {
		t.Fatal("--json must override --color always")
	}
	if res := r.run("--color", "always", "--plain", "status"); strings.Contains(res.out, "\x1b") {
		t.Fatal("--plain must override")
	}
	res := r.runWith("", map[string]string{"TERM": "dumb"}, "--color", "always", "status")
	if strings.Contains(res.out, "\x1b") {
		t.Fatal("TERM=dumb must override")
	}
}

func TestResolveStylePrecedence(t *testing.T) {
	tty := StyleInput{ColorMode: "auto", StdoutTTY: true, StderrTTY: true, Term: "xterm-256color", Width: 100}
	if s := ResolveStyle(tty); !s.Color || s.Plain || !s.Motion {
		t.Fatalf("tty default %+v", s)
	}
	nc := tty
	nc.NoColor = "1"
	if s := ResolveStyle(nc); s.Color || s.Plain || !s.Motion {
		t.Fatalf("NO_COLOR only removes colour: %+v", s)
	}
	nm := tty
	nm.NoMotion = true
	if s := ResolveStyle(nm); s.Motion || !s.Color {
		t.Fatalf("no-motion %+v", s)
	}
	for _, mut := range []func(*StyleInput){func(i *StyleInput) { i.JSON = true }, func(i *StyleInput) { i.Quiet = true }, func(i *StyleInput) { i.Plain = true }, func(i *StyleInput) { i.Term = "dumb" }} {
		in := tty
		in.ColorMode = "always"
		mut(&in)
		if s := ResolveStyle(in); s.Color || s.Motion || !s.Plain {
			t.Fatalf("hard override failed %+v for %+v", s, in)
		}
	}
	pipe := tty
	pipe.StdoutTTY = false
	if s := ResolveStyle(pipe); s.Color || !s.Plain || s.Motion {
		t.Fatalf("pipe %+v", s)
	}
	pipe.ColorMode = "always"
	if s := ResolveStyle(pipe); !s.Color || s.Motion {
		t.Fatalf("explicit always in a pipe: colour but no motion: %+v", s)
	}
}

func TestSanitizeNeutralisesTerminalInjection(t *testing.T) {
	in := "ok\x1b]0;pwned\x07\nline\r‮evil\x9b"
	got := Sanitize(in)
	for _, bad := range []string{"\x1b", "\n", "\r", "\x07", "‮", "\x9b"} {
		if strings.Contains(got, bad) {
			t.Fatalf("%q survived in %q", bad, got)
		}
	}
	if Sanitize("Tài liệu 📁") != "Tài liệu 📁" {
		t.Fatal("unicode names must be kept")
	}
}

func TestHostileDriveNameIsSanitizedInHumanOutputButExactInJSON(t *testing.T) {
	r := newRig(t)
	r.setupDest()
	evil := "evil\x1b[31m\nname"
	src := r.fake.Folder(evil, "root")
	res := r.run("copy", src.ID, "--wait")
	if strings.ContainsAny(res.out, "\x1b") || strings.Count(res.out, "\n") > 12 {
		t.Fatalf("raw control characters reached the terminal: %q", res.out)
	}
	res = r.run("jobs", "list", "--json")
	items := decodeData(t, res.out)["items"].([]any)
	if items[0].(map[string]any)["source_name"] != evil {
		t.Fatal("JSON must carry the exact name")
	}
}

// A proxy that lets one POST /v1/jobs reach the daemon but drops its response.
func TestLostSubmitResponseIsRetriedWithoutSecondJob(t *testing.T) {
	r := newRig(t)
	r.setupDest()
	src := r.fake.Folder("Lost", "root")
	var drops atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		fwd, _ := http.NewRequest(req.Method, r.addr+req.URL.RequestURI(), req.Body)
		fwd.Header = req.Header.Clone()
		resp, err := http.DefaultClient.Do(fwd)
		if err != nil {
			http.Error(w, "x", 502)
			return
		}
		defer resp.Body.Close()
		if req.Method == "POST" && req.URL.Path == "/v1/jobs" && drops.Add(1) <= 2 /* net/http itself retries once because of the Idempotency-Key */ {
			conn, _, _ := w.(http.Hijacker).Hijack()
			conn.Close() // admitted, but the client never hears
			return
		}
		for k, v := range resp.Header {
			w.Header()[k] = v
		}
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	}))
	defer proxy.Close()
	run := func(args ...string) result {
		var out, errb bytes.Buffer
		full := append([]string{"--config", r.cfg, "--endpoint", proxy.URL}, args...)
		code := Execute(context.Background(), full, IO{In: strings.NewReader(""), Out: &out, Err: &errb, Width: 80, Getenv: func(string) string { return "" }})
		return result{code, out.String(), errb.String()}
	}
	first := run("copy", src.ID, "--quiet")
	if first.code != ExitUnavailable || !strings.Contains(first.errOut, "will not create a second job") {
		t.Fatalf("first: %+v", first)
	}
	second := run("copy", src.ID, "--quiet")
	if second.code != 0 {
		t.Fatalf("second: %+v", second)
	}
	list := r.run("jobs", "list", "--json")
	if n := len(decodeData(t, list.out)["items"].([]any)); n != 1 {
		t.Fatalf("%d jobs after a retried submit", n)
	}
}

func TestEventsNDJSON(t *testing.T) {
	r := newRig(t)
	r.setupDest()
	src := r.fake.Folder("E", "root")
	res := r.run("copy", src.ID, "--wait", "--quiet")
	id := strings.TrimSpace(res.out)
	res = r.run("jobs", "events", id, "--json")
	lines := strings.Split(strings.TrimSpace(res.out), "\n")
	if res.code != 0 || len(lines) < 3 {
		t.Fatalf("%+v", res)
	}
	prev := 0.0
	for _, l := range lines {
		d := decodeData(t, l)
		if d["sequence"].(float64) <= prev {
			t.Fatal("sequence must increase")
		}
		prev = d["sequence"].(float64)
	}
}

func TestStdinSourceAndUsageErrors(t *testing.T) {
	r := newRig(t)
	r.setupDest()
	src := r.fake.Folder("S", "root")
	if res := r.runWith(src.ID+"\n", nil, "copy", "--stdin", "--quiet"); res.code != 0 {
		t.Fatalf("%+v", res)
	}
	// Several references are a batch (S6): both lines are admitted, the
	// repeated one is dropped, and the command succeeds.
	if res := r.runWith(src.ID+"\n"+src.ID+"\n", nil, "copy", "--stdin", "--quiet"); res.code != 0 || strings.Count(strings.TrimSpace(res.out), "\n") != 0 {
		t.Fatalf("batch with a repeated ref: %+v", res)
	}
	if res := r.runWith("", nil, "copy", src.ID, "--stdin"); res.code != ExitUsage {
		t.Fatalf("both: %+v", res)
	}
}

func TestSetupCreatesPrivateConfigOnceAndHidesToken(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "c")
	os.Mkdir(dir, 0o700)
	cfg := filepath.Join(dir, "config.json")
	var out bytes.Buffer
	if code := Execute(context.Background(), []string{"--config", cfg, "setup"}, IO{Out: &out, Err: io.Discard}); code != 0 {
		t.Fatal(out.String())
	}
	tok, _ := os.ReadFile(filepath.Join(dir, "api-token"))
	if strings.Contains(out.String(), strings.TrimSpace(string(tok))) {
		t.Fatal("token printed")
	}
	out.Reset()
	if code := Execute(context.Background(), []string{"--config", cfg, "setup"}, IO{Out: &out, Err: io.Discard}); code != 0 || !strings.Contains(out.String(), "existing") {
		t.Fatalf("second setup: %s", out.String())
	}
	after, _ := os.ReadFile(filepath.Join(dir, "api-token"))
	if string(after) != string(tok) {
		t.Fatal("setup rotated the token")
	}
}

func (r *rig) menu(stdin string, args ...string) result {
	r.t.Helper()
	var out, errb bytes.Buffer
	full := append([]string{"--config", r.cfg, "--endpoint", r.addr, "--plain"}, args...)
	full = append(full, "menu")
	code := Execute(context.Background(), full, IO{In: strings.NewReader(stdin), Out: &out, Err: &errb, StdinTTY: true, StdoutTTY: true, Width: 40,
		Getenv: func(string) string { return "" }})
	return result{code, out.String(), errb.String()}
}

func TestMenuNeedsATerminal(t *testing.T) {
	r := newRig(t)
	if res := r.run("menu"); res.code != ExitUsage || !strings.Contains(res.errOut, "interactive terminal") {
		t.Fatalf("%+v", res)
	}
}

func TestMenuCopyFlowAndCleanExit(t *testing.T) {
	r := newRig(t)
	r.setupDest()
	src := r.fake.Folder("Menu Source", "root")
	res := r.menu("1\n" + src.ID + "\n1\n0\n")
	if res.code != 0 || !strings.Contains(res.out, "Accepted: Menu Source") || !strings.Contains(res.out, "SUCCEEDED") {
		t.Fatalf("%+v", res)
	}
	if n := len(decodeData(t, r.run("jobs", "list", "--json").out)["items"].([]any)); n != 1 {
		t.Fatalf("jobs %d", n)
	}
	// EOF and "0" both leave without error or hanging
	for _, in := range []string{"", "0\n", "6\n1\n\n0\n"} {
		if res := r.menu(in); res.code != 0 {
			t.Fatalf("input %q: %+v", in, res)
		}
	}
}

func TestMenuCopiesToMyDriveWhenNoDestinationIsConfigured(t *testing.T) {
	r := newRig(t) // no destination configured: "My Drive" is created on demand
	src := r.fake.Folder("X", "root")
	res := r.menu("1\n" + src.ID + "\n1\n0\n")
	if res.code != 0 || !strings.Contains(res.out, "Accepted: X") {
		t.Fatalf("%+v", res)
	}
}

func TestMenuFitsNarrowTerminals(t *testing.T) {
	r := newRig(t)
	r.setupDest()
	src := r.fake.Folder("A very long folder name that would overflow a forty column terminal", "root")
	r.run("copy", src.ID, "--wait", "--quiet")
	res := r.menu("2\n0\n0\n", "--plain", "--ascii")
	for _, l := range strings.Split(res.out, "\n") {
		if len([]rune(l)) > 42 {
			t.Fatalf("line too wide for a 40-column terminal: %q", l)
		}
	}
	if strings.ContainsAny(res.out, "╭╮╰╯│─") {
		t.Fatal("--ascii must not print box-drawing characters")
	}
}
