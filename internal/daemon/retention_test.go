package daemon

import (
	"context"
	"testing"
	"time"

	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/state"
)

// The daily prune reads the owner's retention preference (what the TUI and
// Telegram save), runs at most once per 24 hours even across restarts, and
// never runs when retention is 0 ("keep everything").
func TestPruneIfDueUsesTheOwnerPreferenceOncePerDay(t *testing.T) {
	store, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	ctx := context.Background()
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC)

	keep := state.DefaultPreferences()
	keep.RetentionDays = 0
	if err := store.SavePreferences(ctx, app.Owner.ID, keep); err != nil {
		t.Fatal(err)
	}
	// A stale "default" entry (the old key) must not decide anything.
	stale := state.DefaultPreferences()
	stale.RetentionDays = 1
	if err := store.SavePreferences(ctx, "default", stale); err != nil {
		t.Fatal(err)
	}
	if _, ran, err := pruneIfDue(ctx, store, now); err != nil || ran {
		t.Fatalf("retention 0 must keep everything: ran=%v err=%v", ran, err)
	}

	keep.RetentionDays = 30
	if err := store.SavePreferences(ctx, app.Owner.ID, keep); err != nil {
		t.Fatal(err)
	}
	if _, ran, err := pruneIfDue(ctx, store, now); err != nil || !ran {
		t.Fatalf("first run of the day: ran=%v err=%v", ran, err)
	}
	if _, ran, _ := pruneIfDue(ctx, store, now.Add(23*time.Hour)); ran {
		t.Fatal("ran twice within 24 hours")
	}
	// A restarted daemon reads the stored time instead of starting a new day.
	if _, ran, _ := pruneIfDue(ctx, store, now.Add(25*time.Hour)); !ran {
		t.Fatal("did not run again after a day")
	}
}
