package telegram

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/itsgiahung/502Drive/internal/app"
)

// Batch G, slice 2c: /find NAME (search Drive, pick a result as the source). Since
// batch IG it uses the Drive search API (see ig_sync_report_test.go for the deep search).

func TestFindListsMatchesAndPickingOneOpensTheCopyPreview(t *testing.T) {
	h := newHarness(t, 100)
	g := vietnamese.g()
	target := h.drive.Folder("Quarterly Report", "root")
	h.drive.File("report-final.pdf", "root", 10)
	h.drive.File("holiday.jpg", "root", 10)

	h.say("/find REPORT")
	card := h.lastCard()
	if !strings.Contains(card.Text, "REPORT") || !hasButton(card, "Quarterly Report") || !hasButton(card, "report-final.pdf") {
		t.Fatalf("matches should be listed case-insensitively: %v\n%s", card.ButtonTexts, card.Text)
	}
	if hasButton(card, "holiday.jpg") {
		t.Fatalf("a non-matching file was listed: %v", card.ButtonTexts)
	}
	if !strings.Contains(card.Text, g.findScope) {
		t.Fatalf("the card must say what is searched:\n%s", card.Text)
	}

	h.tap(card, "Quarterly Report")
	preview := h.lastCard()
	if !strings.Contains(preview.Text, "Quarterly Report") || !hasButton(preview, vietnamese.copyNow) {
		t.Fatalf("picking a result should open the copy preview:\n%s %v", preview.Text, preview.ButtonTexts)
	}
	h.tap(preview, vietnamese.copyNow)
	jobs := h.jobs()
	if len(jobs) != 1 || jobs[0].SourceName != "Quarterly Report" {
		t.Fatalf("the picked folder should be the source: %+v (target %s)", jobs, target.ID)
	}
}

func TestFindWithoutAQueryExplainsTheCommand(t *testing.T) {
	h := newHarness(t, 100)
	h.say("/find")
	if !strings.Contains(h.lastCard().Text, "/find") {
		t.Fatalf("usage missing: %q", h.lastCard().Text)
	}
}

func TestFindWithNoMatchSaysSo(t *testing.T) {
	h := newHarness(t, 100)
	h.say("/find zzz-nothing")
	if !strings.Contains(h.lastCard().Text, vietnamese.g().findNone) {
		t.Fatalf("expected the empty result:\n%s", h.lastCard().Text)
	}
}

func TestFindEscapesNamesAndCapsTheList(t *testing.T) {
	h := newHarness(t, 100)
	g := vietnamese.g()
	for i := range findMaxResults + 4 {
		h.drive.Folder(fmt.Sprintf("<b>x</b> match %02d", i), "root")
	}
	h.say("/find match")
	card := h.lastCard()
	if strings.Contains(card.Text, "<b>x</b>") {
		t.Fatalf("Drive names must be HTML-escaped in the text:\n%s", card.Text)
	}
	picks := 0
	for _, b := range card.ButtonTexts {
		if strings.Contains(b, "match") {
			picks++
		}
	}
	if picks != findMaxResults || !strings.Contains(card.Text, fmt.Sprintf(g.findMore, 4)) {
		t.Fatalf("expected %d results and a remainder note, got %d:\n%s", findMaxResults, picks, card.Text)
	}
}

func TestOperatorFindNeverTouchesDriveAndStrangersStaySilent(t *testing.T) {
	h := newHarness(t, 100)
	if _, _, err := h.svc.AddOperator(context.Background(), app.Owner, "op-f", 555); err != nil {
		t.Fatal(err)
	}
	h.drive.Folder("Private report", "root")
	listsBefore := h.drive.CallCount("list")

	h.api.push(h.messageUpdate(h.nextUpdateID(), 555, 555, "private", "/find report"))
	h.pull()
	text := h.lastCard().Text
	if strings.Contains(text, "Private report") {
		t.Fatalf("an operator saw the owner's Drive:\n%s", text)
	}
	if h.drive.CallCount("list") != listsBefore {
		t.Fatalf("an operator's /find reached Drive")
	}

	before, _ := h.api.messages()
	h.api.push(h.messageUpdate(h.nextUpdateID(), 777, 777, "private", "/find report"))
	h.pull()
	if after, _ := h.api.messages(); len(after) != len(before) {
		t.Fatalf("a stranger got a reply")
	}
	if h.drive.CallCount("list") != listsBefore {
		t.Fatalf("a stranger's /find reached Drive")
	}
}

func TestFindResultButtonsAreOneShot(t *testing.T) {
	h := newHarness(t, 100)
	h.drive.Folder("Once report", "root")
	h.say("/find once")
	card := h.lastCard()
	h.tap(card, "Once report")
	h.api.push(h.click(card, "Once report"))
	h.pull()
	if h.lastAnswer() != vietnamese.expired {
		t.Fatalf("a consumed result button answered %q", h.lastAnswer())
	}
}
