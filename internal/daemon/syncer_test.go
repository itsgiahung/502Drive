package daemon

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/GiaHung07/502Drive/internal/app"
	"github.com/GiaHung07/502Drive/internal/drive"
	"github.com/GiaHung07/502Drive/internal/drive/drivetest"
	"github.com/GiaHung07/502Drive/internal/schedule"
	"github.com/GiaHung07/502Drive/internal/state"
)

// syncRig is a stopped daemon: store, fake provider, app service and the
// syncer, with a settable clock instead of a ticker.
type syncRig struct {
	t     *testing.T
	store *state.Store
	fake  *drivetest.Fake
	svc   *app.Service
	sy    *syncer
	clock *fakeClock
	dest  string

	mu       sync.Mutex
	wakes    int
	provider error
}

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) set(t time.Time) {
	c.mu.Lock()
	c.t = t
	c.mu.Unlock()
}

func newSyncRig(t *testing.T, now time.Time) *syncRig {
	t.Helper()
	store, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	rig := &syncRig{t: t, store: store, fake: drivetest.New(), clock: &fakeClock{t: now}}
	rig.dest = rig.fake.Folder("Archive", "root").ID
	provider := func(context.Context) (drive.Provider, error) {
		rig.mu.Lock()
		defer rig.mu.Unlock()
		if rig.provider != nil {
			return nil, rig.provider
		}
		return rig.fake, nil
	}
	rig.svc = &app.Service{Store: store, Version: "test", Now: rig.clock.now, Provider: provider}
	rig.sy = newSyncer(store, provider, func() {
		rig.mu.Lock()
		rig.wakes++
		rig.mu.Unlock()
	}, slog.New(slog.DiscardHandler))
	rig.sy.now = rig.clock.now
	return rig
}

func (r *syncRig) setProvider(err error) {
	r.mu.Lock()
	r.provider = err
	r.mu.Unlock()
}

func (r *syncRig) wakeCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.wakes
}

// ready adds a copied folder whose baseline finished successfully.
func (r *syncRig) ready(name string) state.Collection {
	r.t.Helper()
	ctx := context.Background()
	src := r.fake.Folder(name, "root")
	r.fake.File("a.txt", src.ID, 5)
	plan := state.Plan{ID: state.ID("plan"), Principal: "owner", SourceID: src.ID, SourceName: src.Name, SourceKind: state.KindFolder,
		DestinationID: "dst", DestinationFolder: r.dest, RootName: src.Name, ExpiresAt: time.Now().Add(time.Hour), Revision: 1}
	if err := r.store.PutPlan(ctx, plan); err != nil {
		r.t.Fatal(err)
	}
	res, err := r.store.Admit(ctx, "owner", state.ID("admit"), plan.ID, 1)
	if err != nil {
		r.t.Fatal(err)
	}
	r.finish(res.Job.ID)
	if err := r.store.SetCollectionState(ctx, res.Job.CollectionID, "ready"); err != nil {
		r.t.Fatal(err)
	}
	c, err := r.store.GetCollection(ctx, "", res.Job.CollectionID)
	if err != nil {
		r.t.Fatal(err)
	}
	return c
}

func (r *syncRig) finish(jobID string) {
	r.t.Helper()
	ctx := context.Background()
	if _, err := r.store.UpdateJob(ctx, jobID, func(j *state.Job) (string, bool) {
		j.State = state.JobSucceeded
		return "finished", true
	}); err != nil {
		r.t.Fatal(err)
	}
	if next, err := r.store.RunFinished(ctx, jobID); err != nil || next != nil {
		r.t.Fatalf("run finished: %+v %v", next, err)
	}
}

func (r *syncRig) scheduled(c state.Collection, tz, hhmm string) state.Subscription {
	r.t.Helper()
	view, _, err := r.svc.ConfigureSync(context.Background(), app.Owner, state.ID("cfg"), c.ID, app.SyncRequest{
		Mode: state.ModeScheduled, ContentPolicy: state.PolicyVersioned,
		Schedule:           &schedule.Spec{Kind: schedule.KindDaily, Timezone: tz, LocalTime: hhmm},
		ExpectedCollection: c.Revision,
	})
	if err != nil {
		r.t.Fatalf("configure: %v", err)
	}
	return view.Subscription
}

