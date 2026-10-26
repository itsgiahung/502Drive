package daemon

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/drive"
	"github.com/itsgiahung/502Drive/internal/power"
	"github.com/itsgiahung/502Drive/internal/state"
)

// Lifecycle states reported by /v1/status.
const (
	stateRunning           = "running"
	stateSuspending        = "suspending"
	stateSleeping          = "sleeping"
	stateResuming          = "resuming"
	stateReconnectRequired = "reconnect_required"
)

const (
	// heartbeatKey stores the last wall-clock time the daemon was known to run.
	heartbeatKey = "power.heartbeat"
	// heartbeatGap is how stale the heartbeat may be before startup assumes the
	// machine slept (or the daemon died) in between.
	heartbeatGap = 30 * time.Second
	// checkpointBudget bounds the work done before the machine sleeps.
	checkpointBudget = 2 * time.Second
	// reconnectCooldown is how often an invalid grant is re-probed: often
	// enough to recover after a login, rare enough to never hammer Google.
	reconnectCooldown = 5 * time.Minute
	// lifecycleTick drives the heartbeat and the opt-in keep-awake.
	lifecycleTick = 10 * time.Second
)

type lifecycleOptions struct {
	Store            *state.Store
	Log              *slog.Logger
	Now              func() time.Time
	Probe            func(ctx context.Context) error
	ResetConnections func()
	Wake             func()
	Keeper           power.WakeKeeper
	KeepAwake        func() bool
	RunningJobs      func(ctx context.Context) int
	// Sleep and Rand are injectable for tests (backoff timing).
	Sleep func(ctx context.Context, d time.Duration) bool
	Rand  func() float64
}

// lifecycle owns the daemon's power/network state machine. It never touches
// the engine directly: the engine waits on the gate and reports transport
// failures back through OnUnreachable.
type lifecycle struct {
	store        *state.Store
	log          *slog.Logger
	now          func() time.Time
	probe        func(ctx context.Context) error
	reset        func()
	wake         func()
	keeper       power.WakeKeeper
	keepAwake    func() bool
	runningJobs  func(ctx context.Context) int
	checkpointFn func(ctx context.Context) error

	gate   *power.Gate
	prober *power.Prober

	mu      sync.Mutex
	state   string
	since   time.Time
	lastGap time.Duration
	probing atomic.Bool
}

func newLifecycle(o lifecycleOptions) *lifecycle {
	l := &lifecycle{store: o.Store, log: o.Log, now: o.Now, probe: o.Probe, reset: o.ResetConnections,
		wake: o.Wake, keeper: o.Keeper, keepAwake: o.KeepAwake, runningJobs: o.RunningJobs}
	if l.now == nil {
		l.now = time.Now
	}
	if l.log == nil {
		l.log = slog.New(slog.DiscardHandler)
	}
	if l.reset == nil {
		l.reset = func() {}
	}
	l.checkpointFn = l.store.Checkpoint
	l.state, l.since = stateRunning, l.now()
	l.gate = power.NewGate(true)
	l.prober = &power.Prober{Probe: l.probeOnce, Base: time.Second, Max: 30 * time.Second,
		Sleep: o.Sleep, Rand: o.Rand, Permanent: isAuthError, Cooldown: reconnectCooldown,
		OnRetry: func(err error, _ time.Duration) {
			if isAuthError(err) {
				l.setState(stateReconnectRequired)
			}
		}}
	return l
}

// probeOnce is one cheap authenticated call. A local credential problem fails
// before any network traffic, so it is safe to retry slowly forever.
func (l *lifecycle) probeOnce(ctx context.Context) error {
	if l.probe == nil {
		return nil
	}
	return l.probe(ctx)
}

func (l *lifecycle) gateRef() *power.Gate { return l.gate }

// suspending reports whether new work should be refused (the checkpoint
// window before sleep, which is at most a couple of seconds).
func (l *lifecycle) suspending() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.state == stateSuspending
}

func (l *lifecycle) setState(s string) {
	l.mu.Lock()
	if l.state != s {
		l.state, l.since = s, l.now()
	}
	l.mu.Unlock()
}

// view is the JSON shape used by /v1/status.
func (l *lifecycle) view() app.Lifecycle {
	l.mu.Lock()
	defer l.mu.Unlock()
	return app.Lifecycle{State: l.state, Online: l.gate.Online(), Since: l.since.UTC().Format(time.RFC3339),
		LastGapSeconds: int(l.lastGap.Seconds()), ReconnectRequired: l.state == stateReconnectRequired}
}

// Run is the daemon-side loop: it starts the state machine, then feeds it OS
// events and periodic ticks until ctx ends.
func (l *lifecycle) Run(ctx context.Context, events <-chan power.Event) {
	l.start(ctx)
	t := time.NewTicker(lifecycleTick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			if l.keeper != nil {
				l.keeper.Release()
			}
			return
		case ev, ok := <-events:
			if !ok {
				events = nil
				continue
			}
			l.handle(ctx, ev)
		case <-t.C:
			l.tick(ctx)
		}
	}
}

