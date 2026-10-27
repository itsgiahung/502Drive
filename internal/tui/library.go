package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/itsgiahung/502Drive/internal/i18n"
)

// The Library tab: integrity audit (S1), collection lifecycle (L1), the
// selective-sync filter (S5) and retired sources. Every action names one fixed
// API call; the daemon decides, the TUI only asks and shows.

// auditState is what the TUI knows about the audit of one collection.
type auditState struct {
	report    *AuditReport
	starting  bool
	polling   bool
	repairJob string // the repair job in flight, "" when none
	repairDry bool
	planned   bool // a repair plan (dry run) exists for this report
	detail    bool // show the findings, not only the counts
	// fromLatest marks a report read from GET .../audits/latest, not started
	// here; a scan started here replaces it.
	fromLatest bool
	err        string
}

// cleanupFlow is the two-step cleanup of a retired collection: a dry-run plan
// first, then the folder name typed again.
type cleanupFlow struct {
	collectionID string
	jobID        string
	stage        int // cleanPlanning, cleanReady
	items        int
	bytes        int64
	rootName     string
	input        *textinput.Model
	err          string
}

const (
	cleanPlanning = iota
	cleanReady
)

type auditMsg struct {
	repairState  string
	collectionID string
	rep          AuditReport
	err          error
	started      bool
}

type repairMsg struct {
	collectionID string
	jobID        string
	dry          bool
	err          error
}

type cleanupMsg struct {
	collectionID string
	jobID        string
	dry          bool
	err          error
}

// filter presets shared by the copy dialog and the collection screen.
var filterPresets = []string{"", "no_video", "docs_only", "under_1gb"}

func (m Model) presetLabel(p string) string {
	switch p {
	case "no_video":
		return m.t(i18n.FilterNoVideo)
	case "docs_only":
		return m.t(i18n.FilterDocsOnly)
	case "under_1gb":
		return m.t(i18n.FilterUnder1GB)
	}
	return m.t(i18n.FilterNone)
}

// presetOf recognises the presets in a stored rule; -1 means a custom rule.
func presetOf(f FilterRules) int {
	if f.ModifiedAfter != "" || f.KeepEmptyFolders {
		return -1
	}
	switch {
	case !f.Active():
		return 0
	case len(f.Exclude) == 1 && f.Exclude[0] == "class:video" && len(f.Include) == 0 && f.MaxSize == 0 && f.MinSize == 0:
		return 1
	case len(f.Include) == 1 && f.Include[0] == "class:doc" && len(f.Exclude) == 0 && f.MaxSize == 0 && f.MinSize == 0:
		return 2
	case f.MaxSize == 1<<30 && len(f.Include) == 0 && len(f.Exclude) == 0 && f.MinSize == 0:
		return 3
	}
	return -1
}

func (m Model) filterText(f FilterRules) string {
	if i := presetOf(f); i >= 0 {
		return m.presetLabel(filterPresets[i])
	}
	return m.tf(i18n.FilterCustom, len(f.Include)+len(f.Exclude)+boolInt(f.MinSize > 0)+boolInt(f.MaxSize > 0))
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (m Model) conflictLabel(v string) string {
	switch v {
	case "skip":
		return m.t(i18n.ConflictSkip)
	case "update":
		return m.t(i18n.ConflictUpdate)
	}
	return m.t(i18n.ConflictRename)
}

// libraryNote is the line under the Library title when retired sources are
// shown too.
func (m Model) libraryNote() []string {
	if m.showRetired {
		return []string{m.dim(m.t(i18n.LibRetiredOn))}
	}
	return nil
}

// hasLibraryExt says whether the daemon-backed Library actions are wired.
func (m Model) hasLibraryExt() bool {
	return m.acts.StartAudit != nil || m.acts.CollectionLifecycle != nil || m.acts.Cleanup != nil
}

func (m Model) jobByID(id string) (Job, bool) {
	for _, j := range m.snap.Jobs {
		if j.ID == id {
			return j, true
		}
	}
	return Job{}, false
}

// ---- keys on the collection screen ----

// collectionKey handles the audit, lifecycle, filter and cleanup keys. It
// reports false for keys the global map should handle.
func (m Model) collectionKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	c, ok := m.currentCollection()
	if !ok {
		return m, nil, false
	}
	switch msg.String() {
	case "B":
		return m.openFilter(c), nil, true
	case "A":
		next, cmd := m.openAuditBrowser(c)
		return next, cmd, true
	case "a":
		next, cmd := m.startAudit(c)
		return next, cmd, true
	case "v":
		next, cmd := m.viewAudit(c)
		return next, cmd, true
	case "f":
		next, cmd := m.repair(c, true)
		return next, cmd, true
	case "F":
		next, cmd := m.repair(c, false)
		return next, cmd, true
	case "b":
		next, cmd := m.cycleFilter(c)
		return next, cmd, true
	case "o":
		next, cmd := m.retire(c)
		return next, cmd, true
	case "u":
		next, cmd := m.resumeCollection(c)
		return next, cmd, true
	case "w":
		next, cmd := m.forget(c)
		return next, cmd, true
	case "c":
		next, cmd := m.startCleanup(c)
		return next, cmd, true
	case "C":
		next, cmd := m.confirmCleanup(c)
		return next, cmd, true
	}
	return m, nil, false
}

