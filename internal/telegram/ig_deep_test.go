package telegram

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/drive"
	"github.com/itsgiahung/502Drive/internal/present"
	"github.com/itsgiahung/502Drive/internal/state"
)

// Batch IG, slice 2: the deep scan on the preview card (S7).

// deepTree adds a sub-folder with files, a Google-native document and a
// file the owner does not let anyone copy to the default source.
func (h *harness) deepTree() {
	sub := h.drive.Folder("Sub", h.src.ID)
	h.drive.File("a.bin", sub.ID, 100)
	h.drive.File("b.bin", sub.ID, 100)
	h.drive.Put("Notes", "application/vnd.google-apps.document", sub.ID, -1)
	locked := h.drive.File("locked.bin", sub.ID, 5000)
	h.drive.Mutate(locked.ID, func(f *drive.File) { f.CanCopy = false })
}

func TestDeepScanShowsTotalsEstimateUnknownAndQuotaWarnings(t *testing.T) {
	h := newHarness(t, 100)
	ig := vietnamese.ig()
	h.deepTree()
	h.drive.QuotaLimit, h.drive.QuotaUsage = 100, 90 // 10 bytes free
	h.svc.DailyCopyLimit = 100                       // the day's allowance is smaller than the tree

	h.say(h.srcLink())
	preview := h.lastCard()
	if !hasButton(preview, ig.deepBtn) {
		t.Fatalf("the preview needs a deep scan button: %v", preview.ButtonTexts)
	}
	h.tap(preview, ig.deepBtn)
	card := h.lastCard()

	// 3 files in the source + 3 copyable-looking files below (the locked one is
	// counted but not in the bytes) + the native document: 7 files, 1 folder.
	bytes := int64(3*12 + 100 + 100)
	for _, want := range []string{
		fmt.Sprintf(ig.deepTotals, 7, 1, present.HumanBytes(bytes)),
		fmt.Sprintf(ig.deepNative, 1),
		fmt.Sprintf(ig.deepCannot, 1),
		ig.deepWarn["estimate_unknown"],
		ig.deepWarn["destination_storage_short"],
		ig.deepWarn["may_hit_daily_limit"],
		fmt.Sprintf(ig.deepFree, present.HumanBytes(10)),
	} {
		if !strings.Contains(card.Text, want) {
			t.Fatalf("the scan card lacks %q:\n%s", want, card.Text)
		}
	}
	if strings.Contains(card.Text, strings.Split(ig.deepRunning, "%d")[0]) {
		t.Fatalf("a finished scan is not 'running':\n%s", card.Text)
	}
	if h.drive.Writes != 0 {
		t.Fatalf("a scan must never write to Drive (%d writes)", h.drive.Writes)
	}
	if len(h.jobs()) != 0 {
		t.Fatalf("a scan must not create a job")
	}

	// Warnings do not block: the same card copies.
	h.tap(card, vietnamese.copyNow)
	if len(h.jobs()) != 1 {
		t.Fatalf("copy from the scan card admitted %d jobs", len(h.jobs()))
	}
}

func TestDeepScanShowsAMeasuredEstimateWhenThereIsHistory(t *testing.T) {
	h := newHarness(t, 100)
	ig := vietnamese.ig()
	ctx := context.Background()
	h.deepTree()
	old := h.queuedJob("owner", "Earlier")
	roots, err := h.store.Items(ctx, old.CollectionID)
	if err != nil || len(roots) == 0 {
		t.Fatalf("items: %v %v", roots, err)
	}
	size := int64(1 << 20)
	done := state.Item{SourceID: "s-done", Kind: state.KindFile, Name: "big.bin", Size: &size, State: state.ItemDone}
	if err = h.store.CommitPage(ctx, roots[0], []state.Item{done}, ""); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond) // a real span to measure a speed from
	if _, err = h.store.UpdateJob(ctx, old.ID, func(j *state.Job) (string, bool) {
		j.State = state.JobSucceeded
		return "finished", true
	}); err != nil {
		t.Fatal(err)
	}
	h.say(h.srcLink())
	h.tap(h.lastCard(), ig.deepBtn)
	text := h.lastCard().Text
	if !strings.Contains(text, strings.Split(ig.deepEstimate, "%s")[0]) || strings.Contains(text, ig.deepWarn["estimate_unknown"]) {
		t.Fatalf("with history the card gives a measured estimate:\n%s", text)
	}
}

