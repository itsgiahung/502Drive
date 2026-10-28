package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// openProtectionScreen drives the collection detail to the Protection row.
func openProtectionScreen(t *testing.T, m Model) Model {
	t.Helper()
	m = sendKeys(t, m, "3", "enter", "end")
	if !strings.Contains(m.View(), "> Protection") {
		t.Fatalf("the detail screen has no Protection row:\n%s", m.View())
	}
	m, cmd := update(t, m, key("enter"))
	m = run(t, m, cmd)
	if m.screen != screenProtect {
		t.Fatalf("Enter on Protection opened screen %v", m.screen)
	}
	return m
}

func TestProtectionMenuOpensAndClosesWithoutWriting(t *testing.T) {
	m, fake := newTestModel(t, Options{Width: 100, Language: "en"})
	m = openProtectionScreen(t, m)
	view := m.View()
	for _, want := range []string{"Protection and restore", "Mirror source deletions", "Automatic snapshots", "30 days", "10 unpinned", "Capture a snapshot now", "Snapshot list", "Restore…"} {
		if !strings.Contains(view, want) {
			t.Fatalf("protection menu misses %q:\n%s", want, view)
		}
	}
	if got := fake.recorded(); len(got) != 0 {
		t.Fatalf("opening the menu wrote something: %v", got)
	}
	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.screen != screenCollection || m.prot != nil {
		t.Fatalf("Esc must return to the collection detail: screen=%v prot=%v", m.screen, m.prot)
	}
	if !strings.Contains(m.View(), "> Protection") {
		t.Fatalf("the Protection row lost focus:\n%s", m.View())
	}
}

func TestMirrorTurnsOnOnlyAfterTheExplanationIsConfirmed(t *testing.T) {
	m, fake := newTestModel(t, Options{Width: 100, Language: "en"})
	m = openProtectionScreen(t, m)
	m, _ = update(t, m, key("enter")) // row 0 = mirror, off
	if m.confirm == nil || !strings.Contains(m.View(), "Drive trash") {
		t.Fatalf("enabling mirror must explain the trash first:\n%s", m.View())
	}
	if got := fake.recorded(); len(got) != 0 {
		t.Fatalf("the explanation itself wrote: %v", got)
	}
	m, cmd := update(t, m, key("n"))
	if cmd != nil || len(fake.recorded()) != 0 {
		t.Fatalf("declining the explanation still wrote: %v", fake.recorded())
	}
	m, _ = update(t, m, key("enter"))
	m, cmd = update(t, m, key("y"))
	m = run(t, m, cmd)
	want := "protect:id=col_1:rev=2:mirror=true:snapshots=false:days=30:count=10"
	if got := strings.Join(fake.recorded(), ","); got != want {
		t.Fatalf("confirmation recorded %q, want %q", got, want)
	}
	if !strings.Contains(m.View(), "trash) · on") {
		t.Fatalf("the row does not show the saved mirror option:\n%s", m.View())
	}
}

func TestTurningMirrorOffNeedsNoConfirmation(t *testing.T) {
	m, fake := newTestModel(t, Options{Width: 100, Language: "en"})
	fake.prot.Options = ProtectionOptions{Mirror: true, KeepDays: 30, KeepCount: 10}
	m = openProtectionScreen(t, m)
	m, cmd := update(t, m, key("enter"))
	if m.confirm != nil {
		t.Fatalf("turning mirror off asked a question")
	}
	m = run(t, m, cmd)
	if got := strings.Join(fake.recorded(), ","); !strings.Contains(got, "mirror=false") {
		t.Fatalf("mirror off call = %q", got)
	}
	if m.status != "Mirror is off." {
		t.Fatalf("mirror off status = %q", m.status)
	}
}

func TestRetentionIsChosenFromTheFixedListAndNeverTyped(t *testing.T) {
	m, fake := newTestModel(t, Options{Width: 100, Language: "en"})
	m = openProtectionScreen(t, m)
	m = sendKeys(t, m, "down", "down") // keep days
	m, cmd := update(t, m, key("enter"))
	m = run(t, m, cmd)
	view := m.View()
	for _, want := range []string{"Keep snapshots for 30 days", "7", "30", "90"} {
		if !strings.Contains(view, want) {
			t.Fatalf("the day choices miss %q:\n%s", want, view)
		}
	}
	m, _ = update(t, m, key("down")) // 90
	m, cmd = update(t, m, key("enter"))
	m = run(t, m, cmd)
	want := "protect:id=col_1:rev=2:mirror=false:snapshots=false:days=90:count=10"
	if got := strings.Join(fake.recorded(), ","); got != want {
		t.Fatalf("day choice recorded %q, want %q", got, want)
	}
	// The value list is fixed: there is no text field to type into.
	if m.typing() {
		t.Fatalf("a fixed choice page started a text input")
	}
}

