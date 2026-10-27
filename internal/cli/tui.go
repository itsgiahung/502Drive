package cli

import (
	"context"
	"net/url"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/itsgiahung/502Drive/internal/present"
	"github.com/itsgiahung/502Drive/internal/schedule"
	"github.com/itsgiahung/502Drive/internal/tui"
)

// tuiActions adapts the CLI HTTP client to the TUI. The TUI never builds a
// request itself; it only names fixed actions.
func (a *App) tuiActions() tui.Actions {
	return tui.Actions{
		Snapshot:                a.tuiSnapshot,
		JobAction:               a.tuiJobAction,
		SyncPreset:              a.tuiSyncPreset,
		SyncAction:              a.tuiSyncAction,
		RunNow:                  a.tuiRunNow,
		SetDefaultDestination:   a.tuiSetDefaultDestination,
		Doctor:                  a.tuiDoctor,
		SetLanguage:             a.tuiSetLanguage,
		Browse:                  a.tuiBrowse,
		Mkdir:                   a.tuiMkdir,
		Inspect:                 a.tuiInspect,
		Submit:                  a.tuiSubmit,
		AddDestination:          a.tuiAddDestination,
		Login:                   a.tuiLogin,
		LoginState:              a.tuiLoginState,
		OpenURL:                 a.tuiOpenURL,
		Watch:                   a.tuiWatch,
		InspectWith:             a.tuiInspectWith,
		SubmitMode:              a.tuiSubmitMode,
		StartAudit:              a.tuiStartAudit,
		GetAudit:                a.tuiGetAudit,
		AuditFindings:           a.tuiAuditFindings,
		GetJob:                  a.tuiGetJob,
		RepairAudit:             a.tuiRepairAudit,
		CollectionLifecycle:     a.tuiCollectionLifecycle,
		Cleanup:                 a.tuiCleanup,
		SetFilterPreset:         a.tuiSetFilterPreset,
		SetCollectionFilter:     a.tuiSetCollectionFilter,
		PreviewCollectionFilter: a.tuiPreviewCollectionFilter,
		ShowRetired:             func(on bool) { a.showRetired.Store(on) },
		SetPreference:           a.tuiSetPreference,
		SetAutostart:            a.tuiSetAutostart,
		SubmitOpts:              a.tuiSubmitOpts,
		SubmitBatch:             a.tuiSubmitBatch,
		GetPlan:                 a.tuiGetPlan,
		CancelPreflight:         a.tuiCancelPreflight,
		GetBatch:                a.tuiGetBatch,
		QueuePause:              a.tuiQueuePause,
		MoveJob:                 a.tuiMoveJob,
		SetJobQuery:             a.tuiSetJobQuery,
		Report:                  a.tuiReport,
		ExportCSV:               a.tuiExportCSV,
		Search:                  a.tuiSearch,
		LatestAudit:             a.tuiLatestAudit,
	}
}

