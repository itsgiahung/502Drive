package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTrayWriteIconsNeedsNoDaemonOrConfig(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "icons")
	var out, errb strings.Builder
	code := Execute(t.Context(), []string{"--config", filepath.Join(t.TempDir(), "missing.json"), "tray", "--write-icons", dir},
		IO{In: strings.NewReader(""), Out: &out, Err: &errb})
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	for _, name := range []string{"502drive.png", "502drive.ico"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil || info.Size() == 0 {
			t.Fatalf("%s missing or empty: %v", name, err)
		}
	}
	if !strings.Contains(out.String(), "502drive.png") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestTrayWithoutADesktopSessionFailsCleanly(t *testing.T) {
	r := newRig(t)
	res := r.runWith("", map[string]string{}, "tray")
	if res.code != ExitInternal {
		t.Fatalf("exit %d: %s", res.code, res.errOut)
	}
	if !strings.Contains(res.errOut, "DBUS_SESSION_BUS_ADDRESS") || !strings.Contains(res.errOut, "502drive menu") {
		t.Fatalf("message = %q", res.errOut)
	}
}

func TestAssetsWriteIconsAndCompletions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "assets")
	var out, errb strings.Builder
	code := Execute(t.Context(), []string{"--config", filepath.Join(t.TempDir(), "missing.json"), "assets", "--dir", dir},
		IO{In: strings.NewReader(""), Out: &out, Err: &errb})
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	for _, name := range []string{"502drive.png", "502drive.ico", "502drive.bash", "_502drive", "502drive.fish", "502drive.ps1"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil || info.Size() == 0 {
			t.Fatalf("%s missing or empty: %v", name, err)
		}
	}
	// Cobra's dynamic completions do not embed subcommand names, so assert the
	// generated registration for each shell instead.
	for _, c := range []struct{ file, want string }{
		{"502drive.bash", "__start_502drive"},
		{"_502drive", "compdef _502drive 502drive"},
		{"502drive.fish", "complete -c 502drive"},
		{"502drive.ps1", "502drive"},
	} {
		raw, err := os.ReadFile(filepath.Join(dir, c.file))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), c.want) {
			t.Fatalf("%s does not look like a %q completion", c.file, c.want)
		}
	}
	var raw, errb2 strings.Builder
	if code = Execute(t.Context(), []string{"assets"}, IO{In: strings.NewReader(""), Out: &raw, Err: &errb2}); code != ExitUsage {
		t.Fatalf("missing --dir exit = %d", code)
	}
}

func TestAssetsCommandIsHidden(t *testing.T) {
	var out, errb strings.Builder
	if code := Execute(t.Context(), []string{"--help"}, IO{In: strings.NewReader(""), Out: &out, Err: &errb}); code != ExitOK {
		t.Fatalf("help exit %d", code)
	}
	help := out.String()
	if !strings.Contains(help, "tray") {
		t.Fatalf("tray missing from help:\n%s", help)
	}
	if strings.Contains(help, "assets") {
		t.Fatalf("the build helper is listed in help:\n%s", help)
	}
}
