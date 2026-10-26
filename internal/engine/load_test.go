package engine

import (
	"context"
	"fmt"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GiaHung07/502Drive/internal/drive/drivetest"
	"github.com/GiaHung07/502Drive/internal/state"
)

// TestLoadTenThousandFiles is a light load probe: a synthetic tree of 10 000
// files in 500 folders with 4 workers. It checks truth (every node copied
// once), control latency while the engine is busy, and that the heap does not
// balloon. It does not measure Google Drive throughput.
//
// Skip with -short.
func TestLoadTenThousandFiles(t *testing.T) {
	if testing.Short() {
		t.Skip("load test: 10 000 files")
	}
	if raceDetector {
		// The race detector makes this probe roughly 80x slower (minutes), so the
		// suite skips it; run it without -race to get the measurements.
		t.Skip("load probe is not run under -race; use go test -run TestLoadTenThousandFiles ./internal/engine")
	}
	const folders, perFolder = 500, 20
	const wantNodes = 1 + folders + folders*perFolder

	e := newEnv(t)
	src := e.fake.Folder("Big", "root")
	for f := range folders {
		dir := e.fake.Folder(fmt.Sprintf("d%03d", f), src.ID)
		for i := range perFolder {
			e.fake.File(fmt.Sprintf("f%02d.bin", i), dir.ID, 16)
		}
	}
	job := e.submit(src)

	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	// Poll GetJob like a client would while the engine is busy.
	probeCtx, stopProbe := context.WithCancel(context.Background())
	var maxGet atomic.Int64
	var probes atomic.Int64
	go func() {
		for probeCtx.Err() == nil {
			start := time.Now()
			if _, err := e.store.GetJob(context.Background(), "owner", job.ID); err == nil {
				if d := int64(time.Since(start)); d > maxGet.Load() {
					maxGet.Store(d)
				}
				probes.Add(1)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()
	defer stopProbe()

	runCtx, stopEngine := context.WithCancel(context.Background())
	engineDone := make(chan struct{})
	go func() {
		defer close(engineDone)
		e.engine().Run(runCtx)
	}()
	defer func() {
		stopEngine()
		<-engineDone
	}()

	started := time.Now()
	wait := func(timeout time.Duration, states ...string) state.Job {
		t.Helper()
		deadline := time.Now().Add(timeout)
		for time.Now().Before(deadline) {
			current, err := e.store.GetJob(context.Background(), "owner", job.ID)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range states {
				if current.State == want {
					return current
				}
			}
			time.Sleep(10 * time.Millisecond)
		}
		current, _ := e.store.GetJob(context.Background(), "owner", job.ID)
		t.Fatalf("timeout waiting for %v; job = %+v", states, current)
		return state.Job{}
	}

	running := wait(30*time.Second, state.JobRunning)
	// A control write must answer quickly while copying.
	pauseStart := time.Now()
	paused, pauseErr := e.store.Action(context.Background(), "owner", "load-pause", job.ID, "pause", running.Revision)
	pauseLatency := time.Since(pauseStart)
	if pauseErr != nil && !state.IsConflict(pauseErr) {
		t.Fatalf("pause: %v", pauseErr)
	}
	if pauseErr == nil {
		if paused.Job.State == state.JobPaused {
			if _, err := e.store.Action(context.Background(), "owner", "load-resume", job.ID, "resume", paused.Job.Revision); err != nil {
				t.Fatalf("resume: %v", err)
			}
		} else {
			stopped := wait(30*time.Second, state.JobPaused, state.JobSucceeded)
			if stopped.State == state.JobPaused {
				if _, err := e.store.Action(context.Background(), "owner", "load-resume", job.ID, "resume", stopped.Revision); err != nil {
					t.Fatalf("resume after pausing: %v", err)
				}
			}
		}
	}

	final := wait(8*time.Minute, state.JobSucceeded, state.JobPartial, state.JobFailed, state.JobBlocked)
	elapsed := time.Since(started)
	stopProbe()
	if final.State != state.JobSucceeded {
		t.Fatalf("state %s err %+v counts %+v", final.State, final.LastError, final.Counts)
	}

	copies := e.fake.Children(e.dest.ID)
	if len(copies) != 1 {
		t.Fatalf("destination children = %d", len(copies))
	}
	got := e.fake.Count(copies[0].ID)
	if got != wantNodes {
		t.Fatalf("copied nodes = %d, want %d", got, wantNodes)
	}
	// Every name appears once per destination folder: a retried write must
	// never have created a second copy.
	if dup := duplicateChild(e.fake, copies[0].ID, ""); dup != "" {
		t.Fatalf("duplicate destination node: %s", dup)
	}

	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	heapDelta := int64(after.HeapAlloc) - int64(before.HeapAlloc)

	t.Logf("copied %d nodes in %s (%.0f items/s)", wantNodes, elapsed.Round(time.Millisecond),
		float64(wantNodes)/elapsed.Seconds())
	t.Logf("GetJob while busy: %d probes, max %s; pause action %s", probes.Load(),
		time.Duration(maxGet.Load()).Round(time.Millisecond), pauseLatency.Round(time.Millisecond))
	t.Logf("heap: before %d MiB, after %d MiB, delta %d MiB", before.HeapAlloc>>20, after.HeapAlloc>>20, heapDelta>>20)
	if maxGet.Load() > int64(200*time.Millisecond) {
		t.Fatalf("GetJob took %s while copying (limit 200ms)", time.Duration(maxGet.Load()))
	}
	if pauseErr == nil && pauseLatency > 200*time.Millisecond {
		t.Fatalf("pause action took %s (limit 200ms)", pauseLatency)
	}
	if heapDelta > 200<<20 {
		t.Fatalf("heap grew %d MiB (limit 200 MiB)", heapDelta>>20)
	}
}

// duplicateChild returns the first path whose folder has two children with the
// same name.
func duplicateChild(f *drivetest.Fake, id, prefix string) string {
	seen := map[string]bool{}
	for _, c := range f.Children(id) {
		if seen[c.Name] {
			return prefix + "/" + c.Name
		}
		seen[c.Name] = true
		if c.IsFolder() {
			if dup := duplicateChild(f, c.ID, prefix+"/"+c.Name); dup != "" {
				return dup
			}
		}
	}
	return ""
}
