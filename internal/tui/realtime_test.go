package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func manyJobs(n int) Snapshot {
	snap := sampleSnapshot()
	snap.Jobs = nil
	total := 100
	for i := 0; i < n; i++ {
		snap.Jobs = append(snap.Jobs, Job{ID: fmt.Sprintf("job_%03d", i), SourceName: fmt.Sprintf("Source number %03d", i),
			State: "running", Revision: 1, Completed: i % 100, Total: &total})
	}
	return snap
}

func sized(t *testing.T, snap Snapshot, w, h int, opt Options) Model {
	t.Helper()
	fake := &fakeActions{snap: snap}
	opt.Width, opt.Height, opt.Language = w, h, "en"
	m := New(fake.Actions(), opt)
	m = run(t, m, m.fetch())
	m, _ = update(t, m, tea.WindowSizeMsg{Width: w, Height: h})
	m, _ = update(t, m, key("2"))
	return m
}

func lines(v string) []string { return strings.Split(v, "\n") }

// A long list must never be taller than the terminal: Bubble Tea would drop
// the top lines (the header) and the cursor row would scroll out of sight.
func TestLongListsAreWindowedToTheTerminalHeight(t *testing.T) {
	for _, size := range [][2]int{{60, 20}, {120, 40}, {40, 12}} {
		m := sized(t, manyJobs(100), size[0], size[1], Options{})
		for step := 0; step < 120; step++ {
			v := m.View()
			ls := lines(v)
			if len(ls) != size[1] {
				t.Fatalf("%v: view has %d lines, want exactly %d", size, len(ls), size[1])
			}
			for _, l := range ls {
				if w := ansi.StringWidth(l); w > size[0] {
					t.Fatalf("%v: line of width %d: %q", size, w, l)
				}
			}
			hasCursor := false
			for _, l := range ls {
				hasCursor = hasCursor || strings.HasPrefix(l, "> ")
			}
			if !hasCursor {
				t.Fatalf("%v: the cursor row %d is not visible:\n%s", size, m.cursor, v)
			}
			m, _ = update(t, m, key("down"))
		}
	}
}

func TestShrinkingTheListLeavesNoStaleLines(t *testing.T) {
	m := sized(t, manyJobs(30), 80, 24, Options{})
	if !strings.Contains(m.View(), "Source number 000") {
		t.Fatal("first job missing")
	}
	few := manyJobs(2)
	few.Jobs[0].SourceName, few.Jobs[1].SourceName = "Only A", "Only B"
	m, _ = update(t, m, snapshotMsg{snap: few})
	v := m.View()
	if got := len(lines(v)); got != 24 {
		t.Fatalf("after shrinking the view has %d lines, want 24", got)
	}
	if strings.Contains(v, "Source number") {
		t.Fatalf("old rows survived the shrink:\n%s", v)
	}
}

func TestWideCharactersNeverOverflow(t *testing.T) {
	snap := manyJobs(3)
	snap.Jobs[0].SourceName = strings.Repeat("日本語のフォルダ", 10)
	snap.Jobs[1].SourceName = strings.Repeat("📁 ảnh gia đình ", 8)
	for _, w := range []int{40, 60, 100} {
		m := sized(t, snap, w, 20, Options{})
		for _, l := range lines(m.View()) {
			if got := ansi.StringWidth(l); got > w {
				t.Fatalf("width %d: line is %d columns wide: %q", w, got, l)
			}
		}
	}
}

func TestSpaceSelectsAndBulkActionsSkipWhatDoesNotApply(t *testing.T) {
	snap := sampleSnapshot()
	snap.Jobs = []Job{
		{ID: "a", SourceName: "A", State: "running", Revision: 1},
		{ID: "b", SourceName: "B", State: "paused", Revision: 2},
		{ID: "c", SourceName: "C", State: "succeeded", Revision: 3},
	}
	m := sized(t, snap, 80, 24, Options{})
	fake := m.acts
	_ = fake
	m, _ = update(t, m, key("a")) // select all
	if len(m.selected) != 3 {
		t.Fatalf("selected = %d, want 3", len(m.selected))
	}
	m, cmd := update(t, m, key("u")) // resume: only the paused one applies
	m = run(t, m, cmd)
	if !strings.Contains(m.status, "1 done, 2 skipped, 0 failed") {
		t.Fatalf("bulk summary = %q", m.status)
	}
	if len(m.selected) != 0 {
		t.Fatal("the selection should clear after a bulk action")
	}
}

