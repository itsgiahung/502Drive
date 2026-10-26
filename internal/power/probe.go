package power

import (
	"context"
	"math/rand/v2"
	"sync"
	"time"
)

// Prober repeats a cheap call with exponential backoff and jitter until it
// succeeds. Only one probe runs at a time: a second caller waits for the
// running loop instead of starting another stream of requests.
type Prober struct {
	Probe func(ctx context.Context) error
	Base  time.Duration // first delay (default 1s)
	Max   time.Duration // delay cap (default 30s)
	// Rand returns [0,1) and stretches the delay by ±20% (tests inject 0.5).
	Rand func() float64
	// Sleep waits for d and reports whether to continue (tests inject it).
	Sleep func(ctx context.Context, d time.Duration) bool
	// OnRetry is called after a failed probe, before sleeping.
	OnRetry func(err error, next time.Duration)
	// Permanent marks a failure that needs the user (an invalid grant). Such
	// failures use Cooldown instead of the exponential backoff and never give
	// up, because the fix is a login, not another attempt.
	Permanent func(err error) bool
	// Cooldown is the delay after a permanent failure (default 5m).
	Cooldown time.Duration

	mu sync.Mutex
}

// Until probes until success, ctx ends or a probe reports a permanent error.
// It returns the last probe error when it gives up.
func (p *Prober) Until(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	base, max := p.Base, p.Max
	if base <= 0 {
		base = time.Second
	}
	if max <= 0 {
		max = 30 * time.Second
	}
	randf := p.Rand
	if randf == nil {
		randf = rand.Float64
	}
	sleep := p.Sleep
	if sleep == nil {
		sleep = sleepCtx
	}
	var last error
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			if last != nil {
				return last
			}
			return err
		}
		err := p.Probe(ctx)
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return err
		}
		last = err
		if p.Permanent != nil && p.Cooldown > 0 && p.Permanent(err) {
			if p.OnRetry != nil {
				p.OnRetry(err, p.Cooldown)
			}
			if !sleep(ctx, p.Cooldown) {
				return last
			}
			continue
		}
		d := base << min(attempt, 10)
		if d > max || d <= 0 {
			d = max
		}
		d = jitter(d, randf())
		if p.OnRetry != nil {
			p.OnRetry(err, d)
		}
		if !sleep(ctx, d) {
			return last
		}
	}
}

// jitter stretches d by ±20%.
func jitter(d time.Duration, r float64) time.Duration {
	if r < 0 {
		r = 0
	}
	if r >= 1 {
		r = 0.999
	}
	out := time.Duration(float64(d) * (0.8 + 0.4*r))
	if out <= 0 {
		return d
	}
	return out
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}
