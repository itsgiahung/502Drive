package tray

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/itsgiahung/502Drive/internal/i18n"
)

// Action IDs are fixed: the tray never passes user text as an action.
const (
	ActionOpenTUI    = "open-tui"
	ActionPaste      = "paste"
	ActionOpenResult = "open-result"
	ActionDaemon     = "daemon"
	// ActionStopDaemon is the confirmed "Stop 502Drive…" item: a graceful
	// POST /v1/shutdown, distinct from ActionQuit (which only leaves the tray).
	ActionStopDaemon = "stop-daemon"
	ActionQuit       = "quit"
	// Job slots are informational menu rows, addressed by index.
	jobSlotID = "job"
	jobSlots  = 3
)

// Status is the subset of /v1/status the tray shows.
type Status struct {
	Version       string
	Ready         bool
	Running       int
	Queued        int
	Blocked       int
	AccountStatus string
	AccountLabel  string
}

// Job is one recent job shown in the tray submenu.
type Job struct {
	ID         string
	SourceName string
	State      string
	Completed  int
	Total      *int
	ResultURL  string
}

// Snapshot is what one poll produced.
type Snapshot struct {
	Status Status
	Jobs   []Job
	At     time.Time
}

// Item is one menu row. The structure is fixed (see BuildView); only titles,
// enabled and hidden flags change, so the tray can create the menu once.
type Item struct {
	ID        string
	Title     string
	Enabled   bool
	Hidden    bool
	Separator bool
	Children  []Item
}

// View is the rendered tray state for one poll.
type View struct {
	State      State
	Title      string
	StatusLine string
	Items      []Item
}

// Classify maps a snapshot to one of the five icon states.
func Classify(s Snapshot, online bool) State {
	if !online {
		return StateOffline
	}
	switch {
	case !s.Status.Ready:
		return StateError
	case s.Status.Blocked > 0:
		return StateNeeds
	case s.Status.AccountStatus != "" && s.Status.AccountStatus != "connected":
		return StateNeeds
	case s.Status.Running+s.Status.Queued > 0:
		return StateBusy
	}
	return StateIdle
}

// BuildView renders the menu in English (the CLI language). It always returns
// the same item IDs in the same order, which lets the systray layer update
// items instead of rebuilding them.
func BuildView(snap Snapshot, online bool, note string) View {
	return BuildViewLang(snap, online, note, "en")
}

// BuildViewLang renders the menu in the given language.
func BuildViewLang(snap Snapshot, online bool, note string, lang string) View {
	state := Classify(snap, online)
	status := snap.Status
	account := status.AccountStatus
	if account == "" {
		account = "unknown"
	}
	line := i18n.T(lang, i18n.OfflineLine)
	if online {
		line = i18n.Tf(lang, i18n.StatusLine, plain(account), status.Running, status.Queued, status.Blocked)
	}
	title := "502Drive · " + stateLabel(state, lang)
	if done, total, ok := runningProgress(snap.Jobs); ok && state == StateBusy {
		title = "502Drive · " + i18n.Tf(lang, i18n.WorkingNow, done, total)
	}
	if online && status.Version != "" {
		title += " v" + plain(status.Version)
	}
	if note != "" {
		title += " · " + plain(note)
	}
	result := latestResult(snap)
	items := []Item{
		{ID: "status", Title: line},
		{ID: ActionOpenTUI, Title: i18n.T(lang, i18n.OpenTUI), Enabled: true},
		{ID: ActionPaste, Title: i18n.T(lang, i18n.PasteLink), Enabled: online},
		{ID: "jobs", Title: i18n.T(lang, i18n.JobsLibrary), Enabled: true, Children: jobItems(snap, lang)},
		{ID: ActionOpenResult, Title: i18n.T(lang, i18n.OpenResult), Enabled: online && result != ""},
		{Separator: true},
		{ID: ActionDaemon, Title: i18n.T(lang, i18n.StartDaemon), Enabled: !online},
		{ID: ActionStopDaemon, Title: i18n.T(lang, i18n.StopApp), Enabled: online},
		{Separator: true},
		{ID: ActionQuit, Title: i18n.T(lang, i18n.Quit), Enabled: true},
	}
	return View{State: state, Title: title, StatusLine: line, Items: items}
}

