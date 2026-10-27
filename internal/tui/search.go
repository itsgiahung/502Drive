package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/itsgiahung/502Drive/internal/i18n"
)

// Drive search: GET /v1/drive/search finds items by name (at most 20) in the
// account's Drive. The results take the place of a folder in the browser, so
// every key that works on a folder listing (info, copy, make destination, open
// a folder) works on them; Esc or ← goes back to where the search began.

const searchLimit = 20

type searchMsg struct {
	req   int
	query string
	items []DriveItem
	err   error
}

// openSearch shows the results for query; an empty query asks for the words
// first. It may be called from the copy dialog, which it replaces.
func (m Model) openSearch(query string) (Model, tea.Cmd) {
	if m.acts.Search == nil {
		return m, nil
	}
	m.cp = nil
	m.screen, m.cursor, m.top = screenDrive, 0, 0
	m.tabFocus = false
	d := m.drv
	if d == nil {
		d = &explorer{loaded: true}
		m.drv = d
	}
	if d.query == "" && d.loaded && len(d.page.Path) > 0 && d.before == nil {
		page := d.page
		d.before = &page
	}
	query = strings.TrimSpace(query)
	if query == "" {
		d.prompt, d.psearch = newInput(m.width, ""), true
		return m, textinput.Blink
	}
	return m, m.runSearch(query)
}

// runSearch asks the daemon; the answer comes back as a searchMsg.
func (m Model) runSearch(query string) tea.Cmd {
	d := m.drv
	d.req++
	d.query, d.err, d.loading = query, "", true
	d.prompt, d.psearch = nil, false
	req, acts, ctx := d.req, m.acts, m.ctx
	return tea.Batch(func() tea.Msg {
		items, err := acts.Search(ctx, query)
		return searchMsg{req: req, query: query, items: items, err: err}
	}, m.startSpinner())
}

func (m Model) onSearch(msg searchMsg) (tea.Model, tea.Cmd) {
	d := m.drv
	if d == nil || msg.req != d.req {
		return m, nil
	}
	d.loading = false
	if msg.err != nil {
		d.err = Sanitize(msg.err.Error())
		return m, nil
	}
	d.page = FolderPage{Folder: DriveItem{Name: msg.query}, Path: []DriveItem{{Name: msg.query}}, Items: msg.items}
	d.loaded = true
	m.cursor, m.top = 0, 0
	return m, nil
}

// leaveSearch returns to the folder the search began in, or to My Drive.
func (m Model) leaveSearch() (tea.Model, tea.Cmd) {
	d := m.drv
	if d == nil || d.query == "" {
		return m, nil
	}
	d.req++ // a late answer must not bring the results back
	d.query, d.loading, d.err = "", false, ""
	if d.before != nil {
		d.page, d.before = *d.before, nil
		m.cursor, m.top = 0, 0
		return m, nil
	}
	pick := d.pick
	next, cmd := m.openDrive(pick)
	return next, cmd
}

// searchLines is what the browser shows under the results: how many at most,
// or that nothing matched.
func (m Model) searchNote() []string {
	d := m.drv
	if d == nil || d.query == "" || d.loading {
		return nil
	}
	if len(d.page.Items) == 0 {
		return nil
	}
	if len(d.page.Items) >= searchLimit {
		return []string{m.dim(m.tf(i18n.SearchLimit, searchLimit))}
	}
	return nil
}
