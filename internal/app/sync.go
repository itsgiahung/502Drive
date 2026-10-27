package app

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/itsgiahung/502Drive/internal/schedule"
	"github.com/itsgiahung/502Drive/internal/state"
)

// SyncRequest is one subscription configuration. The HTTP layer decodes it
// from the SyncInput schema.
type SyncRequest struct {
	Mode                 string         `json:"mode"`
	Schedule             *schedule.Spec `json:"schedule,omitempty"`
	ContentPolicy        string         `json:"content_policy"`
	ExpectedCollection   int            `json:"expected_collection_revision"`
	ExpectedSubscription *int           `json:"expected_subscription_revision"`
}

// SyncView is a subscription together with the collection's in-flight run.
type SyncView struct {
	Subscription state.Subscription
	ActiveJobID  string
	Mirror       bool
	configured   bool
}

// Configured reports whether the collection has a subscription at all.
func (v SyncView) Configured() bool { return v.configured }

// ConfigureSync creates or updates the single subscription of a collection.
// For realtime the change-feed cursor is captured before the subscription is
// written, so a stored cursor is always one the daemon can resume from.
func (s *Service) ConfigureSync(ctx context.Context, p Principal, key, collectionID string, req SyncRequest) (SyncView, bool, error) {
	if err := state.ValidateKey(key); err != nil {
		return SyncView{}, false, Fail(http.StatusBadRequest, "invalid_request", "Idempotency-Key is required (1-128 printable characters).")
	}
	if err := validateSyncRequest(req); err != nil {
		return SyncView{}, false, err
	}
	now := s.now().UTC()
	every := 7 * 24 * time.Hour
	if s.FullReconcileEvery != nil {
		every = *s.FullReconcileEvery
	}
	change := state.SyncChange{
		Mode:                 req.Mode,
		Schedule:             req.Schedule,
		ContentPolicy:        req.ContentPolicy,
		ExpectedCollection:   req.ExpectedCollection,
		ExpectedSubscription: req.ExpectedSubscription,
		Now:                  now,
		FullReconcileEvery:   int64(every / time.Second),
	}
	switch req.Mode {
	case state.ModeScheduled:
		// The next wall-clock slot is computed here, from the validated spec and
		// the injected clock, so a fake clock drives the same code as production.
		slot, err := req.Schedule.Next(now)
		if err != nil {
			return SyncView{}, false, Fail(http.StatusBadRequest, "invalid_request", "The schedule has no future occurrence: "+err.Error())
		}
		change.NextDue = &slot
	case state.ModeRealtime:
		prov, err := s.provider(ctx)
		if err != nil {
			return SyncView{}, false, err
		}
		token, err := prov.StartPageToken(ctx)
		if err != nil {
			return SyncView{}, false, err
		}
		change.Cursor = token
	}
	sub, replayed, err := s.Store.ConfigureSync(ctx, p.ID, scope(p, true), key, collectionID, change)
	if err != nil {
		return SyncView{}, false, err
	}
	s.wake()
	view, _, err := s.SyncStatus(ctx, collectionID)
	if err != nil {
		return SyncView{}, false, err
	}
	view.Subscription = sub
	return view, replayed, nil
}

func validateSyncRequest(req SyncRequest) error {
	switch req.Mode {
	case state.ModeScheduled:
		if req.Schedule == nil {
			return Fail(http.StatusBadRequest, "invalid_request", "schedule is required when mode is scheduled.")
		}
		if err := validateSpecShape(*req.Schedule); err != nil {
			return err
		}
		if err := req.Schedule.Validate(); err != nil {
			return Fail(http.StatusBadRequest, "invalid_request", err.Error())
		}
	case state.ModeRealtime:
		if req.Schedule != nil {
			return Fail(http.StatusBadRequest, "invalid_request", "schedule is not accepted when mode is realtime.")
		}
	default:
		return Fail(http.StatusBadRequest, "invalid_request", "mode must be scheduled or realtime.")
	}
	switch req.ContentPolicy {
	case state.PolicyVersioned, state.PolicyReplace:
	default:
		return Fail(http.StatusBadRequest, "invalid_request", "content_policy must be versioned_copy or replace_copy.")
	}
	return nil
}

