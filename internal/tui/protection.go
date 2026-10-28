package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/itsgiahung/502Drive/internal/i18n"
)

// The protection screen is the opt-in mirror/snapshot control of one
// collection. Values with a fixed set of choices (retention) are picked from a
// list, never typed. Restore is always admitted as a dry run first; the apply
// button only exists once that job succeeded and its plan is known.

type protectPage int

const (
	protectMenu protectPage = iota
	protectDays
	protectCounts
	protectSnapshots
	protectRestore
)

// The only retention values the screen offers.
var (
	protectDayChoices   = []int{7, 30, 90}
	protectCountChoices = []int{5, 10, 20}
)

// Rows of the protection menu.
const (
	pMirror = iota
	pAuto
	pDays
	pCount
	pCapture
	pList
	pRestore
	pRows
)

type protectFlow struct {
	collectionID string
	view         ProtectionView
	snaps        []SnapshotInfo
	page         protectPage
	row          int
	snapRow      int
	restoreMode  bool
	loading      bool
	err          string
	preview      *restorePreview
}

type restorePreview struct {
	snapshotID string
	revision   int
	jobID      string
	jobState   string
	plan       RestorePlan
}

// protectLoadedMsg is the answer of any option change or capture: the fresh
// view, the fresh snapshot list and, when one was admitted, its job.
type protectLoadedMsg struct {
	id    string
	view  ProtectionView
	snaps []SnapshotInfo
	note  i18n.Key
	arg   string
	err   error
}

type restorePreviewMsg struct {
	snapshotID, jobID, jobState string
	plan                        RestorePlan
	err                         error
}

func (m Model) openProtection(c Collection) (Model, tea.Cmd) {
	if m.acts.Protection == nil {
		m.status = m.t(i18n.ProtUnavailable)
		return m, nil
	}
	m.screen = screenProtect
	m.prot = &protectFlow{collectionID: c.ID, loading: true}
	return m, m.loadProtection(c.ID)
}

func (m Model) loadProtection(id string) tea.Cmd {
	acts, ctx := m.acts, m.ctx
	if m.prot != nil {
		m.prot.loading = true
	}
	return func() tea.Msg {
		view, err := acts.Protection(ctx, id)
		if err != nil {
			return protectLoadedMsg{id: id, err: err}
		}
		if acts.ListSnapshots == nil {
			return protectLoadedMsg{id: id, view: view}
		}
		snaps, err := acts.ListSnapshots(ctx, id)
		return protectLoadedMsg{id: id, view: view, snaps: snaps, err: err}
	}
}

// protectReload runs one admitted change and then re-reads the options and the
// snapshot list, so the screen never shows a pre-write revision.
func (m Model) protectReload(fn func(context.Context) (string, error), note i18n.Key) tea.Cmd {
	p, acts, ctx := m.prot, m.acts, m.ctx
	id := p.collectionID
	return func() tea.Msg {
		arg, err := fn(ctx)
		if err != nil {
			return protectLoadedMsg{id: id, err: err}
		}
		view, err := acts.Protection(ctx, id)
		if err != nil {
			return protectLoadedMsg{id: id, err: err}
		}
		if acts.ListSnapshots == nil {
			return protectLoadedMsg{id: id, view: view, note: note, arg: arg}
		}
		snaps, err := acts.ListSnapshots(ctx, id)
		return protectLoadedMsg{id: id, view: view, snaps: snaps, note: note, arg: arg, err: err}
	}
}

func (m Model) saveProtection(opts ProtectionOptions, note i18n.Key) tea.Cmd {
	p := m.prot
	if p == nil || m.acts.SetProtection == nil {
		return nil
	}
	id, rev, acts := p.collectionID, p.view.Revision, m.acts
	return m.protectReload(func(ctx context.Context) (string, error) {
		_, err := acts.SetProtection(ctx, id, rev, opts)
		return "", err
	}, note)
}

func (m Model) captureSnapshot() tea.Cmd {
	p := m.prot
	if p == nil || m.acts.CreateSnapshot == nil {
		return nil
	}
	id, rev, acts := p.collectionID, p.view.Revision, m.acts
	return m.protectReload(func(ctx context.Context) (string, error) {
		return acts.CreateSnapshot(ctx, id, rev)
	}, i18n.ProtCaptureStarted)
}

func (m Model) pinSnapshot(s SnapshotInfo, pin bool) tea.Cmd {
	if m.acts.PinSnapshot == nil {
		return nil
	}
	acts := m.acts
	return m.protectReload(func(ctx context.Context) (string, error) {
		return "", acts.PinSnapshot(ctx, s.ID, s.Revision, pin)
	}, i18n.ProtPinOK)
}

