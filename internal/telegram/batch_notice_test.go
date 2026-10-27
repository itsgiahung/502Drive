package telegram

import (
	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/state"
	"testing"
	"time"
)

func TestBatchSummaryWaitsForAllJobsAndSurvivesBotRestart(t *testing.T) {
	h := newHarness(t, 100)
	ctx := t.Context()
	a := h.queuedJob("owner", "One")
	b := h.queuedJob("owner", "Two")
	for _, id := range []string{a.ID, b.ID} {
		if _, err := h.store.UpdateJob(ctx, id, func(j *state.Job) (string, bool) { j.BatchID = "batch-notice-test"; return "test", true }); err != nil {
			t.Fatal(err)
		}
	}
	if err := h.svc.SealBatch(ctx, app.Owner, "batch-notice-test"); err != nil {
		t.Fatal(err)
	}
	h.setJobState(a.ID, state.JobSucceeded)
	h.advance(4 * time.Second)
	h.tg.flush(ctx)
	sent, _ := h.api.messages()
	if len(sent) != 0 {
		t.Fatal("announced before all jobs ended")
	}
	h.setJobState(b.ID, state.JobFailed)
	h.tg.flush(ctx)
	sent, _ = h.api.messages()
	if len(sent) != 1 {
		t.Fatalf("summaries=%d", len(sent))
	}
	h.tg = h.newTelegram()
	if err := h.svc.SealBatch(ctx, app.Owner, "batch-notice-test"); err != nil {
		t.Fatal(err)
	}
	h.tg.flush(ctx)
	sent, _ = h.api.messages()
	if len(sent) != 1 {
		t.Fatalf("restart duplicated summary: %d", len(sent))
	}
	jobs, err := h.svc.Batch(ctx, app.Owner, "batch-notice-test")
	if err != nil {
		t.Fatal(err)
	}
	callback := state.Callback{Object: "batch-notice-test", Revision: batchRevision(jobs), Action: "batch_retry_failed"}
	h.setJobState(b.ID, state.JobCancelled)
	if err = h.tg.batchNoticeAction(ctx, app.Owner, callback); err != state.ErrStale {
		t.Fatal("stale batch action accepted", err)
	}
}
