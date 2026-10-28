package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/itsgiahung/502Drive/internal/i18n"
	"github.com/itsgiahung/502Drive/internal/present"
)

type rate struct{ samples []present.Sample }

// observe updates the speed estimates and notices jobs that just finished.
func (m *Model) observe(next Snapshot) {
	now := m.opt.now()
	seen := map[string]bool{}
	for _, j := range next.Jobs {
		seen[j.ID] = true
		if prev, ok := m.prevStates[j.ID]; ok && prev != "succeeded" && j.State == "succeeded" {
			m.happyUntil = now.Add(happyFor)
		}
		m.prevStates[j.ID] = j.State
		if j.State != "running" {
			delete(m.rates, j.ID)
			continue
		}
		r := m.rates[j.ID]
		if r == nil {
			r = &rate{}
			m.rates[j.ID] = r
		}
		r.samples = present.AppendSample(r.samples, present.Sample{At: now, Completed: j.Completed})
	}
	for id := range m.prevStates {
		if !seen[id] {
			delete(m.prevStates, id)
			delete(m.rates, id)
		}
	}
}

// speed returns the smoothed rate and the time left, when both are known.
func (m Model) speed(j Job) (perSec float64, eta time.Duration, ok bool) {
	r := m.rates[j.ID]
	if r == nil {
		return 0, 0, false
	}
	perSec, eta = present.Estimate(present.Counts{Total: j.Total, Completed: j.Completed}, r.samples)
	return perSec, eta, perSec > 0
}

// humanDuration formats a duration in the model's language with the shared
// presenter (see internal/present).
func (m Model) humanDuration(d time.Duration) string { return present.HumanDuration(d, m.lang) }

// rowID is the stable identity of the highlighted or listed row.
func (m Model) rowID(i int) string {
	switch m.screen {
	case screenJobs:
		if i >= 0 && i < len(m.snap.Jobs) {
			return m.snap.Jobs[i].ID
		}
	case screenLibrary:
		if i >= 0 && i < len(m.snap.Collections) {
			return m.snap.Collections[i].ID
		}
	}
	return ""
}

func (m Model) toggle() Model {
	id := m.rowID(m.cursor)
	if id == "" {
		return m
	}
	sel := make(map[string]bool, len(m.selected)+1)
	for k, v := range m.selected {
		sel[k] = v
	}
	if sel[id] {
		delete(sel, id)
	} else {
		sel[id] = true
	}
	m.selected = sel
	return m
}

func (m Model) selectAll() Model {
	sel := map[string]bool{}
	for i := 0; i < m.listLen(); i++ {
		if id := m.rowID(i); id != "" {
			sel[id] = true
		}
	}
	m.selected = sel
	return m
}

// pruneSelection forgets rows that no longer exist.
func (m *Model) pruneSelection() {
	if len(m.selected) == 0 {
		return
	}
	live := map[string]bool{}
	for _, j := range m.snap.Jobs {
		live[j.ID] = true
	}
	for _, c := range m.snap.Collections {
		live[c.ID] = true
	}
	for id := range m.selected {
		if !live[id] {
			delete(m.selected, id)
		}
	}
}

// target is one row an action applies to; do == nil means "not applicable".
type target struct {
	id string
	do func(context.Context) error
}

// jobEligible keeps a bulk action from asking the daemon for something it
// would only refuse (resuming a finished job, pausing a paused one).
func jobEligible(action, state string) bool {
	switch action {
	case "pause":
		return state == "queued" || state == "running"
	case "resume":
		return state == "paused"
	case "cancel":
		switch state {
		case "queued", "running", "pausing", "paused", "reconciling", "blocked":
			return true
		}
	case "retry":
		return state == "failed" || state == "partial" || state == "blocked"
	}
	return false
}

// jobRows is the selection, or the highlighted row when nothing is selected.
func (m Model) jobRows() []Job {
	var out []Job
	for _, j := range m.snap.Jobs {
		if m.selected[j.ID] {
			out = append(out, j)
		}
	}
	if len(out) == 0 {
		if j, ok := m.selectedJob(); ok {
			out = append(out, j)
		}
	}
	return out
}

func (m Model) collectionRows() []Collection {
	var out []Collection
	for _, c := range m.snap.Collections {
		if m.selected[c.ID] {
			out = append(out, c)
		}
	}
	if len(out) == 0 {
		if c, ok := m.selectedCollection(); ok {
			out = append(out, c)
		}
	}
	return out
}

