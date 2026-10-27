package daemon

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/drive"
	"github.com/itsgiahung/502Drive/internal/state"
)

const (
	// syncerTick is how often due schedules and polls are checked. The actual
	// next due/poll times are persisted by the store, not kept here.
	syncerTick = 5 * time.Second
	// maxFeedPages bounds one poll's work; a drained feed persists its cursor.
	maxFeedPages = 20
	// idlePolls is how many quiet polls before the interval backs off.
	idlePolls = 30
)

// syncer owns scheduled runs and the Drive change feed. It is not an engine:
// it only admits runs and advances cursors through the store.
type syncer struct {
	store    *state.Store
	provider func(context.Context) (drive.Provider, error)
	wake     func()
	log      *slog.Logger
	now      func() time.Time

	// notifyError and notifyClear are optional (set when a bot is enabled).
	notifyError func(ctx context.Context, collectionID, code string) error
	notifyClear func(ctx context.Context, collectionID string) error
	// applyAfterCopy applies S8 requests recorded when an initial copy
	// succeeded. Optional (tests without a service leave it nil).
	applyAfterCopy func(context.Context) (int, int, error)

	mu      sync.Mutex
	quiet   map[string]int
	lastErr map[string]string
}

func newSyncer(store *state.Store, provider func(context.Context) (drive.Provider, error), wake func(), log *slog.Logger) *syncer {
	return &syncer{store: store, provider: provider, wake: wake, log: log, now: time.Now,
		quiet: map[string]int{}, lastErr: map[string]string{}}
}

