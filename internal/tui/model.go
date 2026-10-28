package tui

import (
	"context"
	"github.com/charmbracelet/bubbles/textinput"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/muesli/termenv"

	"github.com/itsgiahung/502Drive/internal/i18n"
	"github.com/itsgiahung/502Drive/internal/mascot"
)

// screens of the TUI.
type screen int

const (
	screenHome screen = iota
	screenJobs
	screenJob
	screenLibrary
	screenCollection
	screenDestination
	screenAccount
	screenSettings
	screenHelp
	screenDrive
	screenCopy
	screenReport
	screenAudit
	screenFilter
	screenProtect
)

const (
	// pollConnected is only a safety net while the realtime stream is up.
	pollConnected = 8 * time.Second
	pollOffline   = 2 * time.Second
	// fetchDelay batches a burst of change events into one refresh.
	fetchDelay = 100 * time.Millisecond
	animFrame  = 70 * time.Millisecond
	spinFrame  = 250 * time.Millisecond
	greetFor   = 6 * time.Second
	happyFor   = 4 * time.Second
	sleepAfter = 2 * time.Minute
)

// Options control rendering; they mirror the CLI style.
type Options struct {
	JobID    string
	NoMotion bool
	ASCII    bool
	Color    bool
	Width    int
	Height   int
	Version  string
	// Language overrides the preference; empty means use the snapshot's
	// preference (default vi).
	Language string
	// Profile and Dark describe the terminal for the mascot.
	Profile termenv.Profile
	Dark    bool
	Now     func() time.Time
	// UpdateCheck returns the newer release tag, or "" when this build is
	// current. Nil turns the check off. It runs once, in the background.
	UpdateCheck func(context.Context) (string, error)
}

func (o Options) now() time.Time {
	if o.Now != nil {
		return o.Now()
	}
	return time.Now()
}

type confirmation struct {
	prompt string
	run    tea.Cmd
}

// Model is the whole TUI state. It is a value so Update can return copies and
// tests can drive it without a terminal. The maps and the animation are
// shared between copies on purpose: only Update touches them.
type Model struct {
	newVersion string // newer release tag found by Options.UpdateCheck
	acts       Actions
	opt        Options
	ctx        context.Context

	width, height int
	screen        screen
	focusedJob    bool
	cursor, top   int
	detailRow     int
	detailTop     int
	syncMenu      bool
	syncRow       int
	lang          string
	// tabFocus puts the cursor on the tab bar: ←/→ switch tabs, ↓ or Enter
	// goes into the page, ↑ at the top of a page comes back.
	tabFocus bool

	snap   Snapshot
	online bool
	status string // transient feedback from the last action

	selected map[string]bool
	confirm  *confirmation
	login    *loginState
	cmd      *textinput.Model // the Home command prompt
	cmdSel   int
	drv      *explorer
	cp       *copyFlow
	// Library: audits by collection (the map is shared between copies), the
	// cleanup in progress, and whether retired sources are listed.
	audits      map[string]*auditState
	clean       *cleanupFlow
	showRetired bool
	// Settings: the field being typed (quiet hours), if any.
	setInput *textinput.Model
	// Batch IT: the server-side job filter and its source name, the moves
	// made in the queue, batch progress, the newest audit per collection (the
	// maps are shared between copies), the problem report and the CSV path
	// being typed.
	statePick  *statePicker
	jq         JobQuery
	jqName     string
	qm         *queueMoves
	batches    map[string]*batchState
	lastAudits map[string]*lastAuditState
	rep        *reportView
	ab         *auditBrowser
	fe         *filterEditor
	prot       *protectFlow
	exp        *exportFlow

	watching bool // the realtime stream is connected
	watchCh  <-chan WatchEvent
	fetching bool
	dirty    bool

	rates      map[string]*rate
	prevStates map[string]string
	happyUntil time.Time
	started    time.Time
	lastInput  time.Time

	anim   *mascot.Anim
	frame  *mascot.Frame
	animOn bool
	spin   int
	sc     *spinCtl
	cache  *folderCache
	quits  bool
}

