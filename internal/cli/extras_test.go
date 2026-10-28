package cli

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
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

func TestUpdateCheckReportsNewerWithoutInstalling(t *testing.T) {
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
	if code != ExitOK || !strings.Contains(out.String(), "v9.0.0") || !strings.Contains(out.String(), "update --install") {
		t.Fatalf("exit %d out %q err %q", code, out.String(), errb.String())
	}
}

// fakeRelease serves the release API, checksums.txt and one archive holding a
// "502drive" binary for this platform.
func fakeRelease(t *testing.T, binary []byte, corrupt bool) *httptest.Server {
	t.Helper()
	var arch bytes.Buffer
	name := fmt.Sprintf("502drive_9.0.0_%s_%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		zw := zip.NewWriter(&arch)
		for _, n := range []string{"502drive.exe", "502drive-tray.exe"} {
			w, _ := zw.Create(n)
			w.Write(binary)
		}
		zw.Close()
		name += ".zip"
	} else {
		gz := gzip.NewWriter(&arch)
		tw := tar.NewWriter(gz)
		tw.WriteHeader(&tar.Header{Name: "502drive", Mode: 0o755, Size: int64(len(binary)), Typeflag: tar.TypeReg})
		tw.Write(binary)
		tw.Close()
		gz.Close()
		name += ".tar.gz"
	}
	sum := sha256.Sum256(arch.Bytes())
	digest := hex.EncodeToString(sum[:])
	if corrupt {
		digest = strings.Repeat("0", 64)
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases/latest"):
			w.Write([]byte(`{"tag_name":"v9.0.0","html_url":"https://example.test/release"}`))
		case strings.HasSuffix(r.URL.Path, "/checksums.txt"):
			fmt.Fprintf(w, "%s  %s\n", digest, name)
		case strings.HasSuffix(r.URL.Path, "/"+name):
			w.Write(arch.Bytes())
		default:
			http.NotFound(w, r)
		}
	}))
}

func runUpdate(t *testing.T, ts *httptest.Server, target string, args ...string) (int, string, string) {
	t.Helper()
	old := Version
	Version = "1.0.0"
	t.Cleanup(func() { Version = old })
	env := func(k string) string {
		switch k {
		case "DRIVE502_UPDATE_API", "DRIVE502_RELEASE_BASE":
			return ts.URL
		case "DRIVE502_UPDATE_TARGET":
			return target
		case "DRIVE502_ENDPOINT":
			return "http://127.0.0.1:1" // no daemon
		}
		return ""
	}
	var out, errb strings.Builder
	code := Execute(t.Context(), append([]string{"update"}, args...), IO{In: strings.NewReader(""), Out: &out, Err: &errb, Getenv: env})
	return code, out.String(), errb.String()
}

// --install verifies the archive against checksums.txt, swaps the binary and
// keeps the old one as .prev. A bad checksum changes nothing, scripts must say
// --yes, and a package-managed path is left to its package manager.
func TestUpdateInstallVerifiesAndSwapsTheBinary(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "502drive")
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}

	bad := fakeRelease(t, []byte("new"), true)
	defer bad.Close()
	if code, _, _ := runUpdate(t, bad, target, "--install", "--yes"); code == ExitOK {
		t.Fatal("a checksum mismatch was installed")
	}
	if b, _ := os.ReadFile(target); string(b) != "old" {
		t.Fatal("a failed update touched the binary")
	}

	good := fakeRelease(t, []byte("new"), false)
	defer good.Close()
	if code, _, errOut := runUpdate(t, good, target, "--install"); code != ExitUsage || !strings.Contains(errOut, "--yes") {
		t.Fatalf("without a terminal --install must ask for --yes: %d %q", code, errOut)
	}
	code, out, errOut := runUpdate(t, good, target, "--install", "--yes")
	if code != ExitOK || !strings.Contains(out, "checksum verified") {
		t.Fatalf("install: %d %q %q", code, out, errOut)
	}
	if b, _ := os.ReadFile(target); string(b) != "new" {
		t.Fatalf("binary = %q, want the new one", b)
	}
	if b, _ := os.ReadFile(target + ".prev"); string(b) != "old" {
		t.Fatalf("previous binary = %q, want it kept", b)
	}

	if code, _, errOut := runUpdate(t, good, "/usr/bin/502drive", "--install", "--yes"); code != ExitUsage || !strings.Contains(errOut, "package") {
		t.Fatalf("package-managed path: %d %q", code, errOut)
	}
}

