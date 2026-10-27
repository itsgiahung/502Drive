package daemon

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/itsgiahung/502Drive/internal/drive"
	"github.com/itsgiahung/502Drive/internal/power"
	"github.com/itsgiahung/502Drive/internal/state"
)

type fakeKeeper struct {
	mu   sync.Mutex
	hold bool
}

func (f *fakeKeeper) Hold() {
	f.mu.Lock()
	f.hold = true
	f.mu.Unlock()
}
func (f *fakeKeeper) Release() {
	f.mu.Lock()
	f.hold = false
	f.mu.Unlock()
}
func (f *fakeKeeper) held() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hold
}

type lifecycleRig struct {
	t       *testing.T
	store   *state.Store
	clock   time.Time
	keeper  *fakeKeeper
	probes  atomic.Int64
	probe   func(ctx context.Context) error
	resets  atomic.Int64
	wakes   atomic.Int64
	delayMu sync.Mutex
	delays  []time.Duration
	running atomic.Int64
	keep    atomic.Bool
	l       *lifecycle
}

func newLifecycleRig(t *testing.T) *lifecycleRig {
	t.Helper()
	store, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	r := &lifecycleRig{t: t, store: store, clock: time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC), keeper: &fakeKeeper{}}
	r.probe = func(context.Context) error { r.probes.Add(1); return nil }
	r.l = newLifecycle(lifecycleOptions{
		Store:            store,
		Now:              func() time.Time { return r.clock },
		Probe:            func(ctx context.Context) error { return r.probe(ctx) },
		ResetConnections: func() { r.resets.Add(1) },
		Wake:             func() { r.wakes.Add(1) },
		Keeper:           r.keeper,
		KeepAwake:        func() bool { return r.keep.Load() },
		RunningJobs:      func(context.Context) int { return int(r.running.Load()) },
		Sleep: func(_ context.Context, d time.Duration) bool {
			r.delayMu.Lock()
			r.delays = append(r.delays, d)
			r.delayMu.Unlock()
			return true
		},
		Rand: func() float64 { return 0.5 },
	})
	return r
}

func (r *lifecycleRig) delayList() []time.Duration {
	r.delayMu.Lock()
	defer r.delayMu.Unlock()
	return append([]time.Duration(nil), r.delays...)
}

func (r *lifecycleRig) start(ctx context.Context) {
	r.t.Helper()
	r.l.start(ctx)
}

func waitFor(t *testing.T, what string, pred func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if pred() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", what)
}

