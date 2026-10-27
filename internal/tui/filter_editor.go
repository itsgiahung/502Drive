package tui

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/itsgiahung/502Drive/internal/filter"
	"github.com/itsgiahung/502Drive/internal/i18n"
	"strconv"
	"strings"
)

type filterEditor struct {
	collection Collection
	fields     []textinput.Model
	row        int
	spec       filter.Spec
	err        string
	preview    *PlanFilter
	busy       bool
}
type filterEditMsg struct {
	id      string
	preview *PlanFilter
	saved   bool
	err     error
}

var filterFields = []i18n.Key{i18n.FilterInclude, i18n.FilterExclude, i18n.FilterMin, i18n.FilterMax, i18n.FilterModified, i18n.FilterKeep}

func (m Model) openFilter(c Collection) Model {
	m.screen = screenFilter
	f := &filterEditor{collection: c}
	values := []string{strings.Join(c.Filter.Include, ";"), strings.Join(c.Filter.Exclude, ";"), strconv.FormatInt(c.Filter.MinSize, 10), strconv.FormatInt(c.Filter.MaxSize, 10), c.Filter.ModifiedAfter, strconv.FormatBool(c.Filter.KeepEmptyFolders)}
	for _, v := range values {
		in := textinput.New()
		in.CharLimit = 4096
		in.Width = max(12, m.width-28)
		in.SetValue(v)
		f.fields = append(f.fields, in)
	}
	f.fields[0].Focus()
	m.fe = f
	return m.validateFilter()
}
func filterRulesText(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ";")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}
func (m Model) validateFilter() Model {
	f := m.fe
	spec := filter.Spec{Include: filterRulesText(f.fields[0].Value()), Exclude: filterRulesText(f.fields[1].Value()), ModifiedAfter: strings.TrimSpace(f.fields[4].Value())}
	var err error
	parseSize := func(v string) (int64, error) {
		if strings.TrimSpace(v) == "" {
			return 0, nil
		}
		return filter.ParseSize(v)
	}
	spec.MinSize, err = parseSize(f.fields[2].Value())
	if err == nil {
		spec.MaxSize, err = parseSize(f.fields[3].Value())
	}
	if err == nil {
		spec.KeepEmptyFolders, err = strconv.ParseBool(f.fields[5].Value())
	}
	if err == nil {
		err = spec.Validate()
	}
	f.err = ""
	if err != nil {
		f.err = Sanitize(err.Error())
	}
	f.spec = spec
	return m
}
func (m Model) filterTyping(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	f := m.fe
	switch msg.String() {
	case "esc":
		if !f.busy {
			m = m.leaveCollectionSubview(f.collection.ID)
		}
		return m, nil
	case "tab", "shift+tab", "up", "down":
		step := 1
		if msg.String() == "shift+tab" || msg.String() == "up" {
			step = -1
		}
		f.fields[f.row].Blur()
		f.row = (f.row + step + len(f.fields)) % len(f.fields)
		return m, f.fields[f.row].Focus()
	case "ctrl+p", "ctrl+s":
		m = m.validateFilter()
		if f.err != "" || f.busy {
			return m, nil
		}
		save := msg.String() == "ctrl+s"
		c, spec, acts, ctx := f.collection, f.spec, m.acts, m.ctx
		if (save && acts.SetCollectionFilter == nil) || (!save && acts.PreviewCollectionFilter == nil) {
			return m, nil
		}
		f.busy = true
		return m, func() tea.Msg {
			if save {
				err := acts.SetCollectionFilter(ctx, c.ID, c.Revision, spec)
				return filterEditMsg{id: c.ID, saved: true, err: err}
			}
			preview, err := acts.PreviewCollectionFilter(ctx, c, spec)
			return filterEditMsg{id: c.ID, preview: preview, err: err}
		}
	}
	var cmd tea.Cmd
	f.fields[f.row], cmd = f.fields[f.row].Update(msg)
	f.preview = nil
	return m.validateFilter(), cmd
}
func (m Model) onFilterEdit(msg filterEditMsg) (tea.Model, tea.Cmd) {
	f := m.fe
	if f == nil || f.collection.ID != msg.id {
		return m, nil
	}
	f.busy = false
	if msg.err != nil {
		f.err = Sanitize(msg.err.Error())
		return m, nil
	}
	if msg.saved {
		m = m.leaveCollectionSubview(f.collection.ID)
		return m, m.refetch()
	}
	f.preview = msg.preview
	return m, nil
}
func (m Model) filterEditorView() []string {
	f := m.fe
	if f == nil {
		return nil
	}
	lines := []string{m.bold(m.t(i18n.FilterTitle)), m.t(i18n.FilterEditorHelp), ""}
	for i, key := range filterFields {
		mark := "  "
		if i == f.row {
			mark = "> "
		}
		lines = append(lines, mark+m.t(key)+": "+f.fields[i].View())
		if i == f.row && f.err != "" {
			lines = append(lines, m.bad("  "+f.err))
		}
	}
	if f.preview != nil {
		p := f.preview
		lines = append(lines, m.tf(i18n.FilterPreviewCounts, p.Included, p.Excluded), m.t(i18n.FilterPreviewScope)+" "+Sanitize(p.Scope))
	}
	if f.busy {
		lines = append(lines, m.t(i18n.Loading))
	}
	return lines
}