// previewRestore admits the dry run; nothing is written on Drive.
func (m Model) previewRestore(s SnapshotInfo) tea.Cmd {
	if m.acts.RestoreSnapshot == nil {
		return nil
	}
	p, acts, ctx := m.prot, m.acts, m.ctx
	p.loading, p.page, p.preview = true, protectRestore, &restorePreview{snapshotID: s.ID, revision: s.Revision}
	return func() tea.Msg {
		jobID, err := acts.RestoreSnapshot(ctx, s.ID, s.Revision, true)
		if err != nil {
			return restorePreviewMsg{snapshotID: s.ID, err: err}
		}
		return readRestorePreview(ctx, acts, s.ID, jobID)
	}
}

func readRestorePreview(ctx context.Context, acts Actions, snapshotID, jobID string) restorePreviewMsg {
	msg := restorePreviewMsg{snapshotID: snapshotID, jobID: jobID}
	if acts.GetJob == nil {
		return msg
	}
	job, err := acts.GetJob(ctx, jobID)
	if err != nil {
		msg.err = err
		return msg
	}
	msg.jobState = job.State
	if job.State == "succeeded" && acts.RestorePlan != nil {
		msg.plan, msg.err = acts.RestorePlan(ctx, jobID)
	}
	return msg
}

func (m Model) applyRestore() (tea.Model, tea.Cmd) {
	p := m.prot
	v := p.preview
	if v == nil || v.jobState != "succeeded" || m.acts.RestoreSnapshot == nil {
		m.status = m.t(i18n.ProtNoPreview)
		return m, nil
	}
	snapID, rev, acts := v.snapshotID, v.revision, m.acts
	p.page, p.preview = protectSnapshots, nil
	m.confirm = &confirmation{prompt: m.t(i18n.ProtConfirmRestore), run: m.protectReload(func(ctx context.Context) (string, error) {
		return acts.RestoreSnapshot(ctx, snapID, rev, false)
	}, i18n.ProtRestoreStarted)}
	return m, nil
}

func (m Model) onProtectionLoaded(msg protectLoadedMsg) Model {
	p := m.prot
	if p == nil || msg.id != p.collectionID {
		return m
	}
	p.loading = false
	if msg.err != nil {
		p.err = Sanitize(msg.err.Error())
		return m
	}
	p.view, p.snaps, p.err = msg.view, msg.snaps, ""
	if int(p.snapRow) >= len(p.snaps) {
		p.snapRow = max(0, len(p.snaps)-1)
	}
	switch {
	case msg.note != "" && msg.arg != "":
		m.status = m.tf(msg.note, Sanitize(msg.arg))
	case msg.note != "":
		m.status = m.t(msg.note)
	}
	return m
}

func (m Model) onRestorePreview(msg restorePreviewMsg) Model {
	p := m.prot
	if p == nil || p.preview == nil || p.preview.snapshotID != msg.snapshotID {
		return m
	}
	p.loading = false
	if msg.err != nil {
		p.err = Sanitize(msg.err.Error())
		return m
	}
	p.err = ""
	p.preview.jobID, p.preview.jobState, p.preview.plan = msg.jobID, msg.jobState, msg.plan
	return m
}

func (m Model) protectionKey(key string) (tea.Model, tea.Cmd) {
	p := m.prot
	if p == nil {
		return m, nil
	}
	switch key {
	case "q", "ctrl+c":
		m.quits = true
		return m, tea.Quit
	}
	// One write at a time: a second Enter while the daemon answers would be
	// refused as stale at best and duplicate a capture at worst.
	if p.loading {
		switch key {
		case "enter", "right", " ", "p", "a":
			return m, nil
		}
	}
	switch p.page {
	case protectDays, protectCounts:
		return m.protectChoicesKey(key)
	case protectSnapshots:
		return m.protectSnapshotsKey(key)
	case protectRestore:
		return m.protectRestoreKey(key)
	}
	return m.protectMenuKey(key)
}

