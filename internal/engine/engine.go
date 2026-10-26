// Package engine runs admitted jobs: it discovers the source tree, copies it
// server-side and keeps every remote write behind a durable intent.
package engine

import (
	"context"
	"errors"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/itsgiahung/502Drive/internal/drive"
	"github.com/itsgiahung/502Drive/internal/state"
)

// Engine claims queued jobs and drives them to a truthful final state.
type Engine struct {
	Store *state.Store
	// Provider returns the Drive client for a run. Errors from it block the job
	// with account_reconnect_required instead of failing items.
	Provider func(ctx context.Context) (drive.Provider, error)
	Log      *slog.Logger

	Jobs        int           // concurrent jobs (default 2)
	Workers     int           // concurrent copy workers per job (default 4)
	MaxCalls    int           // concurrent provider calls overall (default 8)
	BackoffBase time.Duration // first retry delay (default 2s)
	BackoffMax  time.Duration // cap on a retry delay (default 5m)
	Settle      time.Duration // wait between reconcile queries (default 2s)
	Poll        time.Duration // idle scan interval (default 1s)
	Now         func() time.Time
	// AfterRun is called when a job reaches a final state, for sync bookkeeping.
	AfterRun func(jobID string)

	wake    chan struct{}
	calls   chan struct{}
	mu      sync.Mutex
	running map[string]bool
}

func (e *Engine) defaults() {
	if e.Jobs == 0 {
		e.Jobs = 2
	}
	if e.Workers == 0 {
		e.Workers = 4
	}
	if e.MaxCalls == 0 {
		e.MaxCalls = 8
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
	e.wake = make(chan struct{}, 1)
	e.calls = make(chan struct{}, e.MaxCalls)
	e.running = map[string]bool{}
}

// Wake asks the engine to look for new work now.
func (e *Engine) Wake() {
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

func (e *Engine) acquire(ctx context.Context) error {
	select {
	case e.calls <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (e *Engine) release() { <-e.calls }

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