// New builds the initial model.
func New(acts Actions, opt Options) Model {
	width := opt.Width
	if width <= 0 {
		width = 80
	}
	lang := opt.Language
	if lang == "" {
		lang = "vi"
	}
	lang = i18n.Resolve(lang) // "auto" follows the machine's locale
	now := opt.now()
	m := Model{acts: acts, opt: opt, ctx: context.Background(), screen: screenHome, width: width, height: opt.Height,
		lang: lang, fetching: true, jq: JobQuery{Pages: 1}, qm: &queueMoves{}, batches: map[string]*batchState{}, lastAudits: map[string]*lastAuditState{},
		selected: map[string]bool{}, rates: map[string]*rate{}, prevStates: map[string]string{}, audits: map[string]*auditState{},
		started: now, lastInput: now, anim: mascot.NewMini(), cmd: newPrompt(), sc: &spinCtl{}, cache: newFolderCache()}
	m.frame = mascot.StaticMini(mascot.Idle, .3)
	return m
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.fetch(), pollTick(pollOffline), startWatch(m.acts, m.ctx)}
	if m.animating() {
		cmds = append(cmds, animTick())
	}
	if check := m.opt.UpdateCheck; check != nil {
		cmds = append(cmds, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			tag, _ := check(ctx)
			return updateMsg{tag: tag}
		})
	}
	return tea.Batch(cmds...)
}

// messages
type updateMsg struct{ tag string }
type snapshotMsg struct {
	snap Snapshot
	err  error
	// jq is the job filter the snapshot was read with; nil for a message the
	// model built itself.
	jq *JobQuery
}
type actionMsg struct {
	text string
	err  error
}
type bulkMsg struct {
	label               string
	ok, skipped, failed int
	firstErr            string
}
type loginStartedMsg struct {
	sess LoginSession
	err  error
}
type loginPollMsg struct{ id string }
type loginStateMsg struct {
	id, state string
	err       error
}
type watchStartedMsg struct{ ch <-chan WatchEvent }
type watchMsg struct {
	ev WatchEvent
	ok bool
}
type fetchNowMsg struct{}
type pollTickMsg time.Time
type animTickMsg time.Time
type spinTickMsg time.Time

func pollTick(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return pollTickMsg(t) })
}
func animTick() tea.Cmd {
	return tea.Tick(animFrame, func(t time.Time) tea.Msg { return animTickMsg(t) })
}
func spinTick() tea.Cmd {
	return tea.Tick(spinFrame, func(t time.Time) tea.Msg { return spinTickMsg(t) })
}

func startWatch(acts Actions, ctx context.Context) tea.Cmd {
	if acts.Watch == nil {
		return nil
	}
	return func() tea.Msg { return watchStartedMsg{ch: acts.Watch(ctx)} }
}

func waitWatch(ch <-chan WatchEvent) tea.Cmd {
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		ev, ok := <-ch
		return watchMsg{ev: ev, ok: ok}
	}
}

// fetch reads a snapshot in the background.
func (m Model) fetch() tea.Cmd {
	acts, ctx, q := m.acts, m.ctx, m.jq
	return func() tea.Msg {
		snap, err := acts.Snapshot(ctx)
		return snapshotMsg{snap: snap, err: err, jq: &q}
	}
}

// act runs one fixed action and refreshes afterwards.
func (m Model) act(fn func(context.Context) error) tea.Cmd {
	ctx := m.ctx
	return func() tea.Msg {
		if err := fn(ctx); err != nil {
			return actionMsg{err: err}
		}
		return actionMsg{text: "done"}
	}
}

