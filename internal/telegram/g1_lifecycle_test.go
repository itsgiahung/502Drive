package telegram

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/state"
)

// Batch G, slice 1a: collection lifecycle (L1) from the collection card.

func (h *harness) collectionCardFor(id string) fakeMessage {
	h.t.Helper()
	if err := h.tg.collectionCard(context.Background(), app.Owner, h.ownerID, 10, id, ""); err != nil {
		h.t.Fatal(err)
	}
	return h.lastCard()
}

func (h *harness) collectionOf(id string) state.Collection {
	h.t.Helper()
	c, err := h.store.GetCollection(context.Background(), "", id)
	if err != nil {
		h.t.Fatal(err)
	}
	return c
}

func hasButton(card fakeMessage, label string) bool {
	for _, b := range card.ButtonTexts {
		if strings.Contains(b, label) {
			return true
		}
	}
	return false
}

func styleOf(card fakeMessage, label string) string {
	for i, b := range card.ButtonTexts {
		if strings.Contains(b, label) {
			return card.Styles[i]
		}
	}
	return ""
}

func (h *harness) lastAnswer() string {
	h.t.Helper()
	got := h.api.answerTexts()
	if len(got) == 0 {
		return ""
	}
	return got[len(got)-1]
}

func TestRetireShowsAPreviewThenRetiresOnlyAfterOneConfirmation(t *testing.T) {
	h := newHarness(t, 100)
	g := vietnamese.g()
	id := h.readyCollection("Retire me")

	card := h.collectionCardFor(id)
	h.tap(card, g.retireBtn)
	ask := h.lastCard()
	if !strings.Contains(ask.Text, "Retire me") {
		t.Fatalf("the preview does not name the collection:\n%s", ask.Text)
	}
	if got := h.collectionOf(id).State; got == state.CollectionRetired {
		t.Fatalf("the first tap retired the collection without a confirmation")
	}
	if styleOf(ask, g.retireConfirm) != "danger" {
		t.Fatalf("the confirmation must be the red button: %v / %v", ask.ButtonTexts, ask.Styles)
	}

	h.tap(ask, g.retireConfirm)
	if got := h.collectionOf(id).State; got != state.CollectionRetired {
		t.Fatalf("state after the confirmation = %q", got)
	}

	// The confirmation is one-shot: a second tap on the same button does nothing.
	rev := h.collectionOf(id).Revision
	h.api.push(h.click(ask, g.retireConfirm))
	h.pull()
	if h.lastAnswer() != vietnamese.expired {
		t.Fatalf("second tap answered %q", h.lastAnswer())
	}
	if h.collectionOf(id).Revision != rev {
		t.Fatalf("a consumed button changed the collection again")
	}
}

func TestRetireConfirmationExpiresAndNothingChanges(t *testing.T) {
	h := newHarness(t, 100)
	g := vietnamese.g()
	id := h.readyCollection("Expiring retire")
	h.tap(h.collectionCardFor(id), g.retireBtn)
	ask := h.lastCard()

	h.advance(lifecycleConfirmTTL + time.Minute)
	h.tap(ask, g.retireConfirm)
	if got := h.collectionOf(id).State; got == state.CollectionRetired {
		t.Fatalf("an expired confirmation retired the collection")
	}
	if !strings.Contains(h.lastCard().Text, g.confirmExpired) {
		t.Fatalf("expected the expiry notice:\n%s", h.lastCard().Text)
	}
}

func TestRetireIsBoundToTheCollectionRevision(t *testing.T) {
	h := newHarness(t, 100)
	g := vietnamese.g()
	id := h.readyCollection("Moving target")
	h.tap(h.collectionCardFor(id), g.retireBtn)
	ask := h.lastCard()

	// Someone else changes the collection after the preview was shown.
	c := h.collectionOf(id)
	if _, err := h.svc.SetCollectionFilter(context.Background(), app.Owner, id, c.Filter, c.Revision); err != nil {
		t.Fatal(err)
	}
	h.tap(ask, g.retireConfirm)
	if got := h.collectionOf(id).State; got == state.CollectionRetired {
		t.Fatalf("a stale preview retired a changed collection")
	}
}