// realtime attaches the feed and finishes the reconcile run so the
// subscription is active and due for polling.
func (r *syncRig) realtime(c state.Collection) state.Subscription {
	r.t.Helper()
	ctx := context.Background()
	view, _, err := r.svc.ConfigureSync(ctx, app.Owner, state.ID("rt"), c.ID, app.SyncRequest{
		Mode: state.ModeRealtime, ContentPolicy: state.PolicyVersioned, ExpectedCollection: c.Revision,
	})
	if err != nil {
		r.t.Fatalf("configure realtime: %v", err)
	}
	if view.ActiveJobID == "" {
		r.t.Fatal("realtime attach admitted no reconcile run")
	}
	r.finish(view.ActiveJobID)
	sub, err := r.store.GetSubscription(ctx, c.ID)
	if err != nil {
		r.t.Fatal(err)
	}
	if sub.State != state.SyncActive {
		r.t.Fatalf("realtime state = %q", sub.State)
	}
	return sub
}

func (r *syncRig) tick(at time.Time) {
	r.t.Helper()
	r.clock.set(at)
	r.sy.tick(context.Background())
}

func (r *syncRig) subscription(collectionID string) state.Subscription {
	r.t.Helper()
	sub, err := r.store.GetSubscription(context.Background(), collectionID)
	if err != nil {
		r.t.Fatal(err)
	}
	return sub
}

func (r *syncRig) runs(collectionID, trigger string) []state.Job {
	r.t.Helper()
	jobs, _, err := r.store.ListJobs(context.Background(), "", "", 100, 0)
	if err != nil {
		r.t.Fatal(err)
	}
	var out []state.Job
	for _, j := range jobs {
		if j.CollectionID == collectionID && j.Trigger == trigger {
			out = append(out, j)
		}
	}
	return out
}

func TestScheduledRunCatchesUpExactlyOnceAfterDowntime(t *testing.T) {
	rig := newSyncRig(t, time.Date(2026, 10, 26, 8, 0, 0, 0, time.UTC))
	c := rig.ready("Catch up")
	sub := rig.scheduled(c, "UTC", "09:00")
	if sub.NextDue == nil || !sub.NextDue.Equal(time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC)) {
		t.Fatalf("next due = %v", sub.NextDue)
	}
	rig.tick(time.Date(2026, 10, 26, 8, 30, 0, 0, time.UTC))
	if runs := rig.runs(c.ID, "scheduled"); len(runs) != 0 {
		t.Fatalf("ran before the slot: %d", len(runs))
	}

	// The daemon was offline for days: one catch-up run, not one per missed day.
	offline := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	rig.tick(offline)
	runs := rig.runs(c.ID, "scheduled")
	if len(runs) != 1 || runs[0].Trigger != "scheduled" {
		t.Fatalf("runs after downtime = %+v", runs)
	}
	after := rig.subscription(c.ID)
	if after.NextDue == nil || !after.NextDue.After(offline) {
		t.Fatalf("next due after catch-up = %v", after.NextDue)
	}
	rig.tick(offline)
	if runs = rig.runs(c.ID, "scheduled"); len(runs) != 1 {
		t.Fatalf("a second tick replayed the slot: %d runs", len(runs))
	}
	if rig.wakeCount() == 0 {
		t.Fatal("an admitted run did not wake the engine")
	}
}

