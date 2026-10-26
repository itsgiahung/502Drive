package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/itsgiahung/502Drive/internal/i18n"
)

// How the output folder is named.
const (
	nameKeep   = iota // the source's own name
	nameAuto          // "<name> (copy 2026-10-27)"
	nameCustom        // typed by the user
)

// copyFlow is the guided copy: source → read-only preview (with destination
// and output name) → start. Every change of destination or name re-runs the
// preview, so what is shown is exactly what will be created.
type copyFlow struct {
	from     screen
	source   string
	input    *textinput.Model // set while the source or a custom name is typed
	typing   string           // "source" or "name"
	destID   string
	destName string
	nameMode int
	custom   string
	plan     *Plan
	planning bool
	req      int
	err      string
}

type planMsg struct {
	req  int
	plan Plan
	err  error
}

type submitMsg struct {
	jobID string
	err   error
}

// startCopy opens the dialog. An empty source asks for a link first.
func (m Model) startCopy(source string, from screen) (Model, tea.Cmd) {
	if m.acts.Inspect == nil {
		return m, nil
	}
	m.screen, m.drv = screenCopy, nil
	m.cp = &copyFlow{from: from, source: source}
	if source == "" {
		m.cp.typing, m.cp.input = "source", newInput(m.width, "https://drive.google.com/…")
		return m, textinput.Blink
	}
	return m, m.replan()
}

func (m Model) autoName(base string) string {
	return fmt.Sprintf("%s (copy %s)", base, m.opt.now().Format("2006-01-02"))
}

// rootName is what to ask the daemon for ("" = the source's own name).
func (m Model) rootName() string {
	c := m.cp
	switch {
	case c.nameMode == nameAuto && c.plan != nil:
		return m.autoName(c.plan.SourceName)
	case c.nameMode == nameCustom:
		return c.custom
	}
	return ""
}

// replan asks for a fresh preview with the current destination and name.
func (m Model) replan() tea.Cmd {
	c := m.cp
	c.req++
	c.planning, c.err = true, ""
	req, acts, ctx := c.req, m.acts, m.ctx
	source, dest, name := c.source, c.destID, m.rootName()
	return func() tea.Msg {
		plan, err := acts.Inspect(ctx, source, dest, name)
		return planMsg{req: req, plan: plan, err: err}
	}
}

func (m Model) onPlan(msg planMsg) (tea.Model, tea.Cmd) {
	c := m.cp
	if c == nil || msg.req != c.req {
		return m, nil
	}
	c.planning = false
	if msg.err != nil {
		c.err = Sanitize(msg.err.Error())
		if c.plan == nil { // nothing to show yet: ask for the link again
			c.typing, c.input = "source", newInput(m.width, "")
			c.input.SetValue(c.source)
			return m, textinput.Blink
		}
		return m, nil
	}
	p := msg.plan
	c.plan = &p
	return m, nil
}

func (m Model) onSubmit(msg submitMsg) (tea.Model, tea.Cmd) {
	if m.cp == nil {
		return m, nil
	}
	m.cp.planning = false
	if msg.err != nil {
		m.cp.err = Sanitize(msg.err.Error())
		return m, nil
	}
	m.cp = nil
	m.status = i18n.Tf(m.lang, i18n.CopyStarted, Sanitize(msg.jobID))
	next, cmd := m.goTo(screenJobs)
	return next.(Model), tea.Batch(cmd, next.(Model).refetch())
}

// copyKey handles the plan step (the typing steps go through copyTyping).
func (m Model) copyKey(key string) (tea.Model, tea.Cmd) {
	c := m.cp
	if c == nil {
		return m, nil
	}
	switch key {
	case "enter":
		if c.plan == nil || c.planning || m.acts.Submit == nil {
			return m, nil
		}
		plan, acts, ctx := *c.plan, m.acts, m.ctx
		c.planning, c.err = true, ""
		return m, func() tea.Msg {
			id, err := acts.Submit(ctx, plan)
			return submitMsg{jobID: id, err: err}
		}
	case "1":
		return m.setNameMode(nameKeep)
	case "2":
		return m.setNameMode(nameAuto)
	case "3":
		c.typing, c.input = "name", newInput(m.width, "")
		if c.plan != nil {
			c.input.SetValue(c.plan.RootName)
		}
		return m, textinput.Blink
	case "d":
		next, cmd := m.openDrive(true)
		return next, cmd
	}
	return m, nil
}

