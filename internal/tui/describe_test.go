package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestSettingsExplainsTheHighlightedRow(t *testing.T) {
	m := toHistory(t, newDriveFake())
	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyEsc}) // back to Settings, cursor on Clear history
	v := ansi.Strip(strings.Join(m.settingsView(), "\n"))
	if !strings.Contains(v, "Clear finished jobs from the history now") {
		t.Fatalf("the highlighted row should be explained:\n%s", v)
	}
	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyUp})
	if v = ansi.Strip(strings.Join(m.settingsView(), "\n")); strings.Contains(v, "Clear finished jobs") || !strings.Contains(v, "The language of the app") {
		t.Fatalf("the explanation follows the cursor:\n%s", v)
	}
}

func TestEveryDescribedRowHasText(t *testing.T) {
	m := tabModel(t, 100, 30)
	for _, k := range syncChoiceDesc {
		if m.t(k) == string(k) {
			t.Errorf("sync choice text %s missing", k)
		}
	}
	for _, k := range detailRowDesc {
		if m.t(k) == string(k) {
			t.Errorf("detail row text %s missing", k)
		}
	}
	for _, k := range settingsRowDesc {
		if m.t(k) == string(k) {
			t.Errorf("settings row text %s missing", k)
		}
	}
}
