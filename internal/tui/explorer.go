package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/itsgiahung/502Drive/internal/i18n"
)

// explorer is the Drive browser: breadcrumb, folders first, per-file metadata,
// new folders, "copy this" and "make this the destination". In pick mode it is
// the folder chooser of the copy dialog.
type explorer struct {
	pick    bool
	page    FolderPage
	loaded  bool
	loading bool
	err     string
	info    bool
	prompt  *textinput.Model // set while a new folder name is typed
	req     int
	focus   string // item to put the cursor on after the next load
}

type browseMsg struct {
	req  int
	page FolderPage
	more bool
	err  error
}

type mkdirMsg struct {
	item DriveItem
	err  error
}

type destMsg struct {
	dest        Destination
	err         error
	makeDefault bool
	forCopy     bool
}

func newInput(width int, placeholder string) *textinput.Model {
	ti := textinput.New()
	ti.Prompt = "› "
	ti.Placeholder = placeholder
	ti.CharLimit = 2048
	ti.Width = max(10, width-6)
	ti.Focus()
	return &ti
}

// openDrive switches to the browser at My Drive.
func (m Model) openDrive(pick bool) (Model, tea.Cmd) {
	if m.acts.Browse == nil {
		return m, nil
	}
	m.screen, m.cursor, m.top = screenDrive, 0, 0
	m.tabFocus = false
	m.selected = map[string]bool{}
	m.drv = &explorer{pick: pick}
	return m, m.loadFolder("root", "", false)
}

// folderCache keeps recently seen folders so opening one again is instant: the
// cached page is shown at once and a fresh one replaces it when it arrives.
type folderCache struct {
	pages map[string]cachedFolder
}

type cachedFolder struct {
	page FolderPage
	at   time.Time
}

const folderCacheTTL = 90 * time.Second

func newFolderCache() *folderCache { return &folderCache{pages: map[string]cachedFolder{}} }

func (m Model) loadFolder(id, token string, more bool) tea.Cmd {
	d := m.drv
	d.req++
	d.err = ""
	if c, ok := m.cache.pages[id]; ok && !more && m.opt.now().Sub(c.at) < folderCacheTTL {
		d.page, d.loaded = c.page, true
		m.placeCursor(d)
	}
	d.loading = true
	req, acts, ctx := d.req, m.acts, m.ctx
	fetch := func() tea.Msg {
		page, err := acts.Browse(ctx, id, token)
		return browseMsg{req: req, page: page, more: more, err: err}
	}
	return tea.Batch(fetch, m.startSpinner())
}

// placeCursor puts the cursor on the focus item (or the top) of the shown page.
func (m *Model) placeCursor(d *explorer) {
	m.cursor, m.top = 0, 0
	for i, it := range d.page.Items {
		if d.focus != "" && it.ID == d.focus {
			m.cursor = i
		}
	}
	m.fixTop()
}

func (m Model) onBrowse(msg browseMsg) (tea.Model, tea.Cmd) {
	d := m.drv
	if d == nil || msg.req != d.req {
		return m, nil
	}
	d.loading = false
	if msg.err != nil {
		d.err = Sanitize(msg.err.Error())
		return m, nil
	}
	if msg.more {
		d.page.Items = append(d.page.Items, msg.page.Items...)
		d.page.Next = msg.page.Next
		return m, nil
	}
	// A refresh of the folder already on screen keeps the cursor on the same item.
	keep := ""
	if d.loaded && d.page.Folder.ID == msg.page.Folder.ID && d.focus == "" {
		if it, ok := m.driveItem(); ok {
			keep = it.ID
		}
	}
	m.cache.pages[msg.page.Folder.ID] = cachedFolder{page: msg.page, at: m.opt.now()}
	d.page, d.loaded = msg.page, true
	if keep != "" {
		d.focus = keep
	}
	m.placeCursor(d)
	d.focus = ""
	return m, nil
}

func (m Model) driveItem() (DriveItem, bool) {
	if m.drv == nil || m.cursor < 0 || m.cursor >= len(m.drv.page.Items) {
		return DriveItem{}, false
	}
	return m.drv.page.Items[m.cursor], true
}

