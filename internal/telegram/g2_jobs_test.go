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

// Batch G, slice 2b: the job management page (filter by state, batch actions).

func (h *harness) openManage() fakeMessage {
	h.t.Helper()
	h.say(vietnamese.kbJobs)
	h.tap(h.lastCard(), vietnamese.g().manageBtn)
	return h.lastCard()
}

func (h *harness) setJobState(id, st string) {
	h.t.Helper()
	if _, err := h.store.UpdateJob(context.Background(), id, func(j *state.Job) (string, bool) {
		j.State = st
		return "test", true
	}); err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) jobsIn(st string) int {
	n := 0
	for _, j := range h.jobs() {
		if j.State == st {
			n++
		}
	}
	return n
}

func TestManagePageFiltersJobsByState(t *testing.T) {
	h := newHarness(t, 100)
	g := vietnamese.g()
	queued := h.queuedJob("owner", "Queued one")
	paused := h.queuedJob("owner", "Paused one")
	failed := h.queuedJob("owner", "Failed one")
	done := h.queuedJob("owner", "Done one")
	h.setJobState(paused.ID, state.JobPaused)
	h.setJobState(failed.ID, state.JobFailed)
	h.setJobState(done.ID, state.JobSucceeded)
	_ = queued

	card := h.openManage()
	for _, name := range []string{"Queued one", "Paused one", "Failed one", "Done one"} {
		if !hasButton(card, name) {
			t.Fatalf("the unfiltered page should list %q: %v", name, card.ButtonTexts)
		}
	}
	for _, want := range []struct{ filter, shown, hidden string }{
		{"paused", "Paused one", "Queued one"},
		{"failed", "Failed one", "Paused one"},
		{"done", "Done one", "Failed one"},
		{"queued", "Queued one", "Done one"},
	} {
		h.tap(card, g.filterNames[want.filter])
		page := h.lastCard()
		if !hasButton(page, want.shown) || hasButton(page, want.hidden) {
			t.Fatalf("filter %s: shown=%v\n%s", want.filter, page.ButtonTexts, page.Text)
		}
		card = page
	}
}

func TestBatchPauseAndResumeActOnEveryJobOfTheFilter(t *testing.T) {
	h := newHarness(t, 100)
	g := vietnamese.g()
	for i := range 3 {
		h.queuedJob("owner", fmt.Sprintf("Job %d", i))
	}
	card := h.openManage()
	h.tap(card, fmt.Sprintf(g.batchPause, 3))
	if h.jobsIn(state.JobPaused) != 3 {
		t.Fatalf("all three should be paused: %+v", h.jobs())
	}
	result := h.lastCard()
	if !strings.Contains(result.Text, fmt.Sprintf(g.batchResult, g.batchActions["pause"], 3, 0, 0)) {
		t.Fatalf("the result must be truthful:\n%s", result.Text)
	}

	// The result card goes back to the page, where Resume is now offered.
	h.tap(result, g.manageBtn)
	page := h.lastCard()
	h.tap(page, fmt.Sprintf(g.batchResume, 3))
	if h.jobsIn(state.JobQueued) != 3 {
		t.Fatalf("all three should be queued again: %+v", h.jobs())
	}
}

func TestBatchButtonsAreOneShot(t *testing.T) {
	h := newHarness(t, 100)
	g := vietnamese.g()
	h.queuedJob("owner", "A")
	h.queuedJob("owner", "B")
	card := h.openManage()
	label := fmt.Sprintf(g.batchPause, 2)
	h.tap(card, label)
	// Someone resumes one job meanwhile; the stale batch button must not pause it again.
	if js := h.jobs(); len(js) > 0 {
		j := js[0]
		if _, err := h.svc.JobAction(context.Background(), app.Owner, "resume-"+j.ID, j.ID, "resume", j.Revision); err != nil {
			t.Fatal(err)
		}
	}
	h.api.push(h.click(card, label))
	h.pull()
	if h.lastAnswer() != vietnamese.expired || h.jobsIn(state.JobPaused) != 1 {
		t.Fatalf("a consumed batch button acted again (answer %q): %+v", h.lastAnswer(), h.jobs())
	}
}

func TestBatchRetryRetriesFailedJobsButNotQuotaWaits(t *testing.T) {
	h := newHarness(t, 100)
	g := vietnamese.g()
	ctx := context.Background()
	failed := h.queuedJob("owner", "Failed")
	h.setJobState(failed.ID, state.JobFailed)
	quota := h.queuedJob("owner", "Quota wait")
	resume := h.clock().Add(2 * time.Hour)
	if _, err := h.store.UpdateJob(ctx, quota.ID, func(j *state.Job) (string, bool) {
		j.State = state.JobBlocked
		j.ResumeAfter = &resume
		return "blocked", true
	}); err != nil {
		t.Fatal(err)
	}
	card := h.openManage()
	if hasButton(card, fmt.Sprintf(g.batchRetry, 2)) {
		t.Fatalf("a quota wait must not count as retryable: %v", card.ButtonTexts)
	}
	h.tap(card, fmt.Sprintf(g.batchRetry, 1))
	if got := h.jobState(failed.ID); got != state.JobQueued {
		t.Fatalf("the failed job should be queued, is %s", got)
	}
	if got := h.jobState(quota.ID); got != state.JobBlocked {
		t.Fatalf("the quota wait must be left alone, is %s", got)
	}
}

