package app

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/GiaHung07/502Drive/internal/drive"
	"github.com/GiaHung07/502Drive/internal/drive/drivetest"
	"github.com/GiaHung07/502Drive/internal/schedule"
	"github.com/GiaHung07/502Drive/internal/state"
)

// clock is a settable time source for schedule math.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) set(t time.Time) {
	c.mu.Lock()
	c.t = t
	c.mu.Unlock()
}

type syncEnv struct {
	t     *testing.T
	store *state.Store
	dir   string
	fake  *drivetest.Fake
	svc   *Service
	clock *clock
	dest  string
}

func newSyncEnv(t *testing.T, now time.Time) *syncEnv {
	t.Helper()
	dir := t.TempDir()
	store, err := state.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	fake := drivetest.New()
	e := &syncEnv{t: t, store: store, dir: dir, fake: fake, clock: &clock{t: now}}
	e.svc = &Service{Store: store, Version: "test", Now: e.clock.now,
		Provider: func(context.Context) (drive.Provider, error) { return fake, nil }}
	e.dest = fake.Folder("Archive", "root").ID
	return e
}

// restart closes and reopens the store, as a daemon restart would.
func (e *syncEnv) restart() {
	e.t.Helper()
	if err := e.store.Close(); err != nil {
		e.t.Fatal(err)
	}
	store, err := state.Open(e.dir)
	if err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(func() { store.Close() })
	e.store = store
	e.svc = &Service{Store: store, Version: "test", Now: e.clock.now,
		Provider: func(context.Context) (drive.Provider, error) { return e.fake, nil }}
}

// ready adds a copied folder with a finished baseline job.
func (e *syncEnv) ready(name string) state.Collection {
	e.t.Helper()
	src := e.fake.Folder(name, "root")
	e.fake.File("a.txt", src.ID, 5)
	ctx := context.Background()
	plan := state.Plan{ID: state.ID("plan"), Principal: "owner", SourceID: src.ID, SourceName: src.Name, SourceKind: state.KindFolder,
		DestinationID: "dst", DestinationFolder: e.dest, RootName: src.Name, ExpiresAt: time.Now().Add(time.Hour), Revision: 1}
	if err := e.store.PutPlan(ctx, plan); err != nil {
		e.t.Fatal(err)
	}
	res, err := e.store.Admit(ctx, "owner", state.ID("admit"), plan.ID, 1)
	if err != nil {
		e.t.Fatal(err)
	}
	if _, err = e.store.UpdateJob(ctx, res.Job.ID, func(j *state.Job) (string, bool) {
		j.State = state.JobSucceeded
		return "finished", true
	}); err != nil {
		e.t.Fatal(err)
	}
	if err = e.store.SetCollectionState(ctx, res.Job.CollectionID, "ready"); err != nil {
		e.t.Fatal(err)
	}
	c, err := e.store.GetCollection(ctx, "", res.Job.CollectionID)
	if err != nil {
		e.t.Fatal(err)
	}
	return c
}

func daily(tz, hhmm string) *schedule.Spec {
	return &schedule.Spec{Kind: schedule.KindDaily, Timezone: tz, LocalTime: hhmm}
}

func (e *syncEnv) configure(c state.Collection, req SyncRequest) SyncView {
	e.t.Helper()
	view, _, err := e.svc.ConfigureSync(context.Background(), Owner, state.ID("cfg"), c.ID, req)
	if err != nil {
		e.t.Fatalf("configure: %v", err)
	}
	return view
}

func TestPreferencesTimezoneIsValidated(t *testing.T) {
	e := newSyncEnv(t, time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC))
	ctx := context.Background()
	prefs, err := e.svc.Preferences(ctx, Owner)
	if err != nil {
		t.Fatal(err)
	}
	if prefs.Timezone == "" {
		t.Fatal("a default timezone is required")
	}
	tz := "America/New_York"
	updated, err := e.svc.PatchPreferences(ctx, Owner, "tz-1", PreferencePatch{Timezone: &tz})
	if err != nil || updated.Timezone != tz {
		t.Fatalf("patch: %+v %v", updated, err)
	}
	bad := "Mars/Olympus"
	if _, err = e.svc.PatchPreferences(ctx, Owner, "tz-2", PreferencePatch{Timezone: &bad}); err == nil {
		t.Fatal("an invalid zone was accepted")
	}
	if got := Classify(err); got.Status != 400 {
		t.Fatalf("status = %d", got.Status)
	}
	if prefs, err = e.svc.Preferences(ctx, Owner); err != nil || prefs.Timezone != tz {
		t.Fatalf("stored timezone = %q (%v)", prefs.Timezone, err)
	}
}

