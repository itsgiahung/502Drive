package daemon

import (
	"context"
	"errors"
	"log/slog"
	"math/rand/v2"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/drive"
	"github.com/itsgiahung/502Drive/internal/drive/drivetest"
	"github.com/itsgiahung/502Drive/internal/schedule"
	"github.com/itsgiahung/502Drive/internal/state"
)

// syncRig is a stopped daemon: store, fake provider, app service and the
// syncer, with a settable clock instead of a ticker.
type syncRig struct {
	t     *testing.T
	dir   string
	store *state.Store
	fake  *drivetest.Fake
	svc   *app.Service
	sy    *syncer
	clock *fakeClock
	dest  string

	providerFn func(context.Context) (drive.Provider, error)
	wakeFn     func()
	mu         sync.Mutex
	wakes      int
	provider   error
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
	rig := &syncRig{t: t, dir: t.TempDir(), fake: drivetest.New(), clock: &fakeClock{t: now}}
	rig.dest = rig.fake.Folder("Archive", "root").ID
	rig.providerFn = func(context.Context) (drive.Provider, error) {
		rig.mu.Lock()
		defer rig.mu.Unlock()
		if rig.provider != nil {
			return nil, rig.provider
		}
		return rig.fake, nil
	}
	rig.wakeFn = func() {
		rig.mu.Lock()
		rig.wakes++
		rig.mu.Unlock()
	}
	store, err := state.Open(rig.dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	rig.attach(store)
	return rig
}

// attach rebinds the store after a restart (or the initial open).
func (r *syncRig) attach(store *state.Store) {
	r.store = store
	r.svc = &app.Service{Store: store, Version: "test", Now: r.clock.now, Provider: r.providerFn}
	r.sy = newSyncer(store, r.providerFn, r.wakeFn, slog.New(slog.DiscardHandler))
	r.sy.now = r.clock.now
}

// restart closes and reopens the data directory, as a daemon restart would. The
// sync cursor and the pending replay buffer must survive it.
func (r *syncRig) restart() {
	r.t.Helper()
	if err := r.store.Close(); err != nil {
		r.t.Fatal(err)
	}
	store, err := state.Open(r.dir)
	if err != nil {
		r.t.Fatal(err)
	}
	r.t.Cleanup(func() { store.Close() })
	r.attach(store)
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

// bootstrapping attaches realtime sync but leaves the baseline reconcile
// running, the state in which feed pages must be stored for replay.
func (r *syncRig) bootstrapping(c state.Collection) (state.Subscription, string) {
	r.t.Helper()
	ctx := context.Background()
	view, _, err := r.svc.ConfigureSync(ctx, app.Owner, state.ID("boot"), c.ID, app.SyncRequest{
		Mode: state.ModeRealtime, ContentPolicy: state.PolicyVersioned, ExpectedCollection: c.Revision,
	})
	if err != nil {
		r.t.Fatalf("configure bootstrapping: %v", err)
	}
	if view.ActiveJobID == "" {
		r.t.Fatal("no baseline reconcile run")
	}
	sub, err := r.store.GetSubscription(ctx, c.ID)
	if err != nil {
		r.t.Fatal(err)
	}
	if sub.State != state.SyncBootstrapping {
		r.t.Fatalf("state = %q", sub.State)
	}
	return sub, view.ActiveJobID
}

// pending counts the durable replay rows of one collection.
func (r *syncRig) pending(collectionID string) int {
	r.t.Helper()
	collections, err := r.store.PendingCollections(context.Background())
	if err != nil {
		r.t.Fatal(err)
	}
	for _, id := range collections {
		if id == collectionID {
			// ReplayBatch is the only reader that exposes the buffer size.
			changes, _, _, err := r.store.ReplayBatch(context.Background(), collectionID, state.MaxReplayChanges)
			if err != nil {
				r.t.Fatal(err)
			}
			return len(changes)
		}
	}
	return 0
}

func (r *syncRig) feedCursor() string {
	r.t.Helper()
	token, err := r.fake.StartPageToken(context.Background())
	if err != nil {
		r.t.Fatal(err)
	}
	return token
}

// While the baseline runs, feed pages are stored raw and the subscription
// cursor does not move. Finishing the baseline replays them exactly once.
func TestPollStoresChangesDuringBaselineAndReplaysAfterIt(t *testing.T) {
	t0 := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	rig := newSyncRig(t, t0)
	c := rig.ready("Replay")
	sub, job := rig.bootstrapping(c)
	oldCursor := sub.Cursor

	rig.fake.File("late.txt", c.SourceID, 3)
	rig.tick(t0.Add(time.Second))
	sub = rig.subscription(c.ID)
	if sub.Cursor != oldCursor {
		t.Fatalf("the cursor moved during the baseline: %q -> %q", oldCursor, sub.Cursor)
	}
	if n := rig.pending(c.ID); n != 1 {
		t.Fatalf("pending changes = %d", n)
	}
	if runs := rig.runs(c.ID, "realtime"); len(runs) != 0 {
		t.Fatalf("a change was applied before the baseline ended: %d runs", len(runs))
	}
	want := rig.feedCursor()

	rig.finish(job)
	if sub = rig.subscription(c.ID); sub.State != state.SyncActive {
		t.Fatalf("state after baseline = %q", sub.State)
	}
	rig.tick(t0.Add(2 * time.Second))

	sub = rig.subscription(c.ID)
	if sub.Cursor != want {
		t.Fatalf("cursor after replay = %q, want %q", sub.Cursor, want)
	}
	if n := rig.pending(c.ID); n != 0 {
		t.Fatalf("replay left %d rows", n)
	}
	if runs := rig.runs(c.ID, "realtime"); len(runs) != 1 {
		t.Fatalf("replay admitted %d runs", len(runs))
	}
	if rig.wakeCount() == 0 {
		t.Fatal("a replayed change did not wake the engine")
	}
}

func TestReplayDoesNotRepeatPagesAfterRestart(t *testing.T) {
	t0 := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	rig := newSyncRig(t, t0)
	c := rig.ready("Restart")
	_, job := rig.bootstrapping(c)

	rig.fake.File("one.txt", c.SourceID, 1)
	rig.fake.File("two.txt", c.SourceID, 2)
	rig.tick(t0.Add(time.Second))
	if n := rig.pending(c.ID); n != 2 {
		t.Fatalf("pending after first poll = %d", n)
	}

	// The daemon dies before the baseline finishes and is restarted.
	rig.restart()
	due := time.UnixMilli(rig.subscription(c.ID).NextPollAt).UTC()
	rig.tick(due)
	if n := rig.pending(c.ID); n != 2 {
		t.Fatalf("restart duplicated pages: %d rows", n)
	}

	rig.fake.File("three.txt", c.SourceID, 3)
	due = time.UnixMilli(rig.subscription(c.ID).NextPollAt).UTC()
	rig.tick(due)
	if n := rig.pending(c.ID); n != 3 {
		t.Fatalf("after restart the poll did not resume behind the buffer: %d rows", n)
	}

	rig.finish(job)
	rig.tick(due.Add(time.Second))
	if sub := rig.subscription(c.ID); sub.Cursor != rig.feedCursor() {
		t.Fatalf("cursor after replay = %q", sub.Cursor)
	}
	if n := rig.pending(c.ID); n != 0 {
		t.Fatalf("replay left %d rows", n)
	}
	if runs := rig.runs(c.ID, "realtime"); len(runs) != 1 {
		t.Fatalf("replay admitted %d runs", len(runs))
	}
}

func TestReplayAdvancesTheCursorWithoutARunForUnrelatedChanges(t *testing.T) {
	t0 := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	rig := newSyncRig(t, t0)
	c := rig.ready("Unrelated")
	_, job := rig.bootstrapping(c)

	rig.fake.Folder("Elsewhere", "root")
	rig.tick(t0.Add(time.Second))
	want := rig.feedCursor()
	rig.finish(job)
	rig.tick(t0.Add(2 * time.Second))

	if sub := rig.subscription(c.ID); sub.Cursor != want {
		t.Fatalf("cursor = %q, want %q", sub.Cursor, want)
	}
	if n := rig.pending(c.ID); n != 0 {
		t.Fatalf("pending rows = %d", n)
	}
	if runs := rig.runs(c.ID, "realtime"); len(runs) != 0 {
		t.Fatalf("an unrelated change admitted %d runs", len(runs))
	}
}

func TestPauseKeepsTheReplayBufferForResume(t *testing.T) {
	t0 := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	rig := newSyncRig(t, t0)
	c := rig.ready("Paused replay")
	sub, job := rig.bootstrapping(c)
	rig.fake.File("during.txt", c.SourceID, 1)
	rig.tick(t0.Add(time.Second))
	oldCursor := rig.subscription(c.ID).Cursor

	rev := rig.subscription(c.ID).Revision
	if _, _, err := rig.svc.SyncAction(context.Background(), app.Owner, "pause-1", c.ID, "pause", rev); err != nil {
		t.Fatal(err)
	}
	rig.tick(t0.Add(2 * time.Second))
	if sub = rig.subscription(c.ID); sub.State != state.SyncPaused {
		t.Fatalf("state = %q", sub.State)
	}
	if sub.Cursor != oldCursor {
		t.Fatal("pause advanced the cursor")
	}
	if n := rig.pending(c.ID); n != 1 {
		t.Fatalf("pause dropped the buffer: %d rows", n)
	}

	if _, _, err := rig.svc.SyncAction(context.Background(), app.Owner, "resume-1", c.ID, "resume", sub.Revision); err != nil {
		t.Fatal(err)
	}
	rig.finish(job)
	rig.tick(t0.Add(3 * time.Second))
	if sub = rig.subscription(c.ID); sub.State != state.SyncActive {
		t.Fatalf("state after resume = %q", sub.State)
	}
	if n := rig.pending(c.ID); n != 0 {
		t.Fatalf("resumed replay left %d rows", n)
	}
	if runs := rig.runs(c.ID, "realtime"); len(runs) != 1 {
		t.Fatalf("resumed replay admitted %d runs", len(runs))
	}
}

// A baseline that overflowed its replay buffer never activates silently: the
// syncer takes a fresh cursor and runs a full reconcile instead.
func TestAnOverflowedReplayQueuesAFullReconcile(t *testing.T) {
	t0 := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	rig := newSyncRig(t, t0)
	c := rig.ready("Overflow")
	_, job := rig.bootstrapping(c)
	ctx := context.Background()
	batch := make([]state.ReplayChange, state.MaxReplayChanges)
	for i := range batch {
		batch[i] = state.ReplayChange{FileID: "f" + strconv.Itoa(i), Parents: []string{c.SourceID}}
	}
	if overflow, err := rig.store.AppendPendingChanges(ctx, c.ID, batch, "100", t0); err != nil || overflow {
		t.Fatalf("fill: overflow=%v err=%v", overflow, err)
	}
	if overflow, err := rig.store.AppendPendingChanges(ctx, c.ID, []state.ReplayChange{{FileID: "over"}}, "101", t0); err != nil || !overflow {
		t.Fatalf("overflow: %v %v", overflow, err)
	}
	changes := rig.fake.CallCount("changes")
	rig.tick(t0.Add(time.Second))
	if got := rig.fake.CallCount("changes"); got != changes {
		t.Fatalf("an overflowed buffer kept polling the feed: %d calls", got-changes)
	}

	rig.finish(job)
	sub := rig.subscription(c.ID)
	if sub.State != state.SyncNeedsReconcile || sub.ReplayOverflow {
		t.Fatalf("after overflowed baseline: state=%q overflow=%v", sub.State, sub.ReplayOverflow)
	}
	if n := rig.pending(c.ID); n != 0 {
		t.Fatalf("overflow kept %d rows", n)
	}

	before := len(rig.runs(c.ID, "reconcile"))
	rig.tick(t0.Add(time.Second))
	if got := len(rig.runs(c.ID, "reconcile")); got != before+1 {
		t.Fatalf("reconcile runs = %d, want %d", got, before+1)
	}
	if sub = rig.subscription(c.ID); sub.State != state.SyncBootstrapping || sub.Cursor == "" {
		t.Fatalf("state=%q cursor=%q", sub.State, sub.Cursor)
	}
}

// The periodic full reconcile is jittered by ±10%, admitted once, and never
// overlaps another run.
func TestFullReconcileIsJitteredAndRunsOncePerPeriod(t *testing.T) {
	t0 := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	rig := newSyncRig(t, t0)
	c := rig.ready("Periodic")
	sub := rig.realtime(c)
	every := 7 * 24 * time.Hour

	first := time.UnixMilli(sub.NextFullReconcileAt).UTC()
	if d := first.Sub(t0); d < every*9/10 || d > every*11/10 {
		t.Fatalf("first slot %v outside ±10%% of %v", d, every)
	}
	rig.tick(first.Add(-time.Second))
	if runs := rig.runs(c.ID, "full_reconcile"); len(runs) != 0 {
		t.Fatalf("ran before the slot: %d", len(runs))
	}

	rig.tick(first)
	runs := rig.runs(c.ID, "full_reconcile")
	if len(runs) != 1 {
		t.Fatalf("full reconcile runs = %d", len(runs))
	}
	after := rig.subscription(c.ID)
	next := time.UnixMilli(after.NextFullReconcileAt).UTC()
	if d := next.Sub(first); d < every*9/10 || d > every*11/10 {
		t.Fatalf("next slot %v outside ±10%% of %v", d, every)
	}
	rig.tick(first.Add(time.Second))
	if runs = rig.runs(c.ID, "full_reconcile"); len(runs) != 1 {
		t.Fatalf("a second tick duplicated the reconcile: %d", len(runs))
	}
}

// Seeded crash matrix: a daemon may die before, during or after the baseline.
// Whatever the point, a restart replays the durable buffer once, ends at the
// feed head and never duplicates the run.
func TestReplayCrashMatrix(t *testing.T) {
	for seed := uint64(1); seed <= 12; seed++ {
		t.Run("seed-"+strconv.FormatUint(seed, 10), func(t *testing.T) {
			rng := rand.New(rand.NewPCG(seed, 0x5eed))
			t0 := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
			rig := newSyncRig(t, t0)
			c := rig.ready("Crash")
			_, job := rig.bootstrapping(c)

			files := rng.IntN(5) // 0..4 new files during the baseline
			for i := range files {
				rig.fake.File("crash-"+strconv.Itoa(i)+".txt", c.SourceID, int64(i+1))
			}
			crash := rng.IntN(3)
			if crash == 0 {
				rig.restart() // die before the first feed poll
			}
			rig.tick(t0.Add(time.Second))
			if crash == 1 {
				rig.restart() // die with stored pages, baseline unfinished
			}
			rig.finish(job)
			if crash == 2 {
				rig.restart() // die after the baseline, before replay
			}
			rig.tick(t0.Add(2 * time.Second))

			sub := rig.subscription(c.ID)
			if sub.State != state.SyncActive {
				t.Fatalf("seed %d: state = %q", seed, sub.State)
			}
			if want := rig.feedCursor(); sub.Cursor != want {
				t.Fatalf("seed %d: cursor = %q, want %q", seed, sub.Cursor, want)
			}
			if n := rig.pending(c.ID); n != 0 {
				t.Fatalf("seed %d: %d rows left", seed, n)
			}
			want := 1
			if files == 0 {
				want = 0
			}
			if runs := rig.runs(c.ID, "realtime"); len(runs) != want {
				t.Fatalf("seed %d: %d realtime runs, want %d", seed, len(runs), want)
			}
			// A further tick is a no-op: no second run, no cursor movement.
			rig.tick(t0.Add(3 * time.Second))
			if sub2 := rig.subscription(c.ID); sub2.Cursor != sub.Cursor {
				t.Fatalf("seed %d: the cursor moved again: %q -> %q", seed, sub.Cursor, sub2.Cursor)
			}
			if runs := rig.runs(c.ID, "realtime"); len(runs) != want {
				t.Fatalf("seed %d: a second tick added a run: %d", seed, len(runs))
			}
		})
	}
}
