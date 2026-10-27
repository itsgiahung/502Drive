package cli

import (
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