func (m Model) protectMenuKey(key string) (tea.Model, tea.Cmd) {
	p := m.prot
	switch key {
	case "up", "k":
		p.row = max(0, p.row-1)
	case "down", "j":
		p.row = min(pRows-1, p.row+1)
	case "home", "g":
		p.row = 0
	case "end", "G":
		p.row = pRows - 1
	case "left", "esc":
		return m.leaveCollectionSubview(p.collectionID), nil
	case "r":
		return m, m.loadProtection(p.collectionID)
	case "enter", "right", " ":
		opts := p.view.Options
		switch p.row {
		case pMirror:
			if !opts.Mirror {
				opts.Mirror = true
				m.confirm = &confirmation{prompt: m.t(i18n.ProtConfirmMirror), run: m.saveProtection(opts, i18n.ProtSaved)}
				return m, nil
			}
			opts.Mirror = false
			return m, m.saveProtection(opts, i18n.ProtMirrorOff)
		case pAuto:
			opts.Snapshots = !opts.Snapshots
			return m, m.saveProtection(opts, i18n.ProtSaved)
		case pDays:
			p.page = protectDays
			p.row = choiceIndex(protectDayChoices, opts.KeepDays)
		case pCount:
			p.page = protectCounts
			p.row = choiceIndex(protectCountChoices, opts.KeepCount)
		case pCapture:
			return m, m.captureSnapshot()
		case pList:
			p.page, p.snapRow, p.restoreMode = protectSnapshots, 0, false
		case pRestore:
			p.page, p.snapRow, p.restoreMode = protectSnapshots, 0, true
		}
	}
	return m, nil
}

func choiceIndex(values []int, want int) int {
	for i, v := range values {
		if v == want {
			return i
		}
	}
	return 0
}

func (m Model) protectChoicesKey(key string) (tea.Model, tea.Cmd) {
	p := m.prot
	values := protectDayChoices
	if p.page == protectCounts {
		values = protectCountChoices
	}
	switch key {
	case "up", "k":
		p.row = max(0, p.row-1)
	case "down", "j":
		p.row = min(len(values)-1, p.row+1)
	case "home", "g":
		p.row = 0
	case "end", "G":
		p.row = len(values) - 1
	case "left", "esc":
		back := pCount
		if p.page == protectDays {
			back = pDays
		}
		p.page, p.row = protectMenu, back
	case "enter", "right", " ":
		opts := p.view.Options
		if p.page == protectDays {
			opts.KeepDays = values[min(p.row, len(values)-1)]
			p.page, p.row = protectMenu, pDays
		} else {
			opts.KeepCount = values[min(p.row, len(values)-1)]
			p.page, p.row = protectMenu, pCount
		}
		return m, m.saveProtection(opts, i18n.ProtSaved)
	}
	return m, nil
}

func (m Model) protectSnapshotsKey(key string) (tea.Model, tea.Cmd) {
	p := m.prot
	switch key {
	case "left", "esc":
		back := pList
		if p.restoreMode {
			back = pRestore
		}
		p.page, p.row, p.restoreMode = protectMenu, back, false
	case "up", "k":
		p.snapRow = max(0, p.snapRow-1)
	case "down", "j":
		if len(p.snaps) > 0 {
			p.snapRow = min(len(p.snaps)-1, p.snapRow+1)
		}
	case "p":
		if len(p.snaps) > 0 {
			s := p.snaps[min(p.snapRow, len(p.snaps)-1)]
			return m, m.pinSnapshot(s, !s.Pinned)
		}
	case "r":
		if len(p.snaps) > 0 {
			return m, m.previewRestore(p.snaps[min(p.snapRow, len(p.snaps)-1)])
		}
	case "enter", "right", " ":
		if p.restoreMode && len(p.snaps) > 0 {
			return m, m.previewRestore(p.snaps[min(p.snapRow, len(p.snaps)-1)])
		}
	}
	return m, nil
}

func (m Model) protectRestoreKey(key string) (tea.Model, tea.Cmd) {
	p := m.prot
	switch key {
	case "left", "esc":
		p.page, p.preview, p.restoreMode = protectSnapshots, nil, true
	case "r":
		if p.preview != nil && p.preview.jobID != "" && m.acts.GetJob != nil {
			p.loading = true
			snapshotID, jobID := p.preview.snapshotID, p.preview.jobID
			acts, ctx := m.acts, m.ctx
			return m, func() tea.Msg { return readRestorePreview(ctx, acts, snapshotID, jobID) }
		}
	case "a":
		return m.applyRestore()
	}
	return m, nil
}

