package tui

import (
	"errors"
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
	// submitting is set while the job is being created.
	submitting bool
	// sync is the schedule enabled right after the copy: off, hourly, daily, realtime.
	sync int
	req  int
	err  string
	// opt are the shortcut, name clash and filter choices; they travel with
	// the preview, so the preview always matches what will run.
	opt                          CopyOptions
	filterIdx, clashIdx, scutIdx int
	// choose is set when the daemon says this source is already copied to
	// this destination: the dialog then asks update or new.
	choose bool
	exists string
}

type planMsg struct {
	req  int
	plan Plan
	err  error
}

type submitMsg struct {
	jobID   string
	syncErr error
	err     error
}

// sync choices, in the order the s key cycles through them.
var syncPresets = []string{"", presetHourly, presetDaily, presetRealtime}

// startCopy opens the dialog. An empty source asks for a link first.
func (m Model) startCopy(source string, from screen) (Model, tea.Cmd) {
	if m.acts.Inspect == nil && m.acts.InspectWith == nil {
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
	source, dest, name, opt := c.source, c.destID, m.rootName(), c.opt
	return func() tea.Msg {
		var plan Plan
		var err error
		if acts.InspectWith != nil {
			plan, err = acts.InspectWith(ctx, source, dest, name, opt)
		} else {
			plan, err = acts.Inspect(ctx, source, dest, name)
		}
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
	m.cp.planning, m.cp.submitting = false, false
	if msg.err != nil {
		var existing *ExistingCopyError
		if errors.As(msg.err, &existing) {
			m.cp.choose, m.cp.exists, m.cp.err = true, Sanitize(existing.Message), ""
			return m, nil
		}
		m.cp.err = Sanitize(msg.err.Error())
		return m, nil
	}
	m.cp = nil
	m.status = i18n.Tf(m.lang, i18n.CopyStarted, Sanitize(msg.jobID))
	if msg.syncErr != nil {
		m.status += " ⚠ " + Sanitize(msg.syncErr.Error())
	}
	next, cmd := m.goTo(screenJobs)
	return next.(Model), tea.Batch(cmd, next.(Model).refetch())
}

// copyKey handles the plan step (the typing steps go through copyTyping).
func (m Model) copyKey(key string) (tea.Model, tea.Cmd) {
	c := m.cp
	if c == nil {
		return m, nil
	}
	if c.choose { // the dialog waits for update or new
		switch key {
		case "u":
			return m.submit("update")
		case "n":
			return m.submit("new")
		}
		return m, nil
	}
	switch key {
	case "enter":
		return m.submit("")
	case "f":
		return m.setOption(func(c *copyFlow) {
			c.filterIdx = (c.filterIdx + 1) % len(filterPresets)
			c.opt.FilterPreset = filterPresets[c.filterIdx]
		})
	case "o":
		return m.setOption(func(c *copyFlow) {
			c.clashIdx = (c.clashIdx + 1) % len(clashChoices)
			c.opt.OnConflict = clashChoices[c.clashIdx]
		})
	case "k":
		return m.setOption(func(c *copyFlow) {
			c.scutIdx = (c.scutIdx + 1) % len(shortcutChoices)
			c.opt.Shortcuts = shortcutChoices[c.scutIdx]
		})
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
	case "s":
		c.sync = (c.sync + 1) % len(syncPresets)
	}
	return m, nil
}

// clashChoices and shortcutChoices are what o and k cycle through; "" is the
// daemon default (rename, follow).
var (
	clashChoices    = []string{"", "skip", "update"}
	shortcutChoices = []string{"", "skip"}
)

// setOption changes one copy option and asks for a fresh preview.
func (m Model) setOption(change func(*copyFlow)) (tea.Model, tea.Cmd) {
	c := m.cp
	if c.planning || c.plan == nil {
		return m, nil
	}
	change(c)
	return m, m.replan()
}

// submit starts the job. mode answers an existing copy: "" lets the daemon
// ask (the dialog then offers update or new), "update" refreshes the old copy,
// "new" copies again.
func (m Model) submit(mode string) (tea.Model, tea.Cmd) {
	c := m.cp
	if c.plan == nil || c.planning || (m.acts.Submit == nil && m.acts.SubmitMode == nil) {
		return m, nil
	}
	plan, acts, ctx, preset := *c.plan, m.acts, m.ctx, syncPresets[c.sync]
	c.planning, c.submitting, c.err, c.choose = true, true, "", false
	return m, tea.Batch(func() tea.Msg {
		var id, collection string
		var err error
		if acts.SubmitMode != nil {
			id, collection, err = acts.SubmitMode(ctx, plan, mode)
		} else {
			id, collection, err = acts.Submit(ctx, plan)
		}
		res := submitMsg{jobID: id, err: err}
		if err == nil && preset != "" && collection != "" && acts.SyncPreset != nil {
			// A schedule that cannot be set must not hide that the copy started.
			res.syncErr = acts.SyncPreset(ctx, collection, preset)
		}
		return res
	}, m.startSpinner())
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
		return append(lines, "", m.accent(m.spinChar())+" "+m.dim(m.t(i18n.CopyChecking)))
	}
	p := c.plan
	count := i18n.Tf(m.lang, i18n.PreviewCount, p.Files, p.Folders, partial(m, p))
	lines = append(lines, "",
		m.kv(m.t(i18n.CopyFrom), Sanitize(p.SourceName)+"  "+m.dim("("+Sanitize(p.SourceKind)+" · "+count+")")),
		m.kv(m.t(i18n.CopyTo), Sanitize(m.destinationName(p))),
		m.kv(m.t(i18n.CopyOutName), m.accent(Sanitize(p.RootName))),
		m.kv(m.t(i18n.SyncOption), m.syncChoice()),
		"", m.nameOptions())
	lines = append(lines, m.copyOptionLines()...)
	lines = append(lines, m.filterPreviewLines(p)...)
	for _, w := range p.Warnings {
		lines = append(lines, m.warn("! "+Sanitize(w)))
	}
	if c.choose {
		lines = append(lines, "", m.warn(m.t(i18n.ExistingCopyTitle)))
		if c.exists != "" {
			for _, l := range wrapLines(c.exists, max(20, m.width-2)) {
				lines = append(lines, m.dim(l))
			}
		}
		lines = append(lines, m.accent(m.t(i18n.ExistingCopyAsk)+" "+m.t(i18n.ExistingCopyKeys)))
	}
	switch {
	case c.planning:
		lines = append(lines, "", m.accent(m.spinChar())+" "+m.dim(m.t(i18n.CopyChecking)))
	case c.err != "":
		lines = append(lines, "", m.bad(c.err))
	}
	if c.typing == "name" && c.input != nil {
		lines = append(lines, "", m.accent(m.t(i18n.NamePrompt)), c.input.View())
	}
	return lines
}

// copyOptionLines shows the filter, name clash and shortcut choices in one
// wrapped line, so a narrow terminal reads all three.
func (m Model) copyOptionLines() []string {
	c := m.cp
	clash := c.opt.OnConflict
	short := m.t(i18n.ShortcutsFollow)
	if c.opt.Shortcuts == "skip" {
		short = m.t(i18n.ShortcutsSkip)
	}
	text := m.t(i18n.FilterTitle) + ": " + m.presetLabel(c.opt.FilterPreset) +
		" · " + m.t(i18n.ConflictTitle) + ": " + m.conflictLabel(clash) +
		" · " + m.t(i18n.ShortcutsTitle) + ": " + short
	out := append([]string{""}, wrapLines(text, max(20, m.width-2))...)
	return append(out, m.dim(m.t(i18n.CopyOptsKeys)))
}

// filterPreviewLines tells what the rule would keep and drop. It says the
// numbers cover only the top level, as the daemon does.
func (m Model) filterPreviewLines(p *Plan) []string {
	f := p.Filter
	if f == nil {
		return nil
	}
	head := m.tf(i18n.FilterPreview, f.Included, f.Excluded)
	if f.Scope == "top_level" {
		head += " (" + m.t(i18n.FilterTopLevel) + ")"
	}
	lines := []string{m.good(head)}
	if len(f.ExcludedSamples) > 0 {
		lines = append(lines, m.dim(m.t(i18n.FilterSamples)+":"))
		for i, s := range f.ExcludedSamples {
			if i >= 3 {
				break
			}
			lines = append(lines, "  - "+cut(Sanitize(s), max(10, m.width-6)))
		}
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
		if m.cp != nil && m.cp.choose { // cancel the question, keep the dialog
			m.cp.choose = false
			return m, nil
		}
		return m.leaveCopy()
	}
	return m.copyKey(msg.String())
}

// syncChoice shows the four schedules with the chosen one marked.
func (m Model) syncChoice() string {
	labels := []string{m.t(i18n.SyncOff), m.t(i18n.PresetHourly), m.t(i18n.PresetDaily), m.t(i18n.PresetReal)}
	parts := make([]string, len(labels))
	for i, l := range labels {
		if i == m.cp.sync {
			parts[i] = m.good("● " + l)
		} else {
			parts[i] = m.dim("○ " + l)
		}
	}
	return strings.Join(parts, "  ")
}
