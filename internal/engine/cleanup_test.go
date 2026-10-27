package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/itsgiahung/502Drive/internal/drive"
	"github.com/itsgiahung/502Drive/internal/state"
)

func TestCleanupDryRunDoesNotTrash(t *testing.T) {
	e := newEnv(t)
	_, col, _ := e.copyTree("Photos")

	// Retire collection
	retired, err := e.store.RetireCollection(context.Background(), "owner", "ret-1", col.ID, col.Revision)
	if err != nil {
		t.Fatal(err)
	}

	// Admit dry-run cleanup
	cleanJob, err := e.store.AdmitCleanup(context.Background(), "owner", "clean-dry", col.ID, retired.Revision, true)
	if err != nil {
		t.Fatal(err)
	}

	// Run engine until cleanup job finishes
	finished := e.runUntil(cleanJob.ID, terminal)
	if finished.State != state.JobSucceeded {
		t.Fatalf("cleanup job state = %s, want succeeded", finished.State)
	}

	// Verify root folder is still NOT trashed
	f, err := e.fake.GetFile(context.Background(), *col.RootID, "")
	if err != nil {
		t.Fatal(err)
	}
	if f.Trashed {
		t.Fatal("dry-run cleanup trashed the destination root folder")
	}
}

func TestCleanupLiveTrashesRoot(t *testing.T) {
	e := newEnv(t)
	_, col, _ := e.copyTree("Photos")

	retired, err := e.store.RetireCollection(context.Background(), "owner", "ret-1", col.ID, col.Revision)
	if err != nil {
		t.Fatal(err)
	}

	cleanJob, err := e.store.AdmitCleanup(context.Background(), "owner", "clean-live", col.ID, retired.Revision, false)
	if err != nil {
		t.Fatal(err)
	}

	finished := e.runUntil(cleanJob.ID, terminal)
	if finished.State != state.JobSucceeded {
		t.Fatalf("cleanup job state = %s, want succeeded", finished.State)
	}

	// Verify root folder is now trashed
	f, ok := e.fake.Get(*col.RootID)
	if !ok || !f.Trashed {
		t.Fatal("live cleanup did not trash the destination root folder")
	}
}

func TestCleanupOnlyObjectsWithIntent(t *testing.T) {
	e := newEnv(t)
	_, col, _ := e.copyTree("Photos")

	// Strip the intent marker from the destination root folder
	e.fake.Mutate(*col.RootID, func(f *drive.File) {
		delete(f.AppProperties, drive.IntentProperty)
	})

	retired, err := e.store.RetireCollection(context.Background(), "owner", "ret-1", col.ID, col.Revision)
	if err != nil {
		t.Fatal(err)
	}

	cleanJob, err := e.store.AdmitCleanup(context.Background(), "owner", "clean-no-intent", col.ID, retired.Revision, false)
	if err != nil {
		t.Fatal(err)
	}

	finished := e.runUntil(cleanJob.ID, terminal)
	if finished.State != state.JobFailed {
		t.Fatalf("expected JobFailed when intent missing, got: %s", finished.State)
	}
	if finished.LastError == nil || finished.LastError.Code != "intent_missing" {
		t.Fatalf("expected intent_missing error, got: %+v", finished.LastError)
	}

	// The folder MUST NOT have been trashed!
	f, err := e.fake.GetFile(context.Background(), *col.RootID, "")
	if err != nil {
		t.Fatal(err)
	}
	if f.Trashed {
		t.Fatal("cleanup trashed a folder without 502drive intent!")
	}
}

func TestCleanupTwiceIsIdempotent(t *testing.T) {
	e := newEnv(t)
	_, col, _ := e.copyTree("Photos")

	retired, err := e.store.RetireCollection(context.Background(), "owner", "ret-1", col.ID, col.Revision)
	if err != nil {
		t.Fatal(err)
	}

	// First cleanup
	cleanJob1, err := e.store.AdmitCleanup(context.Background(), "owner", "clean-1", col.ID, retired.Revision, false)
	if err != nil {
		t.Fatal(err)
	}
	finished1 := e.runUntil(cleanJob1.ID, terminal)
	if finished1.State != state.JobSucceeded {
		t.Fatalf("first cleanup failed: %s", finished1.State)
	}

	// Second cleanup on already-trashed root
	cleanJob2, err := e.store.AdmitCleanup(context.Background(), "owner", "clean-2", col.ID, retired.Revision, false)
	if err != nil {
		t.Fatal(err)
	}
	finished2 := e.runUntil(cleanJob2.ID, terminal)
	if finished2.State != state.JobSucceeded {
		t.Fatalf("second cleanup failed: %s", finished2.State)
	}
}

func TestCleanupCrashAndRestart(t *testing.T) {
	e := newEnv(t)
	_, col, _ := e.copyTree("Photos")

	retired, err := e.store.RetireCollection(context.Background(), "owner", "ret-1", col.ID, col.Revision)
	if err != nil {
		t.Fatal(err)
	}

	cleanJob, err := e.store.AdmitCleanup(context.Background(), "owner", "clean-crash", col.ID, retired.Revision, false)
	if err != nil {
		t.Fatal(err)
	}

	// Simulate a crash right after Google applied the trash: the call takes
	// effect, the engine's context dies, and the response is lost.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	crashed := false
	eng := e.engine()
	eng.Provider = func(context.Context) (drive.Provider, error) {
		return &loseTrashResponse{Provider: e.fake, crash: func() { crashed = true; cancel() }}, nil
	}
	done := make(chan struct{})
	go func() { eng.Run(ctx); close(done) }()
	<-done
	if !crashed {
		t.Fatal("crash was not triggered")
	}

	// Restart engine and verify it reconciles and finishes cleanly
	finished := e.runUntil(cleanJob.ID, terminal)
	if finished.State != state.JobSucceeded {
		t.Fatalf("cleanup after restart failed: %s", finished.State)
	}
	f, ok := e.fake.Get(*col.RootID)
	if !ok || !f.Trashed {
		t.Fatal("destination root was not trashed after engine recovery")
	}
}

// loseTrashResponse applies TrashFile on the fake and then reports a lost
// response while cancelling the engine, which is what a crash looks like.
type loseTrashResponse struct {
	drive.Provider
	crash func()
}

func (l *loseTrashResponse) TrashFile(ctx context.Context, id string) error {
	if err := l.Provider.TrashFile(ctx, id); err != nil {
		return err
	}
	l.crash()
	return &drive.Error{Kind: drive.KindUnavailable, Err: errors.New("response lost")}
}
