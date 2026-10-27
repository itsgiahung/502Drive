package cli

import (
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
