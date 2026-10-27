package drive

import (
	"context"
	"sync"
	"time"
)

// Throttle is the account-wide concurrency and read pacing controller.
//
// Concurrency follows AIMD: it starts at Start (the configured worker count),
// grows by one after Successes consecutive successful calls and halves on a
// throttle (429/rateLimitExceeded), never below one and never above Max. Reads
// (list, batched list, get, changes, quota) pass through a token bucket so a
// burst of folders cannot spend the account's per-second quota.
//
// One Throttle must be shared by every component that talks to the same
// Google account (engine, syncer, audits); that is what makes it account-wide.
type Throttle struct {
	mu        sync.Mutex
	limit     int
	start     int
	max       int
	successes int
	streak    int
	inflight  int

	rate   float64
	burst  float64
	tokens float64
	last   time.Time

	throttled []time.Time
	nextRetry time.Time
	now       func() time.Time
	wake      chan struct{}
}

// ThrottleOptions configures a Throttle. Zero values are safe: Start 1, Max =
// Start, Successes 8, rate 0 (no read pacing).
type ThrottleOptions struct {
	Start         int
	Max           int
	Successes     int
	ReadPerSecond float64
	Now           func() time.Time
}

// ThrottleSnapshot is the read-only view exposed by /v1/status.
type ThrottleSnapshot struct {
	WorkersNow      int
	ThrottledLast5m int
	NextRetryAt     *time.Time
}