func (m Model) auditOf(id string) *auditState {
	a := m.audits[id]
	if a == nil {
		a = &auditState{}
		m.audits[id] = a
	}
	return a
}

func (m Model) startAudit(c Collection) (tea.Model, tea.Cmd) {
	if m.acts.StartAudit == nil {
		return m, nil
	}
	a := m.auditOf(c.ID)
	if a.starting || (a.report != nil && a.report.State == "running") {
		return m, nil // one scan at a time; the screen already says "scanning"
	}
	a.starting, a.err, a.planned, a.fromLatest = true, "", false, false
	acts, ctx, id := m.acts, m.ctx, c.ID
	return m, func() tea.Msg {
		rep, err := acts.StartAudit(ctx, id)
		return auditMsg{collectionID: id, rep: rep, err: err, started: true}
	}
}

func (m Model) viewAudit(c Collection) (tea.Model, tea.Cmd) {
	a := m.audits[c.ID]
	if a == nil || a.report == nil {
		m.status = m.t(i18n.AuditNeedsReport)
		return m, nil
	}
	a.detail = !a.detail
	if m.acts.GetAudit == nil || a.polling {
		return m, nil
	}
	a.polling = true
	acts, ctx, id, cid := m.acts, m.ctx, a.report.ID, c.ID
	return m, func() tea.Msg {
		rep, err := acts.GetAudit(ctx, id)
		return auditMsg{collectionID: cid, rep: rep, err: err}
	}
}

// repair plans (dry) or applies the safe fixes of the last audit. Applying
// needs a plan first and a yes: it re-copies through the normal path and only
// touches objects 502Drive made.
func (m Model) repair(c Collection, dry bool) (tea.Model, tea.Cmd) {
	if m.acts.RepairAudit == nil {
		return m, nil
	}
	a := m.audits[c.ID]
	if a == nil || a.report == nil || a.report.State != "completed" {
		m.status = m.t(i18n.AuditNeedsReport)
		return m, nil
	}
	if a.report.Repairable == 0 {
		m.status = m.t(i18n.AuditNothingFix)
		return m, nil
	}
	if a.repairJob != "" {
		return m, nil
	}
	if !dry && !a.planned {
		m.status = m.t(i18n.AuditPlanFirst)
		return m, nil
	}
	acts, ctx, id, rev, cid := m.acts, m.ctx, a.report.ID, a.report.Revision, c.ID
	run := func() tea.Msg {
		job, err := acts.RepairAudit(ctx, id, rev, dry)
		return repairMsg{collectionID: cid, jobID: job, dry: dry, err: err}
	}
	if dry {
		return m, run
	}
	m.confirm = &confirmation{prompt: m.tf(i18n.AuditConfirm, a.report.Repairable), run: run}
	return m, nil
}

func (m Model) onAudit(msg auditMsg) (tea.Model, tea.Cmd) {
	a := m.auditOf(msg.collectionID)
	a.polling = false
	if msg.started {
		a.starting = false
	}
	if msg.err != nil {
		a.err = Sanitize(msg.err.Error())
		m.status = a.err
		return m, nil
	}
	a.err = ""
	rep := msg.rep
	if a.planned && a.report != nil && a.report.Revision != rep.Revision {
		a.planned = false
	}
	a.report = &rep
	if msg.started {
		m.status = m.tf(i18n.AuditStarted, Sanitize(rep.JobID))
		return m, m.refetch()
	}
	if a.repairJob != "" {
		st := msg.repairState
		if j, ok := m.jobByID(a.repairJob); ok {
			st = j.State
		}
		if terminalState(st) {
			a.planned = a.repairDry && st == "succeeded"
			a.repairJob = ""
		}
	}

	return m, nil
}

