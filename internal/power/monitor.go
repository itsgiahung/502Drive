package power

import (
	"context"
	"sync"
	"time"
)

// MonitorOptions configures the event normalizer.
type MonitorOptions struct {
	// Tick is how often the clock gap is sampled (default 5s).
	Tick time.Duration
	// Threshold is the gap size that counts as a sleep (default 30s).
	Threshold time.Duration
	// Flap is how long a duplicate resume after a resume is ignored (default 2s).
	Flap time.Duration
	// Wall and Mono are injectable clocks; Mono must advance while the machine
	// is awake and must not advance while it sleeps.
	Wall func() time.Time
	Mono func() time.Duration
	// Ticks replaces the internal ticker (tests only).
	Ticks <-chan time.Time
}

// Monitor normalizes OS events and clock gaps into the daemon's stream: one
// Suspend per sleep, one Resume per wake (with the measured gap), and a Resume
// for a gap that no OS event reported. Duplicate events and flapping are
// dropped; a dropped suspend releases its delay lock immediately.
type Monitor struct {
	src   Source
	gap   *Gap
	tick  time.Duration
	flap  time.Duration
	ticks <-chan time.Time
	now   func() time.Time

	mu         sync.Mutex
	suspended  bool
	lastChange time.Time
}

// NewMonitor wraps a source. A nil source is allowed: only gaps are reported.
func NewMonitor(src Source, o MonitorOptions) *Monitor {
	if o.Tick <= 0 {
		o.Tick = 5 * time.Second
	}
	if o.Threshold <= 0 {
		o.Threshold = 30 * time.Second
	}
	if o.Wall == nil {
		o.Wall = time.Now
	}
	if o.Mono == nil {
		o.Mono = func() time.Duration { return time.Since(processStart) }
	}
	m := &Monitor{src: src, tick: o.Tick, flap: o.Flap, ticks: o.Ticks, now: o.Wall,
		gap: NewGap(o.Threshold, o.Wall, o.Mono)}
	// Seed both clocks now so the very first tick can already report a gap.
	m.gap.Step()
	return m
}

// Run reads the source until ctx ends and closes the returned channel when it
// stops. The channel is buffered so a slow consumer cannot block an OS
// callback; when the buffer is full a suspend is acknowledged and dropped,
// because the clock gap still detects the sleep.
func (m *Monitor) Run(ctx context.Context) <-chan Event {
	out := make(chan Event, 8)
	var src <-chan Event
	if m.src != nil {
		src = m.src.Events()
	}
	ticks := m.ticks
	if ticks == nil {
		t := time.NewTicker(m.tick)
		defer t.Stop()
		ticks = t.C
	}
	go func() {
		defer close(out)
		defer func() {
			if m.src != nil {
				m.src.Close()
			}
		}()
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-src:
				if !ok {
					src = nil // a closed source must not spin
					continue
				}
				m.forward(out, ev)
			case <-ticks:
				m.checkGap(out)
			}
		}
	}()
	return out
}

func (m *Monitor) forward(out chan<- Event, ev Event) {
	m.mu.Lock()
	switch ev.Kind {
	case Suspend:
		if m.suspended {
			m.mu.Unlock()
			ev.Acknowledge()
			return
		}
		m.suspended, m.lastChange = true, m.now()
		m.mu.Unlock()
		m.send(out, ev)
	case Resume:
		if !m.suspended {
			m.mu.Unlock()
			return // flap: a resume without a suspend
		}
		gap := m.now().Sub(m.lastChange)
		m.suspended, m.lastChange = false, m.now()
		m.mu.Unlock()
		if gap < 0 {
			gap = 0
		}
		ev.Gap = gap
		m.send(out, ev)
	case NetworkChanged, SessionLock, SessionUnlock, Shutdown:
		m.mu.Unlock()
		m.send(out, ev)
	default:
		m.mu.Unlock()
	}
}

// checkGap emits a Resume when the clocks show a sleep that no OS event
// reported; it also covers a wake event that was lost while the machine slept.
func (m *Monitor) checkGap(out chan<- Event) {
	gap, ok := m.gap.Step()
	if !ok {
		return
	}
	m.mu.Lock()
	m.suspended, m.lastChange = false, m.now()
	m.mu.Unlock()
	if gap <= 0 {
		return
	}
	m.send(out, Event{Kind: Resume, At: m.now(), Gap: gap})
}

func (m *Monitor) send(out chan<- Event, ev Event) {
	select {
	case out <- ev:
	default:
		ev.Acknowledge()
	}
}

// processStart anchors the monotonic clock used by the default options.
var processStart = time.Now()