// driveKey handles the browser's keys. Keys that move the cursor are handled
// by the shared navigation; this covers the rest.
func (m Model) driveKey(key string) (tea.Model, tea.Cmd) {
	d := m.drv
	if d == nil {
		return m, nil
	}
	switch key {
	case "enter", "right", "l":
		if it, ok := m.driveItem(); ok {
			if it.IsFolder() {
				d.focus = ""
				return m, m.loadFolder(it.ID, "", false)
			}
			d.info = !d.info
		}
	case "left", "backspace", "h":
		if n := len(d.page.Path); n >= 2 {
			d.focus = d.page.Path[n-1].ID
			return m, m.loadFolder(d.page.Path[n-2].ID, "", false)
		}
	case "i":
		d.info = !d.info
		m.fixTop()
	case "r":
		return m, m.loadFolder(d.page.Folder.ID, "", false)
	case "n":
		if d.page.Folder.CanWrite {
			d.prompt = newInput(m.width, "")
			return m, textinput.Blink
		}
		m.status = i18n.T(m.lang, i18n.No) + ": " + i18n.T(m.lang, i18n.MetaWrite)
	case "y":
		if d.pick && d.page.Folder.ID != "" {
			return m, m.addDestination(d.page.Folder.ID, false, true)
		}
	case "d":
		if d.pick {
			return m, nil
		}
		id := d.page.Folder.ID
		if it, ok := m.driveItem(); ok && it.IsFolder() {
			id = it.ID
		}
		return m, m.addDestination(id, true, false)
	case "c":
		if d.pick {
			return m, nil
		}
		if it, ok := m.driveItem(); ok {
			return m.startCopy(it.ID, screenDrive)
		}
	}
	return m, nil
}

func (m Model) addDestination(folderID string, makeDefault, forCopy bool) tea.Cmd {
	acts, ctx := m.acts, m.ctx
	if acts.AddDestination == nil {
		return nil
	}
	return func() tea.Msg {
		dest, err := acts.AddDestination(ctx, folderID)
		if err == nil && makeDefault && acts.SetDefaultDestination != nil {
			err = acts.SetDefaultDestination(ctx, dest.ID, dest.Revision)
		}
		return destMsg{dest: dest, err: err, makeDefault: makeDefault, forCopy: forCopy}
	}
}

func (m Model) onDest(msg destMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.status = Sanitize(msg.err.Error())
		return m, nil
	}
	if msg.makeDefault {
		m.status = i18n.Tf(m.lang, i18n.DestSet, Sanitize(msg.dest.Name))
		return m, m.refetch()
	}
	if msg.forCopy && m.cp != nil {
		m.cp.destID, m.cp.destName = msg.dest.ID, msg.dest.Name
		m.screen = screenCopy
		m.drv = nil
		return m, m.replan()
	}
	return m, nil
}

func (m Model) onMkdir(msg mkdirMsg) (tea.Model, tea.Cmd) {
	if m.drv == nil {
		return m, nil
	}
	if msg.err != nil {
		m.drv.err = Sanitize(msg.err.Error())
		return m, nil
	}
	m.status = i18n.Tf(m.lang, i18n.FolderCreated, Sanitize(msg.item.Name))
	m.drv.focus = msg.item.ID
	return m, m.loadFolder(m.drv.page.Folder.ID, "", false)
}

// typing reports whether a text field has the keyboard.
func (m Model) typing() bool {
	switch {
	case m.drv != nil && m.drv.prompt != nil && m.screen == screenDrive:
		return true
	case m.cp != nil && m.cp.input != nil && m.screen == screenCopy:
		return true
	case m.screen == screenCollection && m.clean != nil && m.clean.input != nil:
		return true
	case m.screen == screenSettings && m.setInput != nil:
		return true
	}
	return false
}

// typingKey sends keys to the active text field; Enter and Esc finish it.
func (m Model) typingKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.screen == screenCopy {
		return m.copyTyping(msg)
	}
	if m.screen == screenCollection {
		return m.cleanupTyping(msg)
	}
	if m.screen == screenSettings {
		return m.settingsTyping(msg)
	}
	d := m.drv
	switch msg.Type {
	case tea.KeyEsc:
		d.prompt = nil
		return m, nil
	case tea.KeyEnter:
		name := strings.TrimSpace(d.prompt.Value())
		d.prompt = nil
		if name == "" || m.acts.Mkdir == nil {
			return m, nil
		}
		parent, acts, ctx := d.page.Folder.ID, m.acts, m.ctx
		return m, func() tea.Msg {
			item, err := acts.Mkdir(ctx, parent, name)
			return mkdirMsg{item: item, err: err}
		}
	}
	var cmd tea.Cmd
	*d.prompt, cmd = d.prompt.Update(msg)
	return m, cmd
}

// ---- view ----