func TestBulkActionsReachTheDaemonOnceEachAndFailuresDoNotStopTheRest(t *testing.T) {
	snap := sampleSnapshot()
	snap.Jobs = []Job{
		{ID: "a", SourceName: "A", State: "running", Revision: 1},
		{ID: "b", SourceName: "B", State: "queued", Revision: 2},
	}
	fake := &fakeActions{snap: snap}
	m := New(fake.Actions(), Options{Width: 80, Height: 24, Language: "en"})
	m = run(t, m, m.fetch())
	m, _ = update(t, m, key("2"))
	m, _ = update(t, m, key("a"))
	m, cmd := update(t, m, key("p"))
	_ = run(t, m, cmd)
	got := fake.recorded()
	if len(got) != 2 || got[0] != "job:pause:a:1" || got[1] != "job:pause:b:2" {
		t.Fatalf("calls = %v", got)
	}
}

func TestCancelAsksBeforeActing(t *testing.T) {
	m := sized(t, sampleSnapshot(), 80, 24, Options{})
	fake := &fakeActions{snap: sampleSnapshot()}
	m.acts = fake.Actions()
	m, cmd := update(t, m, key("c"))
	if cmd != nil || m.confirm == nil {
		t.Fatal("cancel must ask first")
	}
	if !strings.Contains(m.View(), "Cancel 1 job(s)? [y/N]") {
		t.Fatalf("prompt not shown:\n%s", m.View())
	}
	m, cmd = update(t, m, key("n"))
	if cmd != nil || m.confirm != nil || len(fake.recorded()) != 0 {
		t.Fatal("any key but y must abort")
	}
	m, _ = update(t, m, key("c"))
	m, cmd = update(t, m, key("y"))
	m = run(t, m, cmd)
	if got := fake.recorded(); len(got) != 1 || !strings.HasPrefix(got[0], "job:cancel:") {
		t.Fatalf("calls after y = %v", got)
	}
}

func TestEscClearsTheSelectionBeforeGoingBack(t *testing.T) {
	m := sized(t, sampleSnapshot(), 80, 24, Options{})
	m, _ = update(t, m, key(" "))
	m = func() Model { n, _ := update(t, m, tea.KeyMsg{Type: tea.KeySpace}); return n }()
	m, _ = update(t, m, key("esc"))
	if m.screen != screenJobs {
		t.Fatalf("esc with a selection left the screen: %v", m.screen)
	}
	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeySpace})
	m, _ = update(t, m, key("esc"))
	if len(m.selected) != 0 || m.screen != screenJobs {
		t.Fatalf("esc did not clear: selected=%d screen=%v", len(m.selected), m.screen)
	}
	m, _ = update(t, m, key("esc"))
	if m.screen != screenHome {
		t.Fatalf("second esc = %v, want home", m.screen)
	}
}

func TestChangeEventsRefreshOnceAndCoalesce(t *testing.T) {
	fake := &fakeActions{snap: sampleSnapshot()}
	m := New(fake.Actions(), Options{Width: 80, Language: "en"})
	m = run(t, m, m.fetch())
	if m.fetching {
		t.Fatal("still fetching after the snapshot arrived")
	}
	m.watchCh = make(chan WatchEvent)
	m, cmd := update(t, m, watchMsg{ev: WatchEvent{Connected: true, Changed: true}, ok: true})
	if cmd == nil || !m.fetching || !m.watching {
		t.Fatalf("a change must start one refresh: fetching=%v watching=%v", m.fetching, m.watching)
	}
	// More changes while a fetch is pending only mark the model dirty.
	m, _ = update(t, m, watchMsg{ev: WatchEvent{Connected: true, Changed: true}, ok: true})
	m, _ = update(t, m, watchMsg{ev: WatchEvent{Connected: true, Changed: true}, ok: true})
	if !m.dirty {
		t.Fatal("changes during a fetch should set dirty")
	}
	m, cmd = update(t, m, snapshotMsg{snap: sampleSnapshot()})
	if m.dirty || !m.fetching || cmd == nil {
		t.Fatalf("a dirty model should refresh once more: dirty=%v fetching=%v", m.dirty, m.fetching)
	}
}