func TestQueueFullKeepsTheSlotDue(t *testing.T) {
	rig := newSyncRig(t, time.Date(2026, 10, 26, 8, 0, 0, 0, time.UTC))
	c := rig.ready("Blocked by capacity")
	rig.scheduled(c, "UTC", "09:00")
	ctx := context.Background()

	// Fill the owner's active-job quota with unrelated queued copies.
	src := rig.fake.Folder("Bulk", "root")
	var bulk []string
	for range 20 {
		plan := state.Plan{ID: state.ID("plan"), Principal: "owner", SourceID: src.ID, SourceName: src.Name, SourceKind: state.KindFolder,
			DestinationID: "dst", DestinationFolder: rig.dest, RootName: src.Name, ExpiresAt: time.Now().Add(time.Hour), Revision: 1}
		if err := rig.store.PutPlan(ctx, plan); err != nil {
			t.Fatal(err)
		}
		res, err := rig.store.Admit(ctx, "owner", state.ID("admit"), plan.ID, 1)
		if err != nil {
			t.Fatal(err)
		}
		bulk = append(bulk, res.Job.ID)
	}
	due := time.Date(2026, 10, 26, 9, 30, 0, 0, time.UTC)
	rig.tick(due)
	if runs := rig.runs(c.ID, "scheduled"); len(runs) != 0 {
		t.Fatalf("admitted a run beyond the queue cap: %d", len(runs))
	}
	sub := rig.subscription(c.ID)
	if sub.NextDue == nil || !sub.NextDue.Equal(time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC)) {
		t.Fatalf("the slot drifted: %v", sub.NextDue)
	}

	// Freeing capacity lets the same slot run without rescheduling it.
	for _, id := range bulk {
		j, err := rig.store.GetJob(ctx, "owner", id)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = rig.store.Action(ctx, "owner", state.ID("cancel"), id, "cancel", j.Revision); err != nil {
			t.Fatal(err)
		}
	}
	rig.tick(due)
	if runs := rig.runs(c.ID, "scheduled"); len(runs) != 1 {
		t.Fatalf("slot was not claimed after capacity freed: %d", len(runs))
	}
}

func TestDSTGapAndFoldAtTheScheduler(t *testing.T) {
	rig := newSyncRig(t, time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC))
	gap := rig.ready("Spring forward")
	sub := rig.scheduled(gap, "America/New_York", "02:30")
	if sub.NextDue == nil || !sub.NextDue.Equal(time.Date(2026, 3, 9, 6, 30, 0, 0, time.UTC)) {
		t.Fatalf("gap next due = %v", sub.NextDue)
	}
	// 2026-03-08 02:30 does not exist in New York: no run.
	rig.tick(time.Date(2026, 3, 8, 12, 0, 0, 0, time.UTC))
	if runs := rig.runs(gap.ID, "scheduled"); len(runs) != 0 {
		t.Fatalf("ran in the DST gap: %d", len(runs))
	}

	rig2 := newSyncRig(t, time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC))
	fold := rig2.ready("Fall back")
	sub = rig2.scheduled(fold, "America/New_York", "01:30")
	first := time.Date(2026, 11, 1, 5, 30, 0, 0, time.UTC) // 01:30 EDT
	if sub.NextDue == nil || !sub.NextDue.Equal(first) {
		t.Fatalf("fold first due = %v", sub.NextDue)
	}
	rig2.tick(first)
	if runs := rig2.runs(fold.ID, "scheduled"); len(runs) != 1 {
		t.Fatalf("fold first run = %d", len(runs))
	}
	// The repeated wall clock (01:30 EST) must not run a second time.
	rig2.tick(time.Date(2026, 11, 1, 6, 30, 0, 0, time.UTC))
	if runs := rig2.runs(fold.ID, "scheduled"); len(runs) != 1 {
		t.Fatalf("the fold hour ran twice: %d", len(runs))
	}
	if sub = rig2.subscription(fold.ID); sub.NextDue == nil || !sub.NextDue.Equal(time.Date(2026, 11, 2, 6, 30, 0, 0, time.UTC)) {
		t.Fatalf("fold next due = %v", sub.NextDue)
	}
}

func TestPollerIgnoresUnrelatedChangesAndCoalescesABurst(t *testing.T) {
	rig := newSyncRig(t, time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC))
	c := rig.ready("Poller")
	sub := rig.realtime(c)
	oldCursor := sub.Cursor

	// A change outside the copied tree must not start a run.
	rig.fake.Folder("Elsewhere", "root")
	rig.tick(time.Date(2026, 10, 26, 12, 0, 1, 0, time.UTC))
	if runs := rig.runs(c.ID, "realtime"); len(runs) != 0 {
		t.Fatalf("unrelated change started %d runs", len(runs))
	}
	sub = rig.subscription(c.ID)
	if sub.Cursor == oldCursor {
		t.Fatal("the cursor did not advance")
	}
	wantWarm := time.Date(2026, 10, 26, 12, 1, 1, 0, time.UTC)
	if got := time.UnixMilli(sub.NextPollAt).UTC(); !got.Equal(wantWarm) {
		t.Fatalf("warm poll at %v, want %v", got, wantWarm)
	}

	// Three relevant changes in one page: a single run, fast poll cadence.
	src := c.SourceID
	for i := range 3 {
		rig.fake.File("burst.txt."+string(rune('a'+i)), src, 4)
	}
	rig.tick(wantWarm)
	runs := rig.runs(c.ID, "realtime")
	if len(runs) != 1 || runs[0].Trigger != "realtime" {
		t.Fatalf("burst runs = %+v", runs)
	}
	sub = rig.subscription(c.ID)
	active := time.UnixMilli(sub.NextPollAt).UTC()
	if !active.Equal(wantWarm.Add(app.SyncPollActive)) {
		t.Fatalf("active poll at %v, want %v", active, wantWarm.Add(app.SyncPollActive))
	}
	if rig.wakeCount() == 0 {
		t.Fatal("a realtime run did not wake the engine")
	}
}