func TestConfigureSyncValidatesInput(t *testing.T) {
	e := newSyncEnv(t, time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC))
	c := e.ready("Project")
	intervalWithZone := &schedule.Spec{Kind: schedule.KindInterval, IntervalMinutes: 30, AnchorAt: e.clock.now(), Timezone: "UTC"}
	bad := []struct {
		name string
		key  string
		req  SyncRequest
	}{
		{"no key", "", SyncRequest{Mode: state.ModeScheduled, Schedule: daily("UTC", "10:00"), ContentPolicy: state.PolicyVersioned}},
		{"no mode", "k1", SyncRequest{Schedule: daily("UTC", "10:00"), ContentPolicy: state.PolicyVersioned}},
		{"no schedule", "k2", SyncRequest{Mode: state.ModeScheduled, ContentPolicy: state.PolicyVersioned}},
		{"schedule on realtime", "k3", SyncRequest{Mode: state.ModeRealtime, Schedule: daily("UTC", "10:00"), ContentPolicy: state.PolicyVersioned}},
		{"bad policy", "k4", SyncRequest{Mode: state.ModeScheduled, Schedule: daily("UTC", "10:00"), ContentPolicy: "mirror"}},
		{"bad zone", "k5", SyncRequest{Mode: state.ModeScheduled, Schedule: daily("Mars/Olympus", "10:00"), ContentPolicy: state.PolicyVersioned}},
		{"interval with zone", "k6", SyncRequest{Mode: state.ModeScheduled, Schedule: intervalWithZone, ContentPolicy: state.PolicyVersioned}},
	}
	for _, tc := range bad {
		_, _, err := e.svc.ConfigureSync(context.Background(), Owner, tc.key, c.ID, tc.req)
		if err == nil {
			t.Fatalf("%s: accepted", tc.name)
		}
		if got := Classify(err).Status; got != 400 {
			t.Fatalf("%s: status %d", tc.name, got)
		}
	}
	if view, configured, err := e.svc.SyncStatus(context.Background(), c.ID); err != nil || configured {
		t.Fatalf("invalid input stored a subscription: %+v %v %v", view, configured, err)
	}
	if e.fake.Calls["starttoken"] != 0 {
		t.Fatal("invalid input reached the provider")
	}
}

func TestConfigureSyncScheduledComputesNextRun(t *testing.T) {
	e := newSyncEnv(t, time.Date(2026, 10, 26, 2, 0, 0, 0, time.UTC))
	c := e.ready("Project")
	view := e.configure(c, SyncRequest{Mode: state.ModeScheduled, Schedule: daily("Asia/Ho_Chi_Minh", "22:00"),
		ContentPolicy: state.PolicyVersioned, ExpectedCollection: c.Revision})
	sub := view.Subscription
	if !view.Configured() || sub.State != state.SyncActive || sub.Revision != 1 {
		t.Fatalf("subscription = %+v", sub)
	}
	// 22:00 in UTC+7 is 15:00 UTC, on the same local day.
	want := time.Date(2026, 10, 26, 15, 0, 0, 0, time.UTC)
	if sub.NextDue == nil || !sub.NextDue.Equal(want) {
		t.Fatalf("next due = %v, want %v", sub.NextDue, want)
	}
	if sub.Schedule == nil || sub.Schedule.LocalTime != "22:00" || sub.ContentPolicy != state.PolicyVersioned {
		t.Fatalf("stored spec = %+v", sub.Schedule)
	}
}

func TestConfigureSyncDSTGapSkipsTheMissingHour(t *testing.T) {
	e := newSyncEnv(t, time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC))
	c := e.ready("Spring")
	view := e.configure(c, SyncRequest{Mode: state.ModeScheduled, Schedule: daily("America/New_York", "02:30"),
		ContentPolicy: state.PolicyVersioned, ExpectedCollection: c.Revision})
	want := time.Date(2026, 3, 9, 6, 30, 0, 0, time.UTC) // 02:30 EDT the day after the gap
	if view.Subscription.NextDue == nil || !view.Subscription.NextDue.Equal(want) {
		t.Fatalf("next due = %v, want %v", view.Subscription.NextDue, want)
	}
}