// `update --restart` backs up into <config>/backups/before-TAG-…; the backup
// command creates only the last element, so the folder must be made first
// (the first real run failed here and left the daemon stopped).
func TestUpdateBackupPathCreatesThePrivateBackupsFolder(t *testing.T) {
	cfg := t.TempDir()
	got, err := updateBackupPath(cfg, "v1.2.3", time.Date(2026, 10, 28, 12, 43, 53, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(cfg, "backups", "before-v1.2.3-20261028-124353"); got != want {
		t.Fatalf("path = %s, want %s", got, want)
	}
	st, err := os.Stat(filepath.Join(cfg, "backups"))
	if err != nil || !st.IsDir() {
		t.Fatalf("backups folder missing: %v", err)
	}
	if runtime.GOOS != "windows" && st.Mode().Perm() != 0o700 {
		t.Fatalf("backups folder mode = %o, want 700", st.Mode().Perm())
	}
	if _, err = os.Stat(got); err == nil {
		t.Fatal("the backup folder itself must be left for the backup command to create")
	}
}

// With no daemon running, `update --restart` still backs the data up before
// the new version's first start (it may upgrade the database).
func TestUpdateRestartBacksUpEvenWhenNoDaemonRuns(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as the new binary")
	}
	dir := t.TempDir()
	log := filepath.Join(dir, "args.log")
	target := filepath.Join(dir, "502drive")
	script := "#!/bin/sh\necho \"$@\" >> '" + log + "'\n"
	if err := os.WriteFile(target, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	var out, errb strings.Builder
	cfg := filepath.Join(t.TempDir(), "config.json")
	a := &App{io: IO{Out: &out, Err: &errb}, g: global{configPath: cfg}}
	a.backupBeforeFirstStart(t.Context(), target, "v9.9.9")
	b, _ := os.ReadFile(log)
	if !strings.Contains(string(b), "backup ") || !strings.Contains(string(b), "before-v9.9.9-") {
		t.Fatalf("the new binary was not asked to back up: %q (err %s)", b, errb.String())
	}
	if !strings.Contains(out.String(), "backup of the data is in") {
		t.Fatalf("the user is told where the backup is: %q", out.String())
	}
}

func sigApp(require string, errb *strings.Builder) *App {
	return &App{io: IO{Out: io.Discard, Err: errb, Getenv: func(k string) string {
		if k == "DRIVE502_REQUIRE_SIGNATURE" {
			return require
		}
		return ""
	}}}
}

// A signature bundle that cosign rejects always stops the update; a missing
// cosign only stops it when the signature was made mandatory.
func TestUpdateSignatureCheck(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as cosign")
	}
	bundle := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("{}")) }))
	defer bundle.Close()

	t.Setenv("PATH", t.TempDir()) // no cosign
	var errb strings.Builder
	if err := sigApp("", &errb).verifyReleaseSignature(t.Context(), bundle.URL, []byte("sums")); err != nil || !strings.Contains(errb.String(), "Signature not checked") {
		t.Fatalf("without cosign the update proceeds with a note: %v %q", err, errb.String())
	}
	if err := sigApp("1", &errb).verifyReleaseSignature(t.Context(), bundle.URL, []byte("sums")); err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("a required signature cannot be skipped: %v", err)
	}

	bin := t.TempDir()
	fake := filepath.Join(bin, "cosign")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\necho bad signature >&2\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	if err := sigApp("", &errb).verifyReleaseSignature(t.Context(), bundle.URL, []byte("sums")); err == nil || !strings.Contains(err.Error(), "did not verify") {
		t.Fatalf("a rejected signature must stop the update: %v", err)
	}
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := sigApp("", &errb).verifyReleaseSignature(t.Context(), bundle.URL, []byte("sums")); err != nil {
		t.Fatalf("a verified signature passes: %v", err)
	}
}
