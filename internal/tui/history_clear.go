package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/itsgiahung/502Drive/internal/i18n"
)

// HistoryClearResult is what a history clear did, or would do on a preview.
type HistoryClearResult struct {
	DryRun        bool
	Hidden        int
	Skipped       int
	DeletedJobs   int
	DeletedEvents int
}

// The history screen (Settings → Clear history) always previews before it
// clears. The ranges come from a fixed list, never a typed number.
type histPage int

const (
	histScope histPage = iota
	histAsk
	histResult
)

// histChoices are the ranges offered; 0 means every finished job.
var histChoices = []int{7, 30, 90, 0}

type historyFlow struct {
	page    histPage
	row     int
	days    int
	loading bool
	err     string
	preview HistoryClearResult
	done    HistoryClearResult
}

type histPreviewMsg struct {
	days int
	res  HistoryClearResult
	err  error
}

type histDoneMsg struct {
	res HistoryClearResult
	err error
}

func (m Model) openHistory() (Model, tea.Cmd) {
	if m.acts.ClearHistory == nil {
		return m, nil
	}
	m.screen = screenHistory
	m.hist = &historyFlow{}
	return m, nil
}

func (m Model) histPreviewCmd(days int) tea.Cmd {
	acts, ctx := m.acts, m.ctx
	return func() tea.Msg {
		res, err := acts.ClearHistory(ctx, days, nil, true)
		return histPreviewMsg{days: days, res: res, err: err}
	}
}

func (m Model) histApplyCmd(days int) tea.Cmd {
	acts, ctx := m.acts, m.ctx
	return func() tea.Msg {
		res, err := acts.ClearHistory(ctx, days, nil, false)
		return histDoneMsg{res: res, err: err}
	}
}

func (m Model) onHistPreview(msg histPreviewMsg) Model {
	h := m.hist
	if h == nil || !h.loading || msg.days != h.days {
		return m
	}
	h.loading = false
	if msg.err != nil {
		h.err = Sanitize(msg.err.Error())
		return m
	}
	h.preview = msg.res
	if msg.res.Hidden == 0 && msg.res.DeletedJobs == 0 {
		h.done, h.page = msg.res, histResult
		return m
	}
	h.page = histAsk
	return m
}

func (m Model) onHistDone(msg histDoneMsg) Model {
	h := m.hist
	if h == nil {
		return m
	}
	h.loading = false
	if msg.err != nil {
		h.err = Sanitize(msg.err.Error())
		h.page = histScope
		return m
	}
	h.done, h.page = msg.res, histResult
	return m
}

func (m Model) histKey(key string) (tea.Model, tea.Cmd) {
	h := m.hist
	if h == nil {
		return m, nil
	}
	switch key {
	case "q", "ctrl+c":
		m.quits = true
		return m, tea.Quit
	}
	if h.loading { // one request at a time
		return m, nil
	}
	back := key == "left" || key == "esc" || key == "h"
	switch h.page {
	case histScope:
		switch {
		case key == "up" || key == "k":
			h.row = max(0, h.row-1)
		case key == "down" || key == "j":
			h.row = min(len(histChoices)-1, h.row+1)
		case key == "enter" || key == "right" || key == "l":
			h.days, h.err, h.loading = histChoices[h.row], "", true
			return m, m.histPreviewCmd(h.days)
		case back:
			m.screen, m.hist, m.cursor = screenSettings, nil, max(0, len(m.settingsRows())-1)
		}
	case histAsk:
		switch {
		case key == "y" || key == "Y" || key == "enter":
			h.loading = true
			return m, m.histApplyCmd(h.days)
		case key == "n" || key == "N" || back:
			h.page, h.err = histScope, ""
		}
	case histResult:
		if key == "enter" || back || key == " " {
			m.screen, m.hist, m.cursor = screenSettings, nil, max(0, len(m.settingsRows())-1)
			return m, m.fetch()
		}
	}
	return m, nil
}

func (m Model) histChoiceLabel(days int) string {
	if days == 0 {
		return m.t(i18n.HistAll)
	}
	return m.tf(i18n.HistOlder, days)
}

func (m Model) historyView() []string {
	h := m.hist
	if h == nil {
		return nil
	}
	lines := []string{"", m.bold(m.t(i18n.HistTitle)), ""}
	if h.err != "" {
		lines = append(lines, m.bad(h.err), "")
	}
	switch h.page {
	case histAsk:
		lines = append(lines, m.histChoiceLabel(h.days), "",
			m.tf(i18n.HistPreview, h.preview.Hidden, h.preview.DeletedJobs, h.preview.DeletedEvents),
			m.dim(m.t(i18n.HistKeep)), "")
		if h.loading {
			return append(lines, m.t(i18n.Loading))
		}
		return append(lines, m.warn(m.t(i18n.HistAskKeys)))
	case histResult:
		if h.done.DryRun || (h.done.Hidden == 0 && h.done.DeletedJobs == 0) {
			lines = append(lines, m.t(i18n.HistNothing))
		} else {
			lines = append(lines, m.good(m.tf(i18n.HistDone, h.done.Hidden)), m.dim(m.t(i18n.HistKeep)))
		}
		return append(lines, "", m.dim(m.t(i18n.HistDoneKeys)))
	}
	lines = append(lines, m.t(i18n.HistPick), "")
	for i, d := range histChoices {
		mark := "  "
		if i == h.row {
			mark = m.accent("> ")
		}
		lines = append(lines, mark+m.histChoiceLabel(d))
	}
	if h.loading {
		lines = append(lines, "", m.t(i18n.HistLoading))
	}
	return append(lines, "", m.dim(m.t(i18n.HistScopeKeys)))
}

// histClearJobs removes the given finished jobs from the history (Jobs
// screen, `x`). The daemon skips anything unfinished and says how many.
func (m Model) histClearJobs(ids []string) tea.Cmd {
	acts, ctx := m.acts, m.ctx
	return func() tea.Msg {
		res, err := acts.ClearHistory(ctx, -1, ids, false)
		if err != nil {
			return actionMsg{err: err}
		}
		return historyJobsMsg{res: res}
	}
}

type historyJobsMsg struct{ res HistoryClearResult }