func TestPollerReconcilesAnExpiredCursor(t *testing.T) {
	rig := newSyncRig(t, time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC))
	c := rig.ready("Expired cursor")
	sub := rig.realtime(c)
	if sub.State != state.SyncActive {
		t.Fatalf("realtime state = %q", sub.State)
	}
	before := len(rig.runs(c.ID, "reconcile"))
	rig.fake.InvalidTokens = true

	rig.tick(time.Date(2026, 10, 26, 12, 0, 1, 0, time.UTC))
	after := rig.subscription(c.ID)
	if after.State != state.SyncBootstrapping || after.Cursor == "" {
		t.Fatalf("after an invalid token: state=%q cursor=%q", after.State, after.Cursor)
	}
	if runs := rig.runs(c.ID, "reconcile"); len(runs) != before+1 {
		t.Fatalf("reconcile runs = %d, want %d", len(runs), before+1)
	}
	if after.LastError == nil || after.LastError.Code == "" {
		t.Fatal("the reconcile reason was not recorded")
	}
}

func TestPollDoesNotAdvanceTheCursorOfAChangedSubscription(t *testing.T) {
	rig := newSyncRig(t, time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC))
	c := rig.ready("Stopped")
	sub := rig.realtime(c)
	ctx := context.Background()

	// The user stops sync after this poll snapshot was read.
	view, _, err := rig.svc.SyncAction(ctx, app.Owner, "stop-1", c.ID, "stop", sub.Revision)
	if err != nil {
		t.Fatal(err)
	}
	rig.fake.Folder("After stop", "root")
	rig.sy.poll(ctx, sub)

	after := rig.subscription(c.ID)
	if after.State != state.SyncStopped {
		t.Fatalf("state = %q, want stopped (stop answered %q)", after.State, view.Subscription.State)
	}
	if after.Cursor != sub.Cursor {
		t.Fatal("the cursor advanced for a non-active subscription")
	}
	if runs := rig.runs(c.ID, "realtime"); len(runs) != 0 {
		t.Fatalf("a stopped subscription admitted %d runs", len(runs))
	}
}

func TestPollerHonoursAProviderCooldown(t *testing.T) {
	rig := newSyncRig(t, time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC))
	c := rig.ready("Throttled")
	sub := rig.realtime(c)
	rig.setProvider(&drive.Error{Kind: drive.KindThrottled, Status: 429, RetryAfter: 2 * time.Minute})

	rig.tick(time.Date(2026, 10, 26, 12, 0, 1, 0, time.UTC))
	after := rig.subscription(c.ID)
	if after.Cursor != sub.Cursor {
		t.Fatal("a failed poll advanced the cursor")
	}
	if got := time.UnixMilli(after.NextPollAt).UTC(); !got.Equal(time.Date(2026, 10, 26, 12, 2, 1, 0, time.UTC)) {
		t.Fatalf("cooldown until %v", got)
	}
	// Still inside the cooldown: the feed is not called again.
	rig.tick(time.Date(2026, 10, 26, 12, 1, 1, 0, time.UTC))
	if after = rig.subscription(c.ID); !time.UnixMilli(after.NextPollAt).UTC().Equal(time.Date(2026, 10, 26, 12, 2, 1, 0, time.UTC)) {
		t.Fatalf("cooldown was reset: %v", time.UnixMilli(after.NextPollAt).UTC())
	}

	// A relevant change arrives while the provider is cooling down.
	rig.fake.File("after-recovery.txt", c.SourceID, 2)
	rig.setProvider(nil)
	resume := time.Date(2026, 10, 26, 12, 2, 1, 0, time.UTC)
	rig.tick(resume)
	after = rig.subscription(c.ID)
	if after.Cursor == sub.Cursor {
		t.Fatal("the feed was not resumed after the cooldown")
	}
	if got := time.UnixMilli(after.NextPollAt).UTC(); !got.Equal(resume.Add(app.SyncPollActive)) {
		t.Fatalf("after recovery the poll is due at %v", got)
	}
	if runs := rig.runs(c.ID, "realtime"); len(runs) != 1 {
		t.Fatalf("the recovered change admitted %d runs", len(runs))
	}
}

