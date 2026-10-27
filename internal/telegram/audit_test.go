package telegram

import (
	"context"
	"strings"
	"testing"

	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/state"
)

func auditCardFixture(t *testing.T, h *harness) (tx texts, collectionID string) {
	t.Helper()
	h.t.Helper()
	job := h.queuedJob("owner", "One")
	if _, err := h.store.UpdateJob(context.Background(), job.ID, func(j *state.Job) (string, bool) {
		j.State = state.JobSucceeded
		return "finished", true
	}); err != nil {
		t.Fatal(err)
	}
	return h.tg.text(context.Background(), app.Owner), job.CollectionID
}

func TestCollectionCardOffersTheIntegrityCheckAndRepair(t *testing.T) {
	h := newHarness(t, 100)
	tx, col := auditCardFixture(t, h)
	ctx := context.Background()

	// The card carries the 🔍 button; tapping it admits the audit.
	if err := h.tg.collectionCard(ctx, app.Owner, 100, 10, col, ""); err != nil {
		t.Fatal(err)
	}
	card := h.lastCard()
	_ = h.buttonData(card, tx.auditCheck) // the 🔍 row is on the card
	h.tap(card, tx.auditCheck)
	started := h.lastCard()
	if !strings.Contains(started.Text, "job_") {
		t.Fatalf("started card = %q", started.Text)
	}
	// Exactly one audit job was admitted (the callback is one-shot).
	a, ok, err := h.tg.svc.LastAudit(ctx, app.Owner, col)
	if err != nil || !ok {
		t.Fatalf("no audit recorded: %v", err)
	}
	audit := a

	// The engine finishes it; the card then shows the summary and Sửa.
	if _, err := h.store.UpdateJob(ctx, audit.JobID, func(j *state.Job) (string, bool) {
		j.State = state.JobSucceeded
		return "finished", true
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.CompleteAudit(ctx, audit.ID, state.AuditCompleted,
		map[string]int{state.FindingMissing: 1}, nil); err != nil {
		t.Fatal(err)
	}
	if err := h.tg.collectionCard(ctx, app.Owner, 100, 20, col, ""); err != nil {
		t.Fatal(err)
	}
	card = h.lastCard()
	if !strings.Contains(card.Text, "1") || !strings.Contains(card.Text, "sai lệch") {
		t.Fatalf("summary missing from the card: %q", card.Text)
	}
	_ = h.buttonData(card, tx.auditRepair) // Sửa appears with the findings
	h.tap(card, tx.auditRepair)
	started = h.lastCard()
	if !strings.Contains(started.Text, "job_") {
		t.Fatalf("repair card = %q", started.Text)
	}
	j, err := h.store.GetJob(ctx, "", extractJobID(started.Text))
	if err != nil || j.Kind != state.JobKindRepair {
		t.Fatalf("repair job = %+v err=%v", j, err)
	}
}

func extractJobID(cardText string) string {
	i := strings.Index(cardText, "job_")
	if i < 0 {
		return ""
	}
	rest := cardText[i:]
	for j := 0; j < len(rest); j++ {
		c := rest[j]
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_') {
			return rest[:j]
		}
	}
	return rest
}
