package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
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

func TestIncompleteSetupKeepsHomeControlsAndMascot(t *testing.T) {
	for _, lang := range []string{"en", "vi"} {
		for _, size := range [][2]int{{60, 20}, {80, 24}, {100, 30}, {120, 34}} {
			for _, ascii := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%dx%d/ascii=%t", lang, size[0], size[1], ascii), func(t *testing.T) {
					m := New(Actions{}, Options{NoMotion: true, Language: lang, Width: size[0], Height: size[1], Color: !ascii, ASCII: ascii, Profile: termenv.TrueColor, Dark: true})
					m.snap.Setup = &SetupState{Checks: []Check{{Name: "database", Status: "passed"}, {Name: "oauth_client", Status: "passed"}, {Name: "account", Status: "failed"}, {Name: "destination", Status: "passed"}}}
					view := m.View()
					for _, want := range []string{"502Drive", "OAuth:", "Google:", "credentials", "JSON", "❯", "/"} {
						if !strings.Contains(ansi.Strip(view), want) {
							t.Fatalf("setup hides %q:\n%s", want, view)
						}
					}
					passed, failed := "✓ OAuth:", "✗ Google:"
					if ascii {
						passed, failed = "[x] OAuth:", "[ ] Google:"
					}
					if !strings.Contains(view, passed) || !strings.Contains(view, failed) {
						t.Fatalf("setup loses check status:\n%s", view)
					}
					bar := strings.Split(ansi.Strip(view), "\n")[0]
					for _, digit := range []string{"1", "2", "3", "4", "5", "6", "7"} {
						if !strings.Contains(bar, digit) {
							t.Fatalf("setup hides tab %s: %s", digit, bar)
						}
					}
					if size[0] >= 80 {
						cat := "▄"
						if ascii {
							cat = ".------."
						}
						if !strings.Contains(view, cat) {
							t.Fatalf("setup hides mascot:\n%s", view)
						}
					}
					if got := len(strings.Split(view, "\n")); got != size[1] {
						t.Fatalf("height = %d, want %d:\n%s", got, size[1], view)
					}
					for _, line := range strings.Split(view, "\n") {
						if ansi.StringWidth(line) > size[0] {
							t.Fatalf("setup exceeds terminal width: %q", line)
						}
					}
					m, _ = update(t, m, key("5"))
					if m.screen != screenAccount {
						t.Fatal("setup prevents account navigation")
					}
				})
			}
		}
	}
}