func (m Model) pollEvery() time.Duration {
	if m.watching {
		return pollConnected
	}
	return pollOffline
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.fixTop()
		return m, nil
	case updateMsg:
		m.newVersion = msg.tag
		return m, nil
	case snapshotMsg:
		return m.onSnapshot(msg)
	case browseMsg:
		return m.onBrowse(msg)
	case mkdirMsg:
		return m.onMkdir(msg)
	case destMsg:
		return m.onDest(msg)
	case moveMsg:
		return m.onMove(msg)
	case queueMsg:
		return m.onQueue(msg)
	case batchMsg:
		return m.onBatch(msg)
	case batchSubmitMsg:
		return m.onBatchSubmit(msg)
	case deepTickMsg:
		return m.onDeepTick(msg)
	case deepMsg:
		return m.onDeep(msg)
	case reportMsg:
		return m.onReport(msg)
	case exportMsg:
		return m.onExport(msg)
	case searchMsg:
		return m.onSearch(msg)
	case lastAuditMsg:
		return m.onLastAudit(msg)
	case planMsg:
		return m.onPlan(msg)
	case submitMsg:
		return m.onSubmit(msg)
	case filterEditMsg:
		return m.onFilterEdit(msg)
	case auditPageMsg:
		return m.onFindings(msg)
	case protectLoadedMsg:
		return m.onProtectionLoaded(msg), nil
	case restorePreviewMsg:
		return m.onRestorePreview(msg), nil
	case auditMsg:
		return m.onAudit(msg)
	case repairMsg:
		return m.onRepair(msg)
	case cleanupMsg:
		return m.onCleanup(msg)
	case loginStartedMsg, loginPollMsg, loginStateMsg:
		return m.onLogin(msg)
	case watchStartedMsg:
		m.watchCh = msg.ch
		return m, waitWatch(msg.ch)
	case watchMsg:
		return m.onWatch(msg)
	case fetchNowMsg:
		return m, m.fetch()
	case pollTickMsg:
		if m.fetching {
			return m, pollTick(m.pollEvery())
		}
		m.fetching = true
		return m, tea.Batch(m.fetch(), pollTick(m.pollEvery()))
	case animTickMsg:
		return m.onAnim(time.Time(msg))
	case spinTickMsg:
		m.spin++
		if (m.busy() || m.loading()) && !m.opt.NoMotion {
			return m, spinTick()
		}
		m.sc.on = false
		return m, nil
	case actionMsg:
		if msg.err != nil {
			m.status = Sanitize(msg.err.Error())
		} else {
			m.status = msg.text
		}
		return m, m.refetch()
	case bulkMsg:
		m.status = m.bulkStatus(msg)
		m.selected = map[string]bool{}
		return m, m.refetch()
	case tea.MouseMsg:
		return m.afterNav(m.onMouse(msg))
	case tea.KeyMsg:
		m.lastInput = m.opt.now()
		return m.afterNav(m.key(msg))
	}
	if m.typing() { // cursor blink and paste messages reach the text field
		return m.forwardToInput(msg)
	}
	return m, nil
}

func (m Model) forwardToInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch {
	case m.screen == screenFilter && m.fe != nil:
		// The "keep empty folders" row is a switch with no text field behind it:
		// the blink and paste messages that reach here have nowhere to go.
		if m.fe.row < len(m.fe.fields) {
			m.fe.fields[m.fe.row], cmd = m.fe.fields[m.fe.row].Update(msg)
			m = m.validateFilter()
		}
	case m.screen == screenCopy && m.cp != nil && m.cp.input != nil:
		*m.cp.input, cmd = m.cp.input.Update(msg)
	case m.screen == screenDrive && m.drv != nil && m.drv.prompt != nil:
		*m.drv.prompt, cmd = m.drv.prompt.Update(msg)
	case m.screen == screenCollection && m.clean != nil && m.clean.input != nil:
		*m.clean.input, cmd = m.clean.input.Update(msg)
	case m.screen == screenSettings && m.setInput != nil:
		*m.setInput, cmd = m.setInput.Update(msg)
	case m.exp != nil:
		*m.exp.input, cmd = m.exp.input.Update(msg)
	}
	return m, cmd
}

// refetch starts a refresh unless one is already running.
func (m Model) refetch() tea.Cmd {
	if m.fetching {
		return nil
	}
	return m.fetch()
}

