package tui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// fakeActions records the fixed commands the model sends.
type fakeActions struct {
	mu     sync.Mutex
	snap   Snapshot
	err    error
	calls  []string
	actErr error
}

func (f *fakeActions) Actions() Actions {
	return Actions{
		Snapshot: func(context.Context) (Snapshot, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.err != nil {
				return Snapshot{}, f.err
			}
			return f.snap, nil
		},
		JobAction: func(_ context.Context, id, action string, revision int) error {
			f.record(fmt.Sprintf("job:%s:%s:%d", action, id, revision))
			return f.actErr
		},
		SyncPreset: func(_ context.Context, collectionID, preset string) error {
			f.record("preset:" + preset + ":" + collectionID)
			return f.actErr
		},
		SyncAction: func(_ context.Context, collectionID, action string) error {
			f.record("sync:" + action + ":" + collectionID)
			return f.actErr
		},
		RunNow: func(_ context.Context, collectionID string) error {
			f.record("run:" + collectionID)
			return f.actErr
		},
		SetDefaultDestination: func(_ context.Context, id string, revision int) error {
			f.record(fmt.Sprintf("dest:%s:%d", id, revision))
			return f.actErr
		},
		Doctor: func(context.Context) ([]Check, error) {
			f.record("doctor")
			return []Check{{Name: "database", Status: "passed", Message: "ok"}}, nil
		},
		SetLanguage: func(_ context.Context, lang string) error {
			f.record("lang:" + lang)
			return f.actErr
		},
	}
}

func (f *fakeActions) record(call string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
}

func (f *fakeActions) recorded() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

// update drives the model and checks the returned type.
func update(t *testing.T, m Model, msg tea.Msg) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(msg)
	out, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T", next)
	}
	return out, cmd
}

// run executes a command and feeds its message back, as Bubble Tea would.
func run(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			m = run(t, m, c)
		}
		return m
	}
	if msg == nil {
		return m
	}
	m, _ = update(t, m, msg)
	return m
}

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func sampleSnapshot() Snapshot {
	total := 84
	return Snapshot{
		Status: Status{Version: "1.0.0", Ready: true, Running: 1, Queued: 2, AccountStatus: "connected", BotState: "polling"},
		Jobs: []Job{
			{ID: "job_1", CollectionID: "col_1", SourceName: "Project Alpha", State: "running", Revision: 3, Completed: 63, Total: &total},
			{ID: "job_2", SourceName: "Beta", State: "queued", Revision: 1},
		},
		Collections: []Collection{
			{ID: "col_1", SourceName: "Project Alpha", State: "ready", Revision: 2,
				Sync: &SyncState{Mode: "scheduled", State: "active", Revision: 4, Schedule: "daily at 22:00 (Asia/Ho_Chi_Minh)", NextRun: "2026-10-26T15:00:00Z"}},
		},
		Destinations: []Destination{{ID: "dst_1", Name: "Archive", Revision: 1, Default: true}},
		Account:      Account{Status: "connected", Label: "user@example.test"},
		Prefs:        Prefs{Language: "vi", Timezone: "Asia/Ho_Chi_Minh"},
		At:           time.Unix(1_700_000_000, 0),
	}
}

func newTestModel(t *testing.T, opt Options) (Model, *fakeActions) {
	t.Helper()
	fake := &fakeActions{snap: sampleSnapshot()}
	if opt.Language == "" {
		opt.Language = "en"
	}
	m := New(fake.Actions(), opt)
	m = run(t, m, m.fetch())
	return m, fake
}

func TestTUIRendersVietnameseFromPreferences(t *testing.T) {
	fake := &fakeActions{snap: sampleSnapshot()}
	fake.snap.Prefs.Language = "vi"
	m := New(fake.Actions(), Options{Width: 80, NoMotion: true})
	m = run(t, m, m.fetch())
	view := m.View()
	for _, want := range []string{"Bắt đầu", "Hoạt động gần đây", "Đang copy 63 / 84", "Dán link Drive"} {
		if !strings.Contains(view, want) {
			t.Fatalf("vi home misses %q:\n%s", want, view)
		}
	}
	m, _ = update(t, m, key("6"))
	if !strings.Contains(m.View(), "Múi giờ") || !strings.Contains(m.View(), "Asia/Ho_Chi_Minh") {
		t.Fatalf("vi settings:\n%s", m.View())
	}
	m, _ = update(t, m, key("2"))
	if !strings.Contains(m.View(), "space chọn") {
		t.Fatalf("vi footer:\n%s", m.View())
	}
}

