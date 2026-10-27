package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/itsgiahung/502Drive/internal/drive"
	"github.com/itsgiahung/502Drive/internal/schedule"
	"github.com/itsgiahung/502Drive/internal/state"
)

func setAfterCopyPreference(t *testing.T, e *syncEnv, mode string) {
	t.Helper()
	m := mode
	if _, err := e.svc.PatchPreferences(context.Background(), Owner, "pref-"+mode, PreferencePatch{AfterCopySyncMode: &m}); err != nil {
		t.Fatal(err)
	}
}

func (e *syncEnv) applyAfterCopy() (int, int) {
	e.t.Helper()
	// The request is due at the wall clock of the copy; the syncer next ticks
	// after it, so the test clock moves past that instant.
	if e.clock.now().Before(time.Now().Add(time.Second)) {
		e.clock.set(time.Now().Add(time.Second))
	}
	applied, failed, err := e.svc.ApplyAfterCopySync(context.Background())
	if err != nil {
		e.t.Fatal(err)
	}
	return applied, failed
}

func TestAfterCopySyncAppliesPreferenceAndSurvivesRestart(t *testing.T) {
	e := newSyncEnv(t, time.Now().UTC())
	setAfterCopyPreference(t, e, state.AfterCopyHourly)
	c := e.ready("Photos")
	if _, err := e.store.RunFinished(context.Background(), c.BaselineJobID); err != nil {
		t.Fatal(err)
	}

	// Restart between the copy and the syncer tick: the request is durable.
	e.restart()
	if applied, failed := e.applyAfterCopy(); applied != 1 || failed != 0 {
		t.Fatalf("applied=%d failed=%d", applied, failed)
	}
	sub, err := e.store.GetSubscription(context.Background(), c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if sub.Mode != state.ModeScheduled || sub.State != state.SyncActive || sub.Schedule == nil ||
		sub.Schedule.Kind != schedule.KindInterval || sub.Schedule.IntervalMinutes != 60 {
		t.Fatalf("subscription = %+v", sub)
	}
	// Applying again is idempotent: no second subscription, no error.
	if applied, failed := e.applyAfterCopy(); applied != 0 || failed != 0 {
		t.Fatalf("repeat applied=%d failed=%d", applied, failed)
	}
}

func TestAfterCopySyncSkipsPartialAndNonInitialJobs(t *testing.T) {
	e := newSyncEnv(t, time.Now().UTC())
	setAfterCopyPreference(t, e, state.AfterCopyDaily)
	ctx := context.Background()
	src := e.fake.Folder("Broken", "root")
	plan := state.Plan{ID: state.ID("plan"), Principal: "owner", SourceID: src.ID, SourceName: src.Name, SourceKind: state.KindFolder,
		DestinationID: "dst", DestinationFolder: e.dest, RootName: src.Name, ExpiresAt: time.Now().Add(time.Hour), Revision: 1}
	if err := e.store.PutPlan(ctx, plan); err != nil {
		t.Fatal(err)
	}
	res, err := e.store.Admit(ctx, "owner", state.ID("admit"), plan.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.store.UpdateJob(ctx, res.Job.ID, func(j *state.Job) (string, bool) {
		j.State = state.JobPartial
		return "finished", true
	}); err != nil {
		t.Fatal(err)
	}
	if err = e.store.SetCollectionState(ctx, res.Job.CollectionID, "partial"); err != nil {
		t.Fatal(err)
	}
	if _, err = e.store.RunFinished(ctx, res.Job.ID); err != nil {
		t.Fatal(err)
	}
	if applied, failed := e.applyAfterCopy(); applied != 0 || failed != 0 {
		t.Fatalf("partial copy applied sync: %d/%d", applied, failed)
	}
	if _, err = e.store.GetSubscription(ctx, res.Job.CollectionID); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("partial copy got a subscription: %v", err)
	}

	// A sync run on an existing collection is not a "new collection" either.
	if _, err = e.store.UpdateJob(ctx, res.Job.ID, func(j *state.Job) (string, bool) {
		j.State, j.Trigger = state.JobSucceeded, "manual"
		return "finished", true
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = e.store.RunFinished(ctx, res.Job.ID); err != nil {
		t.Fatal(err)
	}
	if applied, _ := e.applyAfterCopy(); applied != 0 {
		t.Fatalf("manual run applied after-copy sync")
	}
}

func TestAfterCopySyncTwiceOnOneCollectionCreatesOneSubscription(t *testing.T) {
	e := newSyncEnv(t, time.Now().UTC())
	setAfterCopyPreference(t, e, state.AfterCopySixHourly)
	ctx := context.Background()
	c := e.ready("Twice")
	for range 2 {
		if _, err := e.store.RunFinished(ctx, c.BaselineJobID); err != nil {
			t.Fatal(err)
		}
	}
	if applied, failed := e.applyAfterCopy(); applied != 1 || failed != 0 {
		t.Fatalf("applied=%d failed=%d", applied, failed)
	}
	sub, err := e.store.GetSubscription(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if sub.Schedule == nil || sub.Schedule.IntervalMinutes != 360 {
		t.Fatalf("six-hourly schedule = %+v", sub.Schedule)
	}
	// The second enqueue is a no-op even after the first was applied.
	if err = e.store.CompleteAfterCopySync(ctx, c.ID, ""); err != nil {
		t.Fatal(err)
	}
	if _, err = e.store.RunFinished(ctx, c.BaselineJobID); err != nil {
		t.Fatal(err)
	}
	if applied, _ := e.applyAfterCopy(); applied != 0 {
		t.Fatal("a second subscription was applied")
	}
}

func TestAfterCopySyncJobFlagOverridesPreferenceAndFailureKeepsJob(t *testing.T) {
	e := newSyncEnv(t, time.Now().UTC())
	setAfterCopyPreference(t, e, state.AfterCopyOff)
	ctx := context.Background()
	src := e.fake.Folder("Explicit", "root")
	plan := state.Plan{ID: state.ID("plan"), Principal: "owner", SourceID: src.ID, SourceName: src.Name, SourceKind: state.KindFolder,
		DestinationID: "dst", DestinationFolder: e.dest, RootName: src.Name, ExpiresAt: time.Now().Add(time.Hour), Revision: 1}
	if err := e.store.PutPlan(ctx, plan); err != nil {
		t.Fatal(err)
	}
	res, err := e.store.AdmitInBatchWith(ctx, "owner", state.ID("admit"), plan.ID, 1, "", state.AfterCopyRealtime, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.store.UpdateJob(ctx, res.Job.ID, func(j *state.Job) (string, bool) {
		j.State = state.JobSucceeded
		return "finished", true
	}); err != nil {
		t.Fatal(err)
	}
	if err = e.store.SetCollectionState(ctx, res.Job.CollectionID, "ready"); err != nil {
		t.Fatal(err)
	}
	if _, err = e.store.RunFinished(ctx, res.Job.ID); err != nil {
		t.Fatal(err)
	}

	// A broken provider must be recorded, not fail the copy.
	e.fake.Before = func(op string, _ drive.File) error {
		if op == "starttoken" {
			return &drive.Error{Kind: drive.KindUnavailable, Err: errors.New("offline")}
		}
		return nil
	}
	if applied, failed := e.applyAfterCopy(); applied != 0 || failed != 1 {
		t.Fatalf("applied=%d failed=%d", applied, failed)
	}
	job, err := e.store.GetJob(ctx, "owner", res.Job.ID)
	if err != nil || job.State != state.JobSucceeded {
		t.Fatalf("copy job changed: %+v %v", job, err)
	}
	req, err := e.store.AfterCopySyncOf(ctx, res.Job.CollectionID)
	if err != nil || req.State != "pending" || req.LastError == "" {
		t.Fatalf("request = %+v %v", req, err)
	}

	// Provider recovers: the next tick applies realtime.
	e.fake.Before = nil
	e.clock.set(e.clock.now().Add(10 * time.Minute))
	if applied, _ := e.applyAfterCopy(); applied != 1 {
		t.Fatal("recovered request was not applied")
	}
	sub, err := e.store.GetSubscription(ctx, res.Job.CollectionID)
	if err != nil || sub.Mode != state.ModeRealtime {
		t.Fatalf("realtime subscription = %+v %v", sub, err)
	}
}
