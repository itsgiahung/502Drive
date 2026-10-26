package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"golang.org/x/term"
)

// Style decides how human output looks; JSON never uses it.
type Style struct {
	Color  bool
	ASCII  bool
	Plain  bool // no mascot, motion or cursor control
	Motion bool
	Width  int
	// Profile and Dark describe the real terminal for the mascot; text colours
	// keep using the fixed 256-colour renderer below.
	Profile termenv.Profile
	Dark    bool
	r       *lipgloss.Renderer
}

type StyleInput struct {
	ColorMode string // auto|always|never
	ASCII     bool
	Plain     bool
	JSON      bool
	Quiet     bool
	NoMotion  bool
	StdoutTTY bool
	StderrTTY bool
	Term      string
	NoColor   string // value of NO_COLOR
	Width     int
	ColorTerm string // COLORTERM
	ColorFGBG string // COLORFGBG, e.g. "15;0"
}

// ResolveStyle applies the precedence rules from the CLI contract: --json,
// --quiet, --plain and TERM=dumb are hard overrides.
func ResolveStyle(in StyleInput) Style {
	hard := in.JSON || in.Quiet || in.Plain || in.Term == "dumb"
	s := Style{ASCII: in.ASCII || in.Term == "dumb", Width: in.Width}
	if s.Width <= 0 {
		s.Width = 80
	}
	if !hard {
		switch in.ColorMode {
		case "always":
			s.Color = true
		case "never":
		default:
			s.Color = in.StdoutTTY && in.NoColor == ""
		}
	}
	s.Plain = hard || !in.StdoutTTY
	s.Profile = termenv.ANSI256
	if ct := strings.ToLower(in.ColorTerm); ct == "truecolor" || ct == "24bit" {
		s.Profile = termenv.TrueColor
	}
	s.Dark = darkBackground(in.ColorFGBG)
	s.Motion = !s.Plain && !in.NoMotion && in.StderrTTY
	s.r = lipgloss.NewRenderer(io.Discard)
	if s.Color {
		s.r.SetColorProfile(termenv.ANSI256)
	} else {
		s.r.SetColorProfile(termenv.Ascii)
	}
	return s
}

// darkBackground reads COLORFGBG ("fg;bg"): backgrounds 7 and 15 are light.
// Unknown means dark, the common case for developer terminals.
func darkBackground(v string) bool {
	i := strings.LastIndex(v, ";")
	if i < 0 {
		return true
	}
	switch v[i+1:] {
	case "7", "15":
		return false
	}
	return true
}

func terminalWidth(f *os.File) int {
	if w, _, err := term.GetSize(int(f.Fd())); err == nil && w > 0 {
		return w
	}
	return 80
}

func (s Style) accent(t string) string {
	return s.r.NewStyle().Foreground(lipgloss.Color("39")).Bold(true).Render(t)
}
func (s Style) good(t string) string {
	return s.r.NewStyle().Foreground(lipgloss.Color("42")).Render(t)
}
func (s Style) warn(t string) string {
	return s.r.NewStyle().Foreground(lipgloss.Color("214")).Render(t)
}
func (s Style) bad(t string) string {
	return s.r.NewStyle().Foreground(lipgloss.Color("203")).Render(t)
}
func (s Style) dim(t string) string {
	return s.r.NewStyle().Foreground(lipgloss.Color("244")).Render(t)
}
func (s Style) bold(t string) string { return s.r.NewStyle().Bold(true).Render(t) }

// stateColor colours a job/collection state while always keeping the text.
func (s Style) stateColor(state string) string {
	label := strings.ToUpper(state)
	switch state {
	case "succeeded", "connected", "ready", "active", "passed":
		return s.good(label)
	case "running", "queued", "pausing", "cancelling", "reconciling", "copying", "pending":
		return s.accent(label)
	case "partial", "blocked", "paused", "needs_action", "reconnect_required", "revoking":
		return s.warn(label)
	case "failed", "disconnected":
		return s.bad(label)
	}
	return label
}

// Sanitize makes untrusted text (Drive names, error text) safe for a terminal:
// control characters, ESC sequences and bidi overrides are shown as escapes.
func Sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == utf8.RuneError, unicode.IsControl(r), r >= 0x202a && r <= 0x202e, r >= 0x2066 && r <= 0x2069, r == 0x200b, r == 0xfeff:
			if r < 0x100 {
				fmt.Fprintf(&b, "\\x%02x", r)
			} else {
				fmt.Fprintf(&b, "\\u%04x", r)
			}
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Truncate shortens to n display runes with an ellipsis, for human tables only.
func Truncate(s string, n int, ascii bool) string {
	if n <= 3 || utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	if ascii {
		return string(r[:n-3]) + "..."
	}
	return string(r[:n-1]) + "…"
}

type kv struct{ k, v string }

// printKV renders aligned key/value lines; it stays readable at 40 columns.
func printKV(w io.Writer, s Style, rows []kv) {
	width := 0
	for _, r := range rows {
		width = max(width, len(r.k))
	}
	for _, r := range rows {
		fmt.Fprintf(w, "%s  %s\n", s.dim(fmt.Sprintf("%-*s", width, strings.ToUpper(r.k))), r.v)
	}
}

// printTable renders columns for wide terminals and falls back to key/value
// blocks below 60 columns.
func printTable(w io.Writer, s Style, cols []string, rows [][]string) {
	if s.Width < 60 {
		for i, r := range rows {
			if i > 0 {
				fmt.Fprintln(w)
			}
			var kvs []kv
			for j, c := range cols {
				kvs = append(kvs, kv{c, r[j]})
			}
			printKV(w, s, kvs)
		}
		return
	}
	widths := make([]int, len(cols))
	for i, c := range cols {
		widths[i] = len(c)
	}
	for _, r := range rows {
		for i, c := range r {
			widths[i] = max(widths[i], visibleLen(c))
		}
	}
	line := func(cells []string, head bool) {
		var parts []string
		for i, c := range cells {
			pad := strings.Repeat(" ", widths[i]-visibleLen(c))
			if head {
				c = s.dim(c)
			}
			parts = append(parts, c+pad)
		}
		fmt.Fprintln(w, strings.TrimRight(strings.Join(parts, "  "), " "))
	}
	line(cols, true)
	for _, r := range rows {
		line(r, false)
	}
}

// visibleLen ignores ANSI colour sequences when measuring.
func visibleLen(s string) int {
	n, esc := 0, false
	for _, r := range s {
		switch {
		case r == 0x1b:
			esc = true
		case esc && r == 'm':
			esc = false
		case !esc:
			n++
		}
	}
	return n
}

// Mascot returns the small Drive-bot for Home/setup. mood: idle, busy, paused, error.
func (s Style) Mascot(mood string) []string {
	if s.Plain {
		return nil
	}
	eyes := map[string]string{"idle": "o  o", "busy": "^  ^", "paused": "-  -", "error": "x  x"}[mood]
	if eyes == "" {
		eyes = "o  o"
	}
	if s.ASCII {
		return []string{" .------.", " | " + eyes + " |", " |  --  |", " '------'"}
	}
	return []string{" ╭──────╮", " │ " + eyes + " │", " │  ──  │", " ╰──────╯"}
}