func TestRetireRefusedWhileARunIsActiveShowsAHint(t *testing.T) {
	h := newHarness(t, 100)
	g := vietnamese.g()
	id := h.readyCollection("Busy one")
	c := h.collectionOf(id)
	run, _, err := h.svc.RunNow(context.Background(), app.Owner, "busy-run", id, c.Revision)
	if err != nil {
		t.Fatal(err)
	}
	h.tap(h.collectionCardFor(id), g.retireBtn)
	h.tap(h.lastCard(), g.retireConfirm)
	if got := h.collectionOf(id).State; got == state.CollectionRetired {
		t.Fatalf("a collection with an active run was retired")
	}
	text := h.lastCard().Text
	if !strings.Contains(text, run.Job.ID) || !strings.Contains(text, g.errHint["collection_busy"]) {
		t.Fatalf("the error must name the run and say what to do:\n%s", text)
	}
}

func TestRetiredCollectionOffersResumeForgetAndCleanup(t *testing.T) {
	h := newHarness(t, 100)
	g := vietnamese.g()
	id := h.readyCollection("Lifecycle")
	h.tap(h.collectionCardFor(id), g.retireBtn)
	h.tap(h.lastCard(), g.retireConfirm)

	card := h.collectionCardFor(id)
	for _, want := range []string{g.resumeBtn, g.forgetBtn, g.cleanupBtn} {
		if !hasButton(card, want) {
			t.Fatalf("retired card lacks %q: %v", want, card.ButtonTexts)
		}
	}
	if hasButton(card, vietnamese.runNow) || hasButton(card, vietnamese.enableSync) {
		t.Fatalf("a retired collection must not offer sync actions: %v", card.ButtonTexts)
	}

	// It stays reachable from the library through the retired list.
	if err := h.tg.libraryPage(context.Background(), app.Owner, h.ownerID, 11, vietnamese); err != nil {
		t.Fatal(err)
	}
	lib := h.lastCard()
	if !hasButton(lib, fmt.Sprintf(g.retiredButton, 1)) {
		t.Fatalf("the library does not point at the retired list: %v", lib.ButtonTexts)
	}
	h.tap(lib, fmt.Sprintf(g.retiredButton, 1))
	list := h.lastCard()
	if !hasButton(list, "Lifecycle") {
		t.Fatalf("the retired list does not show the collection: %v", list.ButtonTexts)
	}

	// Resume brings it back to the normal card.
	h.tap(card, g.resumeBtn)
	if got := h.collectionOf(id).State; got != state.CollectionReady {
		t.Fatalf("state after resume = %q", got)
	}
}

func TestForgetAsksFirstAndOnlyWorksOnARetiredCollection(t *testing.T) {
	h := newHarness(t, 100)
	g := vietnamese.g()
	id := h.readyCollection("Forget me")

	// A live collection has no Forget button at all.
	if live := h.collectionCardFor(id); hasButton(live, g.forgetBtn) {
		t.Fatalf("a live collection offers Forget: %v", live.ButtonTexts)
	}
	h.tap(h.lastCard(), g.retireBtn)
	h.tap(h.lastCard(), g.retireConfirm)

	h.tap(h.collectionCardFor(id), g.forgetBtn)
	ask := h.lastCard()
	if _, err := h.store.GetCollection(context.Background(), "", id); err != nil {
		t.Fatalf("the preview already forgot the collection: %v", err)
	}
	if styleOf(ask, g.forgetConfirm) != "danger" {
		t.Fatalf("Forget must be confirmed with a red button: %v / %v", ask.ButtonTexts, ask.Styles)
	}
	h.tap(ask, g.forgetConfirm)
	if _, err := h.store.GetCollection(context.Background(), "", id); err == nil {
		t.Fatalf("the collection was not forgotten")
	}
}