func (m Model) onRepair(msg repairMsg) (tea.Model, tea.Cmd) {
	a := m.auditOf(msg.collectionID)
	if msg.err != nil {
		a.err = Sanitize(msg.err.Error())
		m.status = a.err
		return m, nil
	}
	a.repairJob, a.repairDry = msg.jobID, msg.dry
	if msg.dry {
		m.status = m.tf(i18n.AuditRepairPlan, Sanitize(msg.jobID))
	} else {
		m.status = m.tf(i18n.AuditRepairing, Sanitize(msg.jobID))
	}
	return m, m.refetch()
}

// pollAudits re-reads the reports that can still change: a scan in progress or
// a repair job that just ended. It rides on the snapshot refresh, so there is
// no timer of its own.
func (m Model) pollAudits() tea.Cmd {
	if m.acts.GetAudit == nil || len(m.audits) == 0 {
		return nil
	}
	var cmds []tea.Cmd
	for cid, a := range m.audits {
		if a.report == nil || a.polling || a.starting {
			continue
		}
		need := a.report.State == "running"
		if a.repairJob != "" {
			if j, ok := m.jobByID(a.repairJob); !ok || terminalState(j.State) {
				need = true
			}
		}
		if !need {
			continue
		}
		a.polling = true
		acts, ctx, id, collection, repairID := m.acts, m.ctx, a.report.ID, cid, a.repairJob
		cmds = append(cmds, func() tea.Msg {
			rep, err := acts.GetAudit(ctx, id)
			st := ""
			if repairID != "" && acts.GetJob != nil {
				j, e := acts.GetJob(ctx, repairID)
				if e == nil {
					st = j.State
				}
			}
			return auditMsg{collectionID: collection, rep: rep, err: err, repairState: st}
		})
	}
	return tea.Batch(cmds...)
}

// ---- lifecycle (L1) ----

func (m Model) retire(c Collection) (tea.Model, tea.Cmd) {
	if m.acts.CollectionLifecycle == nil {
		return m, nil
	}
	if c.State == "retired" {
		m.status = m.t(i18n.LibRetiredTag)
		return m, nil
	}
	acts, ctx, id, rev := m.acts, m.ctx, c.ID, c.Revision
	m.confirm = &confirmation{prompt: m.tf(i18n.ConfirmRetire, Sanitize(c.SourceName)), run: m.act(func(context.Context) error {
		return acts.CollectionLifecycle(ctx, id, "retire", rev)
	})}
	return m, nil
}

func (m Model) resumeCollection(c Collection) (tea.Model, tea.Cmd) {
	switch {
	case c.State == "retired" && m.acts.CollectionLifecycle != nil:
		acts, id, rev := m.acts, c.ID, c.Revision
		return m, m.act(func(ctx context.Context) error { return acts.CollectionLifecycle(ctx, id, "resume", rev) })
	case c.Sync != nil && c.Sync.State == "paused" && m.acts.SyncAction != nil:
		return m.syncAct(c, "resume")
	}
	m.status = m.t(i18n.NotRetired)
	return m, nil
}

func (m Model) forget(c Collection) (tea.Model, tea.Cmd) {
	if m.acts.CollectionLifecycle == nil {
		return m, nil
	}
	if c.State != "retired" {
		m.status = m.t(i18n.NeedRetiredFirst)
		return m, nil
	}
	acts, ctx, id, rev := m.acts, m.ctx, c.ID, c.Revision
	m.confirm = &confirmation{prompt: m.tf(i18n.ConfirmForget, Sanitize(c.SourceName)), run: m.act(func(context.Context) error {
		return acts.CollectionLifecycle(ctx, id, "forget", rev)
	})}
	return m, nil
}

