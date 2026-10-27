package state

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/itsgiahung/502Drive/internal/schedule"
	_ "modernc.org/sqlite"
)

func TestCollectionLifecycleMigration(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "state-v2.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	ms, err := loadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range ms {
		if m.version > 9 {
			break
		}
		if _, err = db.Exec(m.sql); err != nil {
			t.Fatalf("migration %d: %v", m.version, err)
		}
	}
	if _, err = db.Exec("PRAGMA user_version=9; PRAGMA application_id=502002"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("INSERT INTO plans VALUES(?,?,?,NULL)", "plan1", "owner", []byte(`{"id":"plan1"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("INSERT INTO collections(id, principal, plan_id, body) VALUES(?,?,?,?)", "col1", "owner", "plan1", []byte(`{"id":"col1","source_id":"src","source_kind":"folder","state":"ready","revision":1}`)); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}

	s, err := Open(dir)
	if err != nil {
		t.Fatalf("open after migration: %v", err)
	}
	defer s.Close()

	var ver int
	if err = s.db.QueryRowContext(context.Background(), "PRAGMA user_version").Scan(&ver); err != nil || ver < 20 {
		t.Fatalf("user_version = %d err=%v", ver, err)
	}
	fk, err := s.db.QueryContext(context.Background(), "PRAGMA foreign_key_check")
	if err != nil {
		t.Fatal(err)
	}
	defer fk.Close()
	if fk.Next() {
		t.Fatal("foreign_key_check failed")
	}

	col, err := s.GetCollection(context.Background(), "owner", "col1")
	if err != nil {
		t.Fatalf("get collection: %v", err)
	}
	if col.State != "ready" || col.RetiredAt != nil {
		t.Fatalf("col = %+v", col)
	}
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func lifecycleFixture(t *testing.T, s *Store) (string, Job) {
	t.Helper()
	ctx := context.Background()
	_, _, _ = s.AddDestination(ctx, "owner", "k-dest", "dest", Destination{ID: "d1", FolderID: "root", Name: "My Drive"})
	_, _, _ = s.PutPlanIdempotent(ctx, "owner", "k-plan", "src", Plan{ID: "p1", Principal: "owner", SourceID: "s1", SourceName: "Photos", SourceKind: KindFolder, DestinationID: "d1", RootName: "Photos", ExpiresAt: time.Now().Add(time.Hour), Revision: 1})
	res, err := s.Admit(ctx, "owner", "k-job", "p1", 1)
	if err != nil {
		t.Fatalf("admit: %v", err)
	}
	return res.Job.CollectionID, res.Job
}

func TestRetireCollectionFailsWhenBusy(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()
	ctx := context.Background()

	_, _, _ = s.AddDestination(ctx, "owner", "k-dest", "dest", Destination{ID: "d1", FolderID: "root", Name: "My Drive"})
	_, _, _ = s.PutPlanIdempotent(ctx, "owner", "k-plan", "src", Plan{ID: "p1", Principal: "owner", SourceID: "s1", SourceName: "Photos", SourceKind: KindFolder, DestinationID: "d1", RootName: "Photos", ExpiresAt: time.Now().Add(time.Hour), Revision: 1})
	res, err := s.Admit(ctx, "owner", "k-job", "p1", 1)
	if err != nil {
		t.Fatalf("admit: %v", err)
	}
	colID := res.Job.CollectionID

	col, err := s.GetCollection(ctx, "owner", colID)
	if err != nil {
		t.Fatal(err)
	}

	// Job is queued -> Retire must fail with ErrCollectionBusy
	_, err = s.RetireCollection(ctx, "owner", "retire-1", colID, col.Revision)
	var busy ErrCollectionBusy
	if !errors.As(err, &busy) {
		t.Fatalf("expected ErrCollectionBusy, got: %v", err)
	}
	if busy.ActiveJobID != res.Job.ID {
		t.Fatalf("expected busy active job %s, got %s", res.Job.ID, busy.ActiveJobID)
	}
}

func TestRetireAndResumeCollection(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()
	ctx := context.Background()

	_, _, _ = s.AddDestination(ctx, "owner", "k-dest", "dest", Destination{ID: "d1", FolderID: "root", Name: "My Drive"})
	_, _, _ = s.PutPlanIdempotent(ctx, "owner", "k-plan", "src", Plan{ID: "p1", Principal: "owner", SourceID: "s1", SourceName: "Photos", SourceKind: KindFolder, DestinationID: "d1", RootName: "Photos", ExpiresAt: time.Now().Add(time.Hour), Revision: 1})
	res, err := s.Admit(ctx, "owner", "k-job", "p1", 1)
	if err != nil {
		t.Fatalf("admit: %v", err)
	}
	colID := res.Job.CollectionID

	// Cancel baseline job so collection is not busy
	_, err = s.Action(ctx, "owner", "act-cancel", res.Job.ID, "cancel", res.Job.Revision)
	if err != nil {
		t.Fatal(err)
	}

	col, err := s.GetCollection(ctx, "owner", colID)
	if err != nil {
		t.Fatal(err)
	}

	// Retire collection
	retired, err := s.RetireCollection(ctx, "owner", "retire-key", colID, col.Revision)
	if err != nil {
		t.Fatalf("retire: %v", err)
	}
	if retired.State != CollectionRetired || retired.RetiredAt == nil {
		t.Fatalf("retired col = %+v", retired)
	}

	// Default collection page (all=false) should NOT include retired collection
	activeList, _, err := s.CollectionPage(ctx, "owner", "", "", false, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range activeList {
		if c.ID == colID {
			t.Fatalf("retired collection %s found in active list", colID)
		}
	}

	// Collection page with all=true SHOULD include retired collection
	allList, _, err := s.CollectionPage(ctx, "owner", "", "", true, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range allList {
		if c.ID == colID {
			found = true
			if c.State != CollectionRetired {
				t.Fatalf("allList col state = %s, want retired", c.State)
			}
		}
	}
	if !found {
		t.Fatalf("retired collection %s not found in allList", colID)
	}

	// Resume collection
	resumed, err := s.ResumeCollection(ctx, "owner", "resume-key", colID, retired.Revision)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if resumed.State != "ready" || resumed.RetiredAt != nil {
		t.Fatalf("resumed col = %+v", resumed)
	}

	// Now it appears in activeList again
	activeListAfter, _, err := s.CollectionPage(ctx, "owner", "", "", false, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	foundAfter := false
	for _, c := range activeListAfter {
		if c.ID == colID {
			foundAfter = true
		}
	}
	if !foundAfter {
		t.Fatalf("resumed collection %s not found in activeListAfter", colID)
	}
}

func TestForgetCollectionRefusesWhenNotRetired(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()
	ctx := context.Background()

	_, _, _ = s.AddDestination(ctx, "owner", "k-dest", "dest", Destination{ID: "d1", FolderID: "root", Name: "My Drive"})
	_, _, _ = s.PutPlanIdempotent(ctx, "owner", "k-plan", "src", Plan{ID: "p1", Principal: "owner", SourceID: "s1", SourceName: "Photos", SourceKind: KindFolder, DestinationID: "d1", RootName: "Photos", ExpiresAt: time.Now().Add(time.Hour), Revision: 1})
	res, err := s.Admit(ctx, "owner", "k-job", "p1", 1)
	if err != nil {
		t.Fatalf("admit: %v", err)
	}
	colID := res.Job.CollectionID

	// Cancel job so not busy
	_, _ = s.Action(ctx, "owner", "act-cancel", res.Job.ID, "cancel", res.Job.Revision)
	col, err := s.GetCollection(ctx, "owner", colID)
	if err != nil {
		t.Fatal(err)
	}

	// Forget without retiring -> must return ErrCollectionNotRetired
	err = s.ForgetCollection(ctx, "owner", "forget-key", colID, col.Revision)
	if !errors.Is(err, ErrCollectionNotRetired) {
		t.Fatalf("expected ErrCollectionNotRetired, got: %v", err)
	}
}

func TestForgetCollectionCleansAllRecords(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()
	ctx := context.Background()

	_, _, _ = s.AddDestination(ctx, "owner", "k-dest", "dest", Destination{ID: "d1", FolderID: "root", Name: "My Drive"})
	_, _, _ = s.PutPlanIdempotent(ctx, "owner", "k-plan", "src", Plan{ID: "p1", Principal: "owner", SourceID: "s1", SourceName: "Photos", SourceKind: KindFolder, DestinationID: "d1", RootName: "Photos", ExpiresAt: time.Now().Add(time.Hour), Revision: 1})
	res, err := s.Admit(ctx, "owner", "k-job", "p1", 1)
	if err != nil {
		t.Fatalf("admit: %v", err)
	}
	colID := res.Job.CollectionID

	_, _ = s.Action(ctx, "owner", "act-cancel", res.Job.ID, "cancel", res.Job.Revision)
	col, err := s.GetCollection(ctx, "owner", colID)
	if err != nil {
		t.Fatal(err)
	}

	retired, err := s.RetireCollection(ctx, "owner", "retire-key", colID, col.Revision)
	if err != nil {
		t.Fatalf("retire: %v", err)
	}

	// Forget collection
	err = s.ForgetCollection(ctx, "owner", "forget-key", colID, retired.Revision)
	if err != nil {
		t.Fatalf("forget: %v", err)
	}

	// Must be gone from GetCollection
	_, err = s.GetCollection(ctx, "owner", colID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}

	// PRAGMA foreign_key_check must be clean
	fk, err := s.db.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		t.Fatal(err)
	}
	defer fk.Close()
	if fk.Next() {
		t.Fatal("foreign_key_check failed after forget")
	}
}

func TestAdmitCleanupRequiresRetiredCollection(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()
	ctx := context.Background()

	_, _, _ = s.AddDestination(ctx, "owner", "k-dest", "dest", Destination{ID: "d1", FolderID: "root", Name: "My Drive"})
	_, _, _ = s.PutPlanIdempotent(ctx, "owner", "k-plan", "src", Plan{ID: "p1", Principal: "owner", SourceID: "s1", SourceName: "Photos", SourceKind: KindFolder, DestinationID: "d1", RootName: "Photos", ExpiresAt: time.Now().Add(time.Hour), Revision: 1})
	res, err := s.Admit(ctx, "owner", "k-job", "p1", 1)
	if err != nil {
		t.Fatalf("admit: %v", err)
	}
	colID := res.Job.CollectionID

	_, _ = s.Action(ctx, "owner", "act-cancel", res.Job.ID, "cancel", res.Job.Revision)
	col, err := s.GetCollection(ctx, "owner", colID)
	if err != nil {
		t.Fatal(err)
	}

	// Not retired -> AdmitCleanup fails
	_, err = s.AdmitCleanup(ctx, "owner", "clean-k", colID, col.Revision, true)
	if !errors.Is(err, ErrCollectionNotRetired) {
		t.Fatalf("expected ErrCollectionNotRetired, got: %v", err)
	}

	retired, err := s.RetireCollection(ctx, "owner", "ret-k", colID, col.Revision)
	if err != nil {
		t.Fatal(err)
	}

	// Retired -> AdmitCleanup succeeds
	cj, err := s.AdmitCleanup(ctx, "owner", "clean-k2", colID, retired.Revision, true)
	if err != nil {
		t.Fatalf("admit cleanup: %v", err)
	}
	if cj.Kind != JobKindCleanup || cj.CollectionID != colID || !cj.DryRun {
		t.Fatalf("cleanup job: %+v", cj)
	}
}

func TestRetiredCollectionNotClaimedBySchedules(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()
	ctx := context.Background()

	_, _, _ = s.AddDestination(ctx, "owner", "k-dest", "dest", Destination{ID: "d1", FolderID: "root", Name: "My Drive"})
	_, _, _ = s.PutPlanIdempotent(ctx, "owner", "k-plan", "src", Plan{ID: "p1", Principal: "owner", SourceID: "s1", SourceName: "Photos", SourceKind: KindFolder, DestinationID: "d1", RootName: "Photos", ExpiresAt: time.Now().Add(time.Hour), Revision: 1})
	res, err := s.Admit(ctx, "owner", "k-job", "p1", 1)
	if err != nil {
		t.Fatalf("admit: %v", err)
	}
	colID := res.Job.CollectionID
	_, _ = s.Action(ctx, "owner", "act-cancel", res.Job.ID, "cancel", res.Job.Revision)
	_ = s.SetCollectionState(ctx, colID, "ready")

	col, _ := s.GetCollection(ctx, "owner", colID)

	// Configure schedule
	due := time.Now().Add(-time.Minute)
	sub, _, err := s.ConfigureSync(ctx, "owner", "owner", "sync-k", colID, SyncChange{
		Mode: ModeScheduled, ContentPolicy: PolicyVersioned, ExpectedCollection: col.Revision,
		Schedule: &schedule.Spec{Kind: "interval", IntervalMinutes: 10, AnchorAt: due},
		NextDue:  &schedule.Slot{At: due, Key: "slot-1"},
	})
	if err != nil {
		t.Fatalf("put sync: %v", err)
	}
	if sub.State != SyncActive {
		t.Fatalf("sub state = %s", sub.State)
	}

	// Retire collection
	col, _ = s.GetCollection(ctx, "owner", colID)
	_, err = s.RetireCollection(ctx, "owner", "retire-sync", colID, col.Revision)
	if err != nil {
		t.Fatalf("retire: %v", err)
	}

	// ClaimDueSchedules must not return any jobs for this retired collection
	jobs, err := s.ClaimDueSchedules(ctx, time.Now())
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	for _, j := range jobs {
		if j.CollectionID == colID {
			t.Fatalf("claimed job for retired collection: %+v", j)
		}
	}
}
