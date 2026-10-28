package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func homeModel(t *testing.T, d *driveFake, w, h int) Model {
	t.Helper()
	m := driveModel(t, d, w, h)
	m, _ = update(t, m, key("1"))
	return m
}

func TestPastingALinkAndPressingEnterOpensTheCopyDialog(t *testing.T) {
	d := newDriveFake()
	m := homeModel(t, d, 100, 30)
	m = typeText(t, m, "https://drive.google.com/drive/folders/ABC123")
	if m.screen != screenHome || m.cmd.Value() == "" {
		t.Fatalf("typing a link must stay on Home in the prompt: screen=%v value=%q", m.screen, m.cmd.Value())
	}
	m, cmd := update(t, m, key("enter"))
	m = run(t, m, cmd)
	if m.screen != screenCopy || m.cp == nil || m.cp.plan == nil {
		t.Fatalf("Enter on a link should preview the copy: screen=%v", m.screen)
	}
	if !strings.Contains(strings.Join(d.calls, " "), "inspect:https://drive.google.com/drive/folders/ABC123") {
		t.Fatalf("calls = %v", d.calls)
	}
	if m.cmd.Value() != "" {
		t.Fatal("the prompt must be cleared once the link is taken")
	}
}

func TestShortcutsWorkOnlyWhileThePromptIsEmpty(t *testing.T) {
	d := newDriveFake()
	m := homeModel(t, d, 100, 30)
	m = typeText(t, m, "x") // a non-shortcut starts text
	m = typeText(t, m, "q") // now q is just a letter
	if m.quits || m.cmd.Value() != "xq" {
		t.Fatalf("q must not quit while text is typed: quits=%v value=%q", m.quits, m.cmd.Value())
	}
	m, _ = update(t, m, key("esc"))
	if m.cmd.Value() != "" || m.screen != screenHome {
		t.Fatalf("esc clears the prompt first: value=%q screen=%v", m.cmd.Value(), m.screen)
	}
	m, _ = update(t, m, key("2"))
	if m.screen != screenJobs {
		t.Fatalf("digits are shortcuts again once the prompt is empty: %v", m.screen)
	}
}

func TestSlashListsCommandsAndRunsThem(t *testing.T) {
	d := newDriveFake()
	m := homeModel(t, d, 100, 30)
	m = typeText(t, m, "/")
	v := m.View()
	for _, want := range []string{"/copy", "/drive", "/jobs", "/quit"} {
		if !strings.Contains(v, want) {
			t.Fatalf("the command list lacks %q:\n%s", want, v)
		}
	}
	m = typeText(t, m, "ji") // narrows to /jobs
	if got := m.suggestions(); len(got) != 0 {
		t.Fatalf("/ji matches nothing, got %v", got)
	}
	m, _ = pressKey(t, m, tea.KeyMsg{Type: tea.KeyBackspace})
	if got := m.suggestions(); len(got) != 1 || got[0].name != "jobs" {
		t.Fatalf("/j should narrow to /jobs: %v", got)
	}
	m, _ = update(t, m, key("enter"))
	if m.screen != screenJobs {
		t.Fatalf("/j + Enter should run the highlighted command: %v", m.screen)
	}
}

func TestTabCompletesAndArrowsChooseACommand(t *testing.T) {
	d := newDriveFake()
	m := homeModel(t, d, 100, 30)
	m = typeText(t, m, "/d") // drive, dest
	if n := len(m.suggestions()); n != 2 {
		t.Fatalf("/d should match 2 commands, got %d", n)
	}
	m, _ = pressKey(t, m, tea.KeyMsg{Type: tea.KeyDown})
	m, _ = pressKey(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.cmd.Value() != "/dest " {
		t.Fatalf("tab should complete the chosen command, got %q", m.cmd.Value())
	}
}

func TestUnknownSlashCommandSaysSo(t *testing.T) {
	d := newDriveFake()
	m := homeModel(t, d, 100, 30)
	m = typeText(t, m, "/zzz")
	m, _ = update(t, m, key("enter"))
	if m.screen != screenHome || !strings.Contains(m.status, "zzz") {
		t.Fatalf("screen=%v status=%q", m.screen, m.status)
	}
}

func TestSlashCopyTakesALinkArgument(t *testing.T) {
	d := newDriveFake()
	m := homeModel(t, d, 100, 30)
	m = typeText(t, m, "/copy SRC999")
	m, cmd := update(t, m, key("enter"))
	m = run(t, m, cmd)
	if m.screen != screenCopy || m.cp.source != "SRC999" {
		t.Fatalf("screen=%v", m.screen)
	}
}

func TestHomeLayoutFitsEveryTerminalSizeWithThePromptAtTheBottom(t *testing.T) {
	d := newDriveFake()
	for _, size := range [][2]int{{60, 20}, {80, 24}, {100, 30}, {140, 40}} {
		m := homeModel(t, d, size[0], size[1])
		view := m.View()
		ls := lines(view)
		if len(ls) != size[1] {
			t.Fatalf("%v: %d lines, want %d:\n%s", size, len(ls), size[1], view)
		}
		for _, l := range ls {
			if ansi.StringWidth(l) > size[0] {
				t.Fatalf("%v: a line is %d columns wide: %q", size, ansi.StringWidth(l), l)
			}
		}
		// the prompt box is the last block above the status and footer lines
		if !strings.Contains(strings.Join(ls[len(ls)-4:], "\n"), "❯") {
			t.Fatalf("%v: the prompt is not at the bottom:\n%s", size, view)
		}
	}
}

func TestTheTransferStripShowsOnlyWhileSomethingCopies(t *testing.T) {
	d := newDriveFake()
	idle := d.fakeActions.snap
	idle.Status.Running, idle.Status.Queued = 0, 0
	idle.Jobs = nil
	d.snap = idle
	opt := Options{Width: 100, Height: 40, Language: "en", Color: true}
	m := New(d.acts(), opt)
	m = run(t, m, m.fetch())
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	if strings.Contains(m.View(), "Copying") {
		t.Fatal("nothing is copying, so no transfer block should show")
	}
	d.snap = sampleSnapshot()
	m = run(t, m, m.fetch())
	v := m.View()
	if !strings.Contains(v, "Copying 63 / 84") {
		t.Fatalf("a running job should show the transfer block:\n%s", v)
	}
}

func TestHomeShowsANewerVersionWhenTheCheckFindsOne(t *testing.T) {
	m := homeModel(t, newDriveFake(), 100, 30)
	if strings.Contains(ansi.Strip(m.homeFrame()), "update --install") {
		t.Fatal("no notice before a newer release is found")
	}
	m, _ = update(t, m, updateMsg{tag: "v9.9.9"})
	if out := ansi.Strip(m.homeFrame()); !strings.Contains(out, "v9.9.9") || !strings.Contains(out, "502drive update --install") {
		t.Fatalf("home should tell how to update:\n%s", out)
	}
}
