package telegram

import (
	"fmt"
	"github.com/itsgiahung/502Drive/internal/app"
	"strings"
	"testing"
)

func TestOperatorFindIsPagedScopedAndReadOnly(t *testing.T) {
	h := newHarness(t, 100)
	op := h.addOperator(555)
	for i := range 12 {
		h.queuedJob(op.ID, fmt.Sprintf("Report own %02d", i))
	}
	h.queuedJob("owner", "Report owner hidden")
	h.sayAs(555, "/find Report")
	card := h.lastCard()
	if !strings.Contains(card.Text, "Report own") || strings.Contains(card.Text, "owner hidden") || h.drive.CallCount("search") != 0 {
		t.Fatal(card.Text)
	}
	h.tapAs(555, card, vietnamese.nextBtn)
	if !strings.Contains(h.lastCard().Text, "Report own 00") {
		t.Fatal(h.lastCard().Text)
	}
	all, _, err := h.svc.FindLibrary(t.Context(), app.Owner, "Report", 20, 0)
	if err != nil || len(all) != 13 {
		t.Fatalf("owner sees %d err=%v", len(all), err)
	}
	if _, _, err = h.svc.FindLibrary(t.Context(), app.Principal{}, "", 20, 0); err == nil {
		t.Fatal("empty identity saw library")
	}
	before, _ := h.api.messages()
	h.api.push(h.messageUpdate(h.nextUpdateID(), 777, 777, "private", "/find Report"))
	h.pull()
	h.api.push(h.messageUpdate(h.nextUpdateID(), 555, -100, "group", "/find Report"))
	h.pull()
	after, _ := h.api.messages()
	if len(after) != len(before) {
		t.Fatal("unauthorized chat got results")
	}
}
