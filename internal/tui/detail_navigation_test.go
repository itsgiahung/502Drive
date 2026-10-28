package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestCollectionArrowsFocusSyncWithoutChangingTheCollection(t *testing.T) {
	m, fake := newTestModel(t, Options{Width: 100, Language: "en"})
	m.snap.Collections = append(m.snap.Collections, Collection{ID: "col_other", SourceName: "Other"})
	m, _ = update(t, m, key("3"))
	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyDown}) // the cursor starts on the tab bar
	m, _ = update(t, m, key("enter"))
	m, _ = update(t, m, key("down"))
	c, ok := m.currentCollection()
	if !ok || c.ID != "col_1" || !strings.Contains(m.View(), "> Sync") {
		t.Fatalf("down must focus Sync inside Project Alpha: %+v\n%s", c, m.View())
	}
	m, _ = update(t, m, key("enter"))
	if !strings.Contains(m.View(), "Every hour") || len(fake.recorded()) != 0 {
		t.Fatalf("enter should open sync choices without writing:\n%s", m.View())
	}
	m, _ = update(t, m, key("down"))
	m, cmd := update(t, m, key("enter"))
	m = run(t, m, cmd)
	if got := strings.Join(fake.recorded(), ","); got != "preset:hourly:col_1" {
		t.Fatalf("chosen sync action targets the original collection: %s", got)
	}
	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyLeft})
	if m.screen != screenLibrary {
		t.Fatalf("left must return to Library, got %v", m.screen)
	}
}

func TestCollectionDetailSurvivesSnapshotReordering(t *testing.T) {
	m, _ := newTestModel(t, Options{Width: 100, Language: "en"})
	m, _ = update(t, m, key("3"))
	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyDown}) // the cursor starts on the tab bar
	m, _ = update(t, m, key("enter"))
	snap := m.snap
	snap.Collections = append([]Collection{{ID: "col_other", SourceName: "Other"}}, snap.Collections...)
	m, _ = update(t, m, snapshotMsg{snap: snap})
	if c, ok := m.currentCollection(); !ok || c.ID != "col_1" {
		t.Fatalf("a refresh changed the open collection: %+v", c)
	}
}

func TestDetailArrowsNeverSelectAnotherJobAndLeftGoesBack(t *testing.T) {
	m, _ := newTestModel(t, Options{Width: 80})
	m, _ = update(t, m, key("2"))
	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyDown}) // the cursor starts on the tab bar
	m, _ = update(t, m, key("enter"))
	for _, k := range []string{"down", "j", "pgdown", "end", "up"} {
		m, _ = update(t, m, key(k))
		if j, ok := m.currentJob(); !ok || j.ID != "job_1" {
			t.Fatalf("%s changed the open job: %+v", k, j)
		}
	}
	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyLeft})
	if m.screen != screenJobs {
		t.Fatalf("left must return to Jobs, got %v", m.screen)
	}
}

func TestRemovedJobDetailNeverTargetsItsReplacement(t *testing.T) {
	m, _ := newTestModel(t, Options{Width: 80})
	m = sendKeys(t, m, "2", "down", "enter")
	snap := m.snap
	snap.Jobs = snap.Jobs[1:]
	m, _ = update(t, m, snapshotMsg{snap: snap})
	if m.screen != screenJobs {
		t.Fatalf("a removed job must return to Jobs, not target its replacement: %v", m.screen)
	}
}

func TestLeftClosesSyncChoicesBeforeLeavingCollection(t *testing.T) {
	m, _ := newTestModel(t, Options{Width: 100, Language: "en"})
	m = sendKeys(t, m, "3", "down", "enter", "down", "enter")
	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyLeft})
	if m.screen != screenCollection || !strings.Contains(m.View(), "> Sync") {
		t.Fatalf("left must return from sync choices to their row:\n%s", m.View())
	}
}

func TestCollectionFocusStaysVisibleInAShortTerminal(t *testing.T) {
	m, _ := newTestModel(t, Options{Width: 50, Height: 10, Language: "en"})
	m = sendKeys(t, m, "3", "down", "enter", "end")
	if !strings.Contains(m.View(), "> Protection") {
		t.Fatalf("the selected row was clipped:\n%s", m.View())
	}
}

func TestNestedViewsReturnToTheSameObjectAfterRefresh(t *testing.T) {
	for _, view := range []string{"report", "filter", "audit"} {
		t.Run(view, func(t *testing.T) {
			m, _ := newTestModel(t, Options{Width: 100})
			switch view {
			case "report":
				m.screen, m.rep = screenReport, &reportView{jobID: "job_1"}
			case "filter":
				m = m.openFilter(m.snap.Collections[0])
			case "audit":
				m.screen, m.ab = screenAudit, &auditBrowser{collection: m.snap.Collections[0]}
			}
			snap := m.snap
			snap.Jobs = append([]Job{{ID: "job_other"}}, snap.Jobs...)
			snap.Collections = append([]Collection{{ID: "col_other"}}, snap.Collections...)
			m, _ = update(t, m, snapshotMsg{snap: snap})
			m, _ = update(t, m, key("esc"))
			if view == "report" {
				if j, ok := m.currentJob(); !ok || j.ID != "job_1" {
					t.Fatalf("report returned to a different job: %+v", j)
				}
			} else if c, ok := m.currentCollection(); !ok || c.ID != "col_1" {
				t.Fatalf("%s returned to a different collection: %+v", view, c)
			}
		})
	}
}

func TestLeftDismissesNestedViewsWithoutLeavingTheirParent(t *testing.T) {
	for _, view := range []string{"states", "audit", "report", "copy"} {
		t.Run(view, func(t *testing.T) {
			m := New(Actions{}, Options{NoMotion: true})
			m.snap = sampleSnapshot()
			want := screenJobs
			switch view {
			case "states":
				m.screen = screenJobs
				m = m.openStatePicker()
			case "audit":
				m.screen, m.ab = screenAudit, &auditBrowser{collection: Collection{ID: "col_1"}}
				want = screenCollection
			case "report":
				m.screen, m.rep = screenReport, &reportView{jobID: "job_1"}
				want = screenJob
			case "copy":
				m.screen, m.cp = screenCopy, &copyFlow{from: screenLibrary}
				want = screenLibrary
			}
			m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyLeft})
			if m.screen != want || m.statePick != nil {
				t.Fatalf("left from %s did not return to its parent: %v", view, m.screen)
			}
		})
	}
}
