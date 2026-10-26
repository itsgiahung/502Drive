package cli

import (
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// stopDaemonFake answers /v1/status, /v1/shutdown and /health/live the way a
// real daemon does: the shutdown is accepted first and liveness stops a
// moment later (unless keepAlive stays set).
type stopDaemonFake struct {
	jobs      int32 // running+queued reported by /v1/status
	stopping  atomic.Bool
	keepAlive bool // liveness keeps answering although a shutdown was accepted
	// started records the moment the first /health/live probe was refused,
	// so tests can assert that the CLI really waited.
	refused atomic.Int64
}

func (f *stopDaemonFake) handler(t *testing.T) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/health/live":
			if f.stopping.Load() && !f.keepAlive {
				f.refused.CompareAndSwap(0, time.Now().UnixNano())
				w.WriteHeader(404)
				return
			}
			w.WriteHeader(200)
		case r.Method == "GET" && r.URL.Path == "/v1/status":
			reply(w, 200, map[string]any{"running_jobs": f.jobs, "queued_jobs": 0, "ready": true})
		case r.Method == "POST" && r.URL.Path == "/v1/shutdown":
			if !f.stopping.CompareAndSwap(false, true) {
				t.Errorf("shutdown was posted twice")
			}
			reply(w, 202, map[string]any{"status": "stopping"})
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(500)
		}
	})
}

func TestStopWithoutJobsStopsAtOnce(t *testing.T) {
	f := &stopDaemonFake{}
	code, out, _ := driveCLI(t, f.handler(t), "stop")
	if code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out, "stopped") {
		t.Fatalf("out = %q", out)
	}
	if f.refused.Load() == 0 {
		t.Fatal("the CLI did not wait for liveness to stop")
	}
}

func TestStopNeedsYesWhileJobsRunWithoutATerminal(t *testing.T) {
	f := &stopDaemonFake{jobs: 2}
	code, _, errOut := driveCLI(t, f.handler(t), "stop")
	if code != ExitUsage {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(errOut, "--yes") {
		t.Fatalf("err = %q", errOut)
	}
	if f.stopping.Load() {
		t.Fatal("the daemon was stopped without confirmation")
	}
}

func TestStopWithYesWhileJobsRun(t *testing.T) {
	f := &stopDaemonFake{jobs: 2}
	code, out, _ := driveCLI(t, f.handler(t), "stop", "--yes")
	if code != ExitOK || !strings.Contains(out, "stopped") {
		t.Fatalf("exit = %d, out = %q", code, out)
	}
}

func TestStopAsksInTheTerminal(t *testing.T) {
	f := &stopDaemonFake{jobs: 1}
	code, out, _ := bulkCLI(t, f.handler(t), "n\n", "stop")
	if code != ExitOK || !strings.Contains(out, "aborted") {
		t.Fatalf("exit = %d, out = %q", code, out)
	}
	if f.stopping.Load() {
		t.Fatal("answering no still stopped the daemon")
	}
	code, _, _ = bulkCLI(t, f.handler(t), "y\n", "stop")
	if code != ExitOK || !f.stopping.Load() {
		t.Fatalf("exit = %d, answering yes did not stop the daemon", code)
	}
}

func TestStopJSON(t *testing.T) {
	f := &stopDaemonFake{}
	code, out, _ := driveCLI(t, f.handler(t), "stop", "--json")
	if code != ExitOK || !strings.Contains(out, `"daemon":"stopped"`) {
		t.Fatalf("exit = %d, out = %q", code, out)
	}
}

func TestStopWhenTheDaemonIsDown(t *testing.T) {
	f := &stopDaemonFake{}
	f.stopping.Store(true) // liveness already refuses
	code, out, _ := driveCLI(t, f.handler(t), "stop")
	if code != ExitOK || !strings.Contains(out, "not running") {
		t.Fatalf("exit = %d, out = %q", code, out)
	}
}

func TestStopReportsADaemonThatKeepsRunning(t *testing.T) {
	old := daemonStopWait
	daemonStopWait = 200 * time.Millisecond
	t.Cleanup(func() { daemonStopWait = old })
	f := &stopDaemonFake{keepAlive: true}
	code, _, errOut := driveCLI(t, f.handler(t), "stop", "--yes")
	if code != ExitUnavailable {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(errOut, "did not stop") {
		t.Fatalf("err = %q", errOut)
	}
}