func TestHomeRendersStatusAndMascot(t *testing.T) {
	m, _ := newTestModel(t, Options{Width: 80})
	view := m.View()
	for _, want := range []string{"502Drive", "ready", "Google:", "Archive", "1 running", "2 queued", "Get started", "Recent activity"} {
		if !strings.Contains(view, want) {
			t.Fatalf("home view misses %q:\n%s", want, view)
		}
	}
	if !strings.Contains(view, "╭──────╮") {
		t.Fatalf("mascot missing:\n%s", view)
	}
}

func TestNavigationKeys(t *testing.T) {
	m, _ := newTestModel(t, Options{Width: 80})
	m, _ = update(t, m, key("2"))
	if !strings.Contains(m.View(), "Jobs") || m.screen != screenJobs {
		t.Fatalf("screen = %v\n%s", m.screen, m.View())
	}
	m, _ = update(t, m, key("j"))
	if m.cursor != 1 {
		t.Fatalf("cursor after j = %d", m.cursor)
	}
	m, _ = update(t, m, key("k"))
	if m.cursor != 0 {
		t.Fatalf("cursor after k = %d", m.cursor)
	}
	m, _ = update(t, m, key("down"))
	m, _ = update(t, m, key("up"))
	if m.cursor != 0 {
		t.Fatalf("arrow keys did not move: %d", m.cursor)
	}
	m, _ = update(t, m, key("3"))
	if m.screen != screenLibrary {
		t.Fatalf("screen = %v", m.screen)
	}
	m, _ = update(t, m, key("esc"))
	if m.screen != screenHome {
		t.Fatalf("esc did not return home: %v", m.screen)
	}
	m, cmd := update(t, m, key("esc"))
	if !m.quits || cmd == nil {
		t.Fatal("esc on Home must quit")
	}
	m, cmd = update(t, m, key("q"))
	if !m.quits || cmd == nil {
		t.Fatal("q must quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("q did not return tea.Quit")
	}
}

func TestJobDetailActions(t *testing.T) {
	m, fake := newTestModel(t, Options{Width: 80})
	m, _ = update(t, m, key("2"))
	m, _ = update(t, m, key("enter"))
	if m.screen != screenJob {
		t.Fatalf("screen = %v", m.screen)
	}
	view := m.View()
	if !strings.Contains(view, "job_1") || !strings.Contains(view, "63/84") {
		t.Fatalf("job detail:\n%s", view)
	}
	for _, k := range []string{"p", "u", "c", "t"} {
		m, cmd := update(t, m, key(k))
		m = run(t, m, cmd)
	}
	want := []string{"job:pause:job_1:3", "job:resume:job_1:3", "job:cancel:job_1:3", "job:retry:job_1:3"}
	if got := fake.recorded(); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("calls = %v, want %v", got, want)
	}
}

func TestLibraryPresetsAndSyncActions(t *testing.T) {
	m, fake := newTestModel(t, Options{Width: 80})
	m, _ = update(t, m, key("3"))
	m, _ = update(t, m, key("enter"))
	if m.screen != screenCollection {
		t.Fatalf("screen = %v", m.screen)
	}
	if !strings.Contains(m.View(), "daily at 22:00") {
		t.Fatalf("collection detail:\n%s", m.View())
	}
	for _, k := range []string{"d", "h", "e", "t", "n", "p", "x"} {
		m, cmd := update(t, m, key(k))
		m = run(t, m, cmd)
	}
	want := []string{"preset:daily:col_1", "preset:hourly:col_1", "preset:sixhourly:col_1", "preset:realtime:col_1",
		"run:col_1", "sync:pause:col_1", "sync:stop:col_1"}
	if got := fake.recorded(); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("calls = %v, want %v", got, want)
	}
}

