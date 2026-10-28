package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// toHistory opens Settings → Clear history through the keys a user would press.
func toHistory(t *testing.T, d *driveFake) Model {
	t.Helper()
	m := driveModel(t, d, 100, 30)
	m, _ = update(t, m, key("6"))
	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyDown}) // off the tab bar
	for i := 0; i < len(m.settingsRows())-1; i++ {
		m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyDown})
	}
	if row, _ := m.currentSettingsRow(); row != rowClearHistory {
		t.Fatalf("the last Settings row should be Clear history, got %v", row)
	}
	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.screen != screenHistory {
		t.Fatalf("Enter should open the history screen, got %v", m.screen)
	}
	return m
}

func TestClearHistoryAlwaysPreviewsThenAsksThenClears(t *testing.T) {
	d := newDriveFake()
	d.hist = HistoryClearResult{Hidden: 12, DeletedJobs: 3, DeletedEvents: 40}
	m := toHistory(t, d)
	if v := ansi.Strip(strings.Join(m.historyView(), "\n")); !strings.Contains(v, "Older than 7 days") || !strings.Contains(v, "Every finished job") {
		t.Fatalf("the ranges are a fixed list:\n%s", v)
	}
	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyDown}) // Older than 30 days
	m, cmd := update(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = run(t, m, cmd)
	if got := strings.Join(d.recorded(), " "); !strings.Contains(got, "history:30:[]:true") || strings.Contains(got, ":false") {
		t.Fatalf("the first call is a preview: %s", got)
	}
	v := ansi.Strip(strings.Join(m.historyView(), "\n"))
	if m.hist.page != histAsk || !strings.Contains(v, "Will clear 12 jobs") || !strings.Contains(v, "not touched") {
		t.Fatalf("the preview must say what will happen:\n%s", v)
	}
	// n steps back without clearing.
	m, _ = update(t, m, key("n"))
	if m.hist.page != histScope || strings.Contains(strings.Join(d.recorded(), " "), ":false") {
		t.Fatalf("n must go back and clear nothing: %v", d.recorded())
	}
	m, cmd = update(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = run(t, m, cmd)
	m, cmd = update(t, m, key("y"))
	m = run(t, m, cmd)
	if !strings.Contains(strings.Join(d.recorded(), " "), "history:30:[]:false") {
		t.Fatalf("y applies: %v", d.recorded())
	}
	if v = ansi.Strip(strings.Join(m.historyView(), "\n")); !strings.Contains(v, "Cleared 12 jobs") {
		t.Fatalf("the result is shown:\n%s", v)
	}
	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.screen != screenSettings || m.hist != nil {
		t.Fatalf("Esc returns to Settings: %v", m.screen)
	}
}

func TestClearHistoryWithNothingToClearSaysSoAndNeverAsks(t *testing.T) {
	d := newDriveFake()
	m := toHistory(t, d)
	m, cmd := update(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = run(t, m, cmd)
	if v := ansi.Strip(strings.Join(m.historyView(), "\n")); m.hist.page != histResult || !strings.Contains(v, "Nothing to clear") {
		t.Fatalf("expected the nothing-to-clear result:\n%s", v)
	}
	if strings.Contains(strings.Join(d.recorded(), " "), ":false") {
		t.Fatalf("nothing to clear must never write: %v", d.recorded())
	}
}

func TestClearHistoryLeftArrowOnTheRowDoesNotOpenAndEscBacksOut(t *testing.T) {
	d := newDriveFake()
	m := toHistory(t, d)
	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyLeft})
	if m.screen != screenSettings {
		t.Fatalf("← on the scope list goes back to Settings, got %v", m.screen)
	}
	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyLeft}) // ← on a Settings row does not open it
	if m.screen != screenSettings {
		t.Fatalf("← must not open the history screen: %v", m.screen)
	}
}

func TestJobsXClearsOnlyFinishedJobsAfterAConfirm(t *testing.T) {
	d := newDriveFake()
	d.hist = HistoryClearResult{Hidden: 1, Skipped: 0}
	m := driveModel(t, d, 100, 30)
	m, _ = update(t, m, key("2"))
	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyDown})
	m.snap.Jobs = append(m.snap.Jobs, Job{ID: "job_9", SourceName: "Done", State: "succeeded", Revision: 1})
	m.cursor = 0 // job_1 is running
	m, _ = update(t, m, key("x"))
	if m.confirm != nil || !strings.Contains(m.status, "finished") {
		t.Fatalf("an unfinished job cannot be cleared: confirm=%v status=%q", m.confirm, m.status)
	}
	m.cursor = len(m.snap.Jobs) - 1
	m, _ = update(t, m, key("x"))
	if m.confirm == nil || !strings.Contains(m.confirm.prompt, "Clear 1 finished") {
		t.Fatalf("x on a finished job asks first: %+v", m.confirm)
	}
	m, cmd := update(t, m, key("y"))
	m = run(t, m, cmd)
	if got := strings.Join(d.recorded(), " "); !strings.Contains(got, "history:-1:[") || !strings.Contains(got, ":false") {
		t.Fatalf("the clear goes out by job id: %s", got)
	}
}
