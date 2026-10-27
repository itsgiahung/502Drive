package tui

import (
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/itsgiahung/502Drive/internal/i18n"
	"github.com/itsgiahung/502Drive/internal/present"
)

// The queue (S6) on the Jobs tab: the server-side filter, paging by cursor,
// the order of queued jobs with move and priority, the global pause, and the
// progress of a batch. The daemon owns all of it; every key here names one
// fixed API call and the next refresh shows what really happened.

// jobStateFilters are the states the f key steps through ("" = all).
var jobStateFilters = []string{"", "queued", "running", "paused", "blocked", "failed", "succeeded"}

const (
	minPriority  = -5
	maxPriority  = 5
	batchFreshMS = 5000
)

func (q JobQuery) isDefault() bool { return q.State == "" && q.CollectionID == "" && q.Pages <= 1 }

// moveRec is a move this session made; the order of queued jobs is replayed
// from it because GET /v1/jobs does not report the queue position.
type moveRec struct{ id, before, after string }

// queueMoves is shared between copies of the model (only Update touches it).
type queueMoves struct{ recs []moveRec }

type moveMsg struct {
	rec    moveRec
	err    error
	jobID  string
	prio   int
	isPrio bool
}

type queueMsg struct {
	paused   bool
	affected int
	err      error
}

type batchMsg struct {
	id   string
	prog BatchProgress
	err  error
}

// ---- server-side filter and paging ----

// applyJobQuery sets the filter, tells the client and reads the list again.
func (m Model) applyJobQuery(q JobQuery) (Model, tea.Cmd) {
	m.cursor, m.top = 0, 0
	m.selected = map[string]bool{}
	return m.readJobs(q)
}

// readJobs sets the query and reads the list again, keeping the cursor and the
// selection (loading one more page must not lose either).
func (m Model) readJobs(q JobQuery) (Model, tea.Cmd) {
	q.Pages = max(1, q.Pages)
	m.jq = q
	if m.acts.SetJobQuery != nil {
		m.acts.SetJobQuery(q)
	}
	m.fetching = true
	return m, m.fetch()
}

// resetJobQuery drops the filter when the user leaves the Jobs tab, so Home and
// the other screens see the unfiltered list.
func (m Model) resetJobQuery() (Model, tea.Cmd) {
	if m.jq.isDefault() {
		return m, nil
	}
	m.jqName = ""
	return m.applyJobQuery(JobQuery{Pages: 1})
}

func (m Model) cycleJobState() (Model, tea.Cmd) {
	q := m.jq
	next := 0
	for i, s := range jobStateFilters {
		if s == q.State {
			next = (i + 1) % len(jobStateFilters)
		}
	}
	q.State, q.Pages = jobStateFilters[next], 1
	return m.applyJobQuery(q)
}

// toggleJobSource limits the list to the source of the highlighted job, or
// clears that limit.
func (m Model) toggleJobSource() (Model, tea.Cmd) {
	q := m.jq
	if q.CollectionID != "" {
		q.CollectionID, q.Pages = "", 1
		m.jqName = ""
		return m.applyJobQuery(q)
	}
	j, ok := m.selectedJob()
	if !ok || j.CollectionID == "" {
		return m, nil
	}
	q.CollectionID, q.Pages = j.CollectionID, 1
	m.jqName = j.SourceName
	return m.applyJobQuery(q)
}

// loadMoreJobs reads one more page when the cursor sits on the last row and
// the daemon has more.
func (m Model) loadMoreJobs() (Model, tea.Cmd) {
	if m.screen != screenJobs || m.snap.JobsNext == "" || m.fetching || len(m.snap.Jobs) == 0 || m.cursor < len(m.snap.Jobs)-1 {
		return m, nil
	}
	q := m.jq
	q.Pages = max(1, q.Pages) + 1
	return m.readJobs(q)
}