// startCleanup asks for the dry-run plan; nothing is touched on Drive.
func (m Model) startCleanup(c Collection) (tea.Model, tea.Cmd) {
	if m.acts.Cleanup == nil {
		return m, nil
	}
	if c.State != "retired" {
		m.status = m.t(i18n.NeedRetiredFirst)
		return m, nil
	}
	m.clean = &cleanupFlow{collectionID: c.ID, rootName: c.RootName}
	acts, ctx, id, rev := m.acts, m.ctx, c.ID, c.Revision
	return m, func() tea.Msg {
		job, err := acts.Cleanup(ctx, id, rev, true, "")
		return cleanupMsg{collectionID: id, jobID: job, dry: true, err: err}
	}
}

func (m Model) onCleanup(msg cleanupMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.status = Sanitize(msg.err.Error())
		if m.clean != nil && m.clean.collectionID == msg.collectionID {
			m.clean = nil
		}
		return m, nil
	}
	if msg.dry {
		if m.clean != nil && m.clean.collectionID == msg.collectionID {
			m.clean.jobID = msg.jobID
		}
		return m, m.refetch()
	}
	m.clean = nil
	m.status = m.tf(i18n.CleanupSent, Sanitize(msg.jobID))
	return m, m.refetch()
}

// cleanupProgress moves the plan forward when its dry-run job has finished.
func (m Model) cleanupProgress() {
	cf := m.clean
	if cf == nil || cf.stage != cleanPlanning || cf.jobID == "" {
		return
	}
	j, ok := m.jobByID(cf.jobID)
	if !ok || !terminalState(j.State) {
		return
	}
	switch j.State {
	case "succeeded":
		cf.stage = cleanReady
		cf.bytes = j.Bytes
		if j.Total != nil {
			cf.items = *j.Total
		}
	default:
		cf.err = Sanitize(j.LastError)
		if cf.err == "" {
			cf.err = stateWord(j.State)
		}
		cf.stage = cleanReady
		cf.items = -1 // no plan: nothing to confirm
	}
}

// confirmCleanup opens the typed-name step once the plan is in.
func (m Model) confirmCleanup(c Collection) (tea.Model, tea.Cmd) {
	cf := m.clean
	if cf == nil || cf.collectionID != c.ID || cf.stage != cleanReady || (cf.items <= 0 && cf.bytes == 0) || m.acts.Cleanup == nil {
		return m, nil
	}
	ti := newInput(m.width, "")
	cf.input = ti
	return m, textinput.Blink
}

func (m Model) cleanupTyping(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	cf := m.clean
	switch msg.Type {
	case tea.KeyCtrlC:
		m.quits = true
		return m, tea.Quit
	case tea.KeyEsc:
		cf.input = nil
		return m, nil
	case tea.KeyEnter:
		name := strings.TrimSpace(cf.input.Value())
		cf.input = nil
		c, ok := m.currentCollection()
		if name == "" || !ok {
			return m, nil
		}
		acts, ctx, id, rev := m.acts, m.ctx, c.ID, c.Revision
		return m, func() tea.Msg {
			job, err := acts.Cleanup(ctx, id, rev, false, name)
			return cleanupMsg{collectionID: id, jobID: job, err: err}
		}
	}
	var cmd tea.Cmd
	*cf.input, cmd = cf.input.Update(msg)
	return m, cmd
}

// ---- filter on an existing collection (S5) ----

func (m Model) cycleFilter(c Collection) (tea.Model, tea.Cmd) {
	if m.acts.SetFilterPreset == nil {
		return m, nil
	}
	next := filterPresets[(presetOf(c.Filter)+1+len(filterPresets))%len(filterPresets)]
	acts, id, rev := m.acts, c.ID, c.Revision
	label := m.presetLabel(next)
	return m, func() tea.Msg {
		if err := acts.SetFilterPreset(m.ctx, id, next, rev); err != nil {
			return actionMsg{err: err}
		}
		return actionMsg{text: m.tf(i18n.FilterSaved, label) + " (" + m.t(i18n.FilterApplyLater) + ")"}
	}
}

// toggleRetired shows or hides the retired sources and re-reads the library.
func (m Model) toggleRetired() (tea.Model, tea.Cmd) {
	if m.acts.ShowRetired == nil {
		return m, nil
	}
	m.showRetired = !m.showRetired
	m.acts.ShowRetired(m.showRetired)
	m.cursor, m.top = 0, 0
	m.fetching = true
	return m, m.fetch()
}

// ---- views ----