func TestCleanupRunsAPreviewJobThenTrashesOnlyAfterConfirmation(t *testing.T) {
	h := newHarness(t, 100)
	g := vietnamese.g()
	ctx := context.Background()
	id := h.readyCollection("Trash me")
	h.tap(h.collectionCardFor(id), g.retireBtn)
	h.tap(h.lastCard(), g.retireConfirm)

	h.tap(h.collectionCardFor(id), g.cleanupBtn)
	ask := h.lastCard()
	jobs := h.jobs()
	if len(jobs) == 0 || jobs[0].Kind != state.JobKindCleanup || !jobs[0].DryRun {
		t.Fatalf("the first step must be a dry-run, got %+v", jobs)
	}
	dry := jobs[0]
	if !strings.Contains(ask.Text, dry.ID) || !strings.Contains(ask.Text, "Trash me") {
		t.Fatalf("the preview must name the dry-run job and the folder:\n%s", ask.Text)
	}
	if styleOf(ask, g.cleanupConfirm) != "danger" {
		t.Fatalf("the trash move needs the red button: %v / %v", ask.ButtonTexts, ask.Styles)
	}

	// The dry-run is still running: the real run is refused with advice.
	h.tap(ask, g.cleanupConfirm)
	for _, j := range h.jobs() {
		if j.Kind == state.JobKindCleanup && !j.DryRun {
			t.Fatalf("a real cleanup was admitted while the preview was still running")
		}
	}
	if !strings.Contains(h.lastCard().Text, g.errHint["collection_busy"]) {
		t.Fatalf("expected the busy hint:\n%s", h.lastCard().Text)
	}

	// Once the preview finished, a fresh preview confirms for real.
	if _, err := h.store.UpdateJob(ctx, dry.ID, func(j *state.Job) (string, bool) {
		j.State = state.JobSucceeded
		return "finished", true
	}); err != nil {
		t.Fatal(err)
	}
	h.tap(h.collectionCardFor(id), g.cleanupBtn)
	h.advance(time.Second)
	// That second preview admitted another dry-run; finish it too.
	for _, j := range h.jobs() {
		if j.Kind == state.JobKindCleanup && !state.IsTerminal(j.State) {
			if _, err := h.store.UpdateJob(ctx, j.ID, func(j *state.Job) (string, bool) {
				j.State = state.JobSucceeded
				return "finished", true
			}); err != nil {
				t.Fatal(err)
			}
		}
	}
	h.tap(h.lastCard(), g.cleanupConfirm)
	real := false
	for _, j := range h.jobs() {
		real = real || (j.Kind == state.JobKindCleanup && !j.DryRun)
	}
	if !real {
		t.Fatalf("the confirmed cleanup did not create the real job: %+v", h.jobs())
	}
}

func TestLifecycleButtonsBelongToTheirOwnerAndChat(t *testing.T) {
	h := newHarness(t, 100)
	g := vietnamese.g()
	id := h.readyCollection("Private")
	h.tap(h.collectionCardFor(id), g.retireBtn)
	ask := h.lastCard()

	if _, _, err := h.svc.AddOperator(context.Background(), app.Owner, "op-1", 555); err != nil {
		t.Fatal(err)
	}
	// An operator replaying the owner's button, and a stranger doing the same.
	for _, user := range []int64{555, 777} {
		data := h.buttonData(ask, g.retireConfirm)
		h.api.push(h.callbackUpdate(h.nextUpdateID(), user, user, ask.MessageID, data))
		h.pull()
	}
	if got := h.collectionOf(id).State; got == state.CollectionRetired {
		t.Fatalf("a foreign click retired the collection")
	}
}

func TestOperatorCannotRetireTheOwnersCollection(t *testing.T) {
	h := newHarness(t, 100)
	g := vietnamese.g()
	id := h.readyCollection("Owner only")
	op, _, err := h.svc.AddOperator(context.Background(), app.Owner, "op-2", 555)
	if err != nil {
		t.Fatal(err)
	}
	// Even a valid button of the operator's own cannot reach a collection of
	// another principal: the application service answers not found.
	p := app.Principal{ID: op.ID, Role: app.RoleOperator}
	if _, err := h.svc.RetireCollection(context.Background(), p, "x-1", id, h.collectionOf(id).Revision); err == nil {
		t.Fatalf("an operator retired the owner's collection")
	}
	_ = g
}