// filterLine says which filter is active ("" when none).
func (m Model) filterLine() string {
	if m.jq.State == "" && m.jq.CollectionID == "" {
		return ""
	}
	parts := []string{}
	if m.jq.State != "" {
		parts = append(parts, stateSetLabel(m.jq.State, m.lang))
	}
	if m.jq.CollectionID != "" {
		parts = append(parts, m.tf(i18n.JobsFilterSource, cut(Sanitize(orElse(m.jqName, m.jq.CollectionID)), 28)))
	}
	return m.accent(m.t(i18n.JobsFilterLabel) + ": " + strings.Join(parts, " · "))
}

// jobsHeadLines are the extra lines between the title and the rows: the paused
// queue and the active filter. listShape counts them.
func (m Model) jobsHeadLines() []string {
	var out []string
	if m.snap.Status.QueuePaused {
		for _, l := range wrapLines(m.t(i18n.QueuePausedBanner), max(20, m.width-2)) {
			out = append(out, m.warn(l))
		}
	}
	if l := m.filterLine(); l != "" {
		out = append(out, l)
	}
	return out
}

// ---- jobs keys ----

// hasJobsExt says whether the daemon-backed Jobs keys of batch IT are wired.
func (m Model) hasJobsExt() bool {
	return m.acts.SetJobQuery != nil || m.acts.QueuePause != nil || m.acts.MoveJob != nil
}

// jobsKeyIT handles the keys added by batch IT; it reports false for the rest.
func (m Model) jobsKeyIT(key string) (tea.Model, tea.Cmd, bool) {
	switch key {
	case "F":
		m = m.openStatePicker()
		return m, nil, true
	case "f":
		next, cmd := m.cycleJobState()
		return next, cmd, true
	case "s":
		next, cmd := m.toggleJobSource()
		return next, cmd, true
	case "P":
		next, cmd := m.pauseQueue(true)
		return next, cmd, true
	case "U":
		next, cmd := m.pauseQueue(false)
		return next, cmd, true
	case "K":
		next, cmd := m.moveQueued(-1)
		return next, cmd, true
	case "J":
		next, cmd := m.moveQueued(1)
		return next, cmd, true
	case "+", "=":
		next, cmd := m.bumpPriority(1)
		return next, cmd, true
	case "-", "_":
		next, cmd := m.bumpPriority(-1)
		return next, cmd, true
	case "E":
		next, cmd := m.startExport("history", "")
		return next, cmd, true
	}
	return m, nil, false
}

// pauseQueue pauses (after a yes) or resumes the whole queue.
func (m Model) pauseQueue(pause bool) (Model, tea.Cmd) {
	if m.acts.QueuePause == nil {
		return m, nil
	}
	acts, ctx := m.acts, m.ctx
	run := func() tea.Msg {
		n, err := acts.QueuePause(ctx, pause)
		return queueMsg{paused: pause, affected: n, err: err}
	}
	if pause {
		if m.snap.Status.QueuePaused {
			return m, nil
		}
		m.confirm = &confirmation{prompt: m.t(i18n.QueueConfirmPause), run: run}
		return m, nil
	}
	if !m.snap.Status.QueuePaused {
		return m, nil
	}
	return m, run
}

func (m Model) onQueue(msg queueMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.status = Sanitize(msg.err.Error())
		return m, nil
	}
	m.snap.Status.QueuePaused = msg.paused
	if msg.paused {
		m.status = m.tf(i18n.QueuePausedOK, msg.affected)
	} else {
		m.status = m.t(i18n.QueueResumedOK)
	}
	return m, m.refetch()
}

// ---- order of queued jobs ----

// queueOrder lists the queued jobs best first: higher priority, then the older
// admission, then the moves this session made, replayed in order. The daemon
// does not report the position, so after a move made by another client this is
// the best estimate and the next real start corrects it.
func queueOrder(jobs []Job, moves []moveRec) []string {
	type entry struct {
		id   string
		prio int
		at   time.Time
		idx  int
	}
	var list []entry
	for i, j := range jobs {
		if j.State == "queued" {
			list = append(list, entry{j.ID, j.Priority, j.Created, i})
		}
	}
	sort.SliceStable(list, func(a, b int) bool {
		x, y := list[a], list[b]
		switch {
		case x.prio != y.prio:
			return x.prio > y.prio
		case !x.at.Equal(y.at):
			return x.at.Before(y.at)
		}
		return x.idx > y.idx // the list is newest first
	})
	order := make([]string, len(list))
	for i, e := range list {
		order[i] = e.id
	}
	for _, mv := range moves {
		order = applyMove(order, mv)
	}
	return order
}

