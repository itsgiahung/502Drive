package tui

import (
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/itsgiahung/502Drive/internal/i18n"
	"github.com/itsgiahung/502Drive/internal/present"
	"strings"
)

type auditBrowser struct {
	collection       Collection
	id, kind, cursor string
	previous         []string
	page             FindingPage
	loading          bool
	err              string
}
type auditPageMsg struct {
	id, kind, cursor string
	page             FindingPage
	err              error
}

func (m Model) openAuditBrowser(c Collection) (Model, tea.Cmd) {
	a := m.audits[c.ID]
	if a == nil || a.report == nil || m.acts.AuditFindings == nil {
		m.status = m.t(i18n.AuditNeedsReport)
		return m, nil
	}
	m.screen = screenAudit
	m.cursor, m.top = 0, 0
	m.ab = &auditBrowser{collection: c, id: a.report.ID}
	return m, m.loadFindings()
}
func (m Model) loadFindings() tea.Cmd {
	a := m.ab
	if a == nil || a.loading {
		return nil
	}
	a.loading = true
	id, kind, cursor, acts, ctx := a.id, a.kind, a.cursor, m.acts, m.ctx
	return func() tea.Msg {
		page, err := acts.AuditFindings(ctx, id, kind, cursor)
		return auditPageMsg{id, kind, cursor, page, err}
	}
}
func (m Model) onFindings(msg auditPageMsg) (tea.Model, tea.Cmd) {
	a := m.ab
	if a == nil || a.id != msg.id || a.kind != msg.kind || a.cursor != msg.cursor {
		return m, nil
	}
	a.loading = false
	if msg.err != nil {
		a.err = Sanitize(msg.err.Error())
		return m, nil
	}
	a.page, a.err = msg.page, ""
	m.cursor, m.top = 0, 0
	return m, nil
}
func (m Model) auditBrowserKey(key string) (tea.Model, tea.Cmd) {
	a := m.ab
	if a == nil {
		return m, nil
	}
	switch key {
	case "esc":
		m.screen = screenCollection
		m.cursor, m.top = 0, 0
		for i, c := range m.snap.Collections {
			if c.ID == a.collection.ID {
				m.cursor = i
			}
		}
		m.ab = nil
		return m, nil
	case "q":
		m.quits = true
		return m, tea.Quit
	case "n":
		if !a.loading && a.page.Next != "" {
			a.previous = append(a.previous, a.cursor)
			a.cursor = a.page.Next
			m.cursor, m.top = 0, 0
			return m, m.loadFindings()
		}
	case "p":
		if !a.loading && len(a.previous) > 0 {
			a.cursor = a.previous[len(a.previous)-1]
			a.previous = a.previous[:len(a.previous)-1]
			return m, m.loadFindings()
		}
	case "t":
		if a.loading {
			return m, nil
		}
		idx := -1
		for i, k := range auditKinds {
			if a.kind == k.kind {
				idx = i
			}
		}
		idx++
		a.kind = ""
		if idx < len(auditKinds) {
			a.kind = auditKinds[idx].kind
		}
		a.cursor = ""
		a.previous = nil
		return m, m.loadFindings()
	case "r":
		return m, m.loadFindings()
	case "f":
		return m.repair(a.collection, true)
	case "F":
		return m.repair(a.collection, false)
	case "up", "k":
		m = m.move(-1)
		m.fixTop()
	case "down", "j":
		m = m.move(1)
		m.fixTop()
	}
	return m, nil
}
func (m Model) auditBrowserView() []string {
	a := m.ab
	if a == nil {
		return nil
	}
	kind := a.kind
	if kind == "" {
		kind = m.t(i18n.AuditAllKinds)
	}
	lines := []string{m.bold(m.t(i18n.AuditTitle)), Sanitize(a.id) + " · " + Sanitize(kind), m.t(i18n.AuditBrowserKeys), ""}
	if a.loading {
		return append(lines, m.t(i18n.Loading))
	}
	if a.err != "" {
		return append(lines, m.bad(a.err))
	}
	start, end := m.window()
	for i := start; i < end; i++ {
		f := a.page.Items[i]
		e := present.Explain(f.Kind, m.lang)
		lines = append(lines, fmt.Sprintf("%s %s · %s", m.marker(i, ""), e.Title, Sanitize(orElse(f.Path, orElse(f.Detail, orElse(f.SourceID, f.DestID))))))
		lines = append(lines, "  "+e.Message+" "+e.Action)
	}
	if state := m.audits[a.collection.ID]; state != nil && state.report != nil {
		lines = append(lines, m.tf(i18n.AuditPreviewInfo, len(state.report.Plan)))
		if state.planned {
			for _, op := range state.report.Plan {
				lines = append(lines, Sanitize(strings.Join([]string{op.Action, op.SourceID, op.OldDestID}, " · ")))
			}
		}
	}
	return lines
}