// run ticks until ctx is cancelled; the first tick happens immediately so a
// restart catches up without waiting.
func (s *syncer) run(ctx context.Context) {
	tick := time.NewTicker(syncerTick)
	defer tick.Stop()
	for {
		s.tick(ctx)
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func (s *syncer) tick(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	now := s.now().UTC()
	s.applyAfterCopyRequests(ctx)
	s.claimSchedules(ctx, now)
	s.claimFullReconciles(ctx, now)
	s.replayPending(ctx, now)
	s.pollFeeds(ctx, now)
	s.reconcileFeeds(ctx)
}

// applyAfterCopyRequests creates the subscriptions S8 recorded when an initial
// copy succeeded. A failure is retried by the store, never by the copy job.
func (s *syncer) applyAfterCopyRequests(ctx context.Context) {
	if s.applyAfterCopy == nil {
		return
	}
	if _, _, err := s.applyAfterCopy(ctx); err != nil {
		s.note(ctx, "after_copy_sync", err)
	}
}

// claimFullReconciles admits the periodic full reconciliation a realtime
// subscription is due for. The next slot is stored before the run, so a slow
// run never causes a second one.
func (s *syncer) claimFullReconciles(ctx context.Context, now time.Time) {
	jobs, err := s.store.ClaimDueFullReconciles(ctx, now)
	if err != nil {
		s.note(ctx, "full_reconcile", err)
		return
	}
	if len(jobs) == 0 {
		return
	}
	s.log.Info("sync: full reconciles admitted", "count", len(jobs))
	s.wake()
}

// replayPending flushes the raw change pages stored while a baseline ran. It
// runs before polling so a subscription is never polled past its own buffer.
func (s *syncer) replayPending(ctx context.Context, now time.Time) {
	collections, err := s.store.PendingCollections(ctx)
	if err != nil {
		s.note(ctx, "replay", err)
		return
	}
	for _, id := range collections {
		if ctx.Err() != nil {
			return
		}
		sub, err := s.store.GetSubscription(ctx, id)
		if err != nil {
			continue
		}
		if sub.State != state.SyncActive {
			continue // paused/stopped: the buffer waits, the cursor has not moved
		}
		changes, finalCursor, overflow, err := s.store.ReplayBatch(ctx, id, state.MaxReplayChanges)
		if err != nil {
			s.note(ctx, id, err)
			continue
		}
		if overflow {
			// The buffer is known to be incomplete: never replay part of it.
			if err = s.store.DiscardReplay(ctx, id); err != nil {
				s.note(ctx, id, err)
				continue
			}
			prov, perr := s.provider(ctx)
			if perr != nil {
				s.note(ctx, id, perr)
				continue
			}
			s.reconcileFeed(ctx, prov, sub, "replay_overflow")
			continue
		}
		relevant := false
		for _, ch := range changes {
			hit, err := s.store.RelevantChange(ctx, id, ch.FileID, ch.Parents)
			if err != nil {
				s.note(ctx, id, err)
				break
			}
			relevant = relevant || hit
		}
		run, err := s.store.FinishReplay(ctx, id, sub.Cursor, finalCursor, relevant, now.Add(s.nextPoll(id, relevant)))
		switch {
		case errors.Is(err, state.ErrTransition):
			continue
		case err != nil:
			s.note(ctx, id, err)
			continue
		}
		s.clear(ctx, id)
		if run != nil {
			s.wake()
		}
	}
}

// claimSchedules admits one catch-up run per due subscription. The store keeps
// a slot due when admission is refused (queue full), so it is not lost.
func (s *syncer) claimSchedules(ctx context.Context, now time.Time) {
	jobs, err := s.store.ClaimDueSchedules(ctx, now)
	if err != nil {
		s.note(ctx, "scheduler", err)
		return
	}
	s.clear(ctx, "scheduler")
	if len(jobs) == 0 {
		return
	}
	s.log.Info("sync: scheduled runs admitted", "count", len(jobs))
	s.wake()
}

func (s *syncer) pollFeeds(ctx context.Context, now time.Time) {
	subs, err := s.store.DuePolls(ctx, now)
	if err != nil {
		s.note(ctx, "poller", err)
		return
	}
	for _, sub := range subs {
		if ctx.Err() != nil {
			return
		}
		s.poll(ctx, sub)
	}
}

// poll reads the change feed from the stored cursor. While the baseline runs
// the pages are stored raw (the mapping is still being built, so relevance
// cannot be decided yet) and replayed after it finishes. Once active, the
// cursor only advances together with the run the changes need.
func (s *syncer) poll(ctx context.Context, sub state.Subscription) {
	if sub.State == state.SyncBootstrapping && sub.ReplayOverflow {
		// The buffer is already known to be incomplete; polling further would
		// only spend provider calls. RunFinished turns this into a reconcile.
		return
	}
	prov, err := s.provider(ctx)
	if err != nil {
		s.deferPoll(ctx, sub, err)
		return
	}
	if sub.Cursor == "" {
		s.reconcileFeed(ctx, prov, sub, "cursor_missing")
		return
	}
	cursor := sub.Cursor
	// Resume behind the durable buffer, not behind the subscription cursor:
	// a restart must not re-read (and duplicate) pages already stored.
	if last, ok, err := s.store.LastPendingCursor(ctx, sub.CollectionID); err != nil {
		s.deferPoll(ctx, sub, err)
		return
	} else if ok {
		cursor = last
	}
	changed := false
	var raw []state.ReplayChange
	for page := 0; page < maxFeedPages; page++ {
		changes, next, newStart, err := prov.ListChanges(ctx, cursor)
		if err != nil {
			if drive.IsInvalidToken(err) {
				s.reconcileFeed(ctx, prov, sub, "cursor_expired")
				return
			}
			s.deferPoll(ctx, sub, err)
			return
		}
		if sub.State == state.SyncBootstrapping {
			for _, ch := range changes {
				raw = append(raw, replayChange(ch))
			}
		} else {
			relevant, err := s.anyRelevant(ctx, sub.CollectionID, changes)
			if err != nil {
				s.note(ctx, sub.CollectionID, err)
				return
			}
			changed = changed || relevant
		}
		if next == "" {
			// Drained: newStart is the token for this position. It may equal
			// the cursor of an empty poll, which is not an error.
			cursor = newStart
			break
		}
		cursor = next
	}
	if cursor == "" {
		s.deferPoll(ctx, sub, errors.New("the change feed returned no cursor"))
		return
	}
	if sub.State == state.SyncBootstrapping {
		if _, err := s.store.AppendPendingChanges(ctx, sub.CollectionID, raw, cursor, s.now().Add(app.SyncPollWarm)); err != nil {
			s.deferPoll(ctx, sub, err)
		}
		return
	}
	run, err := s.store.AdvancePoll(ctx, sub.CollectionID, sub.Cursor, cursor, changed, s.now().Add(s.nextPoll(sub.CollectionID, changed)))
	switch {
	case errors.Is(err, state.ErrTransition):
		// Paused, stopped or reconfigured while the pages were read: the old
		// cursor stays, so the next active poll re-reads them.
		return
	case err != nil:
		s.deferPoll(ctx, sub, err)
		return
	}
	s.clear(ctx, sub.CollectionID)
	if run != nil {
		s.wake()
	}
}

func replayChange(ch drive.Change) state.ReplayChange {
	out := state.ReplayChange{FileID: ch.FileID, Removed: ch.Removed}
	if ch.File != nil {
		out.Parents = ch.File.Parents
		out.Trashed = ch.File.Trashed
		out.Removed = out.Removed || ch.File.Trashed
	}
	return out
}

func (s *syncer) anyRelevant(ctx context.Context, collectionID string, changes []drive.Change) (bool, error) {
	relevant := false
	for _, ch := range changes {
		var parents []string
		if ch.File != nil {
			parents = ch.File.Parents
		}
		hit, err := s.store.RelevantChange(ctx, collectionID, ch.FileID, parents)
		if err != nil {
			return false, err
		}
		relevant = relevant || hit
	}
	return relevant, nil
}

// nextPoll follows the documented polling cadence: fast after a change, warm
// while a collection is active, idle after a long quiet stretch.
func (s *syncer) nextPoll(key string, changed bool) time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	if changed {
		s.quiet[key] = 0
		return app.SyncPollActive
	}
	s.quiet[key]++
	if s.quiet[key] >= idlePolls {
		return app.SyncPollIdle
	}
	return app.SyncPollWarm
}

// deferPoll keeps the stored cursor and pushes the next poll out, honouring a
// provider retry hint. A persistent failure therefore does not spin.
func (s *syncer) deferPoll(ctx context.Context, sub state.Subscription, err error) {
	wait := app.SyncPollWarm
	if d := drive.RetryAfterOf(err); d > wait {
		wait = d
	}
	if e := s.store.PollLater(ctx, sub.CollectionID, s.now().Add(wait)); e != nil {
		s.log.Error("sync: cannot defer a poll", "collection", sub.CollectionID, "err", e)
	}
	s.note(ctx, sub.CollectionID, err)
}

// reconcileFeed captures a fresh cursor and asks for a full reconcile: an
// expired or missing cursor must never be replaced by silently skipping ahead.
func (s *syncer) reconcileFeed(ctx context.Context, prov drive.Provider, sub state.Subscription, code string) {
	token, err := prov.StartPageToken(ctx)
	if err != nil {
		s.deferPoll(ctx, sub, err)
		return
	}
	failure := state.Failure{Code: code, Message: "The change cursor cannot be resumed; a full reconcile was queued."}
	if err = s.store.MarkNeedsReconcile(ctx, sub.CollectionID, token, failure); err != nil && !errors.Is(err, state.ErrTransition) {
		s.note(ctx, sub.CollectionID, err)
		return
	}
	s.clear(ctx, sub.CollectionID)
	s.wake()
}

// reconcileFeeds starts the reconcile runs that a waiting subscription needs.
func (s *syncer) reconcileFeeds(ctx context.Context) {
	subs, err := s.store.NeedsReconcileSubs(ctx)
	if err != nil {
		s.note(ctx, "reconcile", err)
		return
	}
	for _, sub := range subs {
		if ctx.Err() != nil {
			return
		}
		prov, err := s.provider(ctx)
		if err != nil {
			s.note(ctx, sub.CollectionID, err)
			continue
		}
		s.reconcileFeed(ctx, prov, sub, "needs_reconcile")
	}
}

// note logs one line per distinct failure per subscription, so a dead provider
// does not produce a log line every five seconds.
func (s *syncer) note(ctx context.Context, key string, err error) {
	code := errorKey(err)
	s.mu.Lock()
	previous := s.lastErr[key]
	s.lastErr[key] = code
	s.mu.Unlock()
	if previous == code {
		return
	}
	s.log.Warn("sync: provider call failed", "scope", key, "code", code)
	if s.notifyError != nil && isCollection(key) && needsAttention(err) {
		if nerr := s.notifyError(ctx, key, code); nerr != nil {
			s.log.Warn("sync: cannot queue a notification", "scope", key, "err", nerr)
		}
	}
}

// clear forgets the last failure of a scope and tells the notifier the
// subscription recovered.
func (s *syncer) clear(ctx context.Context, key string) {
	s.mu.Lock()
	_, had := s.lastErr[key]
	delete(s.lastErr, key)
	s.mu.Unlock()
	if had && s.notifyClear != nil && isCollection(key) {
		if err := s.notifyClear(ctx, key); err != nil {
			s.log.Warn("sync: cannot clear a notification", "scope", key, "err", err)
		}
	}
}

func isCollection(scope string) bool {
	return strings.HasPrefix(scope, "col_")
}

// needsAttention reports whether a provider failure asks the user to act
// (reconnect, permission, missing item) rather than retrying quietly.
func needsAttention(err error) bool {
	var de *drive.Error
	if !errors.As(err, &de) {
		return false
	}
	switch de.Kind {
	case drive.KindAuth, drive.KindPermission, drive.KindNotFound, drive.KindInvalid:
		return true
	}
	return false
}

func errorKey(err error) string {
	var ae *app.Error
	switch {
	case errors.As(err, &ae):
		return ae.Code
	case errors.Is(err, drive.ErrNotConnected), errors.Is(err, drive.ErrReconnectRequired):
		return "account_reconnect_required"
	}
	var de *drive.Error
	if errors.As(err, &de) {
		return string(de.Kind)
	}
	return "internal"
}
