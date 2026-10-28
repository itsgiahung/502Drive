package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func tabModel(t *testing.T, w, h int) Model {
	t.Helper()
	d := newDriveFake()
	return driveModel(t, d, w, h)
}

func TestManyKeysSwitchTheSameTabs(t *testing.T) {
	m := tabModel(t, 100, 30)
	steps := []struct {
		msg  tea.Msg
		want screen
	}{
		{tea.KeyMsg{Type: tea.KeyTab}, screenJobs},
		{tea.KeyMsg{Type: tea.KeyLeft}, screenJobs}, // ← steps out to the tab bar, same tab
		{tea.KeyMsg{Type: tea.KeyRight}, screenLibrary},
		{key("]"), screenDestination},
		{tea.KeyMsg{Type: tea.KeyShiftTab}, screenLibrary},
		{tea.KeyMsg{Type: tea.KeyLeft}, screenLibrary}, // out to the bar again
		{tea.KeyMsg{Type: tea.KeyRight}, screenDestination},
		{tea.KeyMsg{Type: tea.KeyLeft}, screenLibrary},
		{key("["), screenJobs},
		{key("["), screenHome},
		{key("5"), screenAccount},
	}
	m, _ = update(t, m, key("1"))
	for i, s := range steps {
		m, _ = update(t, m, s.msg)
		if m.screen != s.want {
			t.Fatalf("step %d: screen = %v, want %v", i, m.screen, s.want)
		}
	}
}

func TestTabWrapsAroundBothWays(t *testing.T) {
	m := tabModel(t, 100, 30)
	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.screen != screenDrive {
		t.Fatalf("Shift+Tab from the first tab should wrap to the last, got %v", m.screen)
	}
	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.screen != screenHome {
		t.Fatalf("Tab from the last tab should wrap to the first, got %v", m.screen)
	}
}

func TestArrowsKeepTheirJobWhereTheyAreNeeded(t *testing.T) {
	m := tabModel(t, 100, 30)
	m, cmd := update(t, m, key("7")) // Drive: arrows go through folders
	m = run(t, m, cmd)
	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyRight})
	if m.screen != screenDrive {
		t.Fatalf("→ in the Drive browser must not leave it: %v", m.screen)
	}
	m, _ = update(t, m, key("6")) // Settings: arrows change the value
	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyRight})
	if m.screen != screenSettings {
		t.Fatalf("→ in Settings changes the language, it must not switch tab: %v", m.screen)
	}
	// Home with text in the prompt: arrows move the text cursor.
	m, _ = update(t, m, key("1"))
	m = typeText(t, m, "abc")
	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyLeft})
	if m.screen != screenHome {
		t.Fatalf("← while typing must move the cursor, got %v", m.screen)
	}
}