func (m Model) protectionView() []string {
	p := m.prot
	if p == nil {
		return nil
	}
	switch p.page {
	case protectSnapshots:
		return m.protectionSnapshotsView(p)
	case protectRestore:
		return m.protectionRestoreView(p)
	}
	lines := []string{"", m.bold(m.t(i18n.ProtTitle)), ""}
	if p.loading {
		lines = append(lines, m.t(i18n.Loading))
	}
	if p.err != "" {
		lines = append(lines, m.bad(p.err))
	}
	if p.page == protectDays || p.page == protectCounts {
		label, values := i18n.ProtDays, protectDayChoices
		current := p.view.Options.KeepDays
		if p.page == protectCounts {
			label, values = i18n.ProtCount, protectCountChoices
			current = p.view.Options.KeepCount
		}
		lines = append(lines, m.tf(label, current), "")
		for i, v := range values {
			marker := "  "
			if i == p.row {
				marker = m.accent("> ")
			}
			lines = append(lines, marker+fmt.Sprintf("%d", v))
		}
		return append(lines, "", m.dim(m.t(i18n.ProtHint)))
	}
	opts := p.view.Options
	rows := []string{
		m.protectRow(p, pMirror, m.t(i18n.ProtMirror), m.onOffWord(opts.Mirror)),
		m.protectRow(p, pAuto, m.t(i18n.ProtAuto), m.onOffWord(opts.Snapshots)),
		m.protectRow(p, pDays, m.tf(i18n.ProtDays, opts.KeepDays), "›"),
		m.protectRow(p, pCount, m.tf(i18n.ProtCount, opts.KeepCount), "›"),
		m.protectRow(p, pCapture, m.t(i18n.ProtCapture), ""),
		m.protectRow(p, pList, m.t(i18n.ProtList), fmt.Sprintf("%d", len(p.snaps))),
		m.protectRow(p, pRestore, m.t(i18n.ProtRestoreEntry), "›"),
	}
	lines = append(lines, rows...)
	return append(lines, "", m.dim(m.t(i18n.ProtHint)))
}

func (m Model) onOffWord(on bool) string {
	if on {
		return m.good(m.t(i18n.ProtOn))
	}
	return m.dim(m.t(i18n.ProtOff))
}

func (m Model) protectRow(p *protectFlow, row int, label, value string) string {
	marker := "  "
	if p.row == row {
		marker = m.accent("> ")
	}
	if value != "" {
		return marker + label + " · " + value
	}
	return marker + label
}

func (m Model) protectionSnapshotsView(p *protectFlow) []string {
	lines := []string{"", m.bold(m.t(i18n.ProtSnapshotsTitle)), Sanitize(p.collectionID), ""}
	switch {
	case p.loading:
		return append(lines, m.t(i18n.Loading))
	case p.err != "":
		return append(lines, m.bad(p.err))
	case len(p.snaps) == 0:
		return append(lines, m.dim(m.t(i18n.ProtSnapshotsEmpty)))
	}
	// Keep the selected point visible in a short terminal.
	rows := max(3, m.bodyRows()-6)
	start := 0
	if p.snapRow >= rows {
		start = p.snapRow - rows + 1
	}
	end := min(len(p.snaps), start+rows)
	for i := start; i < end; i++ {
		s := p.snaps[i]
		marker := "  "
		if i == p.snapRow {
			marker = m.accent("> ")
		}
		pin := m.t(i18n.ProtUnpinned)
		if s.Pinned {
			pin = m.good(m.t(i18n.ProtPinned))
		}
		row := fmt.Sprintf("%s%s · %s · %s", marker, Sanitize(s.ID), s.CreatedAt.Local().Format("2006-01-02 15:04"), pin)
		if s.State != "ready" {
			row += " · " + Sanitize(s.State)
		}
		lines = append(lines, row)
	}
	hint := i18n.ProtSnapshotsKeys
	if p.restoreMode {
		hint = i18n.ProtSnapshotsPick
	}
	return append(lines, "", m.dim(m.t(hint)))
}

func (m Model) protectionRestoreView(p *protectFlow) []string {
	lines := []string{"", m.bold(m.t(i18n.ProtRestoreTitle)), ""}
	v := p.preview
	if v == nil {
		return append(lines, m.dim(m.t(i18n.ProtNoPreview)))
	}
	lines = append(lines, Sanitize(v.snapshotID))
	if v.jobID != "" {
		lines = append(lines, m.dim("job "+Sanitize(v.jobID)))
	}
	if p.loading {
		return append(lines, "", m.t(i18n.Loading))
	}
	if p.err != "" {
		lines = append(lines, "", m.bad(p.err))
	}
	switch v.jobState {
	case "succeeded":
		if v.plan.Next != "" {
			lines = append(lines, "", m.tf(i18n.ProtActionsMore, len(v.plan.Actions)))
		} else {
			lines = append(lines, "", m.tf(i18n.ProtActions, len(v.plan.Actions)))
		}
		for i, a := range v.plan.Actions {
			if i >= 8 {
				break
			}
			lines = append(lines, fmt.Sprintf("  %s · %s · %s", Sanitize(a.Action), Sanitize(a.State), Sanitize(a.Path+a.Name)))
		}
	case "failed", "blocked", "partial", "cancelled":
		lines = append(lines, "", m.bad(m.tf(i18n.ProtDryFailed, Sanitize(v.jobState))))
	default:
		lines = append(lines, "", m.t(i18n.ProtDryRunning))
	}
	return append(lines, "", m.dim(m.t(i18n.ProtApplyHint)))
}
