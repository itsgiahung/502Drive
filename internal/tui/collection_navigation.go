package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/itsgiahung/502Drive/internal/i18n"
)

func (m Model) leaveCollectionSubview(id string) Model {
	m.fe, m.ab, m.syncMenu = nil, nil, false
	m.top, m.cursor, m.screen = 0, 0, screenLibrary
	for i, c := range m.snap.Collections {
		if c.ID == id {
			m.cursor, m.screen = i, screenCollection
			break
		}
	}
	return m
}

// Detail focus is separate from cursor, which still identifies the open collection.
func (m Model) collectionNavigation(key string) (tea.Model, tea.Cmd, bool) {
	c, ok := m.currentCollection()
	if !ok {
		return m, nil, false
	}
	if m.syncMenu {
		choices := m.collectionSyncChoices(c)
		switch key {
		case "left", "esc":
			m.syncMenu = false
		case "up", "k":
			m.syncRow = max(0, m.syncRow-1)
		case "down", "j":
			m.syncRow = min(max(0, len(choices)-1), m.syncRow+1)
		case "home", "g":
			m.syncRow = 0
		case "end", "G":
			m.syncRow = max(0, len(choices)-1)
		case "enter", "right", " ":
			if len(choices) > 0 {
				choice := choices[min(m.syncRow, len(choices)-1)]
				m.syncMenu = false
				if choice.key == "u" {
					next, cmd := m.syncAct(c, "resume")
					return next, cmd, true
				}
				next, cmd := m.screenKey(choice.key)
				return next, cmd, true
			}
		default:
			return m, nil, key != "q" && key != "ctrl+c"
		}
		return m, nil, true
	}
	rows := []int{0, 1}
	if c.ResultURL != "" {
		rows = append(rows, 2)
	}
	rows = append(rows, 3)
	if m.acts.StartAudit != nil {
		rows = append(rows, 4)
	}
	index := 0
	for i, row := range rows {
		if row == m.detailRow {
			index = i
		}
	}
	switch key {
	case "up", "k":
		if index == 0 {
			m.tabFocus = true
		} else {
			m.detailRow = rows[index-1]
		}
	case "down", "j":
		m.detailRow = rows[min(index+1, len(rows)-1)]
	case "home", "g", "pgup":
		m.detailRow = rows[0]
	case "end", "G", "pgdown":
		m.detailRow = rows[len(rows)-1]
	case "enter", "right":
		switch m.detailRow {
		case 1:
			m.syncMenu, m.syncRow = true, 0
		case 2:
			if m.acts.OpenURL != nil {
				return m, m.act(func(context.Context) error { return m.acts.OpenURL(c.ResultURL) }), true
			}
		case 3:
			return m.openFilter(c), nil, true
		case 4:
			if a := m.audits[c.ID]; a != nil && a.report != nil && m.acts.AuditFindings != nil {
				next, cmd := m.openAuditBrowser(c)
				return next, cmd, true
			}
			next, cmd := m.startAudit(c)
			return next, cmd, true
		}
	case " ":
		return m, nil, true
	default:
		return m, nil, false
	}
	return m, nil, true
}

func (m Model) collectionKV(row int, label, value string) string {
	marker := "  "
	if m.detailRow == row && !m.tabFocus {
		marker = m.accent("> ")
	}
	return marker + m.kv(label, value)
}

type syncChoice struct {
	label i18n.Key
	key   string
}

func (m Model) collectionSyncChoices(c Collection) []syncChoice {
	var choices []syncChoice
	if c.State != "retired" && m.acts.SyncPreset != nil {
		choices = append(choices, syncChoice{i18n.PresetDaily, "d"}, syncChoice{i18n.PresetHourly, "h"},
			syncChoice{i18n.PresetSixH, "e"}, syncChoice{i18n.PresetReal, "t"})
	}
	if c.Sync != nil && c.State != "retired" {
		if m.acts.RunNow != nil {
			choices = append(choices, syncChoice{i18n.RunNow, "n"})
		}
		if m.acts.SyncAction != nil {
			if c.Sync.State == "paused" {
				choices = append(choices, syncChoice{i18n.ResumeSync, "u"})
			} else if c.Sync.State == "active" || c.Sync.State == "bootstrapping" {
				choices = append(choices, syncChoice{i18n.PauseSync, "p"})
			}
			choices = append(choices, syncChoice{i18n.StopSync, "x"})
		}
	}
	return choices
}

func (m Model) collectionSyncView(c Collection) []string {
	lines := []string{"", m.bold(Sanitize(c.SourceName)), "", m.t(i18n.PickPreset), ""}
	for i, choice := range m.collectionSyncChoices(c) {
		marker := "  "
		if i == m.syncRow {
			marker = m.accent("> ")
		}
		lines = append(lines, marker+m.t(choice.label))
	}
	return append(lines, "", m.dim(m.t(i18n.DetailKeys)))
}
