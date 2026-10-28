// Package engine runs admitted jobs: it discovers the source tree, copies it
// server-side and keeps every remote write behind a durable intent.
package engine

import (
	"context"
	"errors"
	"log/slog"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"time"

	"github.com/itsgiahung/502Drive/internal/drive"
	"github.com/itsgiahung/502Drive/internal/state"
)

// NetworkGate tells the engine whether remote work is allowed now. *power.Gate
// implements it; engine only needs these two methods.
type NetworkGate interface {
	Online() bool
	Wait(ctx context.Context) bool
}

// Engine claims queued jobs and drives them to a truthful final state.
type Engine struct {
	Store *state.Store
	// Provider returns the Drive client for a run. Errors from it block the job
	// with account_reconnect_required instead of failing items.
	Provider func(ctx context.Context) (drive.Provider, error)
	Log      *slog.Logger

	Jobs int // concurrent jobs (default 2)
	// Workers is the initial account-wide concurrency (default 4); MaxWorkers
	// is the AIMD ceiling (default 8). The controller grows the limit after a
	// run of successes and halves it on a throttle.
	Workers    int
	MaxWorkers int
	// Throttle is shared with the paced provider, so the engine and the
	// syncer use one account-wide budget. nil creates one for this engine.
	Throttle    *drive.Throttle
	BackoffBase time.Duration // first retry delay (default 2s)
	BackoffMax  time.Duration // cap on a retry delay (default 5m)
	Settle      time.Duration // wait between reconcile queries (default 2s)
	Poll        time.Duration // idle scan interval (default 1s)
	Now         func() time.Time
	// Network gates remote work while the machine is asleep or offline. When
	// nil every call is allowed (tests, maintenance).
	Network NetworkGate
	// OnUnreachable is called when a call provably never left the machine
	// (drive.KindUnreachable). The daemon closes the network gate and starts a
	// connectivity probe; the engine only reports.
	OnUnreachable func(err error)
	// AfterRun is called when a job reaches a final state, for sync bookkeeping.
	AfterRun func(jobID string)

	wake        chan struct{}
	throttleSeq atomic.Int64
	mu          sync.Mutex
	running     map[string]bool
	initOnce    sync.Once
}

// defaults fills in unset fields once. Wake can be called from another
// goroutine (a finished job, an HTTP request) before Run has started, so both
// go through the same Once; without it Wake could read the wake channel while
// Run was still creating it.
func (e *Engine) defaults() { e.initOnce.Do(e.setDefaults) }

func (e *Engine) setDefaults() {
	if e.Jobs == 0 {
		e.Jobs = 2
	}
	if e.Workers == 0 {
		e.Workers = 4
	}
	if e.MaxWorkers < e.Workers {
		e.MaxWorkers = max(8, e.Workers)
	}
	if e.BackoffBase == 0 {
		e.BackoffBase = 2 * time.Second
	}
	if e.BackoffMax == 0 {
		e.BackoffMax = 5 * time.Minute
	}
	if e.Settle == 0 {
		e.Settle = 2 * time.Second
	}
	if e.Poll == 0 {
		e.Poll = time.Second
	}
	if e.Now == nil {
		e.Now = time.Now
	}
	if e.Log == nil {
		e.Log = slog.New(slog.DiscardHandler)
	}
	if e.Throttle == nil {
		e.Throttle = drive.NewThrottle(drive.ThrottleOptions{Start: e.Workers, Max: e.MaxWorkers})
	}
	e.wake = make(chan struct{}, 1)
	e.running = map[string]bool{}
}

// Wake asks the engine to look for new work now.
func (e *Engine) Wake() {
	e.defaults()
	select {
	case e.wake <- struct{}{}:
	default:
	}
}

// Run blocks until ctx is cancelled. On shutdown it lets in-flight jobs reach a
// safe point; unknown writes stay durable and are reconciled after restart.
func (e *Engine) Run(ctx context.Context) error {
	e.defaults()
	db := context.WithoutCancel(ctx)
	var wg sync.WaitGroup
	start := func(j state.Job) {
		e.mu.Lock()
		if e.running[j.ID] {
			e.mu.Unlock()
			return
		}
		e.running[j.ID] = true
		e.mu.Unlock()
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				e.mu.Lock()
				delete(e.running, j.ID)
				e.mu.Unlock()
				e.Wake()
			}()
			r := &run{e: e, db: db, id: j.ID}
			r.execute(ctx)
		}()
	}
	active, err := e.Store.ActiveJobs(db)
	if err != nil {
		return err
	}
	for _, j := range active {
		start(j)
	}
	t := time.NewTicker(e.Poll)
	defer t.Stop()
	for {
		e.mu.Lock()
		room := e.Jobs - len(e.running)
		e.mu.Unlock()
		// Quota-blocked jobs whose resume_after has passed continue with the
		// same identity; nothing is re-admitted or duplicated.
		if n, err := e.Store.ResumeDueBlocked(db, e.Now()); err == nil && n > 0 {
			e.Log.Info("quota: blocked jobs resumed", "count", n)
		}
		for ; room > 0 && ctx.Err() == nil; room-- {
			j, err := e.Store.ClaimQueued(db)
			if errors.Is(err, state.ErrNotFound) {
				break
			}
			if err != nil {
				e.Log.Error("claim failed", "err", err)
				break
			}
			start(j)
		}
		if ids, err := e.Store.StrandedJobs(db); err == nil {
			for _, id := range ids {
				start(state.Job{ID: id})
			}
		}
		select {
		case <-ctx.Done():
			wg.Wait()
			return nil
		case <-e.wake:
		case <-t.C:
		}
	}
}

func (e *Engine) backoff(attempts int) time.Duration {
	d := e.BackoffBase << min(attempts, 10)
	if d > e.BackoffMax || d <= 0 {
		d = e.BackoffMax
	}
	return d/2 + time.Duration(rand.Int64N(int64(d/2)+1))
}

func (e *Engine) acquire(ctx context.Context) error { return e.Throttle.Acquire(ctx) }
func (e *Engine) release()                          { e.Throttle.Release() }

// throttleWait is an account-wide exponential backoff with jitter for retries
// caused by throttling. It is separate from item attempts: a throttle is not a
// failure of the item and must not exhaust its retry budget.
func (e *Engine) throttleWait() time.Duration {
	n := int(e.throttleSeq.Add(1)) - 1
	return e.backoff(n)
}

// throttleClear forgets the throttle streak after a successful call.
func (e *Engine) throttleClear() { e.throttleSeq.Store(0) }

func (e *Engine) sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}