func TestPollerBacksOffToIdleAfterAQuietPeriod(t *testing.T) {
	rig := newSyncRig(t, time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC))
	c := rig.ready("Quiet")
	rig.realtime(c)
	at := time.Date(2026, 10, 26, 12, 0, 1, 0, time.UTC)
	for poll := 1; poll <= idlePolls; poll++ {
		rig.tick(at)
		next := time.UnixMilli(rig.subscription(c.ID).NextPollAt).UTC()
		want := app.SyncPollWarm
		if poll == idlePolls {
			want = app.SyncPollIdle
		}
		if next.Sub(at) != want {
			t.Fatalf("poll %d scheduled %v after, want %v", poll, next.Sub(at), want)
		}
		at = next
	}
}

func TestReconcileRunsAreQueuedForWaitingSubscriptions(t *testing.T) {
	rig := newSyncRig(t, time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC))
	c := rig.ready("Needs reconcile")
	if sub := rig.realtime(c); sub.State != state.SyncActive {
		t.Fatalf("realtime state = %q", sub.State)
	}
	ctx := context.Background()
	before := len(rig.runs(c.ID, "reconcile"))
	// Force the state the syncer must repair without touching the feed.
	if err := rig.store.MarkNeedsReconcile(ctx, c.ID, "", state.Failure{Code: "manual", Message: "fixture"}); err != nil {
		t.Fatal(err)
	}
	rig.tick(time.Date(2026, 10, 26, 12, 0, 1, 0, time.UTC))
	after := rig.subscription(c.ID)
	if after.State != state.SyncBootstrapping || after.Cursor == "" {
		t.Fatalf("state=%q cursor=%q", after.State, after.Cursor)
	}
	if runs := rig.runs(c.ID, "reconcile"); len(runs) != before+1 {
		t.Fatalf("reconcile runs = %d, want %d", len(runs), before+1)
	}
}

func TestSyncerNotifiesOncePerErrorCode(t *testing.T) {
	rig := newSyncRig(t, time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC))
	c := rig.ready("Notify")
	rig.realtime(c)
	var notified, cleared []string
	rig.sy.notifyError = func(_ context.Context, collectionID, code string) error {
		notified = append(notified, collectionID+":"+code)
		return nil
	}
	rig.sy.notifyClear = func(_ context.Context, collectionID string) error {
		cleared = append(cleared, collectionID)
		return nil
	}
	rig.setProvider(&drive.Error{Kind: drive.KindAuth, Status: 401, Reason: "invalid_grant"})
	rig.tick(time.Date(2026, 10, 26, 12, 0, 1, 0, time.UTC))
	if len(notified) != 1 || notified[0] != c.ID+":auth" {
		t.Fatalf("notified = %v", notified)
	}
	// The same failure on the next poll stays silent.
	rig.tick(time.Date(2026, 10, 26, 12, 1, 2, 0, time.UTC))
	if len(notified) != 1 {
		t.Fatalf("a repeated failure was announced again: %v", notified)
	}
	// Recovery clears the marker so the next failure is a new event.
	rig.setProvider(nil)
	rig.tick(time.Date(2026, 10, 26, 12, 2, 3, 0, time.UTC))
	if len(cleared) != 1 || cleared[0] != c.ID {
		t.Fatalf("cleared = %v", cleared)
	}
}

func TestSyncerStopsCleanlyOnCancel(t *testing.T) {
	rig := newSyncRig(t, time.Now().UTC())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		rig.sy.run(ctx)
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the syncer did not stop")
	}
	if err := ctx.Err(); !errors.Is(err, context.Canceled) {
		t.Fatalf("ctx = %v", err)
	}
}