func (m Model) setNameMode(mode int) (tea.Model, tea.Cmd) {
	if m.cp.nameMode == mode || m.cp.planning {
		return m, nil
	}
	m.cp.nameMode = mode
	return m, m.replan()
}

func (m Model) copyTyping(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	c := m.cp
	switch msg.Type {
	case tea.KeyEsc:
		if c.typing == "name" && c.plan != nil {
			c.input, c.typing = nil, ""
			return m, nil
		}
		return m.leaveCopy()
	case tea.KeyEnter:
		value := strings.TrimSpace(c.input.Value())
		if value == "" {
			return m, nil
		}
		typed := c.typing
		c.input, c.typing = nil, ""
		if typed == "source" {
			c.source = value
		} else {
			c.custom, c.nameMode = value, nameCustom
		}
		return m, m.replan()
	}
	var cmd tea.Cmd
	*c.input, cmd = c.input.Update(msg)
	return m, cmd
}

// leaveCopy closes the dialog and returns to where it was opened.
func (m Model) leaveCopy() (tea.Model, tea.Cmd) {
	back := screenHome
	if m.cp != nil && m.cp.from != screenCopy {
		back = m.cp.from
	}
	m.cp = nil
	if back == screenDrive {
		return m.openDrive(false)
	}
	return m.goTo(back)
}

// ---- view ----

func (m Model) copyView() []string {
	c := m.cp
	if c == nil {
		return nil
	}
	lines := []string{"", m.bold(m.t(i18n.CopyTitle))}
	if c.typing == "source" && c.input != nil {
		lines = append(lines, "", m.t(i18n.CopyPaste), c.input.View())
		if c.err != "" {
			lines = append(lines, "", m.bad(c.err))
		}
		return lines
	}
	if c.plan == nil {
		return append(lines, "", m.dim(m.t(i18n.CopyChecking)))
	}
	p := c.plan
	count := i18n.Tf(m.lang, i18n.PreviewCount, p.Files, p.Folders, partial(m, p))
	lines = append(lines, "",
		m.kv(m.t(i18n.CopyFrom), Sanitize(p.SourceName)+"  "+m.dim("("+Sanitize(p.SourceKind)+" · "+count+")")),
		m.kv(m.t(i18n.CopyTo), Sanitize(m.destinationName(p))),
		m.kv(m.t(i18n.CopyOutName), m.accent(Sanitize(p.RootName))),
		"", m.nameOptions())
	for _, w := range p.Warnings {
		lines = append(lines, m.warn("! "+Sanitize(w)))
	}
	switch {
	case c.planning:
		lines = append(lines, "", m.dim(m.t(i18n.CopyChecking)))
	case c.err != "":
		lines = append(lines, "", m.bad(c.err))
	}
	if c.typing == "name" && c.input != nil {
		lines = append(lines, "", m.accent(m.t(i18n.NamePrompt)), c.input.View())
	}
	return lines
}

func partial(m Model, p *Plan) string {
	if p.Complete {
		return ""
	}
	return m.t(i18n.PreviewPartial)
}

func (m Model) destinationName(p *Plan) string {
	for _, d := range m.snap.Destinations {
		if d.ID == p.DestinationID {
			return d.Name
		}
	}
	if m.cp != nil && m.cp.destName != "" {
		return m.cp.destName
	}
	return p.DestinationID
}

// nameOptions marks the active choice with a filled dot.
func (m Model) nameOptions() string {
	opts := []struct {
		mode int
		key  i18n.Key
	}{{nameKeep, i18n.NameKeep}, {nameAuto, i18n.NameAuto}, {nameCustom, i18n.NameCustom}}
	parts := make([]string, 0, 3)
	for _, o := range opts {
		label := m.t(o.key)
		if o.mode == m.cp.nameMode {
			label = m.good("● " + label)
		} else {
			label = m.dim("○ " + label)
		}
		parts = append(parts, label)
	}
	return strings.Join(parts, "   ")
}

// copyScreenKey is the dialog's own key map; everything else is ignored so a
// stray digit cannot jump to another screen mid-copy.
func (m Model) copyScreenKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		m.quits = true
		return m, tea.Quit
	case tea.KeyEsc:
		return m.leaveCopy()
	}
	return m.copyKey(msg.String())
}