func TestRunningDeepScanIsNotATotalAndCanBeCancelled(t *testing.T) {
	h := newHarness(t, 100)
	ig := vietnamese.ig()
	h.tg.deepWait = -1   // show the card at once
	for i := range 400 { // 400 lists x 10 ms: far longer than the click-to-cancel path, even under -race load
		h.drive.Folder(fmt.Sprintf("dir-%03d", i), h.src.ID)
	}
	h.say(h.srcLink())
	preview := h.lastCard()
	h.drive.Before = func(op string, f drive.File) error {
		if op == "list" {
			time.Sleep(10 * time.Millisecond)
		}
		return nil
	}
	h.tap(preview, ig.deepBtn)
	card := h.lastCard()
	if !strings.Contains(card.Text, strings.Split(ig.deepRunning, "%d")[0]) {
		t.Fatalf("a running scan must say it is not a total yet:\n%s", card.Text)
	}
	if strings.Contains(card.Text, strings.Split(ig.deepTotals, "%d")[0]) {
		t.Fatalf("a running scan must not show a total:\n%s", card.Text)
	}
	if !hasButton(card, ig.deepRefresh) || !hasButton(card, ig.deepCancel) || !hasButton(card, vietnamese.copyNow) {
		t.Fatalf("running scan buttons = %v", card.ButtonTexts)
	}

	h.tap(card, ig.deepCancel)
	// The scan stops asynchronously: refresh until the state leaves running.
	deadline := time.Now().Add(20 * time.Second)
	for {
		card = h.lastCard()
		if strings.Contains(card.Text, ig.deepStates["cancelled"]) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the scan never reported cancelled:\n%s", card.Text)
		}
		time.Sleep(10 * time.Millisecond)
		h.tap(card, ig.deepRefresh)
	}
	if !strings.Contains(card.Text, strings.Split(ig.deepPartial, "%d")[0]) || strings.Contains(card.Text, strings.Split(ig.deepTotals, "%d")[0]) {
		t.Fatalf("a cancelled scan counts 'so far', never a total:\n%s", card.Text)
	}
	if !hasButton(card, ig.deepAgain) || hasButton(card, ig.deepCancel) {
		t.Fatalf("cancelled scan buttons = %v", card.ButtonTexts)
	}
	if h.drive.Writes != 0 {
		t.Fatalf("the scan wrote to Drive")
	}
	h.tap(card, vietnamese.backBtn)
	if !hasButton(h.lastCard(), vietnamese.copyNow) || !strings.Contains(h.lastCard().Text, "Project Alpha") {
		t.Fatalf("back must return to the preview:\n%s", h.lastCard().Text)
	}
}

func TestInterruptedDeepScanSaysSoAfterARestart(t *testing.T) {
	h := newHarness(t, 100)
	ig := vietnamese.ig()
	view, _, err := h.svc.Inspect(context.Background(), app.Owner, "deep-int", app.PlanRequest{Source: h.src.ID})
	if err != nil {
		t.Fatal(err)
	}
	// A scan recorded as running with no goroutine behind it, as after a restart.
	if err = h.store.SavePlanDeep(context.Background(), view.ID, state.DeepPreflight{State: "running", Files: 2, Scanned: 2}); err != nil {
		t.Fatal(err)
	}
	if err = h.tg.deepCard(context.Background(), app.Owner, h.ownerID, 0, view.ID, `{"source":"`+h.src.ID+`"}`); err != nil {
		t.Fatal(err)
	}
	text := h.lastCard().Text
	if !strings.Contains(text, ig.deepStates["interrupted"]) || strings.Contains(text, strings.Split(ig.deepTotals, "%d")[0]) {
		t.Fatalf("an interrupted scan is not complete:\n%s", text)
	}
	if hasButton(h.lastCard(), ig.deepCancel) {
		t.Fatalf("there is nothing to cancel after a restart: %v", h.lastCard().ButtonTexts)
	}
}

func TestDeepTextNeverCallsAPartialCountATotal(t *testing.T) {
	for _, tx := range []texts{vietnamese, english} {
		ig := tx.ig()
		for _, st := range []string{"cancelled", "expired", "interrupted", "failed"} {
			plan := state.Plan{SourceName: "Tree", Deep: &state.DeepPreflight{State: st, Files: 3, Folders: 1, Bytes: 10, Scanned: 4}}
			text := deepText(tx, plan)
			if !strings.Contains(text, ig.deepStates[st]) || strings.Contains(text, strings.Split(ig.deepTotals, "%d")[0]) {
				t.Errorf("%s/%s must be flagged as not complete:\n%s", tx.lang, st, text)
			}
		}
		done := state.Plan{SourceName: "Tree", Deep: &state.DeepPreflight{State: "complete", Complete: true, Files: 3, Folders: 1, Bytes: 10,
			Warnings: []string{"brand_new_warning"}}}
		text := deepText(tx, done)
		if !strings.Contains(text, fmt.Sprintf(ig.deepTotals, 3, 1, present.HumanBytes(10))) || !strings.Contains(text, "brand_new_warning") {
			t.Errorf("%s: a complete scan shows the total and keeps unknown warnings visible:\n%s", tx.lang, text)
		}
	}
	html := deepText(vietnamese, state.Plan{SourceName: "<b>x</b>", Deep: &state.DeepPreflight{State: "running"}})
	if strings.Contains(html, "<b>x</b>") {
		t.Fatalf("the source name must be escaped:\n%s", html)
	}
}

func TestDeepScanButtonsAreOneShotAndBelongToTheirOwner(t *testing.T) {
	h := newHarness(t, 100)
	ig := vietnamese.ig()
	op := h.addOperator(555)
	h.say(h.srcLink())
	preview := h.lastCard()
	// An operator cannot press the owner's button.
	h.pushCallback(555, preview.MessageID, h.buttonData(preview, ig.deepBtn))
	if h.lastAnswer() != vietnamese.expired {
		t.Fatalf("a foreign deep scan button answered %q", h.lastAnswer())
	}
	// A forged refresh for the owner's plan cannot read it either.
	view, _, err := h.svc.Inspect(context.Background(), app.Owner, "deep-own", app.PlanRequest{Source: h.src.ID, Deep: true})
	if err != nil {
		t.Fatal(err)
	}
	h.pushCallback(555, preview.MessageID, h.forge(op.ID, 555, "deep_refresh", view.ID, 1, "{}"))
	if strings.Contains(h.lastCard().Text, "Project Alpha") && strings.Contains(h.lastCard().Text, strings.Split(ig.deepTotals, "%d")[0]) {
		t.Fatalf("an operator read the owner's scan:\n%s", h.lastCard().Text)
	}
	// The owner's button works once.
	h.tap(preview, ig.deepBtn)
	h.api.push(h.click(preview, ig.deepBtn))
	h.pull()
	if h.lastAnswer() != vietnamese.expired {
		t.Fatalf("a consumed deep scan button answered %q", h.lastAnswer())
	}
}
