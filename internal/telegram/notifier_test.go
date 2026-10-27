package telegram

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/state"
)

// admit creates one queued job for principal, like a CLI copy would.
func admit(t *testing.T, store *state.Store, principal, name string) state.Job {
	t.Helper()
	ctx := context.Background()
	plan := state.Plan{ID: state.ID("plan"), Principal: principal, SourceID: state.ID("src"), SourceName: name,
		SourceKind: state.KindFolder, DestinationID: "dst", DestinationFolder: "dest", RootName: name,
		ExpiresAt: time.Now().Add(time.Hour), Revision: 1}
	if err := store.PutPlan(ctx, plan); err != nil {
		t.Fatal(err)
	}
	res, err := store.Admit(ctx, principal, state.ID("admit"), plan.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	return res.Job
}

// finish marks a job terminal with an optional failure code.
func finish(t *testing.T, store *state.Store, jobID, jobState, code string) state.Job {
	t.Helper()
	job, err := store.UpdateJob(context.Background(), jobID, func(j *state.Job) (string, bool) {
		j.State = jobState
		if code != "" {
			j.LastError = &state.Failure{Code: code, Message: "fixture " + code}
		}
		return "finished", true
	})
	if err != nil {
		t.Fatal(err)
	}
	return job
}

// completeItems makes the baseline leave no pending work behind.
func completeItems(t *testing.T, store *state.Store, jobID string) {
	t.Helper()
	ctx := context.Background()
	items, err := store.ItemsInState(ctx, jobID, state.ItemPending)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if err := store.CompleteItem(ctx, it, "dst_"+it.SourceID, func(id string) string { return "https://drive.google.com/x/" + id }); err != nil {
			t.Fatal(err)
		}
	}
}