func (m Model) driveView() []string {
	d := m.drv
	if d == nil {
		return nil
	}
	lines := []string{""}
	if d.pick {
		lines[0] = m.warn(m.t(i18n.PickHint))
	}
	crumb := m.breadcrumb()
	if d.loading && d.loaded && !m.opt.NoMotion {
		crumb += " " + m.accent(m.spinChar())
	}
	lines = append(lines, crumb)
	switch {
	case d.loading && !d.loaded:
		return append(lines, "", m.accent(m.spinChar())+" "+m.dim(m.t(i18n.Loading)))
	case d.err != "" && !d.loaded:
		return append(lines, "", m.bad(d.err))
	case len(d.page.Items) == 0:
		lines = append(lines, "", m.dim(m.t(i18n.DriveEmpty)))
	default:
		start, end := m.window()
		for i := start; i < end; i++ {
			lines = append(lines, m.driveRow(i, d.page.Items[i]))
		}
		if d.page.Next != "" && end == len(d.page.Items) {
			lines = append(lines, m.dim(m.t(i18n.MoreBelow)))
		}
	}
	if it, ok := m.driveItem(); ok && d.info {
		lines = append(lines, m.driveInfo(it)...)
	}
	if d.err != "" {
		lines = append(lines, m.bad(d.err))
	}
	if d.prompt != nil {
		lines = append(lines, "", m.accent(m.t(i18n.NewFolderPrompt)), d.prompt.View())
	}
	return lines
}

func (m Model) breadcrumb() string {
	names := make([]string, 0, len(m.drv.page.Path))
	for _, p := range m.drv.page.Path {
		names = append(names, Sanitize(p.Name))
	}
	sep := " › "
	if m.opt.ASCII {
		sep = " > "
	}
	crumb := strings.Join(names, sep)
	if over := visible(crumb) - (m.width - 2); over > 0 {
		r := []rune(crumb)
		crumb = "…" + string(r[min(len(r)-1, over+1):])
	}
	return m.bold(crumb)
}

func (m Model) driveRow(i int, it DriveItem) string {
	glyph := "·"
	switch {
	case it.IsFolder():
		glyph = "▸"
		if m.opt.ASCII {
			glyph = ">"
		}
	case it.Kind == "shortcut":
		glyph = "↪"
		if m.opt.ASCII {
			glyph = "@"
		}
	}
	cursor := " "
	if i == m.cursor {
		cursor = m.accent(">")
	}
	name := Sanitize(it.Name)
	right := ""
	if m.width >= 56 {
		right = fmt.Sprintf(" %9s %s", sizeText(it), dateText(it.Modified))
	}
	width := max(8, m.width-4-len(right))
	name = cut(name, width)
	if it.IsFolder() {
		name = m.accent(name + "/")
		width++
	}
	pad := strings.Repeat(" ", max(0, width-visible(name)))
	return cursor + " " + glyph + " " + name + pad + m.dim(right)
}

func sizeText(it DriveItem) string {
	if it.IsFolder() {
		return "—"
	}
	if it.Size == nil {
		if it.Kind == "google_native" {
			return "Google"
		}
		return "—"
	}
	return humanBytes(*it.Size)
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

func dateText(t time.Time) string {
	if t.IsZero() {
		return "          "
	}
	return t.Local().Format("2006-01-02")
}

func (m Model) driveInfo(it DriveItem) []string {
	yes := func(b bool) string {
		if b {
			return m.good(m.t(i18n.Yes))
		}
		return m.warn(m.t(i18n.No))
	}
	when := func(t time.Time) string {
		if t.IsZero() {
			return "—"
		}
		return t.Local().Format("2006-01-02 15:04")
	}
	lines := []string{"", m.bold(Sanitize(it.Name)),
		m.kv(m.t(i18n.MetaType), Sanitize(orDash(it.Mime))),
		m.kv(m.t(i18n.MetaSize), sizeText(it)),
		m.kv(m.t(i18n.MetaModified), when(it.Modified)),
		m.kv(m.t(i18n.MetaCreated), when(it.Created)),
		m.kv(m.t(i18n.MetaOwner), Sanitize(orDash(it.Owner))),
		m.kv(m.t(i18n.MetaCopy), yes(it.CanCopy)),
		m.kv(m.t(i18n.MetaWrite), yes(it.CanWrite)),
		m.kv(m.t(i18n.MetaID), Sanitize(it.ID)),
	}
	if it.WebURL != "" {
		lines = append(lines, m.kv(m.t(i18n.MetaLink), Sanitize(it.WebURL)))
	}
	return lines
}

// cancelPick leaves the destination chooser and returns to the copy dialog.
func (m Model) cancelPick() (tea.Model, tea.Cmd) {
	m.drv = nil
	if m.cp != nil {
		m.screen = screenCopy
		return m, nil
	}
	return m.goTo(screenHome)
}

// maybeLoadMore fetches the next page when the cursor reaches the last row.
func (m Model) maybeLoadMore() tea.Cmd {
	d := m.drv
	if m.screen != screenDrive || d == nil || d.loading || d.page.Next == "" || m.cursor < len(d.page.Items)-1 {
		return nil
	}
	return m.loadFolder(d.page.Folder.ID, d.page.Next, true)
}
