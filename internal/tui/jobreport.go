package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/itsgiahung/502Drive/internal/i18n"
	"github.com/itsgiahung/502Drive/internal/present"
)

// The problem report (X1) of a job: the failed, blocked and skipped items with
// their stable K1 code, and the CSV export of that report or of the job
// history to a path the user types. The daemon decides what is a problem; the
// CLI writes the file and never overwrites one.

// reportView is the report screen's state. jobCursor and jobTop put the cursor
// back on the job when the user leaves.
type reportView struct {
	jobID     string
	jobState  string
	jobCursor int
	jobTop    int
	loading   bool
	err       string
	rep       *JobReport
}

type reportMsg struct {
	jobID string
	rep   JobReport
	err   error
}

// exportFlow is the CSV path being typed. kind is "report" (jobID is the job)
// or "history".
type exportFlow struct {
	kind, jobID string
	input       *textinput.Model
}

type exportMsg struct {
	rows int
	path string
	err  error
}

// openReport shows the problems of j.
func (m Model) openReport(j Job) (tea.Model, tea.Cmd) {
	if m.acts.Report == nil {
		return m, nil
	}
	m.rep = &reportView{jobID: j.ID, jobState: j.State, jobCursor: m.cursor, jobTop: m.top, loading: true}
	m.screen, m.cursor, m.top = screenReport, 0, 0
	acts, ctx, id := m.acts, m.ctx, j.ID
	return m, tea.Batch(func() tea.Msg {
		rep, err := acts.Report(ctx, id)
		return reportMsg{jobID: id, rep: rep, err: err}
	}, m.startSpinner())
}

func (m Model) onReport(msg reportMsg) (tea.Model, tea.Cmd) {
	if m.rep == nil || m.rep.jobID != msg.jobID {
		return m, nil
	}
	m.rep.loading = false
	if msg.err != nil {
		m.rep.err = Sanitize(msg.err.Error())
		return m, nil
	}
	rep := msg.rep
	m.rep.rep, m.rep.err = &rep, ""
	m.cursor, m.top = 0, 0
	return m, nil
}

// leaveReport goes back to the job with the cursor where it was.
func (m Model) leaveReport() (tea.Model, tea.Cmd) {
	m.screen = screenJobs
	if m.rep != nil {
		m.cursor, m.top = m.rep.jobCursor, m.rep.jobTop
		for i, j := range m.snap.Jobs {
			if j.ID == m.rep.jobID {
				m.cursor, m.screen = i, screenJob
				break
			}
		}
	}
	m.rep, m.exp = nil, nil
	m.clampCursor()
	return m, nil
}

// reportCodes counts the problem items by code, biggest first.
func reportCodes(items []ProblemItem) []codeCount {
	by := map[string]int{}
	for _, it := range items {
		by[it.Code]++
	}
	out := make([]codeCount, 0, len(by))
	for c, n := range by {
		out = append(out, codeCount{c, n})
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].n != out[b].n {
			return out[a].n > out[b].n
		}
		return out[a].code < out[b].code
	})
	return out
}

type codeCount struct {
	code string
	n    int
}

// reportHead is the part above the rows: title, job, the count by code.
func (m Model) reportHead() []string {
	r := m.rep
	if r == nil {
		return []string{""}
	}
	lines := []string{"", m.bold(m.t(i18n.ReportTitle)) + " " + m.dim(Sanitize(r.jobID)), ""}
	switch {
	case r.loading:
		return append(lines[:2], m.accent(m.spinChar())+" "+m.dim(m.t(i18n.ReportLoading)))
	case r.err != "":
		return append(lines[:2], m.bad(r.err))
	case r.rep == nil || len(r.rep.Items) == 0:
		return append(lines[:2], m.dim(m.t(i18n.ReportEmpty)))
	}
	for _, c := range reportCodes(r.rep.Items) {
		lines = append(lines, cut(fmt.Sprintf("%5d  %s", c.n, m.skipLabel(c.code)), max(10, m.width-2)))
	}
	return lines
}