func TestBatchCancelAsksFirstAndTheConfirmationExpires(t *testing.T) {
	h := newHarness(t, 100)
	g := vietnamese.g()
	a := h.queuedJob("owner", "A")
	b := h.queuedJob("owner", "B")
	card := h.openManage()
	h.tap(card, fmt.Sprintf(g.batchCancel, 2))
	ask := h.lastCard()
	if h.jobState(a.ID) == state.JobCancelled || h.jobState(b.ID) == state.JobCancelled {
		t.Fatalf("the first tap cancelled something")
	}
	if !strings.Contains(ask.Text, "2") || styleOf(ask, g.batchCancel[:strings.Index(g.batchCancel, "%")]) != "danger" {
		t.Fatalf("the confirmation must count the jobs and be red: %v / %v\n%s", ask.ButtonTexts, ask.Styles, ask.Text)
	}

	// Expired confirmation: nothing happens.
	h.advance(lifecycleConfirmTTL + time.Minute)
	h.tap(ask, fmt.Sprintf(g.batchCancel, 2))
	if h.jobState(a.ID) == state.JobCancelled {
		t.Fatalf("an expired confirmation cancelled a job")
	}
	if !strings.Contains(h.lastCard().Text, g.confirmExpired) {
		t.Fatalf("expected the expiry notice:\n%s", h.lastCard().Text)
	}

	// A fresh ask and an in-time confirmation cancel both.
	card = h.openManage()
	h.tap(card, fmt.Sprintf(g.batchCancel, 2))
	h.tap(h.lastCard(), fmt.Sprintf(g.batchCancel, 2))
	if h.jobState(a.ID) != state.JobCancelled || h.jobState(b.ID) != state.JobCancelled {
		t.Fatalf("both jobs should be cancelled: %s %s", h.jobState(a.ID), h.jobState(b.ID))
	}
}

func TestBatchSkipsAJobThatMovedOnInsteadOfFailing(t *testing.T) {
	h := newHarness(t, 100)
	g := vietnamese.g()
	a := h.queuedJob("owner", "A")
	b := h.queuedJob("owner", "B")
	card := h.openManage()
	h.tap(card, fmt.Sprintf(g.batchCancel, 2))
	ask := h.lastCard()
	// One job finishes before the confirmation arrives.
	h.setJobState(a.ID, state.JobSucceeded)
	h.tap(ask, fmt.Sprintf(g.batchCancel, 2))
	if h.jobState(a.ID) != state.JobSucceeded {
		t.Fatalf("a finished job was touched")
	}
	if h.jobState(b.ID) != state.JobCancelled {
		t.Fatalf("the open job should be cancelled: %s", h.jobState(b.ID))
	}
}

func TestOperatorBatchOnlyTouchesTheirOwnJobs(t *testing.T) {
	h := newHarness(t, 100)
	g := vietnamese.g()
	op, _, err := h.svc.AddOperator(context.Background(), app.Owner, "op-j", 555)
	if err != nil {
		t.Fatal(err)
	}
	mine := h.queuedJob(op.ID, "Operator job")
	theirs := h.queuedJob("owner", "Owner job")

	h.api.push(h.messageUpdate(h.nextUpdateID(), 555, 555, "private", vietnamese.kbJobs))
	h.pull()
	jobsCard := h.lastCard()
	h.api.push(h.callbackUpdate(h.nextUpdateID(), 555, 555, jobsCard.MessageID, h.buttonData(jobsCard, g.manageBtn)))
	h.pull()
	page := h.lastCard()
	if hasButton(page, "Owner job") || !hasButton(page, "Operator job") {
		t.Fatalf("the operator page must list only their jobs: %v", page.ButtonTexts)
	}
	h.api.push(h.callbackUpdate(h.nextUpdateID(), 555, 555, page.MessageID, h.buttonData(page, fmt.Sprintf(g.batchPause, 1))))
	h.pull()
	if h.jobState(mine.ID) != state.JobPaused || h.jobState(theirs.ID) != state.JobQueued {
		t.Fatalf("operator batch states: mine=%s theirs=%s", h.jobState(mine.ID), h.jobState(theirs.ID))
	}
}

func TestManagePageIsHiddenFromStrangers(t *testing.T) {
	h := newHarness(t, 100)
	h.queuedJob("owner", "Secret")
	h.api.push(h.messageUpdate(h.nextUpdateID(), 777, 777, "private", vietnamese.kbJobs))
	h.pull()
	if sent, _ := h.api.messages(); len(sent) != 0 {
		t.Fatalf("a stranger received a jobs card: %+v", sent)
	}
}

func TestManagePagePagesLongLists(t *testing.T) {
	h := newHarness(t, 100)
	g := vietnamese.g()
	for i := range manageJobsPerPage + 3 {
		h.queuedJob("owner", fmt.Sprintf("Bulk %02d", i))
	}
	card := h.openManage()
	if !hasButton(card, vietnamese.nextBtn) {
		t.Fatalf("a long list needs a next-page button: %v", card.ButtonTexts)
	}
	h.tap(card, vietnamese.nextBtn)
	page := h.lastCard()
	if !hasButton(page, g.pagePrev) || !hasButton(page, "Bulk 00") {
		t.Fatalf("the second page should hold the oldest jobs: %v", page.ButtonTexts)
	}
}
