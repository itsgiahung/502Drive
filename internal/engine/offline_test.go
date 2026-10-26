package engine

import (
	"context"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/itsgiahung/502Drive/internal/drive"
	"github.com/itsgiahung/502Drive/internal/power"
	"github.com/itsgiahung/502Drive/internal/state"
)

func unreachable() error { return &drive.Error{Kind: drive.KindUnreachable} }

// itemsOf returns the collection's items for assertions.
func itemsOf(t *testing.T, store *state.Store, collection string) []state.Item {
	t.Helper()
	out, err := store.Items(context.Background(), collection)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestOfflineStopsClaimsAndDoesNotBurnAttempts(t *testing.T) {
	e := newEnv(t)
	src := e.fake.Folder("Project", "root")
	e.fake.File("a.txt", src.ID, 10)
	e.fake.File("b.txt", src.ID, 10)
	job := e.submit(src)

	gate := power.NewGate(false)
	eng := e.engine()
	eng.Network = gate
	var offlineCalls atomic.Int64
	eng.OnUnreachable = func(error) { offlineCalls.Add(1) }

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { eng.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()

	// Nothing may be written while the gate is closed, and no attempt may be
	// consumed just because the machine is offline.
	time.Sleep(120 * time.Millisecond)
	before := itemsOf(t, e.store, job.CollectionID)
	for _, it := range before {
		if it.State != state.ItemPending || it.Attempts != 0 {
			t.Fatalf("offline claim: %+v", it)
		}
	}
	if e.fake.Writes != 0 {
		t.Fatalf("writes while offline = %d", e.fake.Writes)
	}

	gate.Set(true) // open the gate; the same engine may now claim
	j := e.waitTerminal(job.ID)
	if j.State != state.JobSucceeded {
		t.Fatalf("job = %s (%+v)", j.State, j.LastError)
	}
	for _, it := range itemsOf(t, e.store, job.CollectionID) {
		if it.Attempts != 1 {
			t.Fatalf("%s attempts = %d, want exactly 1", it.SourceID, it.Attempts)
		}
	}
	if offlineCalls.Load() != 0 {
		t.Fatalf("a closed gate reported %d unreachable errors", offlineCalls.Load())
	}
}

func TestUnreachableReleasesWithoutPenaltyAndReportsIt(t *testing.T) {
	e := newEnv(t)
	src := e.fake.Folder("Project", "root")
	e.fake.File("a.txt", src.ID, 10)
	job := e.submit(src)

	gate := power.NewGate(true)
	var fails atomic.Int64
	fails.Store(1)
	e.fake.Before = func(op string, f drive.File) error {
		if op == "copy" && fails.Add(-1) >= 0 {
			return unreachable()
		}
		return nil
	}
	eng := e.engine()
	eng.Workers = 1 // one worker: after the release the loop must see the closed gate
	eng.Network = gate
	// Mirror what the daemon does: the first transport failure closes the gate
	// until the connectivity probe succeeds.
	eng.OnUnreachable = func(error) { gate.Set(false) }

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { eng.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !gate.Online() {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if gate.Online() {
		t.Fatal("the engine did not report the transport failure")
	}
	for _, it := range itemsOf(t, e.store, job.CollectionID) {
		switch it.State {
		case state.ItemPending:
			if it.Attempts != 0 {
				t.Fatalf("pending item kept a failed attempt: %+v", it)
			}
		case state.ItemDone:
			// Work that really finished before the failure is untouched.
		default:
			t.Fatalf("transport failure left item in %s: %+v", it.State, it)
		}
	}
	// A worker that already passed the gate check may claim the released item
	// once more; the invariant is that nothing was written twice and no attempt
	// was consumed by the offline failure.
	if got := e.fake.Calls["copy"]; got != 1 {
		t.Fatalf("copy calls = %d, want exactly the failed one", got)
	}

	gate.Set(true) // probe succeeded
	j := e.waitTerminal(job.ID)
	if j.State != state.JobSucceeded {
		t.Fatalf("job = %s", j.State)
	}
}

func TestListPageWaitsForTheNetworkInsteadOfFailingTheJob(t *testing.T) {
	e := newEnv(t)
	src := e.fake.Folder("Project", "root")
	e.fake.File("a.txt", src.ID, 10)
	job := e.submit(src)

	gate := power.NewGate(true)
	var fails atomic.Int64
	fails.Store(1)
	e.fake.Before = func(op string, f drive.File) error {
		if op == "list" && fails.Add(-1) >= 0 {
			return unreachable()
		}
		return nil
	}
	eng := e.engine()
	eng.Network = gate
	eng.OnUnreachable = func(error) { gate.Set(false) }

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { eng.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()

	time.Sleep(100 * time.Millisecond)
	if gate.Online() {
		t.Fatal("a list transport failure did not close the gate")
	}
	for _, it := range itemsOf(t, e.store, job.CollectionID) {
		if it.State != state.ItemPending || it.Attempts != 0 {
			t.Fatalf("listing failure was recorded as item work: %+v", it)
		}
	}
	gate.Set(true)
	j := e.waitTerminal(job.ID)
	if j.State != state.JobSucceeded {
		t.Fatalf("job = %s (%+v)", j.State, j.LastError)
	}
}

// waitTerminal polls a job driven by an already running engine.
func (e *env) waitTerminal(id string) state.Job {
	e.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		j, err := e.store.GetJob(context.Background(), "owner", id)
		if err != nil {
			e.t.Fatal(err)
		}
		if terminal(j) {
			return j
		}
		time.Sleep(5 * time.Millisecond)
	}
	j, _ := e.store.GetJob(context.Background(), "owner", id)
	e.t.Fatalf("timeout; job = %+v", j)
	return j
}

// runUntil2 starts another engine for the same job (a restart) until pred.
func (e *env) runUntil2(id string, pred func(state.Job) bool) state.Job {
	e.t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	eng := e.engine()
	go func() { eng.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()
	deadline := time.Now().Add(10 * time.Second)
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

// TestSeededCrashMatrixNoDuplicateWrites kills the engine at a seeded point
// (before a write, after a write before commit, after commit) and restarts it.
// The invariants: every source object exists exactly once at the destination,
// and the final job is truthful (succeeded or blocked with a reason).
func TestSeededCrashMatrixNoDuplicateWrites(t *testing.T) {
	if testing.Short() {
		t.Skip("seeded crash matrix is slow; skipped with -short")
	}
	for seed := uint64(0); seed < 5; seed++ {
		t.Run("seed", func(t *testing.T) {
			e := newEnv(t)
			src := e.fake.Folder("Project", "root")
			for i := range 6 {
				e.fake.File("f"+string(rune('a'+i))+".txt", src.ID, 10)
			}
			job := e.submit(src)

			r := rand.New(rand.NewPCG(seed, 0xC0FFEE))
			killAt := 1 + r.IntN(6)
			var calls atomic.Int64
			ctx, kill := context.WithCancel(context.Background())
			defer kill()
			var once sync.Once
			e.fake.Before = func(string, drive.File) error {
				if calls.Add(1) == int64(killAt) {
					once.Do(kill)
				}
				return nil
			}
			eng := e.engine()
			done := make(chan struct{})
			go func() { eng.Run(ctx); close(done) }()
			// Stop at the seeded point, or after the job finished on its own
			// when the seeded call never happened (a shorter tree).
			deadline := time.Now().Add(10 * time.Second)
			for {
				if j, _ := e.store.GetJob(context.Background(), "owner", job.ID); terminal(j) {
					break
				}
				select {
				case <-ctx.Done():
				case <-time.After(5 * time.Millisecond):
				}
				if time.Now().After(deadline) {
					t.Fatalf("seed %d: the first engine neither stopped nor finished", seed)
				}
				if ctx.Err() != nil {
					break
				}
			}
			kill()
			select {
			case <-done:
			case <-time.After(10 * time.Second):
				t.Fatalf("seed %d: the killed engine did not stop", seed)
			}

			// A restart must reconcile unknown writes before sending again.
			e.fake.Before = nil
			j := e.runUntil2(job.ID, terminal)
			if j.State != state.JobSucceeded && j.State != state.JobBlocked {
				t.Fatalf("seed %d: job = %s (%+v)", seed, j.State, j.LastError)
			}
			kids := e.fake.Children(e.dest.ID)
			var rootID string
			for _, k := range kids {
				if k.Name == "Project" {
					if rootID != "" {
						t.Fatalf("seed %d: two root folders at the destination", seed)
					}
					rootID = k.ID
				}
			}
			if rootID == "" {
				t.Fatalf("seed %d: no root folder", seed)
			}
			names := map[string]int{}
			for _, c := range e.fake.Children(rootID) {
				names[c.Name]++
			}
			for i := range 6 {
				name := "f" + string(rune('a'+i)) + ".txt"
				if names[name] != 1 {
					t.Fatalf("seed %d: %s copies = %d", seed, name, names[name])
				}
			}
		})
	}
}

// TestSuspendDuringCopyReconcilesInsteadOfDuplicating models the worst sleep
// window: the copy took effect, the response was lost, and the machine slept
// before the reconcile query could run. After resume the intent must be found
// (one object), and a query that never left the machine must not block the item.
func TestSuspendDuringCopyReconcilesInsteadOfDuplicating(t *testing.T) {
	e := newEnv(t)
	src := e.fake.Folder("Project", "root")
	e.fake.File("a.txt", src.ID, 10)
	job := e.submit(src)

	gate := power.NewGate(true)
	eng := e.engine()
	eng.Network = gate
	eng.OnUnreachable = func(error) { gate.Set(false) }

	var sleepOnce, findOnce sync.Once
	e.fake.LoseResponse = func(op string, f drive.File) bool {
		if op != "copy" {
			return false
		}
		lost := false
		sleepOnce.Do(func() { lost = true; gate.Set(false) }) // suspend right here
		return lost
	}
	e.fake.Before = func(op string, f drive.File) error {
		if op != "find" {
			return nil
		}
		offline := false
		findOnce.Do(func() { offline = true })
		if offline {
			return unreachable() // the machine is asleep: nothing leaves
		}
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { eng.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()

	deadline := time.Now().Add(3 * time.Second)
	waiting := func() bool {
		for _, it := range itemsOf(t, e.store, job.CollectionID) {
			if it.Kind == state.KindFile && it.State == state.ItemReconciling {
				return true
			}
		}
		return false
	}
	for time.Now().Before(deadline) && !waiting() {
		time.Sleep(5 * time.Millisecond)
	}
	items := itemsOf(t, e.store, job.CollectionID)
	var copyItems []state.Item
	for _, it := range items {
		if it.Kind == state.KindFile {
			copyItems = append(copyItems, it)
		}
	}
	if len(copyItems) != 1 || copyItems[0].State != state.ItemReconciling {
		t.Fatalf("the suspended item must wait in reconciling, got %+v", copyItems)
	}
	if got := e.fake.Children(e.fake.Children(e.dest.ID)[0].ID); len(got) != 1 {
		t.Fatalf("copies at the destination before resume = %d", len(got))
	}

	gate.Set(true) // the machine woke and the probe succeeded
	j := e.waitTerminal(job.ID)
	if j.State != state.JobSucceeded {
		t.Fatalf("job = %s (%+v)", j.State, j.LastError)
	}
	root := e.fake.Children(e.dest.ID)[0]
	kids := e.fake.Children(root.ID)
	if len(kids) != 1 || kids[0].Name != "a.txt" {
		t.Fatalf("destination children = %+v", kids)
	}
	for _, it := range itemsOf(t, e.store, job.CollectionID) {
		if it.State != state.ItemDone {
			t.Fatalf("item %s = %s", it.Name, it.State)
		}
	}
}