// reportView renders the screen: the head, then the windowed rows.
func (m Model) reportScreen() []string {
	lines := m.reportHead()
	r := m.rep
	if r == nil || r.rep == nil || len(r.rep.Items) == 0 {
		return append(lines, m.exportLines()...)
	}
	start, end := m.window()
	for i := start; i < end; i++ {
		it := r.rep.Items[i]
		cursor := " "
		if i == m.cursor {
			cursor = m.accent(">")
		}
		path := cut(Sanitize(orElse(it.Path, "—")), max(10, m.width-40))
		row := fmt.Sprintf("%s %-8s %-18s %s", cursor, m.stateWordShort(it.State), cut(Sanitize(it.Code), 18), path)
		if it.Attempts > 1 && m.width >= 70 {
			row += m.dim(" · " + m.tf(i18n.ReportRowTries, it.Attempts))
		}
		lines = append(lines, row)
	}
	if r.rep.Truncated {
		lines = append(lines, m.warn(m.tf(i18n.ReportCut, len(r.rep.Items))))
	}
	if m.cursor >= 0 && m.cursor < len(r.rep.Items) {
		it := r.rep.Items[m.cursor]
		lines = append(lines, wrapLines(Sanitize(present.Advice(it.Code, m.lang)), max(20, m.width-2))...)
	}
	return append(lines, m.exportLines()...)
}

func (m Model) stateWordShort(state string) string { return cut(m.stateWord(state), 14) }

// ---- CSV export ----

// startExport asks for the path. The suggested name is a plain file in the
// current directory; the CLI refuses to overwrite one that exists.
func (m Model) startExport(kind, jobID string) (Model, tea.Cmd) {
	if m.acts.ExportCSV == nil {
		return m, nil
	}
	ti := newInput(m.width, "502drive-"+kind+".csv")
	name := "502drive-history.csv"
	if kind == "report" {
		name = "502drive-report-" + strings.TrimPrefix(jobID, "job_") + ".csv"
	}
	ti.SetValue(name)
	ti.CursorEnd()
	m.exp = &exportFlow{kind: kind, jobID: jobID, input: ti}
	return m, textinput.Blink
}

func (m Model) exportTyping(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	e := m.exp
	switch msg.Type {
	case tea.KeyCtrlC:
		m.quits = true
		return m, tea.Quit
	case tea.KeyEsc:
		m.exp = nil
		return m, nil
	case tea.KeyEnter:
		path := strings.TrimSpace(e.input.Value())
		if path == "" {
			m.status = m.t(i18n.ExportNeedsPath)
			return m, nil
		}
		m.exp = nil
		acts, ctx, kind, id := m.acts, m.ctx, e.kind, e.jobID
		return m, func() tea.Msg {
			rows, err := acts.ExportCSV(ctx, kind, id, path)
			return exportMsg{rows: rows, path: path, err: err}
		}
	}
	var cmd tea.Cmd
	*e.input, cmd = e.input.Update(msg)
	return m, cmd
}

func (m Model) onExport(msg exportMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.status = Sanitize(msg.err.Error())
		return m, nil
	}
	m.status = m.tf(i18n.ExportDone, msg.rows, Sanitize(msg.path))
	return m, nil
}

// exportLines is the path prompt, drawn under whatever screen opened it.
func (m Model) exportLines() []string {
	e := m.exp
	if e == nil {
		return nil
	}
	prompt := m.t(i18n.ExportPrompt)
	if e.kind == "history" {
		prompt = m.t(i18n.ExportHistory)
	}
	lines := []string{""}
	for _, l := range wrapLines(prompt, max(20, m.width-2)) {
		lines = append(lines, m.accent(l))
	}
	lines = append(lines, e.input.View(), m.dim(m.t(i18n.ExportKeys)))
	return lines
}