// start decides whether the previous run ended normally. A stale heartbeat
// means the machine slept (or the daemon was killed) between runs: the gate
// stays closed until a probe proves the network and the credential work.
func (l *lifecycle) start(ctx context.Context) {
	l.reset()
	if hb, ok, err := l.readHeartbeat(ctx); err != nil {
		l.log.Warn("power: cannot read the heartbeat", "err", err)
	} else if ok {
		if gap := l.now().Sub(hb); gap > heartbeatGap {
			l.mu.Lock()
			l.lastGap = gap
			l.mu.Unlock()
			l.setState(stateResuming)
			l.gate.Set(false)
			l.writeHeartbeat(ctx)
			l.startProbe(ctx)
			return
		}
	}
	l.writeHeartbeat(ctx)
}

func (l *lifecycle) handle(ctx context.Context, ev power.Event) {
	switch ev.Kind {
	case power.Suspend:
		l.suspend(ctx, ev)
	case power.Resume:
		l.resume(ctx, ev)
	case power.NetworkChanged:
		l.log.Info("power: network changed; resetting idle connections")
		l.reset()
		if !l.gate.Online() {
			l.startProbe(ctx)
		}
	case power.SessionLock, power.SessionUnlock, power.Shutdown:
		l.log.Info("power: session event", "kind", ev.Kind.String())
	}
}

// suspend checkpoints within a small budget and never blocks the OS: the
// delay lock is released once the state is durable, or when the budget ends.
func (l *lifecycle) suspend(ctx context.Context, ev power.Event) {
	l.setState(stateSuspending)
	l.gate.Set(false)
	cctx, cancel := context.WithTimeout(ctx, checkpointBudget)
	err := l.checkpointFn(cctx)
	cancel()
	if err != nil {
		l.log.Warn("power: WAL checkpoint before sleep failed", "err", err)
	}
	l.writeHeartbeat(ctx)
	ev.Acknowledge()
	l.setState(stateSleeping)
	l.log.Info("power: sleeping")
}

// resume verifies the local state, resets the connection pool and probes until
// the network answers. The gate stays closed meanwhile, so the engine never
// sends a request into a dead connection.
func (l *lifecycle) resume(ctx context.Context, ev power.Event) {
	if ev.Gap > 0 {
		l.mu.Lock()
		if ev.Gap > l.lastGap {
			l.lastGap = ev.Gap
		}
		l.mu.Unlock()
	}
	l.setState(stateResuming)
	if err := l.store.Check(ctx); err != nil {
		l.log.Error("power: the state database is unhealthy after resume", "err", err)
	}
	l.reset()
	l.writeHeartbeat(ctx)
	l.startProbe(ctx)
	l.log.Info("power: resuming", "gap", ev.Gap.String())
}

// noteUnreachable is the engine's report that a call never left the machine.
func (l *lifecycle) noteUnreachable(ctx context.Context, err error) {
	if l.gate.Online() && l.gate.Set(false) {
		l.log.Warn("power: network unreachable; waiting for connectivity", "err", err)
		l.setState(stateResuming)
	}
	l.startProbe(ctx)
}

func (l *lifecycle) startProbe(ctx context.Context) {
	if l.probe == nil {
		return
	}
	if !l.probing.CompareAndSwap(false, true) {
		return // one probe stream per machine, even when events flap
	}
	go func() {
		defer l.probing.Store(false)
		if err := l.prober.Until(ctx); err != nil {
			return // context ended
		}
		l.gate.Set(true)
		l.setState(stateRunning)
		l.writeHeartbeat(ctx)
		if l.wake != nil {
			l.wake()
		}
		l.log.Info("power: online")
	}()
}

// tick keeps the heartbeat fresh and applies the opt-in keep-awake.
func (l *lifecycle) tick(ctx context.Context) {
	l.writeHeartbeat(ctx)
	if l.keeper == nil {
		return
	}
	hold := l.keepAwake != nil && l.keepAwake()
	if hold && l.runningJobs != nil {
		hold = l.runningJobs(ctx) > 0
	}
	if hold {
		l.keeper.Hold()
		return
	}
	l.keeper.Release()
}

func (l *lifecycle) writeHeartbeat(ctx context.Context) {
	if err := l.store.SetSetting(context.WithoutCancel(ctx), heartbeatKey, l.now().UTC().Format(time.RFC3339Nano)); err != nil {
		l.log.Warn("power: cannot write the heartbeat", "err", err)
	}
}

func (l *lifecycle) readHeartbeat(ctx context.Context) (time.Time, bool, error) {
	v, ok, err := l.store.Setting(ctx, heartbeatKey)
	if err != nil || !ok {
		return time.Time{}, false, err
	}
	t, perr := time.Parse(time.RFC3339Nano, v)
	if perr != nil {
		return time.Time{}, false, perr
	}
	return t, true, nil
}

// isAuthError reports credential failures that need the user, not a retry.
func isAuthError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, drive.ErrReconnectRequired) || errors.Is(err, drive.ErrNotConnected) {
		return true
	}
	return drive.KindOf(err) == drive.KindAuth
}
