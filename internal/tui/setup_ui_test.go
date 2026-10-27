package tui

import (
	"strings"
	"testing"
)

func TestSetupHomeDisappearsWhenReady(t *testing.T) {
	m := New(Actions{}, Options{NoMotion: true, Language: "vi", Width: 100})
	m.snap.Setup = &SetupState{Checks: []Check{{Name: "database", Status: "passed"}, {Name: "oauth_client", Status: "failed"}, {Name: "account", Status: "failed"}, {Name: "destination", Status: "failed"}}}
	v := m.View()
	if !strings.Contains(v, "Bắt đầu") || !strings.Contains(v, "✗") || !strings.Contains(v, "✓") {
		t.Fatal(v)
	}
	t.Log("U1 snapshot:\n" + v)
	m.snap.Setup.Ready = true
	if strings.Contains(m.View(), "OAuth:") {
		t.Fatal("setup did not disappear")
	}
}
