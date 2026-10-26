package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/itsgiahung/502Drive/internal/i18n"
	"github.com/itsgiahung/502Drive/internal/mascot"
)

// slashCmd is one entry of the command prompt, like a slash command.
type slashCmd struct {
	name string
	desc i18n.Key
}

var slashCmds = []slashCmd{
	{"copy", i18n.CmdCopy}, {"drive", i18n.CmdDrive}, {"jobs", i18n.CmdJobs}, {"library", i18n.CmdLibrary},
	{"dest", i18n.CmdDest}, {"account", i18n.CmdAccount}, {"login", i18n.CmdLogin}, {"settings", i18n.CmdSettings},
	{"refresh", i18n.CmdRefresh}, {"help", i18n.CmdHelp}, {"quit", i18n.CmdQuit},
}

func newPrompt() *textinput.Model {
	ti := textinput.New()
	ti.Prompt = "❯ "
	ti.CharLimit = 2048
	ti.Focus()
	return &ti
}

// suggestions are the commands matching what is typed after the slash.
func (m Model) suggestions() []slashCmd {
	v := m.cmd.Value()
	if !strings.HasPrefix(v, "/") {
		return nil
	}
	word := strings.ToLower(strings.TrimPrefix(strings.Fields(v + " ")[0], "/"))
	var out []slashCmd
	for _, c := range slashCmds {
		if strings.HasPrefix(c.name, word) {
			out = append(out, c)
		}
	}
	return out
}

// isHomeShortcut: with an empty prompt these single keys act at once; anything
// else (letters, a pasted link, "/") goes to the prompt.
func isHomeShortcut(s string) bool {
	switch s {
	case "1", "2", "3", "4", "5", "6", "7", "c", "l", "q", "?", "r":
		return true
	}
	return false
}

// homeKey routes keys on the Home screen. It reports false for keys the
// global map should handle.
func (m Model) homeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	in := m.cmd
	empty := in.Value() == ""
	sugg := m.suggestions()
	switch msg.Type {
	case tea.KeyCtrlC:
		return m, nil, false
	case tea.KeyEsc:
		if !empty {
			in.SetValue("")
			m.cmdSel = 0
			return m, nil, true
		}
		return m, nil, false
	case tea.KeyEnter:
		if empty {
			return m, nil, false
		}
		next, cmd := m.runPrompt()
		return next, cmd, true
	case tea.KeyUp, tea.KeyDown:
		if len(sugg) > 0 {
			step := 1
			if msg.Type == tea.KeyUp {
				step = len(sugg) - 1
			}
			m.cmdSel = (m.cmdSel + step) % len(sugg)
			return m, nil, true
		}
		return m, nil, false
	case tea.KeyTab:
		if len(sugg) > 0 {
			in.SetValue("/" + sugg[min(m.cmdSel, len(sugg)-1)].name + " ")
			in.CursorEnd()
			m.cmdSel = 0
			return m, nil, true
		}
		return m, nil, false // no command list open: Tab switches tabs
	case tea.KeyShiftTab:
		return m, nil, false
	}
	if empty && isHomeShortcut(msg.String()) {
		return m, nil, false
	}
	if empty && (msg.Type == tea.KeyLeft || msg.Type == tea.KeyRight || msg.Type == tea.KeyPgUp || msg.Type == tea.KeyPgDown) {
		return m, nil, false
	}
	var cmd tea.Cmd
	*in, cmd = in.Update(msg)
	m.cmdSel = 0
	return m, cmd, true
}

