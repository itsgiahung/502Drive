package telegram

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/state"
)

// queuedJob admits one more copy job (its own collection) for principal.
func (h *harness) queuedJob(principal, name string) state.Job {
	h.t.Helper()
	ctx := context.Background()
	src := h.drive.Folder(name, "root")
	plan := state.Plan{ID: state.ID("plan"), Principal: principal, SourceID: src.ID, SourceName: src.Name,
		SourceKind: state.KindFolder, DestinationID: h.dest.ID, DestinationFolder: h.dest.FolderID,
		RootName: src.Name, ExpiresAt: time.Now().Add(time.Hour), Revision: 1}
	if err := h.store.PutPlan(ctx, plan); err != nil {
		h.t.Fatal(err)
	}
	res, err := h.store.Admit(ctx, principal, state.ID("admit"), plan.ID, 1)
	if err != nil {
		h.t.Fatal(err)
	}
	return res.Job
}

func (h *harness) jobState(id string) string {
	h.t.Helper()
	j, err := h.store.GetJob(context.Background(), "", id)
	if err != nil {
		h.t.Fatal(err)
	}
	return j.State
}

func TestCancelAllAsksThenCancelsEveryOpenJob(t *testing.T) {
	h := newHarness(t, 100)
	tx := h.tg.text(context.Background(), app.Owner)
	a := h.queuedJob("owner", "One")
	b := h.queuedJob("owner", "Two")
	done := h.queuedJob("owner", "Three")
	ctx := context.Background()
	if _, err := h.store.UpdateJob(ctx, done.ID, func(j *state.Job) (string, bool) {
		j.State = state.JobSucceeded
		return "finished", true
	}); err != nil {
		t.Fatal(err)
	}

	h.api.push(h.messageUpdate(h.nextUpdateID(), 100, 100, "private", "/cancelall"))
	h.pull()
	card := h.lastCard()
	if !strings.Contains(card.Text, "2 ") {
		t.Fatalf("the card does not count the two open jobs: %q", card.Text)
	}
	var red bool
	for i, label := range card.ButtonTexts {
		if label == tx.cancelAll {
			red = card.Styles[i] == "danger"
		}
	}
	if !red {
		t.Fatalf("the confirm button is missing or not red: %v / %v", card.ButtonTexts, card.Styles)
	}

	h.api.push(h.click(card, tx.cancelAll))
	h.pull()
	for _, id := range []string{a.ID, b.ID} {
		if got := h.jobState(id); got != state.JobCancelled {
			t.Fatalf("%s state = %s", id, got)
		}
	}
	if got := h.jobState(done.ID); got != state.JobSucceeded {
		t.Fatalf("a finished job was touched: %s", got)
	}
	doneCard := h.lastCard()
	if !strings.Contains(doneCard.Text, "2") {
		t.Fatalf("the summary does not report two cancellations: %q", doneCard.Text)
	}
}

func TestCancelAllCallbackIsOneShot(t *testing.T) {
	h := newHarness(t, 100)
	tx := h.tg.text(context.Background(), app.Owner)
	job := h.queuedJob("owner", "One")

	h.api.push(h.messageUpdate(h.nextUpdateID(), 100, 100, "private", "/cancelall"))
	h.pull()
	card := h.lastCard()
	press := h.click(card, tx.cancelAll)
	h.api.push(press)
	h.pull()
	if got := h.jobState(job.ID); got != state.JobCancelled {
		t.Fatalf("state = %s", got)
	}
	// A redelivered/duplicated tap must not act twice.
	h.api.push(h.callbackUpdate(h.nextUpdateID(), 100, 100, card.MessageID, h.buttonData(card, tx.cancelAll)))
	h.pull()
	if !strings.Contains(strings.Join(h.api.answerTexts(), "|"), tx.expired) {
		t.Fatalf("no expired toast: %v", h.api.answerTexts())
	}
}

func TestCancelAllIsScopedToTheCaller(t *testing.T) {
	h := newHarness(t, 100)
	ctx := context.Background()
	op, _, err := h.store.AddOperator(ctx, "owner", "op-1", 200)
	if err != nil {
		t.Fatal(err)
	}
	mine := h.queuedJob(op.ID, "Operator folder")
	theirs := h.queuedJob("owner", "Owner folder")

	h.api.push(h.messageUpdate(h.nextUpdateID(), 200, 200, "private", "/cancelall"))
	h.pull()
	card := h.lastCard()
	if !strings.Contains(card.Text, "1") || strings.Contains(card.Text, "Owner folder") {
		t.Fatalf("an operator saw another principal's jobs: %q", card.Text)
	}
	label := h.tg.text(ctx, app.Principal{ID: op.ID, Role: app.RoleOperator}).cancelAll
	h.api.push(h.callbackUpdate(h.nextUpdateID(), 200, 200, card.MessageID, h.buttonData(card, label)))
	h.pull()
	if got := h.jobState(mine.ID); got != state.JobCancelled {
		t.Fatalf("operator job = %s", got)
	}
	if got := h.jobState(theirs.ID); got != state.JobQueued {
		t.Fatalf("the owner's job was cancelled by an operator: %s", got)
	}
}

func TestCancelAllSaysWhenNothingIsOpen(t *testing.T) {
	h := newHarness(t, 100)
	tx := h.tg.text(context.Background(), app.Owner)
	h.queuedJob("owner", "Done")
	if _, err := h.store.UpdateJob(context.Background(), h.jobs()[0].ID, func(j *state.Job) (string, bool) {
		j.State = state.JobSucceeded
		return "finished", true
	}); err != nil {
		t.Fatal(err)
	}
	h.api.push(h.messageUpdate(h.nextUpdateID(), 100, 100, "private", "/cancelall"))
	h.pull()
	card := h.lastCard()
	if !strings.Contains(card.Text, tx.cancelAllNone) {
		t.Fatalf("card = %q", card.Text)
	}
	for _, label := range card.ButtonTexts {
		if label == tx.cancelAll {
			t.Fatal("an empty list still offers the confirm button")
		}
	}
}

func TestJobsPageOffersCancelAllWhenSomethingIsOpen(t *testing.T) {
	h := newHarness(t, 100)
	tx := h.tg.text(context.Background(), app.Owner)
	h.queuedJob("owner", "Open")
	h.api.push(h.messageUpdate(h.nextUpdateID(), 100, 100, "private", "/jobs"))
	h.pull()
	card := h.lastCard()
	found := false
	for i, label := range card.ButtonTexts {
		if label == tx.cancelAll && card.Styles[i] == "danger" {
			found = true
		}
	}
	if !found {
		t.Fatalf("no red cancel-all button on the jobs page: %v / %v", card.ButtonTexts, card.Styles)
	}
}