func TestDSTFoldRunsOneSlotPerLocalCalendarDay(t *testing.T) {
	e := newSyncEnv(t, time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC))
	c := e.ready("Fall")
	view := e.configure(c, SyncRequest{Mode: state.ModeScheduled, Schedule: daily("America/New_York", "01:30"),
		ContentPolicy: state.PolicyVersioned, ExpectedCollection: c.Revision})
	first := time.Date(2026, 11, 1, 5, 30, 0, 0, time.UTC) // 01:30 EDT
	if view.Subscription.NextDue == nil || !view.Subscription.NextDue.Equal(first) {
		t.Fatalf("first due = %v, want %v", view.Subscription.NextDue, first)
	}
	jobs, err := e.store.ClaimDueSchedules(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].CollectionID != c.ID {
		t.Fatalf("claimed %d runs", len(jobs))
	}
	sub, err := e.store.GetSubscription(context.Background(), c.ID)
	if err != nil {
		t.Fatal(err)
	}
	// The repeated 01:30 EST wall clock must not run again; the next slot is
	// the next local day.
	second := time.Date(2026, 11, 2, 6, 30, 0, 0, time.UTC)
	if sub.NextDue == nil || !sub.NextDue.Equal(second) {
		t.Fatalf("second due = %v, want %v", sub.NextDue, second)
	}
}

func TestRealtimeCapturesTheCursorBeforeWritingTheSubscription(t *testing.T) {
	e := newSyncEnv(t, time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC))
	c := e.ready("Realtime")
	ctx := context.Background()
	e.fake.Before = func(op string, f drive.File) error {
		if op != "starttoken" {
			return nil
		}
		if _, configured, err := e.svc.SyncStatus(ctx, c.ID); err != nil || configured {
			t.Errorf("subscription existed before the cursor was captured: %v %v", configured, err)
		}
		return nil
	}
	view := e.configure(c, SyncRequest{Mode: state.ModeRealtime, ContentPolicy: state.PolicyVersioned, ExpectedCollection: c.Revision})
	if e.fake.Calls["starttoken"] != 1 {
		t.Fatalf("start token calls = %d", e.fake.Calls["starttoken"])
	}
	sub := view.Subscription
	if sub.State != state.SyncBootstrapping || sub.Cursor == "" {
		t.Fatalf("subscription = %+v", sub)
	}
	if view.ActiveJobID == "" {
		t.Fatal("realtime attach did not admit a reconcile run")
	}
	run, err := e.store.GetJob(ctx, "", view.ActiveJobID)
	if err != nil || run.Trigger != "reconcile" {
		t.Fatalf("run = %+v %v", run, err)
	}
}

func TestRealtimeRequiresAConnectedAccount(t *testing.T) {
	e := newSyncEnv(t, time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC))
	c := e.ready("No account")
	e.svc.Provider = func(context.Context) (drive.Provider, error) { return nil, drive.ErrNotConnected }
	_, _, err := e.svc.ConfigureSync(context.Background(), Owner, "rt-1", c.ID,
		SyncRequest{Mode: state.ModeRealtime, ContentPolicy: state.PolicyVersioned, ExpectedCollection: c.Revision})
	if err == nil {
		t.Fatal("realtime without an account was accepted")
	}
	if got := Classify(err); got.Status != 403 {
		t.Fatalf("status = %d", got.Status)
	}
	if _, configured, _ := e.svc.SyncStatus(context.Background(), c.ID); configured {
		t.Fatal("a subscription was stored without a cursor")
	}
}

