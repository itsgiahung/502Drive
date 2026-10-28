package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/itsgiahung/502Drive/internal/i18n"
)

// The tab bar is the one place that shows where you are and how to move:
// Tab, Shift+Tab, the arrow keys (on screens that do not use them), [ and ],
// the number keys and the mouse all switch the same tabs, so nobody has to read
// a help line first.

type tab struct {
	screen screen
	key    i18n.Key
	digit  string
}

var tabs = []tab{
	{screenHome, i18n.TabHome, "1"},
	{screenJobs, i18n.TabJobs, "2"},
	{screenLibrary, i18n.TabLibrary, "3"},
	{screenDestination, i18n.TabDest, "4"},
	{screenAccount, i18n.TabAccount, "5"},
	{screenSettings, i18n.TabSettings, "6"},
	{screenDrive, i18n.TabDrive, "7"},
}

// activeTab maps any screen (detail views included) to the tab it belongs to.
func (m Model) activeTab() int {
	home := screenHome
	switch m.screen {
	case screenJob:
		home = screenJobs
	case screenCollection, screenAudit, screenFilter, screenProtect:
		home = screenLibrary
	case screenHistory:
		home = screenSettings
	case screenCopy:
		home = screenHome
	case screenHelp:
		return -1
	default:
		home = m.screen
	}
	for i, t := range tabs {
		if t.screen == home {
			return i
		}
	}
	return -1
}

// tabSpan is where one tab sits on the bar, for drawing and for mouse hits.
type tabSpan struct{ x0, x1, index int }

func (m Model) tabLabel(i int, short bool) string {
	label := m.t(tabs[i].key)
	if short {
		if i == m.activeTab() {
			return tabs[i].digit + " " + label
		}
		return tabs[i].digit
	}
	return tabs[i].digit + " " + label
}

// tabLayout lays the tabs out for the current width: full names when they fit,
// else only the active one by name.
func (m Model) tabLayout() (spans []tabSpan, labels []string) {
	// The status chip keeps its room; tabs shorten before the chip disappears.
	budget := m.width - 2 - ansi.StringWidth(m.statusChip()) - 1
	for _, short := range []bool{false, true} {
		spans, labels = spans[:0], labels[:0]
		x := 1
		for i := range tabs {
			l := " " + m.tabLabel(i, short) + " "
			if m.tabFocus && i == m.activeTab() {
				l = "‹" + m.tabLabel(i, short) + "›" // same width: the bar marks where the cursor is
			}
			w := ansi.StringWidth(l)
			spans = append(spans, tabSpan{x, x + w, i})
			labels = append(labels, l)
			x += w + 1
		}
		if x-1 <= budget {
			break
		}
	}
	return spans, labels
}

// tabBar is the first line of every screen.
func (m Model) tabBar() string {
	spans, labels := m.tabLayout()
	var sb strings.Builder
	sb.WriteString(" ")
	active := m.activeTab()
	for i, l := range labels {
		switch {
		case i == active && m.opt.Color && m.tabFocus:
			sb.WriteString(m.accentBG(l)) // the cursor is here
		case i == active && m.opt.Color:
			// The current page, with the cursor below: bold and underlined text, NOT a
			// filled block. A filled block (cyan) always means "the cursor is here", and
			// there is only ever one of those on screen.
			sb.WriteString(m.underline(l))
		case i == active:
			sb.WriteString("[" + strings.TrimSpace(l) + "]")
		default:
			sb.WriteString(m.dim(l))
		}
		if i < len(labels)-1 {
			sb.WriteString(" ")
		}
	}
	left := sb.String()
	right := m.statusChip()
	if gap := m.width - ansi.StringWidth(left) - ansi.StringWidth(right) - 1; gap >= 1 {
		return left + strings.Repeat(" ", gap) + right
	}
	_ = spans
	return left
}