func indexOf(ids []string, id string) int {
	for i, v := range ids {
		if v == id {
			return i
		}
	}
	return -1
}

// applyMove mirrors the daemon: take the job out, put it before or after the
// target. A move whose job or target is gone any more is skipped.
func applyMove(order []string, mv moveRec) []string {
	from := indexOf(order, mv.id)
	if from < 0 {
		return order
	}
	rest := append(append([]string{}, order[:from]...), order[from+1:]...)
	target := orElse(mv.before, mv.after)
	at := indexOf(rest, target)
	if at < 0 {
		return order
	}
	if mv.after != "" {
		at++
	}
	out := append(append([]string{}, rest[:at]...), mv.id)
	return append(out, rest[at:]...)
}

// moveQueued moves the highlighted queued job one place earlier (-1) or later (+1).
func (m Model) moveQueued(dir int) (Model, tea.Cmd) {
	j, ok := m.selectedJob()
	if !ok || m.acts.MoveJob == nil {
		return m, nil
	}
	if j.State != "queued" {
		m.status = m.t(i18n.JobPriorityOnly)
		return m, nil
	}
	order := queueOrder(m.snap.Jobs, m.qm.recs)
	at := indexOf(order, j.ID) + dir
	if at < 0 || at >= len(order) {
		m.status = m.t(i18n.JobMoveNone)
		return m, nil
	}
	mv := JobMove{}
	rec := moveRec{id: j.ID}
	if dir < 0 {
		mv.Before, rec.before = order[at], order[at]
	} else {
		mv.After, rec.after = order[at], order[at]
	}
	acts, ctx, id, rev := m.acts, m.ctx, j.ID, j.Revision
	return m, func() tea.Msg {
		return moveMsg{rec: rec, jobID: id, err: acts.MoveJob(ctx, id, rev, mv)}
	}
}

// bumpPriority raises or lowers the priority of the highlighted queued job by one.
func (m Model) bumpPriority(step int) (Model, tea.Cmd) {
	j, ok := m.selectedJob()
	if !ok || m.acts.MoveJob == nil {
		return m, nil
	}
	if j.State != "queued" {
		m.status = m.t(i18n.JobPriorityOnly)
		return m, nil
	}
	p := max(minPriority, min(maxPriority, j.Priority+step))
	if p == j.Priority {
		m.status = m.tf(i18n.JobPriorityOK, Sanitize(j.ID), p)
		return m, nil
	}
	acts, ctx, id, rev := m.acts, m.ctx, j.ID, j.Revision
	return m, func() tea.Msg {
		return moveMsg{jobID: id, prio: p, isPrio: true, err: acts.MoveJob(ctx, id, rev, JobMove{Priority: &p})}
	}
}

func (m Model) onMove(msg moveMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.status = Sanitize(msg.err.Error())
		return m, m.refetch()
	}
	if msg.isPrio {
		m.status = m.tf(i18n.JobPriorityOK, Sanitize(msg.jobID), msg.prio)
	} else {
		m.qm.recs = append(m.qm.recs, msg.rec)
		if len(m.qm.recs) > 64 {
			m.qm.recs = m.qm.recs[len(m.qm.recs)-64:]
		}
		m.status = m.tf(i18n.JobMovedOK, Sanitize(msg.jobID))
	}
	return m, m.refetch()
}

// ---- batch progress ----

// batchTail is the short, readable end of a batch id.
func batchTail(id string) string {
	id = strings.TrimPrefix(id, "batch-")
	if len(id) > 6 {
		return id[len(id)-6:]
	}
	return id
}