func TestRestoreRunsADryRunAndOnlyThenOffersApply(t *testing.T) {
	m, fake := newTestModel(t, Options{Width: 100, Language: "en"})
	fake.snaps = []SnapshotInfo{{ID: "snap_1", State: "ready", Revision: 3, Items: 12, CreatedAt: time.Unix(1_700_000_000, 0)}}
	m = openProtectionScreen(t, m)
	m = sendKeys(t, m, "end") // Restore…
	m, cmd := update(t, m, key("enter"))
	m = run(t, m, cmd)
	if m.prot.page != protectSnapshots || !strings.Contains(m.View(), "snap_1") {
		t.Fatalf("Restore did not open the snapshot list:\n%s", m.View())
	}
	m, cmd = update(t, m, key("enter"))
	m = run(t, m, cmd)
	if got := strings.Join(fake.recorded(), ","); got != "restore:dry:snap_1:3" {
		t.Fatalf("restore must ask for a dry run first, got %q", got)
	}
	if !strings.Contains(m.View(), "dry run is in progress") {
		t.Fatalf("a queued dry run must say it is still running:\n%s", m.View())
	}
	fake.recordedReset()
	m, cmd = update(t, m, key("a"))
	if cmd != nil || m.status != "Let the dry-run preview finish before applying." {
		t.Fatalf("apply before the preview finished: cmd=%v status=%q", cmd, m.status)
	}
	if got := fake.recorded(); len(got) != 0 {
		t.Fatalf("apply before the preview wrote: %v", got)
	}

	// The dry run finishes and its plan is read back.
	fake.jobState = "succeeded"
	fake.restore = RestorePlan{Actions: []RestoreAction{{Ordinal: 1, Action: "untrash", Path: "Project Alpha/a.txt"}}}
	m, cmd = update(t, m, key("r"))
	m = run(t, m, cmd)
	if !strings.Contains(m.View(), "Actions that would run: 1") {
		t.Fatalf("the preview count is missing:\n%s", m.View())
	}
	m, _ = update(t, m, key("a"))
	if m.confirm == nil || !strings.Contains(m.View(), "pauses") {
		t.Fatalf("apply must explain that sync pauses:\n%s", m.View())
	}
	m, cmd = update(t, m, key("y"))
	m = run(t, m, cmd)
	got := strings.Join(fake.recorded(), ",")
	if !strings.Contains(got, "restore:apply:snap_1:3") {
		t.Fatalf("apply did not use the same snapshot and revision: %q", got)
	}
	if !strings.Contains(m.View(), "Snapshots of this source") {
		t.Fatalf("apply must return to the snapshot list:\n%s", m.View())
	}
}

func TestSnapshotPinUsesLiveRevisionAndStaleRevisionIsReported(t *testing.T) {
	m, fake := newTestModel(t, Options{Width: 100, Language: "en"})
	fake.snaps = []SnapshotInfo{{ID: "snap_1", State: "ready", Revision: 3, CreatedAt: time.Unix(1_700_000_000, 0)}}
	m = openProtectionScreen(t, m)
	m = sendKeys(t, m, "down", "down", "down", "down", "down") // Snapshot list
	m, cmd := update(t, m, key("enter"))
	m = run(t, m, cmd)
	m, cmd = update(t, m, key("p"))
	m = run(t, m, cmd)
	if got := strings.Join(fake.recorded(), ","); got != "pin:snap_1:3:true" {
		t.Fatalf("pin recorded %q", got)
	}
	// The next pin carries the fresh revision the daemon returned.
	m, cmd = update(t, m, key("p"))
	_ = run(t, m, cmd)
	if got := strings.Join(fake.recorded(), ","); got != "pin:snap_1:3:true,pin:snap_1:4:false" {
		t.Fatalf("the second pin did not use the new revision: %q", got)
	}
}

func TestProtectionScreenSurvivesARefreshAndLeftGoesBackOneLevel(t *testing.T) {
	m, fake := newTestModel(t, Options{Width: 100, Language: "en"})
	fake.snaps = []SnapshotInfo{{ID: "snap_1", State: "ready", Revision: 3}}
	m = openProtectionScreen(t, m)
	m = sendKeys(t, m, "end", "enter") // Restore… list
	snap := m.snap
	snap.Collections = append([]Collection{{ID: "col_other", SourceName: "Other"}}, snap.Collections...)
	m, _ = update(t, m, snapshotMsg{snap: snap})
	if m.prot == nil || m.prot.collectionID != "col_1" {
		t.Fatalf("a refresh changed the protection target")
	}
	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyLeft})
	if m.prot.page != protectMenu {
		t.Fatalf("Left must leave the snapshot list for the menu, got page %v", m.prot.page)
	}
	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyLeft})
	if m.screen != screenCollection {
		t.Fatalf("Left at the menu must return to the collection, got %v", m.screen)
	}
}
