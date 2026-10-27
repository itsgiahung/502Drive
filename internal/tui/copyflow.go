package tui

import (
	"errors"
	"fmt"
	"strings"
	"time"

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
	from        screen
	source      string
	input       *textinput.Model // set while the source or a custom name is typed
	typing      string           // "source" or "name"
	destID      string
	destName    string
	nameMode    int
	custom      string
	plan        *Plan
	planning    bool
	deepLoading bool
	// submitting is set while the job is being created.
	submitting bool
	// sync is the index in afterModes: the sync mode the new collection gets
	// right after the copy (S8). It starts at the saved preference.
	sync int
	req  int
	err  string
	// stale is set when the last preview failed: what is shown is not what
	// would run, so Enter does nothing until a preview succeeds.
	stale bool
	// sources holds the links of a batch (S6) when several were pasted; a
	// single copy keeps its link in source.
	sources []string
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
	jobID string
	err   error
}

// startCopy opens the dialog. An empty source asks for a link first; several
// links (one per line, or separated by spaces) make a batch.
func (m Model) startCopy(source string, from screen) (Model, tea.Cmd) {
	if m.acts.Inspect == nil && m.acts.InspectWith == nil {
		return m, nil
	}
	// Text typed or pasted may be words to search for; an id from the Drive
	// browser never is.
	if source != "" && from != screenDrive && m.acts.Search != nil && !allRefs(source) {
		m.screen, m.drv = screenCopy, nil // openSearch replaces the dialog
		m.cp = &copyFlow{from: from}
		return m.openSearch(source)
	}
	m.screen, m.drv = screenCopy, nil
	m.cp = &copyFlow{from: from, sync: m.defaultAfterSync()}
	if source == "" {
		m.cp.typing, m.cp.input = "source", newInput(m.width, "https://drive.google.com/…")
		return m, textinput.Blink
	}
	m.cp.setSources(source)
	return m, m.beginPreview()
}

// setSources stores the link, or the links of a batch.
func (c *copyFlow) setSources(text string) {
	refs := splitRefs(text)
	c.sources = nil
	if len(refs) > 1 {
		c.sources, c.source = refs, refs[0]
		return
	}
	c.source = strings.TrimSpace(text)
	if len(refs) == 1 {
		c.source = refs[0]
	}
}

// isBatch says the dialog holds several links.
func (c *copyFlow) isBatch() bool { return len(c.sources) > 1 }

// beginPreview asks for the preview of a single link; a batch has none (each
// link is checked when it is sent).
func (m Model) beginPreview() tea.Cmd {
	if m.cp.isBatch() {
		return nil
	}
	return m.replan()
}

// splitRefs splits pasted text into distinct Drive references. A terminal
// turns the line breaks of a paste into spaces, so both separate links.
func splitRefs(text string) []string {
	var out []string
	seen := map[string]bool{}
	for _, f := range strings.Fields(text) {
		if !seen[f] {
			seen[f] = true
			out = append(out, f)
		}
	}
	return out
}

// allRefs says every word of text can be a Drive link or file id.
func allRefs(text string) bool {
	fields := strings.Fields(text)
	for _, f := range fields {
		if !looksLikeRef(f) {
			return false
		}
	}
	return len(fields) > 0
}

