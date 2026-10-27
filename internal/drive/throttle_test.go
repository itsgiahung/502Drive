package drive

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestThrottleGrowsAfterSuccessesAndHalvesOnThrottle(t *testing.T) {
	thr := NewThrottle(ThrottleOptions{Start: 2, Max: 4, Successes: 2})
	if got := thr.Snapshot().WorkersNow; got != 2 {
		t.Fatalf("start = %d", got)
	}
	thr.Observe(nil)
	thr.Observe(nil)
	if got := thr.Snapshot().WorkersNow; got != 3 {
		t.Fatalf("after two successes = %d", got)
	}
	thr.Observe(nil)
	thr.Observe(nil)
	if got := thr.Snapshot().WorkersNow; got != 4 {
		t.Fatalf("after four successes = %d", got)
	}
	thr.Observe(nil)
	thr.Observe(nil)
	if got := thr.Snapshot().WorkersNow; got != 4 {
		t.Fatalf("grew past the ceiling: %d", got)
	}

	thr.Observe(&Error{Kind: KindThrottled, Status: 429})
	if got := thr.Snapshot().WorkersNow; got != 2 {
		t.Fatalf("after a throttle = %d", got)
	}
	thr.Observe(&Error{Kind: KindThrottled, Status: 429})
	thr.Observe(&Error{Kind: KindThrottled, Status: 429})
	if got := thr.Snapshot().WorkersNow; got != 1 {
		t.Fatalf("halving went below one: %d", got)
	}
	// Network, quota, 5xx and permission say nothing about the safe rate.
	for _, k := range []Kind{KindUnavailable, KindServer, KindQuota, KindPermission, KindNotFound} {
		thr.Observe(&Error{Kind: k})
	}
	if got := thr.Snapshot().WorkersNow; got != 1 {
		t.Fatalf("a non-throttle error changed the limit: %d", got)
	}
}

func TestThrottleAcquireWaitsForTheLimit(t *testing.T) {
	thr := NewThrottle(ThrottleOptions{Start: 1, Max: 1})
	ctx := context.Background()
	if err := thr.Acquire(ctx); err != nil {
		t.Fatal(err)
	}
	got := make(chan error, 1)
	go func() {
		if err := thr.Acquire(context.Background()); err != nil {
			got <- err
			return
		}
		thr.Release()
		got <- nil
	}()
	select {
	case err := <-got:
		t.Fatalf("second acquire did not wait: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	thr.Release()
	select {
	case err := <-got:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("the waiter was never woken")
	}
}

func TestThrottleAcquireHonoursCancellation(t *testing.T) {
	thr := NewThrottle(ThrottleOptions{Start: 1, Max: 1})
	if err := thr.Acquire(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- thr.Acquire(ctx) }()
	cancel()
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Fatalf("err = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancel did not release the waiter")
	}
}

func TestThrottleReadPacesABurst(t *testing.T) {
	thr := NewThrottle(ThrottleOptions{Start: 1, Max: 1, ReadPerSecond: 20})
	ctx := context.Background()
	start := time.Now()
	// The bucket starts full: the first 20 reads pass immediately, the 21st
	// waits about 1/20 s.
	for range 21 {
		if err := thr.Read(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if elapsed := time.Since(start); elapsed < 20*time.Millisecond {
		t.Fatalf("the burst was not paced: %v", elapsed)
	}
}

func TestThrottleReadCanBeDisabled(t *testing.T) {
	thr := NewThrottle(ThrottleOptions{Start: 1, Max: 1, ReadPerSecond: 0})
	ctx := context.Background()
	start := time.Now()
	for range 100 {
		if err := thr.Read(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("pacing was not disabled: %v", elapsed)
	}
}

func TestThrottleSnapshotCountsOnlyTheLastFiveMinutes(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	var mu sync.Mutex
	thr := NewThrottle(ThrottleOptions{Start: 2, Max: 4, Now: func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return now
	}})
	thr.Observe(&Error{Kind: KindThrottled, RetryAfter: 2 * time.Minute})
	thr.Observe(&Error{Kind: KindThrottled})
	snap := thr.Snapshot()
	if snap.ThrottledLast5m != 2 {
		t.Fatalf("throttles = %d", snap.ThrottledLast5m)
	}
	if snap.NextRetryAt == nil || !snap.NextRetryAt.Equal(now.Add(2*time.Minute)) {
		t.Fatalf("next retry = %v", snap.NextRetryAt)
	}
	mu.Lock()
	now = now.Add(3 * time.Minute)
	mu.Unlock()
	snap = thr.Snapshot()
	if snap.ThrottledLast5m != 2 || snap.NextRetryAt != nil {
		// The hint has passed, but the event is still inside the 5m window.
		t.Fatalf("inside the window: %+v", snap)
	}
	mu.Lock()
	now = now.Add(3 * time.Minute)
	mu.Unlock()
	snap = thr.Snapshot()
	if snap.ThrottledLast5m != 0 || snap.NextRetryAt != nil {
		t.Fatalf("outside the window: %+v", snap)
	}
}