// runPrompt runs "/command [argument]", or treats any other text as a source
// link and opens the copy dialog on it.
func (m Model) runPrompt() (tea.Model, tea.Cmd) {
	v := strings.TrimSpace(m.cmd.Value())
	m.cmd.SetValue("")
	sel := m.cmdSel
	m.cmdSel = 0
	if !strings.HasPrefix(v, "/") {
		return m.startCopy(v, screenHome)
	}
	fields := strings.Fields(v)
	word := strings.ToLower(strings.TrimPrefix(fields[0], "/"))
	arg := strings.TrimSpace(strings.TrimPrefix(v, fields[0]))
	name := ""
	for i, c := range m.suggestionsFor(word) {
		if c.name == word || (name == "" && i == sel) {
			name = c.name
			if c.name == word {
				break
			}
		}
	}
	switch name {
	case "copy":
		return m.startCopy(arg, screenHome)
	case "drive":
		return m.openDrive(false)
	case "jobs":
		return m.goTo(screenJobs)
	case "library":
		return m.goTo(screenLibrary)
	case "dest":
		return m.goTo(screenDestination)
	case "account":
		return m.goTo(screenAccount)
	case "settings":
		return m.goTo(screenSettings)
	case "help":
		return m.goTo(screenHelp)
	case "login":
		return m.startLogin()
	case "refresh":
		return m, m.refetch()
	case "quit":
		m.quits = true
		return m, tea.Quit
	}
	m.status = i18n.Tf(m.lang, i18n.UnknownCmd, Sanitize(word))
	return m, nil
}

func (m Model) suggestionsFor(word string) []slashCmd {
	var out []slashCmd
	for _, c := range slashCmds {
		if strings.HasPrefix(c.name, word) {
			out = append(out, c)
		}
	}
	return out
}

// ---- view ----

const (
	panelMax   = 108
	catCols    = 20
	infoCols   = 36
	stripWidth = 56
)

// homeFrame is the whole Home screen: a bordered panel, activity while
// something copies, and the command prompt pinned above the footer.
func (m Model) homeFrame() string {
	pw := min(m.width, panelMax)
	top := append([]string{m.tabBar()}, m.homePanel(pw)...)
	top = append(top, m.busyBlock(pw)...)
	bottom := m.promptBlock(pw)
	room := 6
	if m.height > 0 {
		room = min(6, m.height-len(bottom)-2-len(top)-2)
	}
	top = append(top, m.recentBlock(room)...)
	if m.height > 0 {
		top = clipPad(top, max(1, m.height-len(bottom)-2))
	}
	lines := append(top, bottom...)
	lines = append(lines, m.statusLine(), m.footer())
	return m.fitBlock(lines)
}

func (m Model) homePanel(pw int) []string {
	title := m.bold("502Drive")
	if v := m.opt.Version; v != "" && v != "dev" {
		title += " " + v
	}
	if m.online {
		title += " · " + m.liveWord()
	} else {
		title += " · " + m.bad(m.t(i18n.Offline))
	}
	inner := pw - 4
	info := m.infoColumn()
	act := m.activityColumn()
	var cols [][]string
	var widths []int
	switch {
	case inner >= 90:
		cols, widths = [][]string{m.catColumn(), info, act}, []int{catCols, infoCols, inner - catCols - infoCols - 6}
	case inner >= 64:
		cols, widths = [][]string{m.catColumn(), append(append([]string{}, info...), append([]string{""}, act...)...)}, []int{catCols, inner - catCols - 3}
	default:
		cols, widths = [][]string{append(append([]string{}, info...), append([]string{""}, act...)...)}, []int{inner}
	}
	rows := 0
	for _, c := range cols {
		rows = max(rows, len(c))
	}
	sep := m.dim(" │ ")
	if m.opt.ASCII {
		sep = m.dim(" | ")
	}
	tl, tr, bl, br, h, v := "╭", "╮", "╰", "╯", "─", "│"
	if m.opt.ASCII {
		tl, tr, bl, br, h, v = "+", "+", "+", "+", "-", "|"
	}
	out := make([]string, 0, rows+2)
	out = append(out, m.dim(tl+h+" ")+title+m.dim(" "+strings.Repeat(h, max(1, pw-ansi.StringWidth(title)-5))+tr))
	for r := 0; r < rows; r++ {
		parts := make([]string, len(cols))
		for i, c := range cols {
			cell := ""
			off := (rows - len(c)) / 2 // columns are vertically centred
			if r >= off && r-off < len(c) {
				cell = c[r-off]
			}
			if i == 0 && len(cols) > 1 && len(c) > 0 {
				cell = centerCell(cell, widths[i])
			}
			parts[i] = padCell(cell, widths[i])
		}
		out = append(out, m.dim(v)+" "+strings.Join(parts, sep)+" "+m.dim(v))
	}
	out = append(out, m.dim(bl+strings.Repeat(h, pw-2)+br))
	return out
}