// batchChip is "batch abc123 3/10" with the last progress read, or just the id.
func (m Model) batchChip(id string) string {
	text := m.tf(i18n.BatchTag, batchTail(Sanitize(id)))
	if st := m.batches[id]; st != nil && st.prog != nil && st.prog.Total > 0 {
		text += " " + itoa(st.prog.CompletedJobs) + "/" + itoa(st.prog.Total)
	}
	return text
}

func itoa(n int) string { return strconv.Itoa(n) }

// pollBatches reads the progress of the batches shown, at most every few
// seconds and only while one of their jobs is not finished. It rides on the
// snapshot refresh, so there is no timer of its own.
func (m Model) pollBatches() tea.Cmd {
	if m.acts.GetBatch == nil {
		return nil
	}
	open := map[string]bool{}
	var ids []string
	for _, j := range m.snap.Jobs {
		if j.BatchID == "" {
			continue
		}
		if _, seen := open[j.BatchID]; !seen {
			ids = append(ids, j.BatchID)
			open[j.BatchID] = false
		}
		if !terminalState(j.State) {
			open[j.BatchID] = true
		}
	}
	var cmds []tea.Cmd
	for i, id := range ids {
		if i >= 4 {
			break
		}
		st := m.batches[id]
		if st == nil {
			st = &batchState{}
			m.batches[id] = st
		}
		fresh := st.prog != nil && m.opt.now().Sub(st.at) < batchFreshMS*time.Millisecond
		if st.loading || fresh || (st.prog != nil && !open[id]) {
			continue
		}
		st.loading = true
		acts, ctx, bid := m.acts, m.ctx, id
		cmds = append(cmds, func() tea.Msg {
			p, err := acts.GetBatch(ctx, bid)
			return batchMsg{id: bid, prog: p, err: err}
		})
	}
	return tea.Batch(cmds...)
}

func (m Model) onBatch(msg batchMsg) (tea.Model, tea.Cmd) {
	st := m.batches[msg.id]
	if st == nil {
		st = &batchState{}
		m.batches[msg.id] = st
	}
	st.loading = false
	if msg.err == nil {
		p := msg.prog
		st.prog, st.at = &p, m.opt.now()
	}
	return m, nil
}

// queueLine is what the second row of a queued job says.
func (m Model) queueLine(j Job) string {
	var parts []string
	parts = append(parts, present.Queue(j.QueueAhead, m.snap.Status.QueuePaused, m.lang))
	if j.Priority != 0 {
		parts = append(parts, m.t(i18n.PriorityLabel)+" "+signed(j.Priority))
	}
	if j.BatchID != "" {
		parts = append(parts, m.batchChip(j.BatchID))
	}
	if len(parts) == 0 {
		parts = append(parts, present.StateLabel("queued", m.lang))
	}
	return m.dim(strings.Join(parts, " · "))
}

func signed(n int) string {
	if n > 0 {
		return "+" + itoa(n)
	}
	return itoa(n)
}

// batchDetail is the batch block of the job screen: its id and how far it is.
func (m Model) batchDetail(j Job) []string {
	if j.BatchID == "" {
		return nil
	}
	st := m.batches[j.BatchID]
	if st == nil || st.prog == nil {
		return []string{m.kv("", m.dim(m.batchChip(j.BatchID)))}
	}
	return []string{m.kv("", m.dim(m.tf(i18n.BatchProgressLine, batchTail(Sanitize(j.BatchID)),
		st.prog.CompletedJobs, st.prog.Total, byStateText(st.prog.ByState, m.lang))))}
}

// byStateText is "2 Done, 1 Failed" in a fixed order.
func byStateText(by map[string]int, lang string) string {
	var parts []string
	for _, s := range []string{"running", "queued", "paused", "blocked", "failed", "partial", "cancelled", "succeeded"} {
		if n := by[s]; n > 0 {
			parts = append(parts, itoa(n)+" "+present.StateLabel(s, lang))
		}
	}
	return strings.Join(parts, ", ")
}
