package telegram

import (
	"context"
	"time"

	"github.com/itsgiahung/502Drive/internal/state"
)

// Quiet hours (T5) also hold Telegram notices that nobody asked for just now:
// the finished-job and sync-error messages and the daily report. They are not
// dropped; the outbox row is rescheduled to the end of the quiet window, so a
// failure that happened at night is still delivered in the morning. Replies to
// what you do in the chat, and the progress panel of a job you started there,
// are never held: you are obviously awake.

// quietEnd is the next moment the quiet window ends, in the person's zone.
func quietEnd(p state.Preferences, now time.Time) time.Time {
	loc := time.UTC
	if l, err := time.LoadLocation(p.Timezone); err == nil && p.Timezone != "" {
		loc = l
	}
	if p.QuietHoursEnd == nil {
		return now.Add(time.Hour)
	}
	hour, minute, ok := validClock(*p.QuietHoursEnd)
	if !ok {
		return now.Add(time.Hour)
	}
	local := now.In(loc)
	end := time.Date(local.Year(), local.Month(), local.Day(), hour, minute, 0, 0, loc)
	if !end.After(local) {
		end = time.Date(local.Year(), local.Month(), local.Day()+1, hour, minute, 0, 0, loc)
	}
	if !end.After(now) {
		return now.Add(time.Minute)
	}
	return end
}

// holdForQuiet reschedules a notice that falls into its owner's quiet hours.
func (t *Telegram) holdForQuiet(ctx context.Context, m state.Outbox) bool {
	prefs, err := t.store.Preferences(ctx, m.Principal)
	if err != nil {
		return false
	}
	now := t.now()
	if !prefs.InQuietHours(now) {
		return false
	}
	if err := t.store.DeferOutbox(ctx, m.ID, quietEnd(prefs, now), m.MessageID, m.LastHash, m.LastState); err != nil {
		t.log.Error("telegram: cannot hold a notice for quiet hours", "err", err)
		return false
	}
	return true
}