// jobItems fills the fixed job slots with the last snapshot, which is kept
// while the daemon is unreachable.
func jobItems(snap Snapshot, lang string) []Item {
	items := make([]Item, 0, jobSlots+2)
	for i := range jobSlots {
		item := Item{ID: fmt.Sprintf("%s:%d", jobSlotID, i), Hidden: true}
		if i < len(snap.Jobs) {
			j := snap.Jobs[i]
			item.Title = jobLine(j, lang)
			item.Hidden = false
		}
		items = append(items, item)
	}
	items = append(items, Item{Separator: true})
	items = append(items, Item{ID: ActionOpenTUI + ":jobs", Title: i18n.T(lang, i18n.OpenTUI), Enabled: true})
	return items
}

// jobLine is one compact job row: state, name, progress. No fake ETA.
func jobLine(j Job, lang string) string {
	progress := fmt.Sprintf("%d", j.Completed)
	if j.Total != nil {
		progress = fmt.Sprintf("%d/%d", j.Completed, *j.Total)
	}
	return i18n.Tf(lang, i18n.JobLine, strings.ToUpper(stateWord(j.State, lang)), plain(truncate(j.SourceName, 40)), progress)
}

// stateWord names a job state in the requested language.
func stateWord(state, lang string) string {
	keys := map[string]i18n.Key{
		"queued": i18n.StateQueued, "running": i18n.StateRunning, "pausing": i18n.StatePaused,
		"paused": i18n.StatePaused, "cancelling": i18n.StateRunning, "reconciling": i18n.StateRunning,
		"blocked": i18n.StateNeeds, "succeeded": i18n.StateDone, "partial": i18n.StatePartial,
		"failed": i18n.StateFailed, "cancelled": i18n.StateFailed,
	}
	if key, ok := keys[state]; ok {
		return i18n.T(lang, key)
	}
	return state
}

func latestResult(snap Snapshot) string {
	for _, j := range snap.Jobs {
		if j.ResultURL != "" {
			return j.ResultURL
		}
	}
	return ""
}

func stateLabel(s State, lang string) string {
	switch s {
	case StateBusy:
		return i18n.T(lang, i18n.StateBusy)
	case StateNeeds:
		return i18n.T(lang, i18n.StateNeeds)
	case StateError:
		return i18n.T(lang, i18n.StateError)
	case StateOffline:
		return i18n.T(lang, i18n.StateOffline)
	}
	return i18n.T(lang, i18n.StateIdle)
}

// PollDelay is 3s while jobs run, 10s when idle and a bounded backoff while
// the daemon is down.
func PollDelay(online bool, state State, failures int) time.Duration {
	if !online {
		wait := 5 * time.Second << min(failures, 3)
		if wait > 30*time.Second {
			wait = 30 * time.Second
		}
		return wait
	}
	if state == StateBusy {
		return 3 * time.Second
	}
	return 10 * time.Second
}

// plain strips control characters and bidi overrides from text shown in a GUI
// label (job names come from Drive).
func plain(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case unicode.IsControl(r), r >= 0x202a && r <= 0x202e, r >= 0x2066 && r <= 0x2069, r == 0x200b, r == 0xfeff:
			continue
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// runningProgress sums the real counts of running jobs whose total is known;
// without any known total there is nothing honest to show.
func runningProgress(jobs []Job) (done, total int, ok bool) {
	for _, j := range jobs {
		if j.State != "running" || j.Total == nil {
			continue
		}
		done += j.Completed
		total += *j.Total
		ok = true
	}
	return done, total, ok
}