// readyCollection admits a baseline job and marks the collection ready.
func readyCollection(t *testing.T, store *state.Store, principal, name string) state.Collection {
	t.Helper()
	job := admit(t, store, principal, name)
	if err := store.SetCollectionState(context.Background(), job.CollectionID, "ready"); err != nil {
		t.Fatal(err)
	}
	c, err := store.GetCollection(context.Background(), "", job.CollectionID)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func pendingFinal(t *testing.T, store *state.Store) []state.Outbox {
	t.Helper()
	rows, err := store.DueOutbox(context.Background(), time.Now().Add(24*time.Hour), 50)
	if err != nil {
		t.Fatal(err)
	}
	var out []state.Outbox
	for _, r := range rows {
		if r.Kind == "final" {
			out = append(out, r)
		}
	}
	return out
}

func TestNotifierQueuesACompletedCLIJobOnce(t *testing.T) {
	h := newHarness(t, 42)
	n := NewNotifier(NotifierOptions{Service: h.svc, OwnerID: 42, Now: h.clock})
	job := admit(t, h.store, "owner", "CLI copy")
	finish(t, h.store, job.ID, state.JobSucceeded, "")

	for range 2 { // the hook may run twice (restart, coalesced bookkeeping)
		if err := n.JobFinished(context.Background(), job.ID); err != nil {
			t.Fatal(err)
		}
	}
	rows := pendingFinal(t, h.store)
	if len(rows) != 1 {
		t.Fatalf("final rows = %d, want 1", len(rows))
	}
	if rows[0].ChatID != 42 || rows[0].Principal != "owner" || rows[0].Note != state.JobSucceeded {
		t.Fatalf("row = %+v", rows[0])
	}
}

func TestNotifierSkipsJobsThatAlreadyHaveAPanel(t *testing.T) {
	h := newHarness(t, 42)
	n := NewNotifier(NotifierOptions{Service: h.svc, OwnerID: 42, Now: h.clock})
	job := admit(t, h.store, "owner", "Chat copy")
	if err := h.store.EnqueueProgress(context.Background(), "owner", job.ID, 42, time.Now()); err != nil {
		t.Fatal(err)
	}
	finish(t, h.store, job.ID, state.JobSucceeded, "")
	if err := n.JobFinished(context.Background(), job.ID); err != nil {
		t.Fatal(err)
	}
	if rows := pendingFinal(t, h.store); len(rows) != 0 {
		t.Fatalf("a chat job got a final message: %+v", rows)
	}
}

func TestNotifierStaysSilentForANoChangeSyncRun(t *testing.T) {
	h := newHarness(t, 42)
	n := NewNotifier(NotifierOptions{Service: h.svc, OwnerID: 42, Now: h.clock})
	c := readyCollection(t, h.store, "owner", "Synced")
	baseline, _, err := h.store.ListJobs(context.Background(), "", "", "", 10, 0)
	if err != nil || len(baseline) == 0 {
		t.Fatalf("baseline jobs: %v %v", baseline, err)
	}
	completeItems(t, h.store, baseline[0].ID)
	finish(t, h.store, baseline[0].ID, state.JobSucceeded, "")
	run, _, err := h.store.RunNow(context.Background(), "owner", "", "run-1", c.ID, c.Revision)
	if err != nil {
		t.Fatal(err)
	}
	finish(t, h.store, run.Job.ID, state.JobSucceeded, "")
	if err := n.JobFinished(context.Background(), run.Job.ID); err != nil {
		t.Fatal(err)
	}
	if rows := pendingFinal(t, h.store); len(rows) != 0 {
		t.Fatalf("an unchanged sync run was announced: %+v", rows)
	}
}

func TestNotifierAnnouncesASyncRunThatChangedSomething(t *testing.T) {
	h := newHarness(t, 42)
	n := NewNotifier(NotifierOptions{Service: h.svc, OwnerID: 42, Now: h.clock})
	c := readyCollection(t, h.store, "owner", "Changed")
	baseline, _, err := h.store.ListJobs(context.Background(), "", "", "", 10, 0)
	if err != nil || len(baseline) == 0 {
		t.Fatalf("baseline jobs: %v %v", baseline, err)
	}
	finish(t, h.store, baseline[0].ID, state.JobSucceeded, "")
	run, _, err := h.store.RunNow(context.Background(), "owner", "", "run-1", c.ID, c.Revision)
	if err != nil {
		t.Fatal(err)
	}
	finish(t, h.store, run.Job.ID, state.JobSucceeded, "")
	if err := n.JobFinished(context.Background(), run.Job.ID); err != nil {
		t.Fatal(err)
	}
	if rows := pendingFinal(t, h.store); len(rows) != 1 {
		t.Fatalf("a changed sync run was not announced: %d rows", len(rows))
	}
}

func TestNotifierAnnouncesOneFailurePerCode(t *testing.T) {
	h := newHarness(t, 42)
	n := NewNotifier(NotifierOptions{Service: h.svc, OwnerID: 42, Now: h.clock})
	first := admit(t, h.store, "owner", "Failing")
	finish(t, h.store, first.ID, state.JobFailed, "source_not_found")
	ctx := context.Background()
	if err := n.JobFinished(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	// A second job of the same collection with the same code stays silent.
	second := admitInCollection(t, h.store, first.CollectionID)
	finish(t, h.store, second, state.JobFailed, "source_not_found")
	if err := n.JobFinished(ctx, second); err != nil {
		t.Fatal(err)
	}
	if rows := pendingFinal(t, h.store); len(rows) != 1 {
		t.Fatalf("repeated failure announced %d times", len(rows))
	}
	// A different code is a new event.
	third := admitInCollection(t, h.store, first.CollectionID)
	finish(t, h.store, third, state.JobFailed, "quota_exceeded")
	if err := n.JobFinished(ctx, third); err != nil {
		t.Fatal(err)
	}
	if rows := pendingFinal(t, h.store); len(rows) != 2 {
		t.Fatalf("a new error code was not announced: %d rows", len(rows))
	}
}

// admitInCollection starts another job on the same collection (a delta run).
func admitInCollection(t *testing.T, store *state.Store, collectionID string) string {
	t.Helper()
	ctx := context.Background()
	if err := store.SetCollectionState(ctx, collectionID, "ready"); err != nil {
		t.Fatal(err)
	}
	c, err := store.GetCollection(ctx, "", collectionID)
	if err != nil {
		t.Fatal(err)
	}
	run, _, err := store.RunNow(ctx, "owner", "", state.ID("run"), collectionID, c.Revision)
	if err != nil {
		t.Fatal(err)
	}
	return run.Job.ID
}

func TestNotifierClearsAFailureAfterASuccess(t *testing.T) {
	h := newHarness(t, 42)
	n := NewNotifier(NotifierOptions{Service: h.svc, OwnerID: 42, Now: h.clock})
	c := readyCollection(t, h.store, "owner", "Recovers")
	baseline, _, err := h.store.ListJobs(context.Background(), "", "", "", 10, 0)
	if err != nil || len(baseline) == 0 {
		t.Fatalf("baseline: %v %v", baseline, err)
	}
	finish(t, h.store, baseline[0].ID, state.JobFailed, "provider_unavailable")
	if err := n.JobFinished(context.Background(), baseline[0].ID); err != nil {
		t.Fatal(err)
	}
	if rows := pendingFinal(t, h.store); len(rows) != 1 {
		t.Fatalf("failure not announced: %d", len(rows))
	}
	// A successful run with work clears the marker.
	run, _, err := h.store.RunNow(context.Background(), "owner", "", "run-1", c.ID, c.Revision)
	if err != nil {
		t.Fatal(err)
	}
	finish(t, h.store, run.Job.ID, state.JobSucceeded, "")
	if err := n.JobFinished(context.Background(), run.Job.ID); err != nil {
		t.Fatal(err)
	}
	if rows := pendingFinal(t, h.store); len(rows) != 2 {
		t.Fatalf("the successful run was not announced: %d rows", len(rows))
	}
	// The same failure is a new event now.
	again := admitInCollection(t, h.store, c.ID)
	finish(t, h.store, again, state.JobFailed, "provider_unavailable")
	if err := n.JobFinished(context.Background(), again); err != nil {
		t.Fatal(err)
	}
	if rows := pendingFinal(t, h.store); len(rows) != 3 {
		t.Fatalf("the failure was not re-announced after recovery: %d rows", len(rows))
	}
}

func TestNotifierRespectsPreferences(t *testing.T) {
	h := newHarness(t, 42)
	ctx := context.Background()
	prefs, err := h.svc.Preferences(ctx, app.Owner)
	if err != nil {
		t.Fatal(err)
	}
	prefs.NotifyFailed = false
	prefs.NotifyActionRequired = false
	if err = h.store.SavePreferences(ctx, app.Owner.ID, prefs); err != nil {
		t.Fatal(err)
	}
	n := NewNotifier(NotifierOptions{Service: h.svc, OwnerID: 42, Now: h.clock})
	failed := admit(t, h.store, "owner", "Muted failure")
	finish(t, h.store, failed.ID, state.JobFailed, "x")
	if err = n.JobFinished(ctx, failed.ID); err != nil {
		t.Fatal(err)
	}
	blocked := admit(t, h.store, "owner", "Muted block")
	finish(t, h.store, blocked.ID, state.JobBlocked, "unknown_remote_outcome")
	if err = n.JobFinished(ctx, blocked.ID); err != nil {
		t.Fatal(err)
	}
	if rows := pendingFinal(t, h.store); len(rows) != 0 {
		t.Fatalf("preferences were ignored: %+v", rows)
	}
	// Completion is still allowed.
	done := admit(t, h.store, "owner", "Still told")
	finish(t, h.store, done.ID, state.JobSucceeded, "")
	if err = n.JobFinished(ctx, done.ID); err != nil {
		t.Fatal(err)
	}
	if rows := pendingFinal(t, h.store); len(rows) != 1 {
		t.Fatalf("completed notification = %d", len(rows))
	}
}

func TestNotifierResolvesOperatorsAndTheOwner(t *testing.T) {
	h := newHarness(t, 42)
	ctx := context.Background()
	op, _, err := h.svc.AddOperator(ctx, app.Owner, "op-1", 555)
	if err != nil {
		t.Fatal(err)
	}
	n := NewNotifier(NotifierOptions{Service: h.svc, OwnerID: 42, Now: h.clock})
	job := admit(t, h.store, op.ID, "Operator copy")
	finish(t, h.store, job.ID, state.JobSucceeded, "")
	if err = n.JobFinished(ctx, job.ID); err != nil {
		t.Fatal(err)
	}
	rows := pendingFinal(t, h.store)
	if len(rows) != 1 || rows[0].ChatID != 555 || rows[0].Principal != op.ID {
		t.Fatalf("operator row = %+v", rows)
	}
	// Without a configured owner id, the owner's jobs are not announced.
	silent := NewNotifier(NotifierOptions{Service: h.svc, OwnerID: 0, Now: h.clock})
	mine := admit(t, h.store, "owner", "Unconfigured owner")
	finish(t, h.store, mine.ID, state.JobSucceeded, "")
	if err = silent.JobFinished(ctx, mine.ID); err != nil {
		t.Fatal(err)
	}
	if rows = pendingFinal(t, h.store); len(rows) != 1 {
		t.Fatalf("owner without a chat was announced: %+v", rows)
	}
}

func TestNotifierAnnouncesOneSubscriptionErrorUntilHealthy(t *testing.T) {
	h := newHarness(t, 42)
	ctx := context.Background()
	c := readyCollection(t, h.store, "owner", "Poller")
	n := NewNotifier(NotifierOptions{Service: h.svc, OwnerID: 42, Now: h.clock})
	for range 3 {
		if err := n.SubscriptionError(ctx, c.ID, "auth"); err != nil {
			t.Fatal(err)
		}
	}
	rows := pendingFinal(t, h.store)
	if len(rows) != 1 || rows[0].JobID != "" || rows[0].Note != "auth" {
		t.Fatalf("subscription error rows = %+v", rows)
	}
	if err := n.SubscriptionHealthy(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	if err := n.SubscriptionError(ctx, c.ID, "auth"); err != nil {
		t.Fatal(err)
	}
	if rows = pendingFinal(t, h.store); len(rows) != 2 {
		t.Fatalf("a recovered error was not announced again: %d rows", len(rows))
	}
}

// A job that was announced as blocked and then succeeds after a retry gets a
// second notice; the same outcome reported twice still coalesces.
func TestNotifierAnnouncesTheOutcomeAfterARetry(t *testing.T) {
	h := newHarness(t, 42)
	n := NewNotifier(NotifierOptions{Service: h.svc, OwnerID: 42, Now: h.clock})
	job := admit(t, h.store, "owner", "Crash copy")
	finish(t, h.store, job.ID, state.JobBlocked, "multiple_matches")
	if err := n.JobFinished(context.Background(), job.ID); err != nil {
		t.Fatal(err)
	}
	finish(t, h.store, job.ID, state.JobSucceeded, "")
	for range 2 {
		if err := n.JobFinished(context.Background(), job.ID); err != nil {
			t.Fatal(err)
		}
	}
	rows := pendingFinal(t, h.store)
	if len(rows) != 2 || !strings.HasPrefix(rows[0].Note, state.JobBlocked) || !strings.HasPrefix(rows[1].Note, state.JobSucceeded) {
		t.Fatalf("notices = %+v, want blocked then succeeded", rows)
	}
}

// A first block on an unknown outcome is re-checked by the daemon on its own;
// only a block after that automatic re-check is announced.
func TestNotifierWaitsForTheAutomaticRecheck(t *testing.T) {
	h := newHarness(t, 42)
	n := NewNotifier(NotifierOptions{Service: h.svc, OwnerID: 42, Now: h.clock})
	job := admit(t, h.store, "owner", "Crash copy")
	finish(t, h.store, job.ID, state.JobBlocked, "unknown_remote_outcome")
	if err := n.JobFinished(context.Background(), job.ID); err != nil {
		t.Fatal(err)
	}
	if rows := pendingFinal(t, h.store); len(rows) != 0 {
		t.Fatalf("announced before the automatic re-check: %+v", rows)
	}
	if _, err := h.store.UpdateJob(context.Background(), job.ID, func(j *state.Job) (string, bool) {
		j.AutoRetried = true
		return "", true
	}); err != nil {
		t.Fatal(err)
	}
	if err := n.JobFinished(context.Background(), job.ID); err != nil {
		t.Fatal(err)
	}
	if rows := pendingFinal(t, h.store); len(rows) != 1 {
		t.Fatalf("a block after the re-check must be announced, got %d", len(rows))
	}
}