// auditLines is the integrity section of the collection screen.
func (m Model) auditLines(c Collection) []string {
	if m.acts.StartAudit == nil && m.audits[c.ID] == nil {
		return nil
	}
	a := m.audits[c.ID]
	title := i18n.AuditTitle
	if a != nil && a.fromLatest {
		title = i18n.LastAuditTitle
	}
	lines := []string{"", m.bold(m.t(title))}
	if m.screen == screenCollection {
		lines[1] = m.collectionKV(4, m.t(title), "")
	}
	switch {
	case a == nil || (a.report == nil && !a.starting):
		return append(lines, "  "+m.dim(m.t(i18n.AuditNone)))
	case a.report == nil:
		return append(lines, "  "+m.accent(m.spinChar())+" "+m.dim(m.tf(i18n.AuditRunning, 0)))
	}
	r := a.report
	switch r.State {
	case "running":
		lines = append(lines, "  "+m.accent(m.spinChar())+" "+m.tf(i18n.AuditRunning, r.Scanned))
	case "failed":
		lines = append(lines, "  "+m.bad(m.t(i18n.AuditFailed)))
	case "completed":
		if r.Total() == 0 {
			lines = append(lines, "  "+m.good(m.t(i18n.AuditClean)))
		} else {
			lines = append(lines, "  "+m.warn(m.tf(i18n.AuditFinding, r.Total(), r.Repairable)))
		}
	}
	for _, k := range auditKinds {
		if n := r.Counts[k.kind]; n > 0 {
			lines = append(lines, fmt.Sprintf("  %5d  %s", n, m.t(k.key)))
		}
	}
	if a.repairJob != "" {
		word := i18n.AuditRepairing
		if a.repairDry {
			word = i18n.AuditRepairPlan
		}
		lines = append(lines, "  "+m.dim(m.tf(word, Sanitize(a.repairJob))))
	}
	if a.detail {
		shown := 0
		for _, f := range r.Findings {
			if shown >= 6 {
				break
			}
			tag := ""
			switch {
			case f.Repaired:
				tag = " [" + m.t(i18n.AuditRepairedTag) + "]"
			case f.Repairable:
				tag = " [" + m.t(i18n.AuditRepairable) + "]"
			}
			name := Sanitize(orElse(f.Detail, orElse(f.SourceID, f.DestID)))
			lines = append(lines, cut("  - "+name+tag, max(12, m.width-2)))
			shown++
		}
		if extra := r.Total() - shown; extra > 0 {
			lines = append(lines, "  "+m.dim(m.tf(i18n.AuditMore, extra, Sanitize(r.ID))))
		}
	}
	return lines
}

var auditKinds = []struct {
	kind string
	key  i18n.Key
}{
	{"missing_at_destination", i18n.AuditKindMissing},
	{"extra_at_destination", i18n.AuditKindExtra},
	{"name_mismatch", i18n.AuditKindName},
	{"size_or_md5_mismatch", i18n.AuditKindSize},
	{"wrong_parent", i18n.AuditKindParent},
	{"duplicate_at_destination", i18n.AuditKindDup},
	{"source_removed", i18n.AuditKindRemoved},
	{"unreadable_source", i18n.AuditKindUnread},
}

// cleanupLines is the cleanup section: the plan, then the typed confirmation.
func (m Model) cleanupLines(c Collection) []string {
	cf := m.clean
	if cf == nil || cf.collectionID != c.ID {
		return nil
	}
	lines := []string{""}
	switch {
	case cf.stage == cleanPlanning:
		return append(lines, m.accent(m.spinChar())+" "+m.dim(m.t(i18n.CleanupPlanning)))
	case cf.items < 0:
		return append(lines, m.bad(cf.err))
	case cf.items == 0 && cf.bytes == 0:
		return append(lines, m.dim(m.t(i18n.CleanupEmpty)))
	}
	for _, l := range wrapLines(m.tf(i18n.CleanupPlan, cf.items, bytesText(cf.bytes)), max(20, m.width-2)) {
		lines = append(lines, m.warn(l))
	}
	if cf.input != nil {
		lines = append(lines, "", m.accent(m.tf(i18n.CleanupTypeName, Sanitize(cf.rootName))), cf.input.View(), m.dim(m.t(i18n.CleanupKeys)))
	}
	return lines
}
