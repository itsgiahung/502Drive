package engine

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/itsgiahung/502Drive/internal/drive"
	"github.com/itsgiahung/502Drive/internal/state"
)

// runEngine starts one engine and waits for the job like env.runUntil, but lets
// the test supply the engine (shared throttle, paced provider).
func (e *env) runEngine(eng *Engine, id string) state.Job {
	e.t.Helper()
	return e.runEngineUntil(eng, id, terminal)
}

func (e *env) runEngineUntil(eng *Engine, id string, pred func(state.Job) bool) state.Job {
	e.t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { eng.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		j, err := e.store.GetJob(context.Background(), "owner", id)
		if err != nil {
			e.t.Fatal(err)
		}
		if pred(j) {
			return j
		}
		time.Sleep(5 * time.Millisecond)
	}
	j, _ := e.store.GetJob(context.Background(), "owner", id)
	e.t.Fatalf("timeout; job = %+v", j)
	return j
}

func (e *env) throttledEngine(thr *drive.Throttle, provider drive.Provider) *Engine {
	return &Engine{Store: e.store, Provider: func(context.Context) (drive.Provider, error) { return provider, nil },
		AfterRun: func(id string) { e.store.RunFinished(context.Background(), id) },
		Throttle: thr, Workers: thr.Snapshot().WorkersNow, MaxWorkers: 8,
		BackoffBase: time.Millisecond, BackoffMax: 5 * time.Millisecond, Settle: time.Millisecond, Poll: 5 * time.Millisecond}
}

// Successful calls grow the account-wide limit through the paced provider.
func TestPacedProviderGrowsTheAccountLimit(t *testing.T) {
	e := newEnv(t)
	src := e.fake.Folder("AIMD", "root")
	e.fake.File("a.txt", src.ID, 1)
	e.fake.File("b.txt", src.ID, 2)
	thr := drive.NewThrottle(drive.ThrottleOptions{Start: 1, Max: 4, Successes: 1})
	eng := e.throttledEngine(thr, drive.Paced(e.fake, thr))
	j := e.runEngine(eng, e.submit(src).ID)
	if j.State != state.JobSucceeded {
		t.Fatalf("state=%s err=%+v", j.State, j.LastError)
	}
	if got := thr.Snapshot().WorkersNow; got < 2 {
		t.Fatalf("the limit did not grow after successes: %d", got)
	}
}

// A 429 is not the item's failure: the attempt is given back and the retry
// succeeds once the provider recovers.
func TestThrottleDoesNotBurnItemAttempts(t *testing.T) {
	e := newEnv(t)
	src := e.fake.Folder("Throttled copy", "root")
	file := e.fake.File("a.txt", src.ID, 1)
	var mu sync.Mutex
	throttled := 0
	e.fake.Before = func(op string, _ drive.File) error {
		if op != "copy" {
			return nil
		}
		mu.Lock()
		defer mu.Unlock()
		throttled++
		if throttled <= 3 {
			return &drive.Error{Kind: drive.KindThrottled, Status: 429}
		}
		return nil
	}
	thr := drive.NewThrottle(drive.ThrottleOptions{Start: 2, Max: 4, Successes: 2})
	eng := e.throttledEngine(thr, drive.Paced(e.fake, thr))
	j := e.runEngine(eng, e.submit(src).ID)
	if j.State != state.JobSucceeded {
		t.Fatalf("state=%s err=%+v", j.State, j.LastError)
	}
	mu.Lock()
	calls := throttled
	mu.Unlock()
	if calls != 4 {
		t.Fatalf("copy calls = %d, want 4", calls)
	}
	items, err := e.store.Items(context.Background(), j.CollectionID)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if it.SourceID == file.ID && (it.Attempts > 1 || it.State != state.ItemDone) {
			t.Fatalf("throttled item = state %s attempts %d", it.State, it.Attempts)
		}
	}
	if snap := thr.Snapshot(); snap.ThrottledLast5m != 3 {
		t.Fatalf("throttles recorded = %d", snap.ThrottledLast5m)
	}
}