// looksLikeRef says whether text can be a Drive link or file id, as opposed to
// words the user wants to search for by name.
func looksLikeRef(text string) bool {
	text = strings.TrimSpace(text)
	if strings.Contains(text, "://") || strings.Contains(text, "drive.google.com") || strings.Contains(text, "docs.google.com") {
		return true
	}
	if len(strings.Fields(text)) > 1 {
		return false
	}
	if len(text) < 15 {
		return false
	}
	for _, r := range text {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

// defaultAfterSync is the index of the saved after-copy sync mode.
func (m Model) defaultAfterSync() int {
	for i, mode := range afterModes {
		if mode == m.snap.Prefs.AfterCopySync {
			return i
		}
	}
	return 0
}

// afterSyncChoice is what travels with the job: "" when the user kept the
// saved preference (the daemon applies it), else the chosen mode.
func (m Model) afterSyncChoice() string {
	mode := afterModes[m.cp.sync%len(afterModes)]
	if mode == m.snap.Prefs.AfterCopySync || (mode == afterOff && m.snap.Prefs.AfterCopySync == "") {
		return ""
	}
	return mode
}

// hasAfterSync says whether this build can send the sync choice.
func (m Model) hasAfterSync() bool {
	if m.cp != nil && m.cp.isBatch() {
		return m.acts.SubmitBatch != nil
	}
	return m.acts.SubmitOpts != nil
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
	preview := func() tea.Msg {
		var plan Plan
		var err error
		if acts.InspectWith != nil {
			plan, err = acts.InspectWith(ctx, source, dest, name, opt)
		} else {
			plan, err = acts.Inspect(ctx, source, dest, name)
		}
		return planMsg{req: req, plan: plan, err: err}
	}
	// A new preview replaces the old one: stop its full scan, which nobody reads.
	if c.plan != nil && c.plan.Deep != nil && c.plan.Deep.Running() && acts.CancelPreflight != nil {
		old := c.plan.ID
		return tea.Batch(preview, func() tea.Msg {
			_, _ = acts.CancelPreflight(ctx, old)
			return nil
		})
	}
	return preview
}

func (m Model) onPlan(msg planMsg) (tea.Model, tea.Cmd) {
	c := m.cp
	if c == nil || msg.req != c.req {
		return m, nil
	}
	c.planning = false
	if msg.err != nil {
		c.err = m.planError(msg.err)
		if c.plan == nil { // nothing to show yet: ask for the link again
			c.typing, c.input = "source", newInput(m.width, "")
			c.input.SetValue(c.source)
			return m, textinput.Blink
		}
		c.stale = true
		return m, nil
	}
	p := msg.plan
	c.plan, c.stale = &p, false
	if p.Deep != nil && p.Deep.Running() && m.acts.GetPlan != nil {
		return m, deepTick(p.ID)
	}
	return m, nil
}

// planError words a refused preview; a destination the account cannot write to
// gets its own sentence and the way out.
func (m Model) planError(err error) string {
	var ce *CodedError
	if errors.As(err, &ce) {
		switch ce.Code {
		case "destination_not_writable":
			return m.t(i18n.DestNotWritable)
		case "destination_unverifiable":
			return m.t(i18n.DestUnverifiable)
		}
	}
	return Sanitize(err.Error())
}

// ---- deep preview (S7) ----

const deepEvery = 700 * time.Millisecond

type deepTickMsg struct {
	planID  string
	changed bool
}
type deepMsg struct {
	planID string
	plan   Plan
	err    error
}

func deepTick(planID string) tea.Cmd {
	return tea.Tick(deepEvery, func(time.Time) tea.Msg { return deepTickMsg{planID: planID} })
}

// onDeepTick reads the plan again while its full scan runs. It only reads: the
// scan belongs to the daemon and goes on if the dialog is closed.
func (m Model) onDeepTick(msg deepTickMsg) (tea.Model, tea.Cmd) {
	c := m.cp
	if (m.watching && !msg.changed) || c == nil || c.deepLoading || c.plan == nil || c.plan.ID != msg.planID || c.plan.Deep == nil || !c.plan.Deep.Running() || m.acts.GetPlan == nil {
		return m, nil
	}
	c.deepLoading = true
	acts, ctx, id := m.acts, m.ctx, msg.planID
	return m, func() tea.Msg {
		p, err := acts.GetPlan(ctx, id)
		return deepMsg{planID: id, plan: p, err: err}
	}
}

func (m Model) onDeep(msg deepMsg) (tea.Model, tea.Cmd) {
	c := m.cp
	if c == nil || c.plan == nil || c.plan.ID != msg.planID {
		return m, nil
	}
	c.deepLoading = false
	if msg.err != nil {
		c.err = Sanitize(msg.err.Error())
		return m, nil
	}
	c.err = ""
	if msg.plan.Deep != nil {
		c.plan.Deep = msg.plan.Deep
		c.plan.Files, c.plan.Folders, c.plan.Complete = msg.plan.Files, msg.plan.Folders, msg.plan.Complete
	}
	if !m.watching && c.plan.Deep != nil && c.plan.Deep.Running() {
		return m, deepTick(msg.planID)
	}
	return m, nil
}

// toggleDeep turns the full scan on or off and asks for a fresh preview.
func (m Model) toggleDeep() (tea.Model, tea.Cmd) {
	if m.cp.isBatch() { // a batch has no single plan to scan
		return m, nil
	}
	return m.setOption(func(c *copyFlow) { c.opt.Deep = !c.opt.Deep })
}

// cancelDeep stops the running full scan; the plan stays usable.
func (m Model) cancelDeep() (tea.Model, tea.Cmd) {
	c := m.cp
	if c == nil || c.plan == nil || c.plan.Deep == nil || !c.plan.Deep.Running() || m.acts.CancelPreflight == nil {
		return m, nil
	}
	acts, ctx, id := m.acts, m.ctx, c.plan.ID
	return m, func() tea.Msg {
		p, err := acts.CancelPreflight(ctx, id)
		return deepMsg{planID: id, plan: p, err: err}
	}
}

// ---- batch (S6) ----

type batchSubmitMsg struct {
	res BatchResult
	err error
}

// submitBatch sends every link as one batch. The daemon checks each link and
// refuses the bad ones on their own; the others still start.
func (m Model) submitBatch() (tea.Model, tea.Cmd) {
	c := m.cp
	if c == nil || c.submitting || m.acts.SubmitBatch == nil {
		return m, nil
	}
	c.submitting, c.err = true, ""
	req := BatchRequest{Sources: append([]string(nil), c.sources...), DestinationID: c.destID, AutoName: c.nameMode == nameAuto,
		Options: c.opt, Submit: SubmitOptions{AfterSync: m.afterSyncChoice()}}
	acts, ctx := m.acts, m.ctx
	return m, tea.Batch(func() tea.Msg {
		res, err := acts.SubmitBatch(ctx, req)
		return batchSubmitMsg{res: res, err: err}
	}, m.startSpinner())
}

func (m Model) onBatchSubmit(msg batchSubmitMsg) (tea.Model, tea.Cmd) {
	c := m.cp
	if c == nil {
		return m, nil
	}
	c.submitting = false
	accepted, failed := msg.res.Counts()
	if msg.err != nil && accepted == 0 {
		c.err = Sanitize(msg.err.Error())
		return m, nil
	}
	if accepted == 0 {
		c.err = m.batchFailures(msg.res)
		return m, nil
	}
	m.cp = nil
	m.status = m.tf(i18n.BatchResultText, accepted, failed)
	if failed > 0 {
		m.status += " — " + m.batchFailures(msg.res)
	}
	next, cmd := m.goTo(screenJobs)
	return next.(Model), tea.Batch(cmd, next.(Model).refetch())
}

// batchFailures lists the refused links, first few only.
func (m Model) batchFailures(res BatchResult) string {
	var parts []string
	for _, j := range res.Jobs {
		if j.JobID != "" {
			continue
		}
		if len(parts) == 2 {
			parts = append(parts, "…")
			break
		}
		parts = append(parts, m.tf(i18n.BatchFailedRow, cut(Sanitize(j.Source), 40), Sanitize(j.Error)))
	}
	return strings.Join(parts, "; ")
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
		if c.isBatch() {
			return m.submitBatch()
		}
		return m.submit("")
	case "i":
		return m.toggleDeep()
	case "x":
		return m.cancelDeep()
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
		if c.isBatch() { // one name cannot fit every link
			return m, nil
		}
		c.typing, c.input = "name", newInput(m.width, "")
		if c.plan != nil {
			c.input.SetValue(c.plan.RootName)
		}
		return m, textinput.Blink
	case "d":
		next, cmd := m.openDrive(true)
		return next, cmd
	case "s":
		if m.hasAfterSync() {
			c.sync = (c.sync + 1) % len(afterModes)
		}
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
	if c.isBatch() { // no preview to refresh: the options apply to every link
		change(c)
		return m, nil
	}
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
	if c.plan == nil || c.planning || c.stale || (m.acts.Submit == nil && m.acts.SubmitMode == nil && m.acts.SubmitOpts == nil) {
		return m, nil
	}
	plan, acts, ctx, after := *c.plan, m.acts, m.ctx, m.afterSyncChoice()
	c.planning, c.submitting, c.err, c.choose = true, true, "", false
	return m, tea.Batch(func() tea.Msg {
		var id string
		var err error
		switch {
		case acts.SubmitOpts != nil:
			id, _, err = acts.SubmitOpts(ctx, plan, SubmitOptions{Mode: mode, AfterSync: after})
		case acts.SubmitMode != nil:
			id, _, err = acts.SubmitMode(ctx, plan, mode)
		default:
			id, _, err = acts.Submit(ctx, plan)
		}
		return submitMsg{jobID: id, err: err}
	}, m.startSpinner())
}

func (m Model) setNameMode(mode int) (tea.Model, tea.Cmd) {
	if m.cp.nameMode == mode || m.cp.planning {
		return m, nil
	}
	m.cp.nameMode = mode
	return m, m.beginPreview()
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
		if typed == "source" && !allRefs(value) && m.acts.Search != nil {
			// Words, not a link: look for the item by name in Drive.
			c.input, c.typing = nil, ""
			return m.openSearch(value)
		}
		c.input, c.typing = nil, ""
		if typed == "source" {
			c.setSources(value)
			return m, m.beginPreview()
		}
		c.custom, c.nameMode = value, nameCustom
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
	if c.isBatch() {
		return append(lines, m.batchView()...)
	}
	if c.plan == nil {
		return append(lines, "", m.accent(m.spinChar())+" "+m.dim(m.t(i18n.CopyChecking)))
	}
	p := c.plan
	count := i18n.Tf(m.lang, i18n.PreviewCount, p.Files, p.Folders, partial(m, p))
	lines = append(lines, "",
		m.kv(m.t(i18n.CopyFrom), Sanitize(p.SourceName)+"  "+m.dim("("+Sanitize(p.SourceKind)+" · "+count+")")),
		m.kv(m.t(i18n.CopyTo), Sanitize(m.destinationName(p))),
		m.kv(m.t(i18n.CopyOutName), m.accent(Sanitize(p.RootName))))
	lines = append(lines, m.syncLines()...)
	lines = append(lines, "", m.nameOptions())
	lines = append(lines, m.copyOptionLines()...)
	lines = append(lines, m.filterPreviewLines(p)...)
	lines = append(lines, m.deepLines(p)...)
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
	hint := m.t(i18n.CopyOptsKeys)
	if m.acts.InspectWith != nil && !c.isBatch() {
		hint += " · " + m.t(i18n.CopyItKeys)
	}
	for _, l := range wrapLines(hint, max(20, m.width-2)) {
		out = append(out, m.dim(l))
	}
	return out
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
	case tea.KeyEsc, tea.KeyLeft:
		if m.cp != nil && m.cp.choose { // cancel the question, keep the dialog
			m.cp.choose = false
			return m, nil
		}
		return m.leaveCopy()
	}
	return m.copyKey(msg.String())
}

// syncLines shows the after-copy sync choice (S8) with the chosen mode marked.
// The five choices do not fit beside the label on a narrow terminal, so they
// flow onto the lines below it.
func (m Model) syncLines() []string {
	if !m.hasAfterSync() {
		return nil
	}
	labels := []string{m.t(i18n.AfterOff), m.t(i18n.AfterRealtime), m.t(i18n.AfterHourly), m.t(i18n.AfterSixHourly), m.t(i18n.AfterDaily)}
	parts := make([]string, len(labels))
	for i, l := range labels {
		if i == m.cp.sync%len(afterModes) {
			parts[i] = m.good("● " + l)
		} else {
			parts[i] = m.dim("○ " + l)
		}
	}
	const labelCol = 13
	rows := flowParts(parts, max(12, m.width-labelCol-1))
	out := make([]string, 0, len(rows))
	for i, r := range rows {
		label := ""
		if i == 0 {
			label = m.t(i18n.AfterSyncLabel)
		}
		out = append(out, m.kv(label, r))
	}
	return out
}

// flowParts joins parts with two spaces and breaks the line before it passes
// width display columns.
func flowParts(parts []string, width int) []string {
	var rows []string
	cur := ""
	for _, p := range parts {
		switch {
		case cur == "":
			cur = p
		case visible(cur)+2+visible(p) <= width:
			cur += "  " + p
		default:
			rows = append(rows, cur)
			cur = p
		}
	}
	if cur != "" {
		rows = append(rows, cur)
	}
	return rows
}

// batchView is the dialog for several pasted links.
func (m Model) batchView() []string {
	c := m.cp
	lines := []string{"", m.bold(m.tf(i18n.BatchTitle, len(c.sources)))}
	for i, src := range c.sources {
		if i >= 4 {
			lines = append(lines, m.dim(fmt.Sprintf("  … +%d", len(c.sources)-i)))
			break
		}
		lines = append(lines, "  - "+cut(Sanitize(src), max(10, m.width-6)))
	}
	lines = append(lines, "", m.kv(m.t(i18n.CopyTo), Sanitize(m.batchDestination())))
	lines = append(lines, m.kv(m.t(i18n.CopyOutName), m.batchNameOptions()))
	lines = append(lines, m.syncLines()...)
	lines = append(lines, m.copyOptionLines()...)
	for _, l := range wrapLines(m.t(i18n.BatchNote), max(20, m.width-2)) {
		lines = append(lines, m.dim(l))
	}
	switch {
	case c.submitting:
		lines = append(lines, "", m.accent(m.spinChar())+" "+m.dim(m.t(i18n.CopyChecking)))
	case c.err != "":
		lines = append(lines, "")
		for _, l := range wrapLines(c.err, max(20, m.width-2)) {
			lines = append(lines, m.bad(l))
		}
	}
	return lines
}

// batchDestination is the chosen folder, else the default one.
func (m Model) batchDestination() string {
	c := m.cp
	for _, d := range m.snap.Destinations {
		if (c.destID != "" && d.ID == c.destID) || (c.destID == "" && d.Default) {
			return d.Name
		}
	}
	return orElse(c.destName, orElse(c.destID, "—"))
}

// batchNameOptions offers the source's own name or "name (copy date)"; one
// typed name cannot fit every link.
func (m Model) batchNameOptions() string {
	keep, auto := m.dim("○ "+m.t(i18n.NameKeep)), m.dim("○ "+m.t(i18n.NameAuto))
	if m.cp.nameMode == nameAuto {
		auto = m.good("● " + m.t(i18n.NameAuto))
	} else {
		keep = m.good("● " + m.t(i18n.NameKeep))
	}
	return keep + "  " + auto
}

// deepLines shows the full scan (S7): totals first, what the copy will skip,
// the estimate and the warnings. Until the scan finishes the counts are "so
// far" and say so; they are never presented as a total.
func (m Model) deepLines(p *Plan) []string {
	d := p.Deep
	if d == nil {
		return nil
	}
	lines := []string{"", m.bold(m.t(i18n.DeepTitle))}
	switch d.State {
	case "running":
		lines = append(lines, m.accent(m.spinChar())+" "+m.tf(i18n.DeepRunning, d.Scanned)+" · "+m.t(i18n.DeepTotalUnknown))
	case "complete":
		lines = append(lines, m.good(m.t(i18n.DeepDone)))
	case "cancelled":
		lines = append(lines, m.warn(m.t(i18n.DeepCancelledText)))
	case "expired":
		lines = append(lines, m.warn(m.t(i18n.DeepExpiredText)))
	case "interrupted":
		lines = append(lines, m.warn(m.t(i18n.DeepInterrupted)))
	default:
		lines = append(lines, m.bad(m.t(i18n.DeepFailedText)))
	}
	totals := m.tf(i18n.DeepTotals, d.Files, d.Folders, bytesText(d.Bytes))
	if !d.Complete {
		totals += " · " + m.t(i18n.DeepNotTotal)
	}
	for _, l := range wrapLines(totals, max(20, m.width-2)) {
		lines = append(lines, "  "+l)
	}
	for _, l := range wrapLines(m.tf(i18n.DeepExtra, d.Native, d.Shortcuts, d.CannotCopy, d.Unreadable), max(20, m.width-4)) {
		lines = append(lines, "  "+m.dim(l))
	}
	if d.Complete {
		est := m.t(i18n.DeepEstimateNone)
		if d.EstimateSeconds != nil {
			est = m.tf(i18n.DeepEstimate, m.humanDuration(time.Duration(*d.EstimateSeconds)*time.Second))
		}
		for _, l := range wrapLines(est, max(20, m.width-4)) {
			lines = append(lines, "  "+m.dim(l))
		}
	}
	for _, w := range d.Warnings {
		for i, l := range wrapLines(m.deepWarning(w), max(20, m.width-6)) {
			prefix := "  ! "
			if i > 0 {
				prefix = "    "
			}
			lines = append(lines, m.warn(prefix+l))
		}
	}
	return lines
}

// deepWarning words a stable warning code; a newer code is shown as it is.
func (m Model) deepWarning(code string) string {
	switch code {
	case "estimate_unknown":
		return m.t(i18n.WarnEstimate)
	case "destination_storage_short":
		return m.t(i18n.WarnStorageShort)
	case "may_hit_daily_limit":
		return m.t(i18n.WarnDailyLimit)
	}
	return Sanitize(code)
}
