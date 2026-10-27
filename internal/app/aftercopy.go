package app

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/itsgiahung/502Drive/internal/schedule"
	"github.com/itsgiahung/502Drive/internal/state"
)

// afterCopyDailyTime is when an "after copy: daily" subscription first runs,
// in the principal's preferences timezone. 02:00 keeps the first full scan out
// of working hours; later runs keep that wall-clock slot.
const afterCopyDailyTime = "02:00"

// ApplyAfterCopySync applies the S8 preference to collections whose initial
// copy succeeded. It runs from the syncer loop, never from the copy job: a
// failure here is recorded on the request and never changes the job result.
func (s *Service) ApplyAfterCopySync(ctx context.Context) (applied, failed int, err error) {
	now := s.now().UTC()
	rows, err := s.Store.ClaimDueAfterCopySync(ctx, now, 5)
	if err != nil {
		return 0, 0, err
	}
	for _, row := range rows {
		warning, e := s.applyAfterCopySync(ctx, row)
		if e != nil {
			if ferr := s.Store.FailAfterCopySync(ctx, row, e.Error()); ferr != nil {
				return applied, failed, ferr
			}
			failed++
			continue
		}
		if e = s.Store.CompleteAfterCopySync(ctx, row.CollectionID, warning); e != nil {
			return applied, failed, e
		}
		applied++
	}
	return applied, failed, nil
}

func (s *Service) applyAfterCopySync(ctx context.Context, row state.AfterCopySync) (string, error) {
	c, err := s.Store.GetCollection(ctx, "", row.CollectionID)
	if err != nil {
		return "", err
	}
	if c.State != state.CollectionReady {
		return "the collection is not ready yet", nil
	}
	if _, err = s.Store.GetSubscription(ctx, c.ID); err == nil {
		return "a sync subscription already exists; left it unchanged", nil
	} else if !errors.Is(err, state.ErrNotFound) {
		return "", err
	}
	prefs, err := s.Store.Preferences(ctx, row.Principal)
	if err != nil {
		return "", err
	}
	mode, spec, err := afterCopySchedule(row.Mode, row.CreatedAt, prefs.Timezone)
	if err != nil {
		return "", err
	}
	req := SyncRequest{Mode: mode, Schedule: spec, ContentPolicy: state.PolicyVersioned, ExpectedCollection: c.Revision}
	key := "after-copy:" + c.ID + ":" + row.Mode
	p := Principal{ID: row.Principal}
	if _, _, err = s.ConfigureSync(ctx, p, key, c.ID, req); err != nil {
		return "", err
	}
	return "", nil
}

// afterCopySchedule maps the preference value to the subscription's own mode
// and schedule. It is deterministic from the request's creation instant, so a
// retry after a crash produces byte-identical input and replays cleanly.
func afterCopySchedule(mode string, created time.Time, timezone string) (string, *schedule.Spec, error) {
	switch mode {
	case state.AfterCopyRealtime:
		return state.ModeRealtime, nil, nil
	case state.AfterCopyHourly, state.AfterCopySixHourly:
		minutes := 60
		if mode == state.AfterCopySixHourly {
			minutes = 360
		}
		return state.ModeScheduled, &schedule.Spec{Kind: schedule.KindInterval, IntervalMinutes: minutes, AnchorAt: created.UTC()}, nil
	case state.AfterCopyDaily:
		tz := timezone
		if tz == "" {
			tz = state.DefaultTimezone()
		}
		return state.ModeScheduled, &schedule.Spec{Kind: schedule.KindDaily, Timezone: tz, LocalTime: afterCopyDailyTime}, nil
	}
	return "", nil, Fail(http.StatusBadRequest, "invalid_request", "Unknown after-copy sync mode.")
}