func TestClickingATabSwitchesToIt(t *testing.T) {
	m := tabModel(t, 100, 30)
	spans, _ := m.tabLayout()
	var jobs tabSpan
	for _, s := range spans {
		if tabs[s.index].screen == screenJobs {
			jobs = s
		}
	}
	m, _ = update(t, m, tea.MouseMsg{X: jobs.x0 + 1, Y: 0, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if m.screen != screenJobs {
		t.Fatalf("a click on the Jobs tab opened %v", m.screen)
	}
	m, _ = update(t, m, tea.MouseMsg{X: 5, Y: 3, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if m.screen != screenJobs {
		t.Fatal("a click below the bar must not switch tabs")
	}
}

func TestTheTabBarFitsEveryWidthAndNamesTheActiveTab(t *testing.T) {
	for _, w := range []int{30, 40, 60, 80, 120} {
		m := tabModel(t, w, 30)
		m, _ = update(t, m, key("3"))
		bar := lines(m.View())[0]
		if got := ansi.StringWidth(bar); got > w {
			t.Fatalf("width %d: the tab bar is %d columns wide: %q", w, got, bar)
		}
		if !strings.Contains(bar, "Library") && !strings.Contains(bar, "3") {
			t.Fatalf("width %d: the active tab is not visible: %q", w, bar)
		}
	}
	m := tabModel(t, 100, 30)
	m, _ = update(t, m, key("2"))
	if bar := lines(m.View())[0]; !strings.Contains(bar, "[2 Jobs]") {
		t.Fatalf("without colour the active tab is bracketed: %q", bar)
	}
}

func TestTheCatWorksOnTheJobsTabWhileSomethingCopies(t *testing.T) {
	d := newDriveFake()
	m := New(d.acts(), Options{Width: 120, Height: 34, Language: "en", Color: true})
	m = run(t, m, m.fetch())
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 34})
	m, _ = update(t, m, key("2"))
	if len(m.jobsBanner()) == 0 {
		t.Fatal("a running job should bring the cat to the Jobs tab")
	}
	if !m.animating() {
		t.Fatal("the cat must animate on the Jobs tab while a job runs")
	}
	if got := len(lines(m.View())); got != 34 {
		t.Fatalf("the view is %d lines, want 34", got)
	}
	d.snap.Jobs[0].State = "succeeded"
	d.snap.Status.Running = 0
	m = run(t, m, m.fetch())
	if len(m.jobsBanner()) != 0 || m.animating() {
		t.Fatal("nothing is running: no banner and no animation")
	}
}

func TestSettingsStartsOnTheTabBar(t *testing.T) {
	m := tabModel(t, 100, 30)
	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyShiftTab}) // Drive → Settings
	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyShiftTab})
	if !m.tabFocus {
		t.Fatal("Settings must open with the cursor on the tab bar")
	}
	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyRight})
	if m.screen != screenDrive {
		t.Fatalf("→ on the tab bar must go to the next tab, got %v", m.screen)
	}
	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyLeft})
	m, _ = update(t, m, key("down"))
	if m.tabFocus || m.screen != screenSettings {
		t.Fatal("↓ must go into the page")
	}
	m, _ = update(t, m, key("up"))
	if !m.tabFocus {
		t.Fatal("↑ at the top of the page must return to the tab bar")
	}
	if !strings.Contains(m.View(), "‹6") {
		t.Fatalf("the tab bar must mark the cursor:\n%s", m.View())
	}
}

func TestHomeShowsGoogleThrottlingOnlyWhenItHappens(t *testing.T) {
	d := newDriveFake()
	m := driveModel(t, d, 100, 30)
	m, _ = update(t, m, key("1"))
	if strings.Contains(m.View(), "throttled") {
		t.Fatal("no throttle line expected while nothing is limited")
	}
	d.snap.Status.Workers, d.snap.Status.Throttled5m = 2, 3
	m = run(t, m, m.fetch())
	if !strings.Contains(m.View(), "Workers: 2") {
		t.Fatalf("throttle line missing:\n%s", m.View())
	}
}

// The cursor is in the tab bar or in the page, and the screen says which. From
// the page, → opens the selected row and ← steps out to the bar; ←/→ switch tabs
// only while the bar has the cursor.
func TestArrowsOpenRowsInThePageAndSwitchTabsOnlyFromTheBar(t *testing.T) {
	m := tabModel(t, 100, 30)
	m, _ = update(t, m, key("2"))
	if m.tabFocus {
		t.Fatal("a digit must leave the cursor in the page")
	}
	before := m.cursor
	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyRight})
	if m.screen != screenJob {
		t.Fatalf("→ on a selected job must open it, got %v", m.screen)
	}
	if got := m.snap.Jobs[m.cursor].ID; got != m.snap.Jobs[before].ID {
		t.Fatal("→ opened a different job than the selected one")
	}
	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyLeft})
	if m.screen != screenJobs {
		t.Fatalf("← from a job must return to the list, got %v", m.screen)
	}

	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyLeft})
	if !m.tabFocus || m.screen != screenJobs {
		t.Fatal("← from the list must step out to the tab bar without changing tab")
	}
	if strings.Contains(m.View(), "> ") && strings.Contains(strings.Join(lines(m.View())[3:], "\n"), "> SUCC") {
		t.Fatalf("with the cursor on the tab bar no row may look selected:\n%s", m.View())
	}
	if !strings.Contains(m.View(), "‹2") {
		t.Fatalf("the bar must show it has the cursor:\n%s", m.View())
	}
	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyRight})
	if m.screen != screenLibrary {
		t.Fatalf("→ on the bar must switch tab, got %v", m.screen)
	}
	m, _ = update(t, m, key("down"))
	if m.tabFocus {
		t.Fatal("↓ must go from the bar into the page")
	}
	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyRight})
	if m.screen == screenDestination {
		t.Fatal("→ from the page switched tab; it must open the selected row")
	}
}