func TestRunNowCoalescesWithTheActiveRun(t *testing.T) {
	e := newSyncEnv(t, time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC))
	c := e.ready("Coalesce")
	ctx := context.Background()
	e.configure(c, SyncRequest{Mode: state.ModeScheduled, Schedule: daily("UTC", "23:00"),
		ContentPolicy: state.PolicyVersioned, ExpectedCollection: c.Revision})
	first, _, err := e.svc.RunNow(ctx, Owner, "run-1", c.ID, c.Revision)
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := e.svc.RunNow(ctx, Owner, "run-2", c.ID, c.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if second.Job.ID != first.Job.ID || !second.Coalesced {
		t.Fatalf("second run = %+v (first %s)", second, first.Job.ID)
	}
	jobs, _, err := e.store.ListJobs(ctx, "", "", 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 { // the finished baseline plus one delta run
		t.Fatalf("jobs = %d, want 2", len(jobs))
	}
	sub, err := e.store.GetSubscription(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !sub.RerunRequested {
		t.Fatal("a coalesced run did not mark rerun_requested")
	}
	// The follow-up is only admitted once the active run finishes.
	if _, err = e.store.UpdateJob(ctx, first.Job.ID, func(j *state.Job) (string, bool) {
		j.State = state.JobSucceeded
		return "finished", true
	}); err != nil {
		t.Fatal(err)
	}
	next, err := e.store.RunFinished(ctx, first.Job.ID)
	if err != nil || next == nil || next.ID == first.Job.ID {
		t.Fatalf("follow-up = %+v %v", next, err)
	}
}

func TestPauseResumeStopSurviveRestart(t *testing.T) {
	e := newSyncEnv(t, time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC))
	c := e.ready("Restart")
	ctx := context.Background()
	view := e.configure(c, SyncRequest{Mode: state.ModeScheduled, Schedule: daily("UTC", "23:00"),
		ContentPolicy: state.PolicyVersioned, ExpectedCollection: c.Revision})

	paused, _, err := e.svc.SyncAction(ctx, Owner, "pause-1", c.ID, "pause", view.Subscription.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if paused.Subscription.State != state.SyncPaused {
		t.Fatalf("state = %q", paused.Subscription.State)
	}

	e.restart()
	after, configured, err := e.svc.SyncStatus(ctx, c.ID)
	if err != nil || !configured || after.Subscription.State != state.SyncPaused {
		t.Fatalf("after restart: %+v %v %v", after, configured, err)
	}
	due := time.Date(2026, 10, 26, 23, 0, 0, 0, time.UTC)
	if jobs, err := e.store.ClaimDueSchedules(ctx, due); err != nil || len(jobs) != 0 {
		t.Fatalf("paused subscription ran: %d %v", len(jobs), err)
	}

	resumed, _, err := e.svc.SyncAction(ctx, Owner, "resume-1", c.ID, "resume", after.Subscription.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Subscription.State != state.SyncActive || resumed.Subscription.NextDue == nil {
		t.Fatalf("resumed = %+v", resumed.Subscription)
	}
	e.restart()
	stopped, _, err := e.svc.SyncAction(ctx, Owner, "stop-1", c.ID, "stop", resumed.Subscription.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if stopped.Subscription.State != state.SyncStopped || stopped.Subscription.NextDue != nil {
		t.Fatalf("stopped = %+v", stopped.Subscription)
	}
	e.restart()
	final, configured, err := e.svc.SyncStatus(ctx, c.ID)
	if err != nil || !configured || final.Subscription.State != state.SyncStopped {
		t.Fatalf("stopped did not survive: %+v %v %v", final, configured, err)
	}
	if jobs, err := e.store.ClaimDueSchedules(ctx, due); err != nil || len(jobs) != 0 {
		t.Fatalf("stopped subscription ran: %d %v", len(jobs), err)
	}
}

func TestSyncActionRejectsAStaleRevision(t *testing.T) {
	e := newSyncEnv(t, time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC))
	c := e.ready("Stale")
	view := e.configure(c, SyncRequest{Mode: state.ModeScheduled, Schedule: daily("UTC", "23:00"),
		ContentPolicy: state.PolicyVersioned, ExpectedCollection: c.Revision})
	_, _, err := e.svc.SyncAction(context.Background(), Owner, "pause-stale", c.ID, "pause", view.Subscription.Revision+1)
	if !errors.Is(err, state.ErrStale) {
		t.Fatalf("error = %v, want stale", err)
	}
	if got := Classify(err); got.Status != 409 {
		t.Fatalf("status = %d", got.Status)
	}
}

func TestSyncWaitsForTheBaselineThenActivates(t *testing.T) {
	e := newSyncEnv(t, time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC))
	ctx := context.Background()
	src := e.fake.Folder("Still copying", "root")
	plan := state.Plan{ID: state.ID("plan"), Principal: "owner", SourceID: src.ID, SourceName: src.Name, SourceKind: state.KindFolder,
		DestinationID: "dst", DestinationFolder: e.dest, RootName: src.Name, ExpiresAt: time.Now().Add(time.Hour), Revision: 1}
	if err := e.store.PutPlan(ctx, plan); err != nil {
		t.Fatal(err)
	}
	res, err := e.store.Admit(ctx, "owner", state.ID("admit"), plan.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	c, err := e.store.GetCollection(ctx, "", res.Job.CollectionID)
	if err != nil {
		t.Fatal(err)
	}
	view := e.configure(c, SyncRequest{Mode: state.ModeScheduled, Schedule: daily("UTC", "23:00"),
		ContentPolicy: state.PolicyVersioned, ExpectedCollection: c.Revision})
	if view.Subscription.State != state.SyncWaitingBaseline || view.Subscription.NextDue != nil {
		t.Fatalf("subscription = %+v", view.Subscription)
	}
	if _, err = e.store.UpdateJob(ctx, res.Job.ID, func(j *state.Job) (string, bool) {
		j.State = state.JobSucceeded
		return "finished", true
	}); err != nil {
		t.Fatal(err)
	}
	if err = e.store.SetCollectionState(ctx, c.ID, "ready"); err != nil {
		t.Fatal(err)
	}
	next, err := e.store.RunFinished(ctx, res.Job.ID)
	if err != nil || next != nil {
		t.Fatalf("run finished: %+v %v", next, err)
	}
	sub, err := e.store.GetSubscription(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if sub.State != state.SyncActive || sub.NextDue == nil {
		t.Fatalf("after the baseline: %+v", sub)
	}
}