func padCell(s string, w int) string {
	if n := ansi.StringWidth(s); n > w {
		return ansi.Truncate(s, w, "…")
	} else {
		return s + strings.Repeat(" ", w-n)
	}
}

func centerCell(s string, w int) string {
	n := ansi.StringWidth(s)
	if n >= w {
		return s
	}
	return strings.Repeat(" ", (w-n)/2) + s
}

// catColumn is the small cat, or a text bot where colour or height is missing.
func (m Model) catColumn() []string {
	mood := m.mood(m.opt.now())
	if m.opt.Color && !m.opt.ASCII && (m.height == 0 || m.height >= 24) {
		o := mascot.Options{Profile: m.opt.Profile, Dark: m.opt.Dark, Mini: true}
		if m.opt.NoMotion {
			return mascot.StaticMini(mood, .4).Lines(o)
		}
		return m.frame.Lines(o)
	}
	return mascotArt(string(mood), m.opt.ASCII)
}

func (m Model) infoColumn() []string {
	account := m.snap.Account.Label
	if account == "" {
		account = m.snap.Status.AccountStatus
	}
	lines := []string{m.bold(m.greeting()), ""}
	lines = append(lines, m.t(i18n.Google)+": "+m.accountWordWith(account))
	dest := m.warn(m.t(i18n.NoDestLine))
	for _, d := range m.snap.Destinations {
		if d.Default {
			dest = Sanitize(d.Name)
		}
	}
	lines = append(lines, m.t(i18n.DestLine)+": "+dest)
	lines = append(lines, m.dim(fmt.Sprintf("%s · %s · %s",
		countWord(m.snap.Status.Running, m.t(i18n.Running)),
		countWord(m.snap.Status.Queued, m.t(i18n.Queued)),
		countWord(m.snap.Status.Blocked, m.t(i18n.Blocked)))))
	if b := m.snap.Status.BotState; b != "" && b != "disabled" {
		lines = append(lines, m.dim(m.t(i18n.Telegram)+": "+Sanitize(b)))
	}
	if m.needsGoogle() {
		lines = append(lines, "", m.warn(m.t(i18n.LoginHome)))
	}
	return lines
}

func (m Model) accountWordWith(label string) string {
	if m.snap.Status.AccountStatus == "connected" {
		return m.good(Sanitize(label))
	}
	return m.accountWord(m.snap.Status.AccountStatus)
}

// activityColumn is the "get started" tips of the panel.
func (m Model) activityColumn() []string {
	return []string{m.bold(m.t(i18n.PanelStart)), m.t(i18n.PanelTip1), m.t(i18n.PanelTip2)}
}

// recentBlock lists the latest jobs under the panel, as many as fit.
func (m Model) recentBlock(room int) []string {
	if room < 3 {
		return nil
	}
	lines := []string{"", "  " + m.bold(m.t(i18n.PanelRecent))}
	if len(m.snap.Jobs) == 0 {
		return append(lines, "  "+m.dim(m.t(i18n.NoRecent)))
	}
	for i, j := range m.snap.Jobs {
		if i >= room-2 {
			break
		}
		lines = append(lines, fmt.Sprintf("  %s %-32s %s %s", m.stateGlyph(j.State), cut(Sanitize(j.SourceName), 32),
			m.stateWord(j.State), m.dim(jobBrief(j))))
	}
	return lines
}

func jobBrief(j Job) string {
	if j.Total != nil && *j.Total > 0 {
		return fmt.Sprintf("%d%%", min(100, j.Completed*100 / *j.Total))
	}
	return stateWord(j.State)
}

func (m Model) stateGlyph(state string) string {
	glyph := "•"
	switch state {
	case "succeeded":
		return m.good("✔")
	case "failed", "cancelled":
		return m.bad("✖")
	case "partial", "blocked", "paused":
		return m.warn("‖")
	case "running", "queued", "pausing", "cancelling", "reconciling":
		return m.accent(m.spinChar())
	}
	return glyph
}

