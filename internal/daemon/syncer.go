package daemon

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/GiaHung07/502Drive/internal/app"
	"github.com/GiaHung07/502Drive/internal/drive"
	"github.com/GiaHung07/502Drive/internal/state"
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
	s.claimSchedules(ctx, now)
	s.pollFeeds(ctx, now)
	s.reconcileFeeds(ctx)
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

// poll reads the change feed from the stored cursor and only advances the
// cursor together with the run the changes need.
func (s *syncer) poll(ctx context.Context, sub state.Subscription) {
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
	changed := false
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
		relevant, err := s.anyRelevant(ctx, sub.CollectionID, changes)
		if err != nil {
			s.note(ctx, sub.CollectionID, err)
			return
		}
		changed = changed || relevant
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
