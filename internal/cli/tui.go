package cli

import (
	"context"
	"net/url"
	"sync"
	"time"

	"github.com/GiaHung07/502Drive/internal/schedule"
	"github.com/GiaHung07/502Drive/internal/tui"
)

// tuiActions adapts the CLI HTTP client to the TUI. The TUI never builds a
// request itself; it only names fixed actions.
func (a *App) tuiActions() tui.Actions {
	return tui.Actions{
		Snapshot:              a.tuiSnapshot,
		JobAction:             a.tuiJobAction,
		SyncPreset:            a.tuiSyncPreset,
		SyncAction:            a.tuiSyncAction,
		RunNow:                a.tuiRunNow,
		SetDefaultDestination: a.tuiSetDefaultDestination,
		Doctor:                a.tuiDoctor,
		SetLanguage:           a.tuiSetLanguage,
		Watch:                 a.tuiWatch,
	}
}

// tuiWatch adapts the client's stream to the TUI's event type.
func (a *App) tuiWatch(ctx context.Context) <-chan tui.WatchEvent {
	c, err := a.api()
	if err != nil {
		return nil
	}
	src := c.Watch(ctx)
	out := make(chan tui.WatchEvent, 8)
	go func() {
		defer close(out)
		for ev := range src {
			select {
			case out <- tui.WatchEvent{Connected: ev.Connected, Changed: ev.Changed}:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

// tuiSnapshot reads everything the screens show. Only /v1/status decides
// "offline"; a secondary read failure leaves that section empty.
func (a *App) tuiSnapshot(ctx context.Context) (tui.Snapshot, error) {
	var snap tui.Snapshot
	r, err := a.call(ctx, "GET", "/v1/status", nil, nil, "")
	if err != nil {
		return snap, err
	}
	var st struct {
		Version string `json:"version"`
		Ready   bool   `json:"ready"`
		Running int    `json:"running_jobs"`
		Queued  int    `json:"queued_jobs"`
		Blocked int    `json:"blocked_jobs"`
		Bot     string `json:"bot_state"`
		Account struct {
			Status string `json:"status"`
		} `json:"account"`
	}
	if err = r.Decode(&st); err != nil {
		return snap, err
	}
	snap.Status = tui.Status{Version: st.Version, Ready: st.Ready, Running: st.Running, Queued: st.Queued,
		Blocked: st.Blocked, AccountStatus: st.Account.Status, BotState: st.Bot}
	a.tuiJobs(ctx, &snap)
	a.tuiCollections(ctx, &snap)
	a.tuiSlowSections(ctx, &snap)
	snap.At = time.Now()
	return snap, nil
}

// slowCache holds the sections that rarely change (destinations, account,
// preferences). A realtime refresh happens many times a second during a copy;
// re-reading these every time would triple the requests for nothing.
type slowCache struct {
	mu   sync.Mutex
	at   time.Time
	dest []tui.Destination
	acc  tui.Account
	pref tui.Prefs
}

const slowTTL = 30 * time.Second

func (a *App) tuiSlowSections(ctx context.Context, snap *tui.Snapshot) {
	c := &a.slow
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Since(c.at) > slowTTL {
		var fresh tui.Snapshot
		a.tuiDestinations(ctx, &fresh)
		a.tuiAccount(ctx, &fresh)
		a.tuiPrefs(ctx, &fresh)
		c.dest, c.acc, c.pref, c.at = fresh.Destinations, fresh.Account, fresh.Prefs, time.Now()
	}
	snap.Destinations, snap.Account, snap.Prefs = c.dest, c.acc, c.pref
}

// forgetSlow makes the next snapshot re-read the slow sections after a change
// that affects them.
func (a *App) forgetSlow() {
	a.slow.mu.Lock()
	a.slow.at = time.Time{}
	a.slow.mu.Unlock()
}

func (a *App) tuiJobs(ctx context.Context, snap *tui.Snapshot) {
	r, err := a.call(ctx, "GET", "/v1/jobs", url.Values{"limit": {"20"}}, nil, "")
	if err != nil {
		return
	}
	var page struct {
		Items []jobView `json:"items"`
	}
	if r.Decode(&page) != nil {
		return
	}
	for _, j := range page.Items {
		job := tui.Job{ID: j.ID, CollectionID: j.CollectionID, SourceName: j.SourceName, State: j.State,
			Desired: j.Desired, Revision: j.Revision, Completed: j.Counts.Completed, Failed: j.Counts.Failed,
			Blocked: j.Counts.Blocked, Skipped: j.Counts.Skipped, Total: j.Counts.Total}
		if j.Phase != nil {
			job.Phase = *j.Phase
		}
		if j.ResultURL != nil {
			job.ResultURL = *j.ResultURL
		}
		if j.LastError != nil {
			job.LastError = j.LastError.Message
		}
		snap.Jobs = append(snap.Jobs, job)
	}
}

func (a *App) tuiCollections(ctx context.Context, snap *tui.Snapshot) {
	r, err := a.call(ctx, "GET", "/v1/collections", nil, nil, "")
	if err != nil {
		return
	}
	var page struct {
		Items []collectionView `json:"items"`
	}
	if r.Decode(&page) != nil {
		return
	}
	for _, c := range page.Items {
		coll := tui.Collection{ID: c.ID, SourceName: c.SourceName, State: c.State, Revision: c.Revision}
		if c.ResultURL != nil {
			coll.ResultURL = *c.ResultURL
		}
		if c.Sync != nil {
			sync := &tui.SyncState{Mode: c.Sync.Mode, State: c.Sync.State, Revision: c.Sync.Revision,
				Schedule: c.Sync.Schedule.Describe()}
			if c.Sync.NextRunAt != nil {
				sync.NextRun = *c.Sync.NextRunAt
			}
			if c.Sync.LastSuccessAt != nil {
				sync.LastSuccess = *c.Sync.LastSuccessAt
			}
			if c.Sync.ActiveJobID != nil {
				sync.ActiveJob = *c.Sync.ActiveJobID
			}
			if c.Sync.LastError != nil {
				sync.LastError = c.Sync.LastError.Message
			}
			coll.Sync = sync
		}
		snap.Collections = append(snap.Collections, coll)
	}
}

func (a *App) tuiDestinations(ctx context.Context, snap *tui.Snapshot) {
	r, err := a.call(ctx, "GET", "/v1/destinations", nil, nil, "")
	if err != nil {
		return
	}
	var page struct {
		Items []destView `json:"items"`
	}
	if r.Decode(&page) != nil {
		return
	}
	for _, d := range page.Items {
		snap.Destinations = append(snap.Destinations, tui.Destination{ID: d.ID, Name: d.Name, Revision: d.Revision, Default: d.Default})
	}
}

func (a *App) tuiAccount(ctx context.Context, snap *tui.Snapshot) {
	r, err := a.call(ctx, "GET", "/v1/account", nil, nil, "")
	if err != nil {
		return
	}
	var acc struct {
		Status    string   `json:"status"`
		Label     *string  `json:"label"`
		LastError *failure `json:"last_error"`
	}
	if r.Decode(&acc) != nil {
		return
	}
	snap.Account = tui.Account{Status: acc.Status}
	if acc.Label != nil {
		snap.Account.Label = *acc.Label
	}
	if acc.LastError != nil {
		snap.Account.Problem = acc.LastError.Message
	}
}

func (a *App) tuiPrefs(ctx context.Context, snap *tui.Snapshot) {
	r, err := a.call(ctx, "GET", "/v1/preferences", nil, nil, "")
	if err != nil {
		return
	}
	var prefs struct {
		Language string `json:"language"`
		Timezone string `json:"timezone"`
	}
	if r.Decode(&prefs) != nil {
		return
	}
	snap.Prefs = tui.Prefs{Language: prefs.Language, Timezone: prefs.Timezone}
}

// tuiJobAction uses the revision the user saw; a stale click is reported and
// the next refresh shows the truth.
func (a *App) tuiJobAction(ctx context.Context, id, action string, revision int) error {
	_, err := a.call(ctx, "POST", "/v1/jobs/"+url.PathEscape(id)+"/"+action, nil,
		map[string]any{"expected_revision": revision}, NewKey())
	return err
}

// tuiSyncPreset maps the TUI preset keys to structured schedules.
func (a *App) tuiSyncPreset(ctx context.Context, collectionID, preset string) error {
	var (
		mode string
		spec *schedule.Spec
	)
	tz := a.preferredTimezone(ctx)
	switch preset {
	case "daily":
		mode = "scheduled"
		spec = &schedule.Spec{Kind: schedule.KindDaily, Timezone: tz, LocalTime: "22:00"}
	case "hourly":
		mode = "scheduled"
		spec = &schedule.Spec{Kind: schedule.KindInterval, IntervalMinutes: 60, AnchorAt: time.Now().UTC()}
	case "sixhourly":
		mode = "scheduled"
		spec = &schedule.Spec{Kind: schedule.KindInterval, IntervalMinutes: 360, AnchorAt: time.Now().UTC()}
	case "realtime":
		mode = "realtime"
	default:
		return localErr(ExitUsage, "invalid_request", "unknown sync preset")
	}
	_, err := a.enableSync(ctx, collectionID, mode, "versioned_copy", spec)
	return err
}

func (a *App) tuiSyncAction(ctx context.Context, collectionID, action string) error {
	_, err := a.syncActionRequest(ctx, collectionID, action)
	return err
}

func (a *App) tuiRunNow(ctx context.Context, collectionID string) error {
	_, err := a.runSync(ctx, collectionID)
	return err
}

func (a *App) tuiSetDefaultDestination(ctx context.Context, id string, revision int) error {
	_, err := a.call(ctx, "PUT", "/v1/destinations/default", nil,
		map[string]any{"destination_id": id, "expected_revision": revision}, NewKey())
	a.forgetSlow()
	return err
}

func (a *App) tuiDoctor(ctx context.Context) ([]tui.Check, error) {
	r, err := a.call(ctx, "GET", "/v1/doctor", url.Values{"online": {"false"}}, nil, "")
	if err != nil {
		return nil, err
	}
	var body struct {
		Checks []struct{ Name, Status, Message string } `json:"checks"`
	}
	if err = r.Decode(&body); err != nil {
		return nil, err
	}
	out := make([]tui.Check, 0, len(body.Checks))
	for _, c := range body.Checks {
		out = append(out, tui.Check{Name: c.Name, Status: c.Status, Message: c.Message})
	}
	return out, nil
}

func (a *App) tuiSetLanguage(ctx context.Context, lang string) error {
	_, err := a.call(ctx, "PATCH", "/v1/preferences", nil, map[string]any{"language": lang}, NewKey())
	a.forgetSlow()
	return err
}
