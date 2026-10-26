package power

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestGapDetectsSleepFromWallAndMonotonicClocks(t *testing.T) {
	wall := time.Date(2026, 10, 27, 8, 0, 0, 0, time.UTC)
	mono := time.Duration(0)
	g := NewGap(30*time.Second, func() time.Time { return wall }, func() time.Duration { return mono })

	// A normal 5s tick: wall and monotonic move together, no gap.
	wall, mono = wall.Add(5*time.Second), mono+5*time.Second
	if d, ok := g.Step(); ok {
		t.Fatalf("a normal tick reported a gap: %v", d)
	}

	// Eight hours of sleep: the wall clock moves, the monotonic clock does not.
	wall = wall.Add(8 * time.Hour)
	if d, ok := g.Step(); !ok || d < 8*time.Hour-time.Second {
		t.Fatalf("gap = %v, %v", d, ok)
	}

	// A backward wall-clock step (NTP/manual) is not a sleep.
	wall = wall.Add(-time.Hour)
	mono += 5 * time.Second
	if d, ok := g.Step(); ok {
		t.Fatalf("a backward clock step was reported as a gap: %v", d)
	}
}

func TestGapIgnoresShortSuspendsBelowThreshold(t *testing.T) {
	wall := time.Unix(1_000_000, 0)
	mono := time.Duration(0)
	g := NewGap(30*time.Second, func() time.Time { return wall }, func() time.Duration { return mono })
	// A container freeze of 10s is below the threshold and must not look like sleep.
	wall = wall.Add(10 * time.Second)
	if d, ok := g.Step(); ok {
		t.Fatalf("10s freeze reported as a gap: %v", d)
	}
}

func TestMonitorForwardsSuspendThenResumeOnce(t *testing.T) {
	src := newFakeSource()
	wall := time.Unix(1_700_000_000, 0)
	m := NewMonitor(src, MonitorOptions{Tick: time.Hour, Threshold: 30 * time.Second,
		Wall: func() time.Time { return wall }, Mono: func() time.Duration { return 0 }})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out := m.Run(ctx)

	acked := 0
	src.send(Event{Kind: Suspend, Ack: func() { acked++ }})
	src.send(Event{Kind: Suspend, Ack: func() { acked++ }}) // duplicate from the OS
	src.send(Event{Kind: Resume})
	src.send(Event{Kind: Resume}) // flap

	got := drain(t, out, 2)
	if got[0].Kind != Suspend || got[1].Kind != Resume {
		t.Fatalf("events = %v", kinds(got))
	}
	if acked != 1 {
		t.Fatalf("dropped duplicate suspend did not release its lock: acked=%d", acked)
	}
	select {
	case ev := <-out:
		t.Fatalf("unexpected extra event %v", ev.Kind)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestMonitorReportsAGapAsResumeWithoutAnOSEvent(t *testing.T) {
	src := newFakeSource()
	wall := time.Unix(1_700_000_000, 0)
	mono := time.Duration(0)
	ticks := make(chan time.Time)
	m := NewMonitor(src, MonitorOptions{Tick: time.Hour, Threshold: 30 * time.Second, Ticks: ticks,
		Wall: func() time.Time { return wall }, Mono: func() time.Duration { return mono }})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out := m.Run(ctx)

	// The machine slept without any event: the wall clock jumped, mono did not.
	wall = wall.Add(3 * time.Hour)
	ticks <- wall
	ev := drain(t, out, 1)[0]
	if ev.Kind != Resume || ev.Gap < 3*time.Hour-time.Minute {
		t.Fatalf("event = %v gap=%v", ev.Kind, ev.Gap)
	}
}

func TestMonitorForwardsNetworkChangesEvenWhileAwake(t *testing.T) {
	src := newFakeSource()
	m := NewMonitor(src, MonitorOptions{Tick: time.Hour})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out := m.Run(ctx)
	src.send(Event{Kind: NetworkChanged})
	if got := drain(t, out, 1)[0]; got.Kind != NetworkChanged {
		t.Fatalf("event = %v", got.Kind)
	}
}

func TestGateWaitIsReleasedByTransitions(t *testing.T) {
	g := NewGate(false)
	if g.Online() {
		t.Fatal("a new closed gate reports online")
	}
	done := make(chan bool, 1)
	go func() { done <- g.Wait(context.Background()) }()
	select {
	case <-done:
		t.Fatal("Wait returned while offline")
	case <-time.After(20 * time.Millisecond):
	}
	g.Set(true)
	select {
	case ok := <-done:
		if !ok {
			t.Fatal("Wait returned false after the gate opened")
		}
	case <-time.After(time.Second):
		t.Fatal("Wait was not released by the transition")
	}
	// Offline again: Wait must block until the context ends.
	g.Set(false)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if g.Wait(ctx) {
		t.Fatal("Wait returned true on an offline gate")
	}
}

func TestProberBacksOffAndNeverRunsTwoProbesAtOnce(t *testing.T) {
	var mu sync.Mutex
	active, maxActive, calls := 0, 0, 0
	var delays []time.Duration
	p := &Prober{
		Base: 100 * time.Millisecond, Max: time.Second, Rand: func() float64 { return 0.5 },
		Probe: func(context.Context) error {
			mu.Lock()
			active++
			calls++
			if active > maxActive {
				maxActive = active
			}
			n := calls
			mu.Unlock()
			time.Sleep(time.Millisecond)
			mu.Lock()
			active--
			mu.Unlock()
			if n < 3 {
				return errProbe
			}
			return nil
		},
		Sleep: func(_ context.Context, d time.Duration) bool {
			mu.Lock()
			delays = append(delays, d)
			mu.Unlock()
			return true
		},
	}
	if err := p.Until(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 3 || maxActive != 1 {
		t.Fatalf("calls=%d maxActive=%d", calls, maxActive)
	}
	want := []time.Duration{100 * time.Millisecond, 200 * time.Millisecond}
	for i := range want {
		if delays[i] != want[i] {
			t.Fatalf("delays = %v, want %v", delays, want)
		}
	}
}

func TestProberStopsWhenTheContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	p := &Prober{Probe: func(context.Context) error { return errProbe }, Base: time.Millisecond,
		Sleep: func(context.Context, time.Duration) bool { cancel(); return true }}
	if err := p.Until(ctx); err == nil {
		t.Fatal("Until returned nil after cancellation")
	}
}

var errProbe = &probeError{}

type probeError struct{}

func (*probeError) Error() string { return "probe failed" }

// ---- fakes ----

type fakeSource struct {
	ch chan Event
}

func newFakeSource() *fakeSource           { return &fakeSource{ch: make(chan Event, 8)} }
func (f *fakeSource) Events() <-chan Event { return f.ch }
func (f *fakeSource) Close() error         { return nil }
func (f *fakeSource) send(e Event)         { f.ch <- e }

func drain(t *testing.T, ch <-chan Event, n int) []Event {
	t.Helper()
	out := make([]Event, 0, n)
	deadline := time.After(2 * time.Second)
	for len(out) < n {
		select {
		case ev := <-ch:
			out = append(out, ev)
		case <-deadline:
			t.Fatalf("only %d/%d events arrived: %v", len(out), n, kinds(out))
		}
	}
	return out
}

func kinds(evs []Event) []Kind {
	out := make([]Kind, len(evs))
	for i, e := range evs {
		out[i] = e.Kind
	}
	return out
}
