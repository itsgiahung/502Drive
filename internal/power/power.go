// Package power turns OS power/network notifications and clock gaps into one
// stream of lifecycle events, and gives the engine an online gate. Platform
// code lives in files with build tags; the rest is pure Go and testable.
package power

import (
	"time"
)

// Kind is one lifecycle event.
type Kind int

const (
	Suspend Kind = iota + 1
	Resume
	NetworkChanged
	SessionLock
	SessionUnlock
	Shutdown
)

func (k Kind) String() string {
	switch k {
	case Suspend:
		return "suspend"
	case Resume:
		return "resume"
	case NetworkChanged:
		return "network_changed"
	case SessionLock:
		return "session_lock"
	case SessionUnlock:
		return "session_unlock"
	case Shutdown:
		return "shutdown"
	}
	return "unknown"
}

// Event is what the monitor hands to the daemon.
type Event struct {
	Kind Kind
	At   time.Time
	// Gap is how long the machine was away, for a resume caused by a clock gap.
	Gap time.Duration
	// Ack releases an OS delay lock. The daemon calls it once its checkpoint is
	// done; a dropped event is acknowledged immediately. It is idempotent.
	Ack func()
}

// Acknowledge releases the OS delay lock, if the source took one.
func (e Event) Acknowledge() {
	if e.Ack != nil {
		e.Ack()
	}
}

// Source reports what the OS tells us. A Source that knows nothing is empty;
// the monitor still detects sleeps from the clock.
type Source interface {
	Events() <-chan Event
	Close() error
}

// EmptySource is the fallback when the OS offers no notifications.
type EmptySource struct{}

func (EmptySource) Events() <-chan Event { return nil }
func (EmptySource) Close() error         { return nil }

// Gap measures wall-clock time that passed while the process was not running
// (sleep, hibernate, a frozen container). It compares the wall clock with the
// monotonic clock, so it needs no OS support and works everywhere.
type Gap struct {
	Threshold time.Duration
	Wall      func() time.Time
	Mono      func() time.Duration

	lastWall time.Time
	lastMono time.Duration
	started  bool
}

// NewGap returns a detector; threshold defaults to 30s.
func NewGap(threshold time.Duration, wall func() time.Time, mono func() time.Duration) *Gap {
	if threshold <= 0 {
		threshold = 30 * time.Second
	}
	return &Gap{Threshold: threshold, Wall: wall, Mono: mono}
}

// Step samples the clocks and reports a gap when the wall clock moved further
// than the monotonic clock by at least Threshold. A backward wall step (NTP,
// a manual clock change) is not a gap.
func (g *Gap) Step() (time.Duration, bool) {
	wall, mono := g.Wall(), g.Mono()
	if !g.started {
		g.lastWall, g.lastMono, g.started = wall, mono, true
		return 0, false
	}
	wallDelta := wall.Sub(g.lastWall)
	monoDelta := mono - g.lastMono
	// Keep the newest sample even when it looks like a clock step: the next
	// check should measure from here, not accumulate a bogus offset.
	g.lastWall, g.lastMono = wall, mono
	if wallDelta <= 0 {
		return 0, false
	}
	gap := wallDelta - monoDelta
	if gap < g.Threshold {
		return 0, false
	}
	return gap, true
}