// driveFileView is the JSON shape of one Drive item (see /v1/drive/folders).
type driveFileView struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Kind       string  `json:"kind"`
	Mime       string  `json:"mime_type"`
	Size       *int64  `json:"size_bytes"`
	CreatedAt  *string `json:"created_at"`
	ModifiedAt *string `json:"modified_at"`
	Owner      *struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"owner"`
	WebURL   string `json:"web_url"`
	CanCopy  bool   `json:"can_copy"`
	CanWrite bool   `json:"can_add_children"`
}

func (f driveFileView) item() tui.DriveItem {
	it := tui.DriveItem{ID: f.ID, Name: f.Name, Kind: f.Kind, Mime: f.Mime, Size: f.Size, WebURL: f.WebURL, CanCopy: f.CanCopy, CanWrite: f.CanWrite}
	it.Created = parseTime(f.CreatedAt)
	it.Modified = parseTime(f.ModifiedAt)
	if f.Owner != nil {
		it.Owner = f.Owner.Name
		if f.Owner.Email != "" {
			it.Owner = strings.TrimSpace(f.Owner.Name + " <" + f.Owner.Email + ">")
		}
	}
	return it
}

func parseTime(s *string) time.Time {
	if s == nil {
		return time.Time{}
	}
	t, _ := time.Parse(time.RFC3339Nano, *s)
	return t
}

func (a *App) tuiBrowse(ctx context.Context, folderID, pageToken string) (tui.FolderPage, error) {
	var out tui.FolderPage
	q := url.Values{}
	if pageToken != "" {
		q.Set("page_token", pageToken)
	}
	r, err := a.call(ctx, "GET", "/v1/drive/folders/"+url.PathEscape(folderID), q, nil, "")
	if err != nil {
		return out, err
	}
	var body struct {
		Folder driveFileView   `json:"folder"`
		Path   []driveFileView `json:"path"`
		Items  []driveFileView `json:"items"`
		Next   *string         `json:"next_page_token"`
	}
	if err = r.Decode(&body); err != nil {
		return out, err
	}
	out.Folder = body.Folder.item()
	for _, p := range body.Path {
		out.Path = append(out.Path, tui.DriveItem{ID: p.ID, Name: p.Name})
	}
	for _, it := range body.Items {
		out.Items = append(out.Items, it.item())
	}
	if body.Next != nil {
		out.Next = *body.Next
	}
	return out, nil
}

func (a *App) tuiMkdir(ctx context.Context, parentID, name string) (tui.DriveItem, error) {
	r, err := a.call(ctx, "POST", "/v1/drive/folders", nil, map[string]any{"parent_id": parentID, "name": name}, NewKey())
	if err != nil {
		return tui.DriveItem{}, err
	}
	var f driveFileView
	if err = r.Decode(&f); err != nil {
		return tui.DriveItem{}, err
	}
	return f.item(), nil
}

// tuiInspect is the read-only preview of a copy; each call is a fresh plan.
func (a *App) tuiInspect(ctx context.Context, source, destinationID, rootName string) (tui.Plan, error) {
	plan, _, err := a.makePlan(ctx, source, destinationID, rootName, NewKey(), planOpts{})
	if err != nil {
		return tui.Plan{}, err
	}
	return tui.Plan{ID: plan.ID, Revision: plan.Revision, SourceID: plan.SourceID, SourceName: plan.SourceName, SourceKind: plan.SourceKind,
		DestinationID: plan.Destination, RootName: plan.RootName, Files: plan.Preview.Files, Folders: plan.Preview.Folders,
		Complete: plan.Preview.Complete, Warnings: plan.Warnings}, nil
}

func (a *App) tuiSubmit(ctx context.Context, plan tui.Plan) (string, string, error) {
	r, err := a.call(ctx, "POST", "/v1/jobs", nil, map[string]any{"plan_id": plan.ID, "expected_revision": plan.Revision}, NewKey())
	if err != nil {
		return "", "", err
	}
	var job jobView
	if err = r.Decode(&job); err != nil {
		return "", "", err
	}
	return job.ID, job.CollectionID, nil
}

// tuiAddDestination reuses the destination that already points at the folder,
// so choosing the same folder twice does not create duplicates.
func (a *App) tuiAddDestination(ctx context.Context, folderID string) (tui.Destination, error) {
	if r, err := a.call(ctx, "GET", "/v1/destinations", nil, nil, ""); err == nil {
		var page struct {
			Items []struct {
				ID       string `json:"id"`
				Name     string `json:"name"`
				FolderID string `json:"folder_id"`
				Revision int    `json:"revision"`
				Default  bool   `json:"is_default"`
			} `json:"items"`
		}
		if r.Decode(&page) == nil {
			for _, d := range page.Items {
				if d.FolderID == folderID {
					return tui.Destination{ID: d.ID, Name: d.Name, Revision: d.Revision, Default: d.Default}, nil
				}
			}
		}
	}
	r, err := a.call(ctx, "POST", "/v1/destinations", nil, map[string]any{"source": folderID}, NewKey())
	if err != nil {
		return tui.Destination{}, err
	}
	var d destView
	if err = r.Decode(&d); err != nil {
		return tui.Destination{}, err
	}
	a.forgetSlow()
	return tui.Destination{ID: d.ID, Name: d.Name, Revision: d.Revision, Default: d.Default}, nil
}

// tuiLogin starts a Google sign-in; the TUI opens the browser itself.
func (a *App) tuiLogin(ctx context.Context) (tui.LoginSession, error) {
	var out tui.LoginSession
	r, err := a.call(ctx, "POST", "/v1/account/auth-sessions", nil, map[string]any{"no_browser": true}, NewKey())
	if err != nil {
		return out, err
	}
	var sess struct {
		ID  string  `json:"id"`
		URL *string `json:"authorization_url"`
	}
	if err = r.Decode(&sess); err != nil || sess.URL == nil {
		return out, localErr(ExitInternal, "bad_response", "No authorization URL was returned.")
	}
	return tui.LoginSession{ID: sess.ID, URL: *sess.URL}, nil
}

func (a *App) tuiLoginState(ctx context.Context, id string) (string, error) {
	r, err := a.call(ctx, "GET", "/v1/account/auth-sessions/"+url.PathEscape(id), nil, nil, "")
	if err != nil {
		return "", err
	}
	var st struct {
		State string `json:"state"`
	}
	if err = r.Decode(&st); err != nil {
		return "", err
	}
	return st.State, nil
}

// tuiOpenURL opens the sign-in page when this machine has a desktop; over SSH
// it does nothing and the user opens the shown link by hand.
func (a *App) tuiOpenURL(u string) error {
	if runtime.GOOS == "linux" && a.env("DISPLAY") == "" && a.env("WAYLAND_DISPLAY") == "" {
		return nil
	}
	return openBrowser(u)
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
		Quota    *present.QuotaStatus `json:"quota"`
		Version  string               `json:"version"`
		Ready    bool                 `json:"ready"`
		Running  int                  `json:"running_jobs"`
		Queued   int                  `json:"queued_jobs"`
		Blocked  int                  `json:"blocked_jobs"`
		Bot      string               `json:"bot_state"`
		Throttle struct {
			Workers   int        `json:"workers_now"`
			Throttled int        `json:"throttled_last_5m"`
			RetryAt   *time.Time `json:"next_retry_at"`
		} `json:"throttle"`
		Lifecycle struct {
			State  string `json:"state"`
			Online bool   `json:"online"`
			Gap    int    `json:"last_gap_seconds"`
		} `json:"lifecycle"`
		Account struct {
			Status string `json:"status"`
		} `json:"account"`
		Queue struct {
			Paused bool `json:"paused"`
		} `json:"queue"`
	}
	if err = r.Decode(&st); err != nil {
		return snap, err
	}
	snap.Status = tui.Status{Quota: st.Quota, Version: st.Version, Ready: st.Ready, Running: st.Running, Queued: st.Queued,
		Blocked: st.Blocked, AccountStatus: st.Account.Status, BotState: st.Bot,
		Workers: st.Throttle.Workers, Throttled5m: st.Throttle.Throttled,
		Lifecycle: st.Lifecycle.State, LifecycleOnline: st.Lifecycle.Online, GapSeconds: st.Lifecycle.Gap,
		QueuePaused: st.Queue.Paused}
	if st.Throttle.RetryAt != nil {
		snap.Status.RetryAt = *st.Throttle.RetryAt
	}
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
	mu    sync.Mutex
	at    time.Time
	dest  []tui.Destination
	acc   tui.Account
	pref  tui.Prefs
	setup *tui.SetupState
	// jq is the Jobs filter the TUI asked for; the next snapshots read it.
	jq tui.JobQuery
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
		if setup, err := a.setupChecklist(ctx); err == nil {
			c.setup = &tui.SetupState{Ready: setup.Ready}
			for _, ch := range setup.Checks {
				c.setup.Checks = append(c.setup.Checks, tui.Check{Name: ch.Name, Status: ch.Status})
			}
		}
		c.dest, c.acc, c.pref, c.at = fresh.Destinations, fresh.Account, fresh.Prefs, time.Now()
	}
	snap.Destinations, snap.Account, snap.Prefs = c.dest, c.acc, c.pref
	snap.Setup = c.setup
}

// forgetSlow makes the next snapshot re-read the slow sections after a change
// that affects them.
func (a *App) forgetSlow() {
	a.slow.mu.Lock()
	a.slow.at = time.Time{}
	a.slow.mu.Unlock()
}

// maxJobPages bounds how far "load more" reads (20 jobs a page).
const maxJobPages = 10

// tuiJobs reads the Jobs list with the server-side filter the TUI chose
// (state, collection) and as many cursor pages as it asked for. The cursor of
// the page after the last one read goes to the snapshot, so the TUI knows
// whether there is more.
func (a *App) tuiJobs(ctx context.Context, snap *tui.Snapshot) {
	q := a.tuiJobQuery()
	cursor := ""
	for page := 0; page < min(max(1, q.Pages), maxJobPages); page++ {
		vals := url.Values{"limit": {"20"}}
		if q.State != "" {
			vals.Set("state", q.State)
		}
		if q.CollectionID != "" {
			vals.Set("collection_id", q.CollectionID)
		}
		if cursor != "" {
			vals.Set("cursor", cursor)
		}
		r, err := a.call(ctx, "GET", "/v1/jobs", vals, nil, "")
		if err != nil {
			return
		}
		var body struct {
			Items []tuiJobView `json:"items"`
			Next  *string      `json:"next_cursor"`
		}
		if r.Decode(&body) != nil {
			return
		}
		for _, j := range body.Items {
			snap.Jobs = append(snap.Jobs, j.job())
		}
		cursor = ""
		if body.Next != nil {
			cursor = *body.Next
		}
		if cursor == "" {
			break
		}
	}
	snap.JobsNext = cursor
}

func (a *App) tuiCollections(ctx context.Context, snap *tui.Snapshot) {
	var q url.Values
	if a.showRetired.Load() {
		// include_retired lists the retired sources of this principal beside
		// the live ones, without other principals' copies.
		q = url.Values{"include_retired": {"true"}}
	}
	items, ok := a.collectionItems(ctx, q)
	if !ok {
		return
	}
	for _, c := range items {
		coll := tui.Collection{SourceID: c.SourceID, DestinationID: c.DestinationID, ID: c.ID, SourceName: c.SourceName, State: c.State, Revision: c.Revision, RootName: c.RootName,
			OnConflict: c.OnConflict,
			Filter:     c.Filter}
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

// collectionItems reads one page of the library.
func (a *App) collectionItems(ctx context.Context, q url.Values) ([]collectionView, bool) {
	r, err := a.call(ctx, "GET", "/v1/collections", q, nil, "")
	if err != nil {
		return nil, false
	}
	var page struct {
		Items []collectionView `json:"items"`
	}
	if r.Decode(&page) != nil {
		return nil, false
	}
	return page.Items, true
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
		Language        string  `json:"language"`
		Timezone        string  `json:"timezone"`
		NotifyCompleted bool    `json:"notify_completed"`
		NotifyFailed    bool    `json:"notify_failed"`
		NotifyAction    bool    `json:"notify_action_required"`
		QuietStart      *string `json:"quiet_hours_start"`
		QuietEnd        *string `json:"quiet_hours_end"`
		RetentionDays   int     `json:"retention_days"`
		AfterCopySync   string  `json:"after_copy_sync_mode"`
	}
	if r.Decode(&prefs) != nil {
		return
	}
	snap.Prefs = tui.Prefs{Language: prefs.Language, Timezone: prefs.Timezone,
		NotifyCompleted: prefs.NotifyCompleted, NotifyFailed: prefs.NotifyFailed, NotifyAction: prefs.NotifyAction,
		RetentionDays: prefs.RetentionDays, AfterCopySync: prefs.AfterCopySync}
	if prefs.QuietStart != nil && prefs.QuietEnd != nil {
		snap.Prefs.QuietStart, snap.Prefs.QuietEnd = *prefs.QuietStart, *prefs.QuietEnd
	}
	// Autostart is this machine's switch, not a daemon preference.
	snap.Prefs.Autostart, snap.Prefs.AutostartKnown = a.autostartForTUI()
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
