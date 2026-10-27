package telegram

import (
	"context"
	"strings"
	"testing"

	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/schedule"
	"github.com/itsgiahung/502Drive/internal/state"
)

// TestSyncControlUsesTheRevisionOnTheCard is finding #17: the pause/resume/stop
// buttons carry the revision the user is looking at, so the handler must send
// that revision instead of re-reading the current one and defeating the stale
// check.
func TestSyncControlUsesTheRevisionOnTheCard(t *testing.T) {
	h := newHarness(t, 100)
	id := h.readyCollection("Stale Sync")
	ctx := context.Background()
	daily := func(at string) *schedule.Spec {
		return &schedule.Spec{Kind: schedule.KindDaily, Timezone: "UTC", LocalTime: at}
	}
	if _, _, err := h.svc.ConfigureSync(ctx, app.Owner, "cfg-1", id, app.SyncRequest{Mode: state.ModeScheduled,
		Schedule: daily("22:00"), ContentPolicy: state.PolicyVersioned,
		ExpectedCollection: h.collectionOf(id).Revision}); err != nil {
		t.Fatal(err)
	}

	// Render the collection card through the bot so the button carries the
	// revision the user actually sees.
	h.api.push(h.messageUpdate(h.nextUpdateID(), 100, 100, "private", "/start"))
	h.pull()
	home := h.lastCard()
	h.api.push(h.callbackUpdate(h.nextUpdateID(), 100, 100, home.MessageID, "nav:library"))
	h.pull()
	library := h.lastCard()
	h.api.push(h.click(library, "Stale Sync"))
	h.pull()
	card := h.lastCard()
	stop := h.buttonData(card, vietnamese.stopSync)

	// Another device reschedules the sync: the subscription revision moves on.
	view, _, err := h.svc.SyncStatus(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	rev := view.Subscription.Revision
	if _, _, err = h.svc.ConfigureSync(ctx, app.Owner, "cfg-2", id, app.SyncRequest{Mode: state.ModeScheduled,
		Schedule: daily("23:00"), ContentPolicy: state.PolicyVersioned,
		ExpectedCollection: h.collectionOf(id).Revision, ExpectedSubscription: &rev}); err != nil {
		t.Fatal(err)
	}

	// The old button must not stop the schedule the user never saw.
	h.api.push(h.callbackUpdate(h.nextUpdateID(), 100, 100, card.MessageID, stop))
	h.pull()
	after, _, err := h.svc.SyncStatus(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if after.Subscription.State != state.SyncActive {
		t.Fatalf("a stale card stopped the subscription: %s", after.Subscription.State)
	}
	if got := h.lastCard(); !strings.Contains(got.Text, vietnamese.problemLabel) {
		t.Fatalf("stale click card = %q", got.Text)
	}
}