func (m Model) onSnapshot(msg snapshotMsg) (tea.Model, tea.Cmd) {
	m.fetching = false
	if msg.err == nil && msg.jq != nil && *msg.jq != m.jq {
		// The filter changed while this was being read: ask again.
		m.fetching = true
		return m, m.fetch()
	}
	if msg.err != nil {
		m.online = false
	} else {
		m.online = true
		jobID := ""
		collectionID := ""
		if m.screen == screenCollection {
			if c, ok := m.currentCollection(); ok {
				collectionID = c.ID
			}
		}
		if m.screen == screenJob {
			if j, ok := m.selectedJob(); ok {
				jobID = j.ID
			}
		}
		if !m.focusedJob && m.opt.JobID != "" {
			jobID = m.opt.JobID
		}
		m.observe(msg.snap)
		m.snap = msg.snap
		if collectionID != "" {
			m.cursor = -1
			for i, c := range m.snap.Collections {
				if c.ID == collectionID {
					m.cursor = i
					break
				}
			}
			if m.cursor < 0 {
				m.screen, m.syncMenu = screenLibrary, false
			}
		}
		if jobID != "" {
			found := false
			for i, j := range m.snap.Jobs {
				if j.ID == jobID {
					found = true
					m.cursor = i
					m.screen = screenJob
					m.focusedJob = true
					break
				}
			}
			if !found && m.screen == screenJob {
				m.screen, m.detailTop = screenJobs, 0
			}
		}
		if m.opt.Language == "" && msg.snap.Prefs.Language != "" {
			m.lang = i18n.Resolve(msg.snap.Prefs.Language)
		}
		m.pruneSelection()
		m.clampCursor()
		m.fixTop()
		m.cleanupProgress()
	}
	cmds := []tea.Cmd{}
	if msg.err == nil {
		if m.watching && m.cp != nil && m.cp.plan != nil {
			next, cmd := m.onDeepTick(deepTickMsg{planID: m.cp.plan.ID, changed: true})
			m = next.(Model)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		if cmd := m.pollAudits(); cmd != nil {
			cmds = append(cmds, cmd)
		}
		if cmd := m.pollBatches(); cmd != nil {
			cmds = append(cmds, cmd)
		}
		if cmd := m.pollLastAudits(); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	if m.dirty {
		m.dirty, m.fetching = false, true
		cmds = append(cmds, tea.Tick(fetchDelay, func(time.Time) tea.Msg { return fetchNowMsg{} }))
	}
	if cmd := m.startSpinner(); cmd != nil {
		cmds = append(cmds, cmd)
	}
	// A job that starts while the Jobs tab is open brings the cat to life.
	var kick tea.Cmd
	m, kick = m.startAnim()
	if kick != nil {
		cmds = append(cmds, kick)
	}
	return m, tea.Batch(cmds...)
}

func (m Model) onWatch(msg watchMsg) (tea.Model, tea.Cmd) {
	if !msg.ok {
		m.watching, m.watchCh = false, nil
		return m, nil
	}
	wasWatching := m.watching
	m.watching = msg.ev.Connected
	cmds := []tea.Cmd{waitWatch(m.watchCh)}
	if wasWatching && !m.watching && m.cp != nil && m.cp.plan != nil && m.cp.plan.Deep != nil && m.cp.plan.Deep.Running() {
		cmds = append(cmds, deepTick(m.cp.plan.ID))
	}
	if msg.ev.Changed {
		if m.fetching {
			m.dirty = true
		} else {
			m.fetching = true
			cmds = append(cmds, tea.Tick(fetchDelay, func(time.Time) tea.Msg { return fetchNowMsg{} }))
		}
	}
	return m, tea.Batch(cmds...)
}

func (m Model) startSpinner() tea.Cmd {
	if m.sc.on || !(m.busy() || m.loading()) || m.opt.NoMotion {
		return nil
	}
	m.sc.on = true
	return spinTick()
}

// spinner is kept for the tests: it reports whether a spinner tick would run.
func (m Model) spinner() tea.Cmd {
	if m.busy() && !m.opt.NoMotion {
		return spinTick()
	}
	return nil
}

func (m Model) busy() bool {
	return m.snap.Status.Running > 0 || m.snap.Status.Queued > 0
}

// animating reports whether the mascot should move right now.
func (m Model) animating() bool {
	if m.opt.NoMotion || !m.opt.Color || m.opt.ASCII {
		return false
	}
	return m.screen == screenHome || (m.screen == screenJobs && m.hasRunning())
}

func (m Model) onAnim(now time.Time) (tea.Model, tea.Cmd) {
	if !m.animating() {
		m.animOn = false
		return m, nil
	}
	m.frame = m.anim.Step(m.mood(m.opt.now()), animFrame.Seconds())
	return m, animTick()
}

// startAnim begins the animation chain once; leaving Home ends it.
func (m Model) startAnim() (Model, tea.Cmd) {
	if m.animOn || !m.animating() {
		return m, nil
	}
	m.animOn = true
	return m, animTick()
}

// mood maps daemon state to a mascot mood; the mascot never decides truth.
func (m Model) mood(now time.Time) mascot.Mood {
	switch {
	case !m.online:
		return mascot.Error
	case m.snap.Status.Blocked > 0 || (m.snap.Status.AccountStatus != "" && m.snap.Status.AccountStatus != "connected"):
		return mascot.Wait
	case m.snap.Status.Running > 0:
		return mascot.Busy
	case now.Before(m.happyUntil):
		return mascot.Happy
	case now.Sub(m.lastInput) > sleepAfter:
		return mascot.Sleep
	}
	return mascot.Idle
}

// key handles navigation and per-screen actions.
func (m Model) key(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.typing() {
		return m.typingKey(msg)
	}
	if m.screen == screenHome && m.confirm == nil {
		if next, cmd, handled := m.homeKey(msg); handled {
			return next, cmd
		}
	}
	if m.confirm != nil {
		c := m.confirm
		m.confirm = nil
		if s := msg.String(); s == "y" || s == "Y" {
			return m, c.run
		}
		m.status = ""
		return m, nil
	}
	if m.statePick != nil {
		return m.statePickerKey(msg.String())
	}
	if m.screen == screenAudit {
		return m.auditBrowserKey(msg.String())
	}
	if m.screen == screenProtect {
		return m.protectionKey(msg.String())
	}
	if m.screen == screenCopy { // the dialog owns 1/2/3, d and enter
		return m.copyScreenKey(msg)
	}
	if m.screen == screenCollection && !m.tabFocus {
		if next, cmd, handled := m.collectionNavigation(msg.String()); handled {
			return next, cmd
		}
		if next, cmd, handled := m.collectionKey(msg); handled {
			return next, cmd
		}
	}
	if m.tabFocus {
		switch msg.String() {
		case "down", "j", "enter", " ":
			m.tabFocus = false
			return m, nil
		case "up", "k":
			return m, nil
		case "h":
			return m.stepTab(-1)
		case "l":
			return m.stepTab(1)
		}
	}
	switch msg.Type {
	case tea.KeyTab:
		return m.moveTab(1)
	case tea.KeyShiftTab:
		return m.moveTab(-1)
	case tea.KeyRight:
		if m.arrowsSwitchTabs() {
			return m.stepTab(1)
		}
		if m.screen == screenJobs || m.screen == screenLibrary {
			return m.enter() // → goes into the selected row, like Enter
		}
	case tea.KeyLeft:
		if m.arrowsSwitchTabs() {
			return m.stepTab(-1)
		}
		if m.topList() && m.activeTab() >= 0 {
			m.tabFocus = true // ← steps out of the list, up to the tab bar
			return m, nil
		}
		if m.screen == screenJob || m.screen == screenCollection || m.screen == screenReport || m.screen == screenHelp {
			return m.back()
		}
	case tea.KeyCtrlC:
		m.quits = true
		return m, tea.Quit
	case tea.KeyEsc:
		if m.screen == screenDrive && m.drv != nil && m.drv.query != "" {
			return m.leaveSearch()
		}
		if m.screen == screenDrive && m.drv != nil && m.drv.pick {
			return m.cancelPick()
		}
		if m.login != nil {
			m.login = nil
			return m, nil
		}
		if len(m.selected) > 0 {
			m.selected = map[string]bool{}
			return m, nil
		}
		return m.back()
	case tea.KeySpace:
		if m.screen == screenSettings {
			return m.settingsStep(1)
		}
		return m.toggle(), nil
	}
	switch msg.String() {
	case "q":
		m.quits = true
		return m, tea.Quit
	case "?":
		return m.goTo(screenHelp)
	case "]":
		return m.moveTab(1)
	case "[":
		return m.moveTab(-1)
	case "1":
		return m.goTo(screenHome)
	case "2":
		return m.goTo(screenJobs)
	case "3":
		return m.goTo(screenLibrary)
	case "4":
		return m.goTo(screenDestination)
	case "5":
		return m.goTo(screenAccount)
	case "6":
		return m.goTo(screenSettings)
	case "7":
		return m.openDrive(false)
	case "up", "k":
		if m.screen == screenJob {
			m.detailTop = max(0, m.detailTop-1)
			return m, nil
		}
		if m.cursor == 0 && m.activeTab() >= 0 {
			m.tabFocus = true
			return m, nil
		}
		return m.move(-1), nil
	case "down", "j":
		if m.screen == screenJob {
			m.detailTop = min(max(0, len(m.jobView())-m.bodyRows()), m.detailTop+1)
			return m, nil
		}
		return m.afterMove(m.move(1))
	case "pgup":
		if m.screen == screenJob {
			m.detailTop = max(0, m.detailTop-m.bodyRows())
			return m, nil
		}
		return m.move(-m.pageSize()), nil
	case "pgdown":
		if m.screen == screenJob {
			m.detailTop = min(max(0, len(m.jobView())-m.bodyRows()), m.detailTop+m.bodyRows())
			return m, nil
		}
		return m.afterMove(m.move(m.pageSize()))
	case "home", "g":
		if m.screen == screenJob {
			m.detailTop = 0
			return m, nil
		}
		return m.move(-1 << 20), nil
	case "end", "G":
		if m.screen == screenJob {
			m.detailTop = max(0, len(m.jobView())-m.bodyRows())
			return m, nil
		}
		return m.afterMove(m.move(1 << 20))
	case "r":
		if m.screen == screenDrive {
			return m.driveKey("r")
		}
		return m, m.refetch()
	case "a":
		return m.selectAll(), nil
	case "enter":
		return m.enter()
	}
	return m.screenKey(msg.String())
}

func (m Model) goTo(s screen) (tea.Model, tea.Cmd) {
	m.screen, m.cursor, m.top = s, 0, 0
	m.detailRow, m.detailTop, m.syncRow, m.syncMenu = 0, 0, 0, false
	m.prot = nil
	m.tabFocus = false
	m.selected = map[string]bool{}
	return m.startAnim()
}

// afterMove loads the next page of whatever list the cursor just reached the
// end of: more jobs, or more of a Drive folder.
func (m Model) afterMove(next Model) (tea.Model, tea.Cmd) {
	if next.screen == screenJobs {
		return next.loadMoreJobs()
	}
	return next, next.maybeLoadMore()
}

// afterNav runs after every key or click: it leaves a job filter behind when
// the user goes to another tab and reads the newest audits on the Library.
func (m Model) afterNav(next tea.Model, cmd tea.Cmd) (tea.Model, tea.Cmd) {
	nm, ok := next.(Model)
	if !ok {
		return next, cmd
	}
	if nm.screen != screenJobs && nm.screen != screenJob && nm.screen != screenReport {
		var reset tea.Cmd
		nm, reset = nm.resetJobQuery()
		if reset != nil {
			cmd = tea.Batch(cmd, reset)
		}
	}
	if audit := nm.pollLastAudits(); audit != nil {
		cmd = tea.Batch(cmd, audit)
	}
	return nm, cmd
}

func (m Model) move(delta int) Model {
	m.cursor += delta
	m.clampCursor()
	m.fixTop()
	return m
}

func (m Model) pageSize() int {
	per, title := m.listShape()
	return max(1, m.listRows(per, title)-1)
}

func (m Model) back() (tea.Model, tea.Cmd) {
	switch m.screen {
	case screenJob:
		m.screen = screenJobs
	case screenCollection:
		m.screen = screenLibrary
		m.clean = nil
		m.syncMenu = false
	case screenProtect:
		if m.prot != nil {
			return m.leaveCollectionSubview(m.prot.collectionID), nil
		}
		m.screen = screenLibrary
	case screenReport:
		return m.leaveReport()
	case screenHome:
		m.quits = true
		return m, tea.Quit
	default:
		m.screen = screenHome
	}
	if m.screen != screenJobs && m.screen != screenLibrary {
		m.cursor, m.top = 0, 0
	}
	m.tabFocus = false
	m.detailTop = 0
	return m.startAnim()
}

func (m Model) enter() (tea.Model, tea.Cmd) {
	switch m.screen {
	case screenSettings:
		return m.settingsStep(1)
	case screenDrive:
		return m.driveKey("enter")
	case screenJobs:
		if _, ok := m.selectedJob(); ok {
			m.screen = screenJob
			m.detailTop = 0
			return m, nil
		}
	case screenLibrary:
		if _, ok := m.selectedCollection(); ok {
			m.screen = screenCollection
			m.detailRow, m.syncRow, m.syncMenu = 0, 0, false
			return m, nil
		}
	case screenDestination:
		if d, ok := m.selectedDestination(); ok && m.acts.SetDefaultDestination != nil {
			return m, m.act(func(ctx context.Context) error {
				return m.acts.SetDefaultDestination(ctx, d.ID, d.Revision)
			})
		}
	}
	return m, nil
}

// screenKey handles the action keys of each screen.
func (m Model) screenKey(key string) (tea.Model, tea.Cmd) {
	// The "sign in again" banner says "press l": it must work wherever the
	// banner shows, not only on Home and Account.
	if key == "l" && m.snap.Status.Lifecycle == "reconnect_required" && m.online {
		switch m.screen {
		case screenJobs, screenJob, screenLibrary, screenCollection, screenDestination:
			return m.startLogin()
		}
	}
	switch m.screen {
	case screenJobs:
		return m.jobsKey(key)
	case screenReport:
		if key == "E" {
			return m.startExport("report", m.rep.jobID)
		}
	case screenJob:
		j, ok := m.selectedJob()
		if ok && key == "R" {
			return m.openReport(j)
		}
		if ok && key == "E" {
			return m.startExport("report", j.ID)
		}
		if !ok || m.acts.JobAction == nil {
			return m, nil
		}
		actions := map[string]string{"p": "pause", "u": "resume", "c": "cancel", "t": "retry"}
		if action, found := actions[key]; found {
			if action == "retry" && m.waitingForQuota(j) {
				m.status = m.tf(i18n.QuotaNoRetry, m.when(j.ResumeAfter))
				return m, nil
			}
			return m, m.act(func(ctx context.Context) error {
				return m.acts.JobAction(ctx, j.ID, action, j.Revision)
			})
		}
	case screenLibrary:
		return m.libraryKey(key)
	case screenCollection:
		c, ok := m.selectedCollection()
		if !ok {
			return m, nil
		}
		presets := map[string]string{"d": presetDaily, "h": presetHourly, "e": presetSixHour, "t": presetRealtime}
		if preset, found := presets[key]; found && m.acts.SyncPreset != nil {
			return m, m.act(func(ctx context.Context) error {
				return m.acts.SyncPreset(ctx, c.ID, preset)
			})
		}
		syncActions := map[string]string{"n": "run", "p": "pause", "x": "stop"}
		if action, found := syncActions[key]; found {
			return m.syncAct(c, action)
		}
	case screenDrive:
		return m.driveKey(key)
	case screenCopy:
		return m.copyKey(key)
	case screenHome:
		if key == "l" {
			return m.startLogin()
		}
		if key == "c" {
			return m.startCopy("", screenHome)
		}
	case screenAccount:
		if key == "l" {
			return m.startLogin()
		}
		if key == "d" && m.acts.Doctor != nil {
			ctx := m.ctx
			return m, func() tea.Msg {
				checks, err := m.acts.Doctor(ctx)
				if err != nil {
					return actionMsg{err: err}
				}
				return snapshotMsg{snap: m.snap.withChecks(checks), err: nil}
			}
		}
	case screenSettings:
		switch key {
		case "l", "right", "tab":
			return m.settingsStep(1)
		case "h", "left":
			return m.settingsStep(-1)
		}
	}
	return m, nil
}

// syncAct maps the run/pause/stop keys, which use different API calls.
func (m Model) syncAct(c Collection, action string) (tea.Model, tea.Cmd) {
	if action == "run" {
		if m.acts.RunNow == nil {
			return m, nil
		}
		return m, m.act(func(ctx context.Context) error { return m.acts.RunNow(ctx, c.ID) })
	}
	if m.acts.SyncAction == nil {
		return m, nil
	}
	return m, m.act(func(ctx context.Context) error { return m.acts.SyncAction(ctx, c.ID, action) })
}

func (m Model) selectedJob() (Job, bool) {
	if m.cursor < 0 || m.cursor >= len(m.snap.Jobs) {
		return Job{}, false
	}
	return m.snap.Jobs[m.cursor], true
}

func (m Model) selectedCollection() (Collection, bool) {
	if m.cursor < 0 || m.cursor >= len(m.snap.Collections) {
		return Collection{}, false
	}
	return m.snap.Collections[m.cursor], true
}

func (m Model) selectedDestination() (Destination, bool) {
	if m.cursor < 0 || m.cursor >= len(m.snap.Destinations) {
		return Destination{}, false
	}
	return m.snap.Destinations[m.cursor], true
}

func (m Model) listLen() int {
	switch m.screen {
	case screenDrive:
		if m.drv != nil {
			return len(m.drv.page.Items)
		}
	case screenAudit:
		if m.ab != nil {
			return len(m.ab.page.Items)
		}
	case screenJobs, screenJob:
		return len(m.snap.Jobs)
	case screenLibrary, screenCollection, screenFilter:
		return len(m.snap.Collections)
	case screenDestination:
		return len(m.snap.Destinations)
	case screenSettings:
		return len(m.settingsRows())
	case screenHelp:
		return len(i18n.HelpLines(m.lang))
	case screenReport:
		if m.rep != nil && m.rep.rep != nil {
			return len(m.rep.rep.Items)
		}
	}
	return 0
}

func (m *Model) clampCursor() {
	limit := m.listLen()
	if limit == 0 {
		m.cursor = 0
		return
	}
	if m.cursor >= limit {
		m.cursor = limit - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

// listShape is how many text lines one row takes and how many lines the title
// block above the rows takes.
func (m Model) listShape() (per, title int) {
	switch m.screen {
	case screenAudit:
		return 2, 4
	case screenJobs:
		return 2, 2 + len(m.jobsBanner()) + len(m.lifeBanner()) + len(m.jobsHeadLines()) + m.jobsMoreLines() + len(m.exportLines())
	case screenReport:
		return 1, len(m.reportHead()) + len(m.exportLines())
	case screenLibrary:
		return 2, 2 + len(m.lifeBanner()) + len(m.libraryNote())
	case screenDestination:
		return 1, 2
	case screenHelp:
		return 1, 2
	case screenDrive:
		title := 2
		if m.drv != nil {
			if m.drv.info {
				title += 12
			}
			if m.drv.prompt != nil {
				title += 4
			}
			if m.drv.query != "" && len(m.drv.page.Items) >= searchLimit {
				title++
			}
			if m.drv.err != "" {
				title++
			}
		}
		return 1, title
	}
	return 1, 0
}

// bodyRows is the room between the header and the status/footer lines. Zero
// height means "unknown": nothing is windowed.
func (m Model) bodyRows() int {
	if m.height <= 0 {
		return 1 << 20
	}
	return max(3, m.height-3)
}

func (m Model) listRows(per, title int) int {
	return max(1, (m.bodyRows()-title)/per)
}

// fixTop keeps the cursor inside the visible window of a long list.
func (m *Model) fixTop() {
	per, title := m.listShape()
	rows := m.listRows(per, title)
	n := m.listLen()
	if m.cursor < m.top {
		m.top = m.cursor
	}
	if m.cursor >= m.top+rows {
		m.top = m.cursor - rows + 1
	}
	if m.top > n-rows {
		m.top = max(0, n-rows)
	}
	if m.top < 0 {
		m.top = 0
	}
}

// window returns the slice of rows to draw for the current list.
func (m Model) window() (start, end int) {
	per, title := m.listShape()
	rows := m.listRows(per, title)
	start = min(m.top, max(0, m.listLen()-1))
	end = min(m.listLen(), start+rows)
	return start, end
}

// withChecks keeps the snapshot while adding doctor results.
func (s Snapshot) withChecks(checks []Check) Snapshot {
	s.Checks = checks
	return s
}

// currentJob resolves the job of the detail screen from the live snapshot, so
// a refresh never shows stale counts.
func (m Model) currentJob() (Job, bool) {
	if m.screen != screenJob {
		return Job{}, false
	}
	return m.selectedJob()
}

// currentCollection resolves the collection of the detail screen.
func (m Model) currentCollection() (Collection, bool) {
	if m.screen != screenCollection {
		return Collection{}, false
	}
	return m.selectedCollection()
}

func stateWord(s string) string { return strings.ToUpper(s) }

// spinCtl is shared by every copy of the model: it says whether a spinner tick
// is already scheduled, so a burst of loads starts exactly one tick chain.
type spinCtl struct{ on bool }

// loading reports work the user is waiting for (a folder or a preview).
func (m Model) loading() bool {
	return (m.drv != nil && m.drv.loading) || (m.cp != nil && (m.cp.planning || m.cp.submitting))
}

// cycleLanguage steps English → Vietnamese → Auto (or back) at once: the
// screen changes immediately and the preference is saved in the background (a
// failed save is reported and the next refresh shows the truth). Auto follows
// the machine's locale.
func (m Model) cycleLanguage(step int) (tea.Model, tea.Cmd) {
	if m.acts.SetLanguage == nil {
		return m, nil
	}
	cur := m.effectiveLanguage()
	idx := -1
	for i, l := range langOrder {
		if l == cur {
			idx = i
		}
	}
	n := len(langOrder)
	next := langOrder[((idx+step)%n+n)%n]
	m.snap.Prefs.Language = next
	if m.opt.Language != "" {
		m.opt.Language = next
	}
	m.lang = i18n.Resolve(next)
	return m, m.act(func(ctx context.Context) error { return m.acts.SetLanguage(ctx, next) })
}

// effectiveLanguage is what the screens use: the --language override when set,
// else the saved preference, else the default.
func (m Model) effectiveLanguage() string {
	switch {
	case m.opt.Language != "":
		return m.opt.Language
	case m.snap.Prefs.Language != "":
		return m.snap.Prefs.Language
	}
	return m.lang
}
