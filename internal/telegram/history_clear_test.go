package telegram

import (
	"reflect"
	"strings"
	"testing"

	"github.com/itsgiahung/502Drive/internal/state"
)

func TestClearHistoryFromChatPreviewsThenConfirms(t *testing.T) {
	h := newHarness(t, 100)
	ht := vietnamese.hist()
	done := h.queuedJob("owner", "Done one")
	live := h.queuedJob("owner", "Still queued")
	h.setJobState(done.ID, state.JobSucceeded)

	card := h.openManage()
	if !hasButton(card, ht.btn) {
		t.Fatalf("the jobs page lacks the clear-history button: %v", card.ButtonTexts)
	}
	h.tap(card, ht.btn)
	card = h.lastCard()
	for _, want := range []string{"7 ngày", "30 ngày", ht.all} {
		if !hasButton(card, want) {
			t.Fatalf("the range picker lacks %q: %v", want, card.ButtonTexts)
		}
	}
	h.tap(card, ht.all)
	card = h.lastCard()
	if !strings.Contains(card.Text, "1</b> job") || !strings.Contains(card.Text, "không bị đụng tới") || !hasButton(card, ht.confirmBtn) {
		t.Fatalf("the preview must say what happens and ask once:\n%s\n%v", card.Text, card.ButtonTexts)
	}
	if n := len(h.jobs()); n != 2 {
		t.Fatalf("the preview must not clear anything, %d jobs listed", n)
	}
	h.tap(card, ht.confirmBtn)
	if !strings.Contains(h.lastCard().Text, "Đã xóa <b>1</b>") {
		t.Fatalf("result card: %s", h.lastCard().Text)
	}
	jobs := h.jobs()
	if len(jobs) != 1 || jobs[0].ID != live.ID {
		t.Fatalf("only the finished job leaves the list: %+v", jobs)
	}
}

func TestClearHistoryFromChatWithNothingFinishedSaysSo(t *testing.T) {
	h := newHarness(t, 100)
	ht := vietnamese.hist()
	h.queuedJob("owner", "Queued one")
	card := h.openManage()
	h.tap(card, ht.btn)
	h.tap(h.lastCard(), ht.all)
	if card = h.lastCard(); !strings.Contains(card.Text, ht.nothing) || hasButton(card, ht.confirmBtn) {
		t.Fatalf("nothing to clear must not offer a confirmation:\n%s\n%v", card.Text, card.ButtonTexts)
	}
}

func TestHistoryTextsExistInBothLanguages(t *testing.T) {
	a, b := reflect.ValueOf(historyVI), reflect.ValueOf(historyEN)
	for i := 0; i < a.NumField(); i++ {
		if a.Field(i).String() == "" || b.Field(i).String() == "" {
			t.Errorf("historyTexts.%s is empty in one language", a.Type().Field(i).Name)
		}
	}
}

func TestOperatorClearsOnlyTheirOwnFinishedJobs(t *testing.T) {
	h := newHarness(t, 100)
	ht := vietnamese.hist()
	op := h.addOperator(777)
	mine := h.queuedJob(op.ID, "Operator done")
	owners := h.queuedJob("owner", "Owner done")
	h.setJobState(mine.ID, state.JobSucceeded)
	h.setJobState(owners.ID, state.JobSucceeded)

	h.sayAs(777, vietnamese.kbJobs)
	h.tapAs(777, h.lastCard(), vietnamese.g().manageBtn)
	h.tapAs(777, h.lastCard(), ht.btn)
	card := h.lastCard()
	if hasButton(card, ht.all) || !hasButton(card, ht.own) {
		t.Fatalf("an operator only gets their own jobs: %v", card.ButtonTexts)
	}
	h.tapAs(777, card, ht.own)
	h.tapAs(777, h.lastCard(), ht.confirmBtn)
	ids := map[string]bool{}
	for _, j := range h.jobs() {
		ids[j.ID] = true
	}
	if ids[mine.ID] || !ids[owners.ID] {
		t.Fatalf("the operator cleared their job only: %v", ids)
	}
}

func TestFinishedJobWithSkippedDuplicatesReadsOneHundredPercent(t *testing.T) {
	total := 35
	j := state.Job{State: state.JobSucceeded, RootName: "Folder", Counts: state.Counts{Completed: 32, Skipped: 3, Total: &total}}
	if got := jobText(vietnamese, j); !strings.Contains(got, "100%") {
		t.Fatalf("32 copied + 3 skipped of 35 is settled, got:\n%s", got)
	}
}