func TestDestinationSetDefault(t *testing.T) {
	m, fake := newTestModel(t, Options{Width: 80})
	m, _ = update(t, m, key("4"))
	m, cmd := update(t, m, key("enter"))
	m = run(t, m, cmd)
	if got := fake.recorded(); len(got) != 1 || got[0] != "dest:dst_1:1" {
		t.Fatalf("calls = %v", got)
	}
}

func TestAccountDoctorAndSettingsLanguage(t *testing.T) {
	m, fake := newTestModel(t, Options{Width: 80})
	m, _ = update(t, m, key("5"))
	m, cmd := update(t, m, key("d"))
	m = run(t, m, cmd)
	if !strings.Contains(m.View(), "database") {
		t.Fatalf("doctor results not shown:\n%s", m.View())
	}
	m, _ = update(t, m, key("6"))
	m, cmd = update(t, m, key("l"))
	m = run(t, m, cmd)
	if got := fake.recorded(); got[len(got)-1] != "lang:en" {
		t.Fatalf("calls = %v", got)
	}
}

func TestOfflineKeepsLastData(t *testing.T) {
	m, _ := newTestModel(t, Options{Width: 80})
	m, _ = update(t, m, snapshotMsg{err: errors.New("connection refused")})
	m, _ = update(t, m, key("2"))
	view := m.View()
	if !strings.Contains(view, "OFFLINE") || !strings.Contains(view, "Project Alpha") {
		t.Fatalf("offline view lost data:\n%s", view)
	}
	if m.online {
		t.Fatal("model still online after a failed poll")
	}
}

func TestResizeWidths(t *testing.T) {
	for _, width := range []int{40, 80, 120} {
		m, _ := newTestModel(t, Options{Width: width})
		m, _ = update(t, m, tea.WindowSizeMsg{Width: width, Height: 24})
		m, _ = update(t, m, key("2"))
		view := m.View()
		for _, line := range strings.Split(view, "\n") {
			if n := len([]rune(line)); n > width {
				t.Fatalf("width %d: line of %d runes: %q", width, n, line)
			}
		}
		if width == 40 {
			if strings.Contains(view, "████") || strings.Contains(view, "░░░░") {
				t.Fatalf("40 columns should use the compact counter:\n%s", view)
			}
			if !strings.Contains(view, "63/84") {
				t.Fatalf("compact counter missing:\n%s", view)
			}
		}
		if width >= 80 && !strings.Contains(view, "█") {
			t.Fatalf("width %d should draw the progress bar:\n%s", width, view)
		}
	}
}

func TestInjectionIsSanitized(t *testing.T) {
	fake := &fakeActions{snap: sampleSnapshot()}
	evil := "evil\x1b[31m\nname\u202ereversed"
	fake.snap.Jobs[0].SourceName = evil
	fake.snap.Collections[0].SourceName = evil
	fake.snap.Destinations[0].Name = evil
	fake.snap.Account.Label = evil
	m := New(fake.Actions(), Options{Width: 80})
	m = run(t, m, m.fetch())
	m, _ = update(t, m, key("x"))
	for _, screen := range []screen{screenHome, screenJobs, screenJob, screenLibrary, screenCollection, screenDestination, screenAccount} {
		m.screen = screen
		view := m.View()
		if strings.ContainsAny(view, "\x1b\u202e") {
			t.Fatalf("screen %v leaked control characters:\n%q", screen, view)
		}
		if strings.Contains(view, "\nname") {
			t.Fatalf("screen %v rendered a raw newline from a name:\n%q", screen, view)
		}
	}
	m.screen = screenJobs
	if !strings.Contains(m.View(), "evil\\x1b[31m") {
		t.Fatalf("escape was not shown as text:\n%s", m.View())
	}
}

