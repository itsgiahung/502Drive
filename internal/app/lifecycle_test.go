package app

import (
	"context"
	"net/http"
	"testing"

	"github.com/itsgiahung/502Drive/internal/drive"
	"github.com/itsgiahung/502Drive/internal/drive/drivetest"
	"github.com/itsgiahung/502Drive/internal/state"
)

func newTestService(t *testing.T) (*Service, *drivetest.Fake) {
	t.Helper()
	st, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	fake := drivetest.New()
	svc := &Service{
		Store: st,
		Provider: func(ctx context.Context) (drive.Provider, error) {
			return fake, nil
		},
	}
	return svc, fake
}

func TestServiceCollectionLifecycle(t *testing.T) {
	svc, fake := newTestService(t)
	ctx := context.Background()

	// Setup destination & collection
	dstFolder := fake.Folder("My Dest", "root")
	dst, _, err := svc.Store.AddDestination(ctx, "owner", "k-dest", "dest", state.Destination{
		FolderID: dstFolder.ID, Name: "My Dest",
	})
	if err != nil {
		t.Fatal(err)
	}
	srcFolder := fake.Folder("My Source", "root")
	pv, _, err := svc.Inspect(ctx, Owner, "k-plan", PlanRequest{
		Source: srcFolder.ID, RootName: "My Copied Source", DestinationID: dst.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.Submit(ctx, Owner, "k-job", pv.Plan.ID, pv.Plan.Revision)
	if err != nil {
		t.Fatal(err)
	}
	colID := res.Job.CollectionID

	col, err := svc.Collection(ctx, Owner, colID)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Retire while job is running -> 409 collection_busy
	_, err = svc.RetireCollection(ctx, Owner, "ret-busy", colID, col.Revision)
	ae := Classify(err)
	if ae == nil || ae.Status != http.StatusConflict || ae.Code != "collection_busy" {
		t.Fatalf("expected collection_busy, got: %+v (raw: %v)", ae, err)
	}

	// Cancel job so not busy
	_, _ = svc.JobAction(ctx, Owner, "cancel-job", res.Job.ID, "cancel", res.Job.Revision)

	// 2. Forget before retired -> 409 collection_not_retired
	err = svc.ForgetCollection(ctx, Owner, "forget-unret", colID, col.Revision)
	ae = Classify(err)
	if ae == nil || ae.Status != http.StatusConflict || ae.Code != "collection_not_retired" {
		t.Fatalf("expected collection_not_retired, got: %+v", ae)
	}

	// 3. Cleanup before retired -> 409 collection_not_retired
	_, err = svc.CleanupCollection(ctx, Owner, "clean-unret", colID, CleanupRequest{
		ExpectedRevision: col.Revision, DryRun: true,
	})
	ae = Classify(err)
	if ae == nil || ae.Status != http.StatusConflict || ae.Code != "collection_not_retired" {
		t.Fatalf("expected collection_not_retired, got: %+v", ae)
	}

	// 4. Retire now succeeds
	retiredCol, err := svc.RetireCollection(ctx, Owner, "ret-ok", colID, col.Revision)
	if err != nil {
		t.Fatalf("retire: %v", err)
	}
	if retiredCol.State != state.CollectionRetired {
		t.Fatalf("col state = %s, want retired", retiredCol.State)
	}

	// 5. Cleanup with wrong confirm name fails when dry_run=false
	_, err = svc.CleanupCollection(ctx, Owner, "clean-name-err", colID, CleanupRequest{
		ExpectedRevision: retiredCol.Revision, DryRun: false, ConfirmName: "Wrong Name",
	})
	ae = Classify(err)
	if ae == nil || ae.Status != http.StatusBadRequest || ae.Code != "name_confirmation_mismatch" {
		t.Fatalf("expected name_confirmation_mismatch, got: %+v", ae)
	}

	// 6. Cleanup with right confirm name succeeds (admits cleanup job)
	cleanJob, err := svc.CleanupCollection(ctx, Owner, "clean-ok", colID, CleanupRequest{
		ExpectedRevision: retiredCol.Revision, DryRun: false, ConfirmName: "My Copied Source",
	})
	if err != nil {
		t.Fatalf("cleanup admit: %v", err)
	}
	if cleanJob.Kind != state.JobKindCleanup || cleanJob.CollectionID != colID {
		t.Fatalf("cleanJob = %+v", cleanJob)
	}

	// Cancel cleanup job so not busy
	_, _ = svc.JobAction(ctx, Owner, "cancel-clean", cleanJob.ID, "cancel", cleanJob.Revision)

	// 7. Resume works
	resumedCol, err := svc.ResumeCollection(ctx, Owner, "res-ok", colID, retiredCol.Revision)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if resumedCol.State != "ready" {
		t.Fatalf("resumedCol state = %s", resumedCol.State)
	}

	// 8. Retire again and Forget
	retiredCol2, err := svc.RetireCollection(ctx, Owner, "ret-again", colID, resumedCol.Revision)
	if err != nil {
		t.Fatal(err)
	}
	err = svc.ForgetCollection(ctx, Owner, "forget-ok", colID, retiredCol2.Revision)
	if err != nil {
		t.Fatalf("forget: %v", err)
	}
	_, err = svc.Collection(ctx, Owner, colID)
	if Classify(err).Status != http.StatusNotFound {
		t.Fatalf("expected not found after forget, got: %v", err)
	}
}
