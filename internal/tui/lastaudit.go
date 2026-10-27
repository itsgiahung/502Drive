package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/itsgiahung/502Drive/internal/i18n"
)

// The newest integrity audit of each collection (GET .../audits/latest). The
// Library shows it on every visible row and the collection screen seeds its
// audit section from it, so "repairable" is known without running a new scan.

const lastAuditTTL = 60 * time.Second

type lastAuditMsg struct {
	id    string
	rep   AuditReport
	found bool
	err   error
}

// pollLastAudits reads the newest audit of the collections the user can see:
// the visible rows of the Library, or the open collection. A result is reused
// for a minute; a collection with no audit is remembered too.
func (m Model) pollLastAudits() tea.Cmd {
	if m.acts.LatestAudit == nil || !m.online {
		return nil
	}
	var cols []Collection
	switch m.screen {
	case screenLibrary:
		start, end := m.window()
		if end > len(m.snap.Collections) {
			end = len(m.snap.Collections)
		}
		if start < end {
			cols = m.snap.Collections[start:end]
		}
	case screenCollection:
		if c, ok := m.selectedCollection(); ok {
			cols = []Collection{c}
		}
	}
	var cmds []tea.Cmd
	for _, c := range cols {
		if c.State == "retired" {
			continue
		}
		st := m.lastAudits[c.ID]
		if st == nil {
			st = &lastAuditState{}
			m.lastAudits[c.ID] = st
		}
		known := !st.at.IsZero() && m.opt.now().Sub(st.at) < lastAuditTTL
		if st.loading || known {
			continue
		}
		st.loading = true
		acts, ctx, id := m.acts, m.ctx, c.ID
		cmds = append(cmds, func() tea.Msg {
			rep, found, err := acts.LatestAudit(ctx, id)
			return lastAuditMsg{id: id, rep: rep, found: found, err: err}
		})
	}
	return tea.Batch(cmds...)
}

func (m Model) onLastAudit(msg lastAuditMsg) (tea.Model, tea.Cmd) {
	st := m.lastAudits[msg.id]
	if st == nil {
		st = &lastAuditState{}
		m.lastAudits[msg.id] = st
	}
	st.loading = false
	if msg.err != nil {
		return m, nil // keep what was known; the next refresh tries again
	}
	st.at = m.opt.now()
	if !msg.found {
		st.rep = nil
		return m, nil
	}
	rep := msg.rep
	st.rep = &rep
	// The collection screen works on the audit it holds: seed it, unless a
	// scan from this session is newer.
	a := m.auditOf(msg.id)
	newer := a.report == nil || (a.fromLatest && (a.report.ID != rep.ID || a.report.State != rep.State))
	if !a.starting && newer && a.repairJob == "" {
		a.report, a.fromLatest = &rep, true
	}
	return m, nil
}

// auditChip is the short audit state on a Library row ("" when unknown).
func (m Model) auditChip(c Collection) string {
	st := m.lastAudits[c.ID]
	if st == nil || st.rep == nil {
		return ""
	}
	r := st.rep
	switch r.State {
	case "running":
		return m.dim(m.tf(i18n.LastAuditChip, m.t(i18n.LastAuditRunning)))
	case "failed":
		return m.bad(m.tf(i18n.LastAuditChip, m.t(i18n.LastAuditFailed)))
	}
	if r.Total() == 0 {
		return m.good(m.tf(i18n.LastAuditChip, m.t(i18n.LastAuditClean)))
	}
	return m.warn(m.tf(i18n.LastAuditChip, m.tf(i18n.LastAuditFinding, r.Total(), r.Repairable)))
}