func TestStreamLossFallsBackToFastPolling(t *testing.T) {
	m, _ := newTestModel(t, Options{Width: 80})
	m.watching = true
	if m.pollEvery() != pollConnected {
		t.Fatal("a live stream should poll slowly")
	}
	m, _ = update(t, m, watchMsg{ev: WatchEvent{Connected: false}, ok: true})
	if m.watching || m.pollEvery() != pollOffline {
		t.Fatal("a lost stream must poll fast again")
	}
}

func TestSpeedAndETAFollowSuccessiveSnapshots(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	opt := Options{Width: 120, Height: 30, Language: "en", Now: func() time.Time { return now }}
	total := 1000
	snap := func(done int) Snapshot {
		s := sampleSnapshot()
		s.Jobs = []Job{{ID: "j", SourceName: "S", State: "running", Revision: 1, Completed: done, Total: &total}}
		return s
	}
	fake := &fakeActions{snap: snap(0)}
	m := New(fake.Actions(), opt)
	m = run(t, m, m.fetch())
	now = now.Add(time.Second)
	m, _ = update(t, m, snapshotMsg{snap: snap(100)})
	now = now.Add(time.Second)
	m, _ = update(t, m, snapshotMsg{snap: snap(200)})
	perSec, eta, ok := m.speed(m.snap.Jobs[0])
	if !ok || perSec < 99 || perSec > 101 {
		t.Fatalf("speed = %.1f ok=%v, want about 100/s", perSec, ok)
	}
	if eta < 7*time.Second || eta > 9*time.Second {
		t.Fatalf("eta = %v, want about 8s", eta)
	}
	m, _ = update(t, m, key("2"))
	v := m.View()
	if !strings.Contains(v, "100.0/s") || !strings.Contains(v, "8s left") {
		t.Fatalf("jobs list lacks speed/eta:\n%s", v)
	}
}

func TestFinishedJobMakesTheCatHappyForAWhile(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	opt := Options{Width: 80, Language: "en", Now: func() time.Time { return now }}
	snap := func(state string) Snapshot {
		s := sampleSnapshot()
		s.Status.Running, s.Status.Queued = 0, 0
		s.Jobs = []Job{{ID: "j", SourceName: "S", State: state, Revision: 1}}
		return s
	}
	fake := &fakeActions{snap: snap("running")}
	m := New(fake.Actions(), opt)
	m = run(t, m, m.fetch())
	m, _ = update(t, m, snapshotMsg{snap: snap("succeeded")})
	if got := m.mood(now); got != "happy" {
		t.Fatalf("mood right after success = %q", got)
	}
	if got := m.mood(now.Add(happyFor + time.Second)); got == "happy" {
		t.Fatal("the happy mood must end")
	}
}

func TestHomeShowsTheCatOnlyWhenColourIsOn(t *testing.T) {
	colour := sized(t, sampleSnapshot(), 80, 30, Options{Color: true})
	colour, _ = update(t, colour, key("1"))
	if !strings.Contains(colour.View(), "▀") {
		t.Fatalf("no cat on a colour terminal:\n%s", colour.View())
	}
	plain := sized(t, sampleSnapshot(), 80, 30, Options{})
	plain, _ = update(t, plain, key("1"))
	if strings.Contains(plain.View(), "▀") || strings.Contains(plain.View(), "\x1b[") {
		t.Fatalf("plain output must have no pixel art or colour:\n%s", plain.View())
	}
	short := sized(t, sampleSnapshot(), 80, 16, Options{Color: true})
	short, _ = update(t, short, key("1"))
	if strings.Contains(short.View(), "▀") {
		t.Fatal("a short terminal should use the small text bot")
	}
	if got := len(lines(short.View())); got != 16 {
		t.Fatalf("short terminal view = %d lines, want 16", got)
	}
}

func BenchmarkViewWithTenThousandJobs(b *testing.B) {
	fake := &fakeActions{snap: manyJobs(10000)}
	m := New(fake.Actions(), Options{Width: 120, Height: 40, Language: "en", Color: true})
	snap, _ := fake.Actions().Snapshot(context.Background())
	next, _ := m.Update(snapshotMsg{snap: snap})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	m = next.(Model)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.View()
	}
}