func NewThrottle(o ThrottleOptions) *Throttle {
	if o.Start < 1 {
		o.Start = 1
	}
	if o.Max < o.Start {
		o.Max = o.Start
	}
	if o.Successes < 1 {
		o.Successes = 8
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	// The bucket must hold at least one token or a valid rate below 1 request
	// per second never accumulates enough to let a read through.
	burst := o.ReadPerSecond
	if o.ReadPerSecond > 0 && burst < 1 {
		burst = 1
	}
	t := &Throttle{limit: o.Start, start: o.Start, max: o.Max, successes: o.Successes,
		rate: o.ReadPerSecond, burst: burst, tokens: burst, last: o.Now(), now: o.Now,
		wake: make(chan struct{}, 1)}
	return t
}

// signal wakes callers waiting for a concurrency slot without blocking.
func (t *Throttle) signal() {
	select {
	case t.wake <- struct{}{}:
	default:
	}
}

// Acquire takes one concurrency slot, waiting while the current AIMD limit is
// reached. It returns ctx.Err() if the caller gives up first.
func (t *Throttle) Acquire(ctx context.Context) error {
	for {
		t.mu.Lock()
		if t.inflight < t.limit {
			t.inflight++
			t.mu.Unlock()
			return nil
		}
		t.mu.Unlock()
		select {
		case <-t.wake:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// Release returns a concurrency slot.
func (t *Throttle) Release() {
	t.mu.Lock()
	if t.inflight > 0 {
		t.inflight--
	}
	t.mu.Unlock()
	t.signal()
}

// Read waits for one token of the read budget. A non-positive rate disables
// pacing (tests, maintenance).
func (t *Throttle) Read(ctx context.Context) error {
	if t.rate <= 0 {
		return nil
	}
	for {
		t.mu.Lock()
		now := t.now()
		if elapsed := now.Sub(t.last); elapsed > 0 {
			t.tokens = min(t.burst, t.tokens+elapsed.Seconds()*t.rate)
			t.last = now
		}
		if t.tokens >= 1 {
			t.tokens--
			t.mu.Unlock()
			return nil
		}
		wait := time.Duration((1 - t.tokens) / t.rate * float64(time.Second))
		t.mu.Unlock()
		if wait < time.Millisecond {
			wait = time.Millisecond
		}
		timer := time.NewTimer(wait)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		}
	}
}

// Observe records the outcome of one provider call. Only a throttled call
// halves the concurrency; every other outcome either counts toward the next
// increase or leaves the limit alone (permission, quota, network and 5xx are
// not evidence about the safe request rate).
func (t *Throttle) Observe(err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if err == nil {
		t.streak++
		if t.streak >= t.successes {
			t.streak = 0
			if t.limit < t.max {
				t.limit++
				t.signal()
			}
		}
		return
	}
	if KindOf(err) != KindThrottled {
		return
	}
	t.limit /= 2
	if t.limit < 1 {
		t.limit = 1
	}
	t.streak = 0
	now := t.now()
	t.throttled = append(t.throttled, now)
	if d := RetryAfterOf(err); d > 0 {
		t.nextRetry = now.Add(d)
	}
	t.signal()
}

// Snapshot reports the current limit and the throttle events of the last five
// minutes. next_retry_at is only non-nil while a provider hint is still in the
// future.
func (t *Throttle) Snapshot() ThrottleSnapshot {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	cutoff := now.Add(-5 * time.Minute)
	drop := 0
	for drop < len(t.throttled) && !t.throttled[drop].After(cutoff) {
		drop++
	}
	if drop > 0 {
		t.throttled = append([]time.Time(nil), t.throttled[drop:]...)
	}
	var next *time.Time
	if t.nextRetry.After(now) {
		n := t.nextRetry
		next = &n
	}
	return ThrottleSnapshot{WorkersNow: t.limit, ThrottledLast5m: len(t.throttled), NextRetryAt: next}
}

// Paced wraps a provider so its reads share one token bucket and every call
// feeds the AIMD controller. The wrapper does not take concurrency slots: the
// engine already does that around the calls it makes, and doing it twice would
// halve the effective worker count.
//
// Optional interfaces survive the wrap exactly: the engine asserts BatchLister
// for F1 batching and deep preflight asserts QuotaReader, so a wrapper that
// hid them would silently disable both when the daemon wraps its Client.
func Paced(p Provider, t *Throttle) Provider {
	if p == nil || t == nil {
		return p
	}
	base := pacedProvider{Provider: p, t: t}
	batch, isBatch := p.(BatchLister)
	quota, isQuota := p.(QuotaReader)
	switch {
	case isBatch && isQuota:
		return pacedProviderBatchQuota{pacedProvider: base, batch: batch, quota: quota}
	case isBatch:
		return pacedProviderBatch{pacedProvider: base, batch: batch}
	case isQuota:
		return pacedProviderQuota{pacedProvider: base, quota: quota}
	default:
		return base
	}
}

type pacedProvider struct {
	Provider
	t *Throttle
}

// pacedProviderBatch, pacedProviderQuota and pacedProviderBatchQuota are the
// same wrapper plus the optional interfaces the inner provider really has.
type pacedProviderBatch struct {
	pacedProvider
	batch BatchLister
}

type pacedProviderQuota struct {
	pacedProvider
	quota QuotaReader
}

type pacedProviderBatchQuota struct {
	pacedProviderBatch
	quota QuotaReader
}

func (p pacedProviderBatch) ListChildrenBatch(ctx context.Context, parents []Parent, token string) ([]File, string, error) {
	return p.listBatch(ctx, p.batch, parents, token)
}

func (p pacedProviderQuota) StorageQuota(ctx context.Context) (int64, int64, error) {
	return p.storageQuota(ctx, p.quota)
}

func (p pacedProviderBatchQuota) StorageQuota(ctx context.Context) (int64, int64, error) {
	return p.storageQuota(ctx, p.quota)
}

func (p pacedProvider) listBatch(ctx context.Context, bl BatchLister, parents []Parent, token string) ([]File, string, error) {
	if err := p.read(ctx); err != nil {
		return nil, "", err
	}
	files, next, err := bl.ListChildrenBatch(ctx, parents, token)
	p.t.Observe(err)
	return files, next, err
}

func (p pacedProvider) storageQuota(ctx context.Context, q QuotaReader) (int64, int64, error) {
	if err := p.read(ctx); err != nil {
		return 0, 0, err
	}
	limit, usage, err := q.StorageQuota(ctx)
	p.t.Observe(err)
	return limit, usage, err
}

func (p pacedProvider) read(ctx context.Context) error { return p.t.Read(ctx) }

func (p pacedProvider) GetFile(ctx context.Context, id, key string) (File, error) {
	if err := p.read(ctx); err != nil {
		return File{}, err
	}
	f, err := p.Provider.GetFile(ctx, id, key)
	p.t.Observe(err)
	return f, err
}

func (p pacedProvider) ListChildren(ctx context.Context, folder, key, token string) ([]File, string, error) {
	if err := p.read(ctx); err != nil {
		return nil, "", err
	}
	f, next, err := p.Provider.ListChildren(ctx, folder, key, token)
	p.t.Observe(err)
	return f, next, err
}

func (p pacedProvider) FindByIntent(ctx context.Context, parent, intent string) ([]File, error) {
	if err := p.read(ctx); err != nil {
		return nil, err
	}
	f, err := p.Provider.FindByIntent(ctx, parent, intent)
	p.t.Observe(err)
	return f, err
}

func (p pacedProvider) FindChildrenByName(ctx context.Context, parent, name string) ([]File, error) {
	if err := p.read(ctx); err != nil {
		return nil, err
	}
	f, err := p.Provider.FindChildrenByName(ctx, parent, name)
	p.t.Observe(err)
	return f, err
}

func (p pacedProvider) Search(ctx context.Context, query string, limit int) ([]File, error) {
	if err := p.read(ctx); err != nil {
		return nil, err
	}
	f, err := p.Provider.Search(ctx, query, limit)
	p.t.Observe(err)
	return f, err
}

func (p pacedProvider) StartPageToken(ctx context.Context) (string, error) {
	if err := p.read(ctx); err != nil {
		return "", err
	}
	token, err := p.Provider.StartPageToken(ctx)
	p.t.Observe(err)
	return token, err
}

func (p pacedProvider) ListChanges(ctx context.Context, pageToken string) ([]Change, string, string, error) {
	if err := p.read(ctx); err != nil {
		return nil, "", "", err
	}
	changes, next, start, err := p.Provider.ListChanges(ctx, pageToken)
	p.t.Observe(err)
	return changes, next, start, err
}

func (p pacedProvider) CreateFolder(ctx context.Context, name, parent, intent string) (File, error) {
	f, err := p.Provider.CreateFolder(ctx, name, parent, intent)
	p.t.Observe(err)
	return f, err
}

func (p pacedProvider) CopyFile(ctx context.Context, src File, parent, intent string) (File, error) {
	f, err := p.Provider.CopyFile(ctx, src, parent, intent)
	p.t.Observe(err)
	return f, err
}

func (p pacedProvider) CreateShortcut(ctx context.Context, name, parent, targetID, targetKey, intent string) (File, error) {
	f, err := p.Provider.CreateShortcut(ctx, name, parent, targetID, targetKey, intent)
	p.t.Observe(err)
	return f, err
}

func (p pacedProvider) RenameFile(ctx context.Context, id, name string) error {
	err := p.Provider.RenameFile(ctx, id, name)
	p.t.Observe(err)
	return err
}

func (p pacedProvider) MoveFile(ctx context.Context, id, from, to string) error {
	err := p.Provider.MoveFile(ctx, id, from, to)
	p.t.Observe(err)
	return err
}

func (p pacedProvider) TrashFile(ctx context.Context, id string) error {
	err := p.Provider.TrashFile(ctx, id)
	p.t.Observe(err)
	return err
}

func (p pacedProvider) RestoreFile(ctx context.Context, id string) error {
	err := p.Provider.RestoreFile(ctx, id)
	p.t.Observe(err)
	return err
}