// validateSpecShape rejects fields that do not belong to the chosen kind, so
// the stored spec matches exactly one ScheduleSpec branch.
func validateSpecShape(sp schedule.Spec) error {
	switch sp.Kind {
	case schedule.KindInterval:
		if sp.Timezone != "" || sp.LocalTime != "" || len(sp.Weekdays) > 0 {
			return Fail(http.StatusBadRequest, "invalid_request", "an interval schedule has no timezone, local_time or weekdays.")
		}
	case schedule.KindDaily:
		if sp.IntervalMinutes != 0 || !sp.AnchorAt.IsZero() || len(sp.Weekdays) > 0 {
			return Fail(http.StatusBadRequest, "invalid_request", "a daily schedule has no interval_minutes, anchor_at or weekdays.")
		}
	case schedule.KindWeekly:
		if sp.IntervalMinutes != 0 || !sp.AnchorAt.IsZero() {
			return Fail(http.StatusBadRequest, "invalid_request", "a weekly schedule has no interval_minutes or anchor_at.")
		}
	}
	return nil
}

// SyncAction pauses, resumes or stops a subscription. Resume recomputes the
// next slot from the stored spec, and a slot missed while paused catches up
// exactly once (the store keeps the due slot and ClaimDueSchedules claims it).
func (s *Service) SyncAction(ctx context.Context, p Principal, key, collectionID, action string, expected int) (SyncView, bool, error) {
	if err := state.ValidateKey(key); err != nil {
		return SyncView{}, false, Fail(http.StatusBadRequest, "invalid_request", "Idempotency-Key is required (1-128 printable characters).")
	}
	switch action {
	case "pause", "resume", "stop":
	default:
		return SyncView{}, false, Fail(http.StatusNotFound, "not_found", "Not found.")
	}
	now := s.now().UTC()
	next := func(sub state.Subscription) (*schedule.Slot, error) {
		if sub.Schedule == nil {
			return nil, state.ErrTransition
		}
		slot, err := sub.Schedule.Next(now)
		if err != nil {
			return nil, state.ErrTransition
		}
		return &slot, nil
	}
	sub, replayed, err := s.Store.SyncAction(ctx, p.ID, scope(p, true), key, collectionID, action, expected, now, next)
	if err != nil {
		return SyncView{}, false, err
	}
	view, _, err := s.SyncStatus(ctx, collectionID)
	if err != nil {
		return SyncView{}, false, err
	}
	view.Subscription = sub
	if action != "pause" {
		s.wake()
	}
	return view, replayed, nil
}

// RunNow admits a delta run even when sync is off, and coalesces into the
// collection's active run instead of spawning a second writer.
func (s *Service) RunNow(ctx context.Context, p Principal, key, collectionID string, expected int) (state.RunOutcome, bool, error) {
	if err := state.ValidateKey(key); err != nil {
		return state.RunOutcome{}, false, Fail(http.StatusBadRequest, "invalid_request", "Idempotency-Key is required (1-128 printable characters).")
	}
	res, replayed, err := s.Store.RunNow(ctx, p.ID, scope(p, true), key, collectionID, expected)
	if err != nil {
		return state.RunOutcome{}, false, err
	}
	if !res.Coalesced {
		s.wake()
	}
	return res, replayed, nil
}

// SyncStatus reports the subscription of a collection and its active run. The
// caller must already be authorised for the collection.
func (s *Service) SyncStatus(ctx context.Context, collectionID string) (SyncView, bool, error) {
	active, err := s.Store.ActiveRunID(ctx, collectionID)
	if err != nil {
		return SyncView{}, false, err
	}
	sub, err := s.Store.GetSubscription(ctx, collectionID)
	if errors.Is(err, state.ErrNotFound) {
		return SyncView{ActiveJobID: active}, false, nil
	}
	if err != nil {
		return SyncView{}, false, err
	}
	c, err := s.Store.GetCollection(ctx, "", collectionID)
	if err != nil {
		return SyncView{}, false, err
	}
	return SyncView{Subscription: sub, ActiveJobID: active, Mirror: c.Protection.Mirror, configured: true}, true, nil
}

// SyncInterval keeps the daemon's poll cadence in one place for tests.
const (
	SyncPollActive = 10 * time.Second
	SyncPollWarm   = 60 * time.Second
	SyncPollIdle   = 300 * time.Second
)