func TestLifecycleSuspendCheckpointsRefusesAdmissionAndReleasesTheOS(t *testing.T) {
	r := newLifecycleRig(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.start(ctx)
	if v := r.l.view(); v.State != stateRunning || !v.Online {
		t.Fatalf("fresh start = %+v", v)
	}

	var acks atomic.Int64
	var sought bool
	r.l.checkpointFn = func(ctx context.Context) error {
		sought = r.l.suspending()
		return r.store.Checkpoint(ctx)
	}
	r.l.handle(ctx, power.Event{Kind: power.Suspend, Ack: func() { acks.Add(1) }})

	if !sought {
		t.Fatal("new work was admitted while the daemon was suspending")
	}
	if acks.Load() != 1 {
		t.Fatalf("the OS delay lock was released %d times", acks.Load())
	}
	if v := r.l.view(); v.State != stateSleeping || v.Online {
		t.Fatalf("after suspend = %+v", v)
	}
	if _, ok, err := r.store.Setting(ctx, heartbeatKey); err != nil || !ok {
		t.Fatalf("no heartbeat written: %v %v", ok, err)
	}
}

func TestLifecycleStartupDetectsAStaleHeartbeat(t *testing.T) {
	r := newLifecycleRig(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	old := r.clock.Add(-8 * time.Hour)
	if err := r.store.SetSetting(ctx, heartbeatKey, old.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	// The probe is slow: the first state must already be "resuming".
	r.probe = func(ctx context.Context) error { r.probes.Add(1); <-ctx.Done(); return ctx.Err() }
	r.start(ctx)
	v := r.l.view()
	if v.State != stateResuming || v.Online {
		t.Fatalf("startup after an 8h gap = %+v", v)
	}
	if v.LastGapSeconds < int((8*time.Hour - time.Minute).Seconds()) {
		t.Fatalf("gap = %d", v.LastGapSeconds)
	}
	if r.resets.Load() == 0 {
		t.Fatal("idle connections were not reset after the gap")
	}
}

func TestLifecycleResumeProbesWithBackoffThenOpensTheGate(t *testing.T) {
	r := newLifecycleRig(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.start(ctx)
	var fails atomic.Int64
	fails.Store(2)
	r.probe = func(context.Context) error {
		r.probes.Add(1)
		if fails.Add(-1) >= 0 {
			return errors.New("no route")
		}
		return nil
	}
	r.l.handle(ctx, power.Event{Kind: power.Suspend, Ack: func() {}})
	r.l.handle(ctx, power.Event{Kind: power.Resume, Gap: 10 * time.Minute})
	waitFor(t, "the gate to open", func() bool { return r.l.view().Online })
	if got := r.probes.Load(); got != 3 {
		t.Fatalf("probes = %d, want 3", got)
	}
	delays := r.delayList()
	if len(delays) < 2 || delays[0] <= 0 || delays[1] <= delays[0] {
		t.Fatalf("delays = %v", delays)
	}
	// The gate can read as open a moment before the wake callback runs.
	waitFor(t, "the engine to be woken after the probe", func() bool { return r.wakes.Load() > 0 })
	v := r.l.view()
	if v.State != stateRunning || !v.Online {
		t.Fatalf("view = %+v", v)
	}
	if v.LastGapSeconds < 600 {
		t.Fatalf("gap = %d", v.LastGapSeconds)
	}
}

func TestLifecycleFlappingResumeStartsOnlyOneProbe(t *testing.T) {
	r := newLifecycleRig(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.start(ctx)
	release := make(chan struct{})
	var concurrent, maxConcurrent atomic.Int64
	r.probe = func(context.Context) error {
		n := concurrent.Add(1)
		if n > maxConcurrent.Load() {
			maxConcurrent.Store(n)
		}
		r.probes.Add(1)
		<-release
		concurrent.Add(-1)
		return nil
	}
	r.l.handle(ctx, power.Event{Kind: power.Suspend, Ack: func() {}})
	r.l.handle(ctx, power.Event{Kind: power.Resume})
	r.l.handle(ctx, power.Event{Kind: power.Resume})
	r.l.handle(ctx, power.Event{Kind: power.Resume})
	waitFor(t, "the first probe", func() bool { return r.probes.Load() >= 1 })
	close(release)
	waitFor(t, "the gate", func() bool { return r.l.view().Online })
	if maxConcurrent.Load() != 1 || r.probes.Load() != 1 {
		t.Fatalf("probes=%d concurrent=%d", r.probes.Load(), maxConcurrent.Load())
	}
}

func TestLifecycleInvalidGrantNeedsAReconnectAndProbesSlowly(t *testing.T) {
	r := newLifecycleRig(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.start(ctx)
	r.probe = func(context.Context) error {
		r.probes.Add(1)
		return drive.ErrReconnectRequired
	}
	r.l.handle(ctx, power.Event{Kind: power.Suspend, Ack: func() {}})
	r.l.handle(ctx, power.Event{Kind: power.Resume})
	waitFor(t, "the reconnect state", func() bool { return r.l.view().State == stateReconnectRequired })
	if r.l.view().Online {
		t.Fatal("the gate opened while the grant was invalid")
	}
	waitFor(t, "two auth cooldowns", func() bool { return len(r.delayList()) >= 2 })
	delays := r.delayList()
	if delays[0] != reconnectCooldown || delays[1] != reconnectCooldown {
		t.Fatalf("auth failures must use the reconnect cooldown, not backoff: %v", delays)
	}
	if r.probes.Load() < 2 {
		t.Fatalf("the probe stopped instead of waiting for a login: %d", r.probes.Load())
	}
}

func TestLifecycleKeepAwakeFollowsRunningJobs(t *testing.T) {
	r := newLifecycleRig(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.start(ctx)
	r.keep.Store(true)
	r.running.Store(1)
	r.l.tick(ctx)
	if !r.keeper.held() {
		t.Fatal("the machine was allowed to idle-sleep during a copy")
	}
	r.running.Store(0)
	r.l.tick(ctx)
	if r.keeper.held() {
		t.Fatal("the idle inhibitor was not released")
	}
	r.keep.Store(false)
	r.running.Store(2)
	r.l.tick(ctx)
	if r.keeper.held() {
		t.Fatal("keep-awake is on by default")
	}
}

func TestLifecycleNetworkChangeResetsIdleConnectionsAndProbesWhenOffline(t *testing.T) {
	r := newLifecycleRig(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.start(ctx)
	before := r.resets.Load()
	r.l.handle(ctx, power.Event{Kind: power.NetworkChanged})
	if r.resets.Load() != before+1 {
		t.Fatalf("resets = %d, want %d", r.resets.Load(), before+1)
	}
	if r.probes.Load() != 0 {
		t.Fatalf("an online network change started %d probes", r.probes.Load())
	}

	// Offline and the network changes: probe instead of waiting for a resume.
	r.l.handle(ctx, power.Event{Kind: power.Suspend, Ack: func() {}})
	waitFor(t, "the gate to close", func() bool { return !r.l.view().Online })
	r.l.handle(ctx, power.Event{Kind: power.NetworkChanged})
	waitFor(t, "a probe", func() bool { return r.probes.Load() > 0 })
	waitFor(t, "the gate", func() bool { return r.l.view().Online })
}
