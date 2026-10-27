package tui

import (
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/itsgiahung/502Drive/internal/filter"
	"github.com/itsgiahung/502Drive/internal/i18n"
)

// filterEditor edits the full filter. Rows 0-4 are text fields; row keepRow
// is a switch (Space, Enter or arrows), never free text.
type filterEditor struct {
	collection Collection
	fields     []textinput.Model
	keep       bool
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

const keepRow = 5

// boundText shows a stored byte bound the way a person would type it; zero
// (no bound) is an empty field.
func boundText(n int64) string {
	for _, u := range []struct {
		suffix string
		m      int64
	}{{"TB", 1 << 40}, {"GB", 1 << 30}, {"MB", 1 << 20}, {"KB", 1 << 10}} {
		if n > 0 && n%u.m == 0 {
			return strconv.FormatInt(n/u.m, 10) + u.suffix
		}
	}
	if n <= 0 {
		return ""
	}
	return strconv.FormatInt(n, 10)
}

// boundDate shows a stored RFC 3339 bound as a plain date when it is midnight UTC.
func boundDate(v string) string {
	if t, err := time.Parse(time.RFC3339, v); err == nil && t.UTC().Format("15:04:05") == "00:00:00" {
		return t.UTC().Format("2006-01-02")
	}
	return v
}

// parseDate accepts 2026-01-31 (midnight UTC) or a full RFC 3339 time.
func parseDate(v string) (string, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", true
	}
	if t, err := time.Parse("2006-01-02", v); err == nil {
		return t.UTC().Format(time.RFC3339), true
	}
	if _, err := time.Parse(time.RFC3339, v); err == nil {
		return v, true
	}
	return "", false
}

func (m Model) openFilter(c Collection) Model {
	m.screen = screenFilter
	f := &filterEditor{collection: c, keep: c.Filter.KeepEmptyFolders}
	values := []string{strings.Join(c.Filter.Include, ";"), strings.Join(c.Filter.Exclude, ";"), boundText(c.Filter.MinSize), boundText(c.Filter.MaxSize), boundDate(c.Filter.ModifiedAfter)}
	placeholders := []string{"docs/**;*.pdf", "*.tmp;class:video", "500KB", "2GB", "2026-01-31"}
	for i, v := range values {
		in := textinput.New()
		in.CharLimit = 4096
		in.Width = max(12, m.width-28)
		in.SetValue(v)
		in.Placeholder = placeholders[i]
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
	spec := filter.Spec{Include: filterRulesText(f.fields[0].Value()), Exclude: filterRulesText(f.fields[1].Value()), KeepEmptyFolders: f.keep}
	f.err = ""
	parseSize := func(v string) (int64, bool) {
		if strings.TrimSpace(v) == "" {
			return 0, true
		}
		n, err := filter.ParseSize(v)
		return n, err == nil
	}
	var ok bool
	if spec.MinSize, ok = parseSize(f.fields[2].Value()); !ok {
		f.err = m.t(i18n.FilterBadSize)
	} else if spec.MaxSize, ok = parseSize(f.fields[3].Value()); !ok {
		f.err = m.t(i18n.FilterBadSize)
	} else if spec.ModifiedAfter, ok = parseDate(f.fields[4].Value()); !ok {
		f.err = m.t(i18n.FilterBadDate)
	} else if spec.MaxSize > 0 && spec.MinSize > spec.MaxSize {
		f.err = m.t(i18n.FilterMinOverMax)
	} else if err := spec.Validate(); err != nil {
		f.err = Sanitize(err.Error()) // a rule (glob, ext:, class:) the user can read and fix
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
		if f.row < keepRow {
			f.fields[f.row].Blur()
		}
		f.row = (f.row + step + keepRow + 1) % (keepRow + 1)
		if f.row < keepRow {
			return m, f.fields[f.row].Focus()
		}
		return m, nil
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
	if f.row == keepRow {
		// A switch: Space, Enter or the arrows flip it; letters do nothing.
		switch msg.String() {
		case " ", "enter", "left", "right":
			f.keep = !f.keep
			f.preview = nil
			return m.validateFilter(), nil
		}
		return m, nil
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
		if i == keepRow {
			box := "[ ] " + m.t(i18n.FilterNo)
			if f.keep {
				box = "[✓] " + m.t(i18n.FilterYes)
			}
			lines = append(lines, mark+m.t(key)+": "+box)
			continue
		}
		lines = append(lines, mark+m.t(key)+": "+f.fields[i].View())
	}
	if f.err != "" {
		lines = append(lines, "", m.bad(f.err))
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