func (m Model) jobsKey(key string) (tea.Model, tea.Cmd) {
	if next, cmd, handled := m.jobsKeyIT(key); handled {
		return next, cmd
	}
	if key == "x" && m.acts.ClearHistory != nil {
		var ids []string
		for _, j := range m.jobRows() {
			switch j.State {
			case "succeeded", "partial", "failed", "cancelled":
				ids = append(ids, j.ID)
			}
		}
		if len(ids) == 0 {
			m.status = m.t(i18n.HistJobsNone)
			return m, nil
		}
		m.confirm = &confirmation{prompt: m.tf(i18n.HistJobsAsk, len(ids)), run: m.histClearJobs(ids)}
		return m, nil
	}
	actions := map[string]string{"p": "pause", "u": "resume", "c": "cancel", "t": "retry"}
	action, ok := actions[key]
	if !ok || m.acts.JobAction == nil {
		return m, nil
	}
	rows := m.jobRows()
	targets := make([]target, 0, len(rows))
	eligible := 0
	for _, j := range rows {
		t := target{id: j.ID}
		if m.jobEligibleFor(action, j) {
			job := j
			t.do = func(ctx context.Context) error { return m.acts.JobAction(ctx, job.ID, action, job.Revision) }
			eligible++
		}
		targets = append(targets, t)
	}
	cmd := m.bulk(action, targets)
	if action == "cancel" && eligible > 0 {
		m.confirm = &confirmation{prompt: i18n.Tf(m.lang, i18n.ConfirmCan, eligible), run: cmd}
		return m, nil
	}
	return m, cmd
}

func (m Model) libraryKey(key string) (tea.Model, tea.Cmd) {
	rows := m.collectionRows()
	var targets []target
	label := key
	switch key {
	case "d", "h", "e", "t":
		if m.acts.SyncPreset == nil {
			return m, nil
		}
		preset := map[string]string{"d": presetDaily, "h": presetHourly, "e": presetSixHour, "t": presetRealtime}[key]
		label = preset
		for _, c := range rows {
			id := c.ID
			targets = append(targets, target{id: id, do: func(ctx context.Context) error { return m.acts.SyncPreset(ctx, id, preset) }})
		}
	case "n":
		if m.acts.RunNow == nil {
			return m, nil
		}
		label = "run"
		for _, c := range rows {
			id := c.ID
			t := target{id: id}
			if c.Sync != nil {
				t.do = func(ctx context.Context) error { return m.acts.RunNow(ctx, id) }
			}
			targets = append(targets, t)
		}
	case "H":
		return m.toggleRetired()
	case "p", "x":
		if m.acts.SyncAction == nil {
			return m, nil
		}
		action := map[string]string{"p": "pause", "x": "stop"}[key]
		label = action
		for _, c := range rows {
			id := c.ID
			t := target{id: id}
			if c.Sync != nil {
				t.do = func(ctx context.Context) error { return m.acts.SyncAction(ctx, id, action) }
			}
			targets = append(targets, t)
		}
	default:
		return m, nil
	}
	cmd := m.bulk(label, targets)
	if key == "x" {
		eligible := 0
		for _, t := range targets {
			if t.do != nil {
				eligible++
			}
		}
		if eligible > 0 {
			m.confirm = &confirmation{prompt: i18n.Tf(m.lang, i18n.ConfirmStop, eligible), run: cmd}
			return m, nil
		}
	}
	return m, cmd
}

// bulk runs the targets one after another in the background; one failure does
// not stop the rest, and the result is summarised once.
func (m Model) bulk(label string, targets []target) tea.Cmd {
	ctx := m.ctx
	return func() tea.Msg {
		res := bulkMsg{label: label}
		for _, t := range targets {
			if t.do == nil {
				res.skipped++
				continue
			}
			if err := t.do(ctx); err != nil {
				res.failed++
				if res.firstErr == "" {
					res.firstErr = m.errText(err)
				}
				continue
			}
			res.ok++
		}
		return res
	}
}

func (m Model) bulkStatus(r bulkMsg) string {
	text := i18n.Tf(m.lang, i18n.BulkResult, r.label, r.ok, r.skipped, r.failed)
	if r.firstErr != "" {
		text += " — " + r.firstErr
	}
	return text
}

// terminalState says a job has ended for good (a blocked job may still resume).
func terminalState(s string) bool {
	switch s {
	case "succeeded", "partial", "failed", "cancelled":
		return true
	}
	return false
}