// busyBlock is shown only while something copies: the file-transfer strip and
// the running jobs with speed and time left.
func (m Model) busyBlock(pw int) []string {
	var running []Job
	for _, j := range m.snap.Jobs {
		if j.State == "running" {
			running = append(running, j)
		}
	}
	if len(running) == 0 {
		return nil
	}
	lines := []string{""}
	if m.opt.Color && !m.opt.ASCII && pw >= stripWidth+4 && (m.height == 0 || m.height >= 30) {
		o := mascot.Options{Profile: m.opt.Profile, Dark: m.opt.Dark}
		t := float64(m.opt.now().UnixMilli()%100000) / 1000
		if m.opt.NoMotion {
			t = .4
		}
		for _, l := range mascot.Strip(t).StripLines(o) {
			lines = append(lines, "  "+l)
		}
	}
	lines = append(lines, "  "+m.accent(m.workLine()))
	for i, j := range running {
		if i >= 3 {
			break
		}
		lines = append(lines, "  "+cut(Sanitize(j.SourceName), 30)+"  "+m.progressRow(j, min(28, max(10, pw-70))))
	}
	return lines
}

// promptBlock is the command box with the slash-command list above it.
func (m Model) promptBlock(pw int) []string {
	var out []string
	sugg := m.suggestions()
	for i, c := range sugg {
		line := fmt.Sprintf("  /%-9s %s", c.name, m.t(c.desc))
		if i == min(m.cmdSel, len(sugg)-1) {
			line = m.accent("> /"+fmt.Sprintf("%-9s", c.name)) + " " + m.t(c.desc)
		} else {
			line = m.dim(line)
		}
		out = append(out, line)
	}
	m.cmd.Placeholder = m.t(i18n.PromptHint)
	m.cmd.Width = max(8, pw-8)
	h, tl, tr, bl, br, v := "─", "╭", "╮", "╰", "╯", "│"
	if m.opt.ASCII {
		h, tl, tr, bl, br, v = "-", "+", "+", "+", "+", "|"
	}
	body := m.cmd.View()
	out = append(out, m.dim(tl+strings.Repeat(h, pw-2)+tr),
		m.dim(v)+" "+padCell(body, pw-4)+" "+m.dim(v),
		m.dim(bl+strings.Repeat(h, pw-2)+br))
	return out
}

var _ = time.Second

// jobsBanner is the cat at work: shown on the Jobs tab while something copies,
// with the transfer strip and the totals beside it. It needs colour and room.
func (m Model) jobsBanner() []string {
	if !m.hasRunning() || !m.opt.Color || m.opt.ASCII || m.width < 60 || (m.height != 0 && m.height < 28) {
		return nil
	}
	o := mascot.Options{Profile: m.opt.Profile, Dark: m.opt.Dark, Mini: true}
	var cat []string
	if m.opt.NoMotion {
		cat = mascot.StaticMini(mascot.Busy, .4).Lines(o)
	} else {
		cat = m.frame.Lines(o)
	}
	right := []string{m.accent(m.workLine()), ""}
	if m.width >= mascot.MiniCols+4+mascot.StripCols+2 {
		t := float64(m.opt.now().UnixMilli()%100000) / 1000
		if m.opt.NoMotion {
			t = .4
		}
		right = append(right, mascot.Strip(t).StripLines(mascot.Options{Profile: m.opt.Profile, Dark: m.opt.Dark})...)
	}
	rows := max(len(cat), len(right))
	out := make([]string, 0, rows+1)
	for r := 0; r < rows; r++ {
		l, rt := "", ""
		if r < len(cat) {
			l = cat[r]
		}
		if r < len(right) {
			rt = right[r]
		}
		out = append(out, " "+padCell(l, mascot.MiniCols)+"  "+rt)
	}
	return append(out, "")
}

func (m Model) hasRunning() bool {
	for _, j := range m.snap.Jobs {
		if j.State == "running" {
			return true
		}
	}
	return false
}