func TestNoMotionNeverAnimates(t *testing.T) {
	m := New((&fakeActions{snap: sampleSnapshot()}).Actions(), Options{Width: 80, Color: true, NoMotion: true})
	if m.animating() {
		t.Fatal("--no-motion must not animate the mascot")
	}
	if _, cmd := update(t, m, animTickMsg(time.Now())); cmd != nil {
		t.Fatal("an animation tick re-armed itself under --no-motion")
	}
	moving := New((&fakeActions{snap: sampleSnapshot()}).Actions(), Options{Width: 80, Color: true})
	if !moving.animating() {
		t.Fatal("the home screen should animate when motion and colour are on")
	}
	moving, _ = update(t, moving, key("2"))
	if moving.animating() {
		t.Fatal("only the home screen animates; other screens must stay still")
	}
}

func TestSpinnerOnlyWhenBusyAndAllowed(t *testing.T) {
	m, _ := newTestModel(t, Options{Width: 80})
	if m.spinner() == nil {
		t.Fatal("a busy job should animate the spinner")
	}
	m.opt.NoMotion = true
	if m.spinner() != nil {
		t.Fatal("--no-motion must not animate")
	}
	m.opt.NoMotion = false
	m.snap.Status.Running, m.snap.Status.Queued = 0, 0
	if m.spinner() != nil {
		t.Fatal("an idle daemon must not animate")
	}
}

func TestProgressUsesRealCountsOnly(t *testing.T) {
	m, _ := newTestModel(t, Options{Width: 80})
	m.screen = screenJob
	view := m.View()
	if !strings.Contains(view, "63/84") {
		t.Fatalf("progress missing:\n%s", view)
	}
	m.snap.Jobs[0].Total = nil
	view = m.View()
	if strings.Contains(view, "%") || strings.Contains(view, "63/") {
		t.Fatalf("unknown total produced a fake percentage:\n%s", view)
	}
	if !strings.Contains(view, "total unknown") {
		t.Fatalf("unknown total not labelled:\n%s", view)
	}
}

func TestHelpScreen(t *testing.T) {
	m, _ := newTestModel(t, Options{Width: 80})
	m, _ = update(t, m, key("?"))
	if m.screen != screenHelp {
		t.Fatalf("screen = %v", m.screen)
	}
	view := m.View()
	if !strings.Contains(view, "Keys") || !strings.Contains(view, "quit") {
		t.Fatalf("help view:\n%s", view)
	}
	m, _ = update(t, m, key("esc"))
	if m.screen != screenHome {
		t.Fatalf("esc from help = %v", m.screen)
	}
}

func TestActionErrorIsShown(t *testing.T) {
	m, fake := newTestModel(t, Options{Width: 80})
	fake.actErr = errors.New("sync is off for this collection")
	m, _ = update(t, m, key("2"))
	m, _ = update(t, m, key("enter"))
	m, cmd := update(t, m, key("p"))
	m = run(t, m, cmd)
	if !strings.Contains(m.View(), "sync is off") {
		t.Fatalf("action error not shown:\n%s", m.View())
	}
}

// TestProgramRendersAndRestoresTheTerminal runs the real Bubble Tea program
// with an in-memory terminal: it proves the alt screen is entered and left (so
// the terminal is restored) and that the model renders through Run.
func TestProgramRendersAndRestoresTheTerminal(t *testing.T) {
	fake := &fakeActions{snap: sampleSnapshot()}
	var out bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	m := New(fake.Actions(), Options{Width: 80, NoMotion: true})
	// "q" quits through Update, so Run returns only after the terminal was
	// restored.
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithInput(strings.NewReader("q")), tea.WithOutput(&out), tea.WithContext(ctx))
	if _, err := p.Run(); err != nil {
		t.Fatalf("program failed: %v", err)
	}
	rendered := out.String()
	if !strings.Contains(rendered, "502Drive") {
		t.Fatalf("the program never rendered the header:\n%q", rendered)
	}
	if !strings.Contains(rendered, "\x1b[?1049h") || !strings.Contains(rendered, "\x1b[?1049l") {
		t.Fatal("the alternate screen was not entered and restored")
	}
}

func TestPollTickRefetches(t *testing.T) {
	m, _ := newTestModel(t, Options{Width: 80})
	_, cmd := update(t, m, pollTickMsg(time.Now()))
	if cmd == nil {
		t.Fatal("a poll tick must fetch a snapshot")
	}
}
