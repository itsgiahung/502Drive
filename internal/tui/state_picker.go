package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/itsgiahung/502Drive/internal/i18n"
	"github.com/itsgiahung/502Drive/internal/present"
	"sort"
	"strings"
)

var stateChoices = []string{"queued", "running", "pausing", "paused", "cancelling", "reconciling", "blocked", "succeeded", "partial", "failed", "cancelled"}

type statePicker struct {
	selected map[string]bool
	row      int
}

func (m Model) openStatePicker() Model {
	m.statePick = &statePicker{selected: map[string]bool{}}
	for _, st := range strings.Split(m.jq.State, ",") {
		if st != "" {
			m.statePick.selected[st] = true
		}
	}
	return m
}
func (m Model) statePickerKey(key string) (tea.Model, tea.Cmd) {
	p := m.statePick
	switch key {
	case "esc", "left":
		m.statePick = nil
	case "up", "k":
		p.row = max(0, p.row-1)
	case "down", "j":
		p.row = min(len(stateChoices)-1, p.row+1)
	case " ":
		st := stateChoices[p.row]
		p.selected[st] = !p.selected[st]
	case "enter":
		var states []string
		for st, on := range p.selected {
			if on {
				states = append(states, st)
			}
		}
		sort.Strings(states)
		q := m.jq
		q.State = strings.Join(states, ",")
		q.Pages = 1
		m.statePick = nil
		return m.applyJobQuery(q)
	}
	return m, nil
}
func (m Model) statePickerView() []string {
	lines := []string{m.t(i18n.JobStatesChoose), ""}
	for i, st := range stateChoices {
		mark := "[ ]"
		if m.statePick.selected[st] {
			mark = "[x]"
		}
		cursor := "  "
		if i == m.statePick.row {
			cursor = "> "
		}
		lines = append(lines, cursor+mark+" "+present.StateLabel(st, m.lang))
	}
	return lines
}
func stateSetLabel(raw, lang string) string {
	var labels []string
	for _, st := range strings.Split(raw, ",") {
		labels = append(labels, present.StateLabel(st, lang))
	}
	return strings.Join(labels, ", ")
}