// statusChip is the live/offline dot with the running, waiting and blocked
// counts: everything the old header said, in a few characters.
func (m Model) statusChip() string {
	if !m.online {
		return m.bad("○ " + m.t(i18n.Offline))
	}
	st := m.snap.Status
	dot := m.good("●")
	if !m.watching && m.acts.Watch != nil {
		dot = m.warn("○")
	}
	chip := dot
	if lc := m.lifeChip(); lc != "" {
		chip += " " + lc
	}
	if st.Running > 0 {
		chip += " " + m.accent(fmt.Sprintf("▶%d", st.Running))
	}
	if st.Queued > 0 {
		chip += " " + m.dim(fmt.Sprintf("⏳%d", st.Queued))
	}
	if st.Blocked > 0 {
		chip += " " + m.warn(fmt.Sprintf("⚠%d", st.Blocked))
	}
	if st.QueuePaused {
		chip += " " + m.warn("||") // the queue is paused
	}
	if st.AccountStatus != "" && st.AccountStatus != "connected" {
		chip += " " + m.warn("G!")
	}
	return chip
}

func (m Model) accentBG(s string) string {
	return m.paintBG("39", "16", s)
}

// moveTab switches to the next (+1) or previous (-1) tab, wrapping around.
func (m Model) moveTab(delta int) (tea.Model, tea.Cmd) {
	cur := m.activeTab()
	if cur < 0 {
		cur = 0
	}
	return m.gotoTab((cur + delta + len(tabs)) % len(tabs))
}

// stepTab is moveTab for ←/→ and h/l: when the cursor is on the tab bar it stays
// there, so the arrows keep walking across the tabs instead of falling into a page.
func (m Model) stepTab(delta int) (tea.Model, tea.Cmd) {
	onBar := m.tabFocus
	next, cmd := m.moveTab(delta)
	if nm, ok := next.(Model); ok && onBar {
		nm.tabFocus = true
		return nm, cmd
	}
	return next, cmd
}

func (m Model) gotoTab(i int) (tea.Model, tea.Cmd) {
	if i < 0 || i >= len(tabs) {
		return m, nil
	}
	var next tea.Model
	var cmd tea.Cmd
	if tabs[i].screen == screenDrive {
		next, cmd = m.openDrive(false)
	} else {
		m.drv, m.cp = nil, nil
		next, cmd = m.goTo(tabs[i].screen)
	}
	// Switching tabs leaves the cursor on the tab bar (no row is selected) until
	// ↓ or Enter goes into the page. Home keeps its prompt.
	if nm, ok := next.(Model); ok {
		nm.tabFocus = tabs[i].screen != screenHome
		return nm, cmd
	}
	return next, cmd
}

// The cursor is always in one of two places, and the screen shows which:
//
//   - the tab bar (the active tab is cyan and drawn ‹like this›, rows show no
//     marker): ←/→ switch tabs, ↓ or Enter goes into the page;
//   - the page (the selected row has a cyan marker, the active tab is grey):
//     →/Enter opens the selected row, ←/Esc steps back out (from a top-level
//     list that means up to the tab bar). ←/→ never switch tabs from here.
//
// Tab, Shift+Tab, [ ], 1-7 and a click on the bar switch tabs from anywhere.

// arrowsSwitchTabs says whether ←/→ mean "previous/next tab" on this screen:
// only with the cursor on the tab bar, and on Home (no rows) while the prompt is empty.
func (m Model) arrowsSwitchTabs() bool {
	if m.tabFocus {
		return true
	}
	return m.screen == screenHome && m.cmd.Value() == ""
}

// topList says whether the screen is a top-level list whose rows open with →.
func (m Model) topList() bool {
	switch m.screen {
	case screenJobs, screenLibrary, screenDestination, screenAccount, screenHelp:
		return true
	}
	return false
}

// onMouse handles a left click on the tab bar.
func (m Model) onMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft || msg.Y != 0 {
		return m, nil
	}
	spans, _ := m.tabLayout()
	for _, s := range spans {
		if msg.X >= s.x0 && msg.X < s.x1 {
			return m.gotoTab(s.index)
		}
	}
	return m, nil
}
