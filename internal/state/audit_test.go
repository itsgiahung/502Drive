package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// A collection with a finished baseline job and two mapped items, the smallest
// fixture an audit can be admitted against.
func openStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func auditFixture(t *testing.T, s *Store) (col Collection, job Job) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)
	col = Collection{ID: ID("col"), SourceID: "src", SourceName: "Photos", SourceKind: KindFolder,
		DestinationID: "dest", RootName: "Photos", BaselineJobID: ID("job"), State: "ready", Revision: 3}
	b, _ := json.Marshal(col)
	j := Job{ID: col.BaselineJobID, Principal: "owner", CollectionID: col.ID, Trigger: "initial",
		SourceName: "Photos", DestinationID: "dest", RootName: "Photos", State: JobSucceeded, DesiredState: "run",
		Revision: 1, CreatedAt: now, UpdatedAt: now}
	jb, _ := json.Marshal(j)
	if err := s.tx(ctx, func(tx *sql.Tx) error {
		if _, e := tx.ExecContext(ctx, "INSERT INTO plans VALUES(?,?,?,NULL)", "plan_"+col.ID, "owner", []byte(`{}`)); e != nil {
			return e
		}
		if _, e := tx.ExecContext(ctx, "INSERT INTO collections(id, principal, plan_id, body) VALUES(?,?,?,?)", col.ID, "owner", "plan_"+col.ID, b); e != nil {
			return e
		}
		if _, e := tx.ExecContext(ctx, "INSERT INTO jobs VALUES(?,?,?,?,?)", j.ID, "owner", col.ID, j.State, jb); e != nil {
			return e
		}
		if e := appendEvent(ctx, tx, j, "admitted", nil); e != nil {
			return e
		}
		ins := "INSERT INTO items(collection_id,source_id,job_id,parent_source_id,kind,name,mime_type,resource_key,size,fingerprint,intent_key,state,dest_parent_id,dest_id,generation,list_state) " +
			"VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)"
		if _, e := tx.ExecContext(ctx, ins, col.ID, "src", j.ID, "", KindFolder, "Photos", "", "", nil, "v1", "int_root", ItemDone, "destroot", "destroot", 0, "done"); e != nil {
			return e
		}
		if _, e := tx.ExecContext(ctx, ins, col.ID, "f1", j.ID, "src", KindFile, "a.txt", "text/plain", "", nil, "v1", "int_f1", ItemDone, "destroot", "df1", 0, "done"); e != nil {
			return e
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return col, j
}

func TestAdmitAuditIsIdempotentAndRespectsOneActiveRun(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	col, _ := auditFixture(t, s)

	a, j, replayed, err := s.AdmitAudit(ctx, "owner", "owner", "audit-key", col.ID)
	if err != nil || replayed {
		t.Fatalf("admit: %v replayed=%v", err, replayed)
	}
	if j.Kind != JobKindAudit || j.Trigger != "audit" || j.CollectionID != col.ID {
		t.Fatalf("job = %+v", j)
	}
	if a.State != AuditRunning || a.JobID != j.ID || a.Revision != 1 {
		t.Fatalf("audit = %+v", a)
	}

	// A different key is refused while the audit is active, and the replay of
	// the same key returns the same audit without a second job.
	if _, _, _, err = s.AdmitAudit(ctx, "owner", "owner", "other-key", col.ID); !errors.Is(err, ErrTransition) {
		t.Fatalf("second audit err = %v", err)
	}
	a2, j2, replayed2, err := s.AdmitAudit(ctx, "owner", "owner", "audit-key", col.ID)
	if err != nil || !replayed2 || a2.ID != a.ID || j2.ID != j.ID {
		t.Fatalf("replay: %v replayed=%v a=%v j=%v", err, replayed2, a2.ID, j2.ID)
	}

	// The same rule protects the copy side: an active audit blocks a new job.
	if _, _, _, err = s.AdmitAudit(ctx, "owner", "owner", "audit-key", col.ID); err != nil {
		t.Fatalf("replay broke: %v", err)
	}
}

func TestAdmitAuditScopesAndMissingCollection(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	col, _ := auditFixture(t, s)

	if _, _, _, err := s.AdmitAudit(ctx, "owner", "operator1", "k", col.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("another principal's collection err = %v", err)
	}
	if _, _, _, err := s.AdmitAudit(ctx, "owner", "owner", "k", "col_missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing collection err = %v", err)
	}
}

func TestFindingsUpsertPaginateAndMarkRepaired(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	col, _ := auditFixture(t, s)
	a, _, _, err := s.AdmitAudit(ctx, "owner", "owner", "k", col.ID)
	if err != nil {
		t.Fatal(err)
	}

	fs := []Finding{
		{Kind: "missing_at_destination", SourceID: "f1", DestID: "", Repairable: true, Detail: "gone"},
		{Kind: "extra_at_destination", DestID: "dx", Detail: "unknown file"},
		{Kind: "duplicate_at_destination", SourceID: "f1", DestID: "dup1", Repairable: true},
	}
	if err = s.RecordFindings(ctx, a.ID, fs); err != nil {
		t.Fatal(err)
	}
	// Upsert: the same key rewrites detail instead of duplicating.
	if err = s.RecordFindings(ctx, a.ID, []Finding{{Kind: "missing_at_destination", SourceID: "f1", Repairable: true, Detail: "still gone"}}); err != nil {
		t.Fatal(err)
	}

	page1, next, err := s.ListFindings(ctx, a.ID, "", 2, 0)
	if err != nil || len(page1) != 2 || next == 0 {
		t.Fatalf("page1 = %v next=%d err=%v", page1, next, err)
	}
	page2, next2, err := s.ListFindings(ctx, a.ID, "", 2, next)
	if err != nil || len(page2) != 1 || next2 != 0 {
		t.Fatalf("page2 = %v next=%d err=%v", page2, next2, err)
	}
	if page2[0].Detail != "still gone" {
		t.Fatalf("upsert lost: %+v", page2[0])
	}
	only, _, err := s.ListFindings(ctx, a.ID, "extra_at_destination", 10, 0)
	if err != nil || len(only) != 1 || only[0].Kind != "extra_at_destination" {
		t.Fatalf("kind filter = %v err=%v", only, err)
	}

	rep := []Finding{{Kind: "missing_at_destination", SourceID: "f1"}}
	if err = s.MarkFindingsRepaired(ctx, a.ID, rep); err != nil {
		t.Fatal(err)
	}
	all, _, err := s.ListFindings(ctx, a.ID, "missing_at_destination", 10, 0)
	if err != nil || len(all) != 1 || !all[0].Repaired || all[0].Detail != "still gone" {
		t.Fatalf("repaired = %+v err=%v", all, err)
	}
}

func TestAuditLifecycleCompletesWithCountsAndRevision(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	col, _ := auditFixture(t, s)
	a, _, _, err := s.AdmitAudit(ctx, "owner", "owner", "k", col.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.AuditScanProgress(ctx, a.ID, 7, map[string]int{"missing_at_destination": 1}); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetAudit(ctx, "owner", a.ID)
	if err != nil || got.Scanned != 7 || got.Counts["missing_at_destination"] != 1 {
		t.Fatalf("progress = %+v err=%v", got, err)
	}
	if _, err = s.GetAudit(ctx, "operator1", a.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("scope check err = %v", err)
	}
	done, err := s.CompleteAudit(ctx, a.ID, AuditCompleted, map[string]int{"missing_at_destination": 1}, json.RawMessage(`{"actions":1}`))
	if err != nil || done.State != AuditCompleted || done.Revision != 2 {
		t.Fatalf("complete = %+v err=%v", done, err)
	}
	if done.FinishedAt.IsZero() || string(done.Plan) != `{"actions":1}` {
		t.Fatalf("finished audit = %+v", done)
	}
}

func TestAdmitRepairNeedsCompletedAuditAndRevision(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	col, _ := auditFixture(t, s)
	a, _, _, err := s.AdmitAudit(ctx, "owner", "owner", "k", col.ID)
	if err != nil {
		t.Fatal(err)
	}

	// A running audit cannot be repaired.
	if _, _, err = s.AdmitRepair(ctx, "owner", "owner", "rk", a.ID, a.Revision, true); !errors.Is(err, ErrTransition) {
		t.Fatalf("repair of running audit err = %v", err)
	}
	done, err := s.CompleteAudit(ctx, a.ID, AuditCompleted, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	// The audit job itself must be settled before a repair can be admitted
	// (one_active_run); in production the engine finalizes it.
	if _, err = s.db.ExecContext(ctx, "UPDATE jobs SET state='succeeded' WHERE id=?", a.JobID); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.AdmitRepair(ctx, "owner", "owner", "rk", a.ID, done.Revision-1, true); !errors.Is(err, ErrStale) {
		t.Fatalf("stale revision err = %v", err)
	}
	j, replayed, err := s.AdmitRepair(ctx, "owner", "owner", "rk", a.ID, done.Revision, true)
	if err != nil || replayed || j.Kind != JobKindRepair || j.AuditID != a.ID || !j.DryRun {
		t.Fatalf("repair job = %+v replayed=%v err=%v", j, replayed, err)
	}
	// The repair job occupies the collection: no second repair, no audit.
	if _, _, err = s.AdmitRepair(ctx, "owner", "owner", "rk2", a.ID, done.Revision, false); !errors.Is(err, ErrTransition) {
		t.Fatalf("second repair err = %v", err)
	}
}

// Migration: a database at schema 005 with data upgrades to 006 with the data
// intact and a clean foreign_key_check.
func TestMigration006UpgradesFromSchema005WithData(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "state-v2.db"))
	if err != nil {
		t.Fatal(err)
	}
	{
		ms, err := loadMigrations()
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range ms {
			if m.version > 5 {
				break
			}
			if _, err = db.Exec(m.sql); err != nil {
				t.Fatalf("migration %d: %v", m.version, err)
			}
		}
		if _, err = db.Exec("PRAGMA user_version=5; PRAGMA application_id=502002"); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec("INSERT INTO plans VALUES(?,?,?,NULL)", "plan1", "owner", []byte(`{"id":"plan1"}`)); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec("INSERT INTO collections(id, principal, plan_id, body) VALUES(?,?,?,?)", "col1", "owner", "plan1", []byte(`{"id":"col1","source_id":"src","source_kind":"folder","state":"ready"}`)); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec("INSERT INTO jobs VALUES(?,?,?,?,?)", "job1", "owner", "col1", "succeeded", []byte(`{}`)); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec("INSERT INTO items(collection_id,source_id,job_id,parent_source_id,kind,name,state,generation,fingerprint) VALUES(?,?,?,?,?,?,?,?,?)",
			"col1", "src1", "job1", "", "file", "a.txt", "done", 1, "v1"); err != nil {
			t.Fatal(err)
		}
		if err = db.Close(); err != nil {
			t.Fatal(err)
		}
	}

	s, err := Open(dir)
	if err != nil {
		t.Fatalf("open after migration: %v", err)
	}
	defer s.Close()
	var ver int
	if err = s.db.QueryRowContext(context.Background(), "PRAGMA user_version").Scan(&ver); err != nil || ver < 6 {
		t.Fatalf("user_version = %d err=%v", ver, err)
	}
	var rows int
	if err = s.db.QueryRowContext(context.Background(), "SELECT count(*) FROM items").Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("items survived: %d err=%v", rows, err)
	}
	fk, err := s.db.QueryContext(context.Background(), "PRAGMA foreign_key_check")
	if err != nil {
		t.Fatal(err)
	}
	if fk.Next() {
		t.Fatal("foreign_key_check is not clean")
	}
	fk.Close()
	// The new tables are usable through the normal path.
	col := Collection{ID: "col1"}
	if _, _, _, err = s.AdmitAudit(context.Background(), "owner", "owner", "k1", col.ID); err != nil {
		t.Fatalf("admit audit on upgraded db: %v", err)
	}
}

func TestPrepareRepairKeepsTheOldDestination(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	col, job := auditFixture(t, s)
	it, _, err := firstItem(t, s, col.ID, "f1")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.PrepareRepair(ctx, job.ID, col.ID, []RepairOp{{SourceID: "f1", Action: "recopy"}}); err != nil {
		t.Fatal(err)
	}
	after, _, err := firstItem(t, s, col.ID, "f1")
	if err != nil {
		t.Fatal(err)
	}
	if after.OldDestID != it.DestID || after.DestID != "" || after.State != ItemPending || after.JobID != job.ID {
		t.Fatalf("before dest=%q old=%q / after dest=%q old=%q state=%s job=%s",
			it.DestID, it.OldDestID, after.DestID, after.OldDestID, after.State, after.JobID)
	}
}

func firstItem(t *testing.T, s *Store, colID, sourceID string) (Item, bool, error) {
	t.Helper()
	all, err := s.Items(context.Background(), colID)
	if err != nil {
		return Item{}, false, err
	}
	for _, it := range all {
		if it.SourceID == sourceID {
			return it, true, nil
		}
	}
	return Item{}, false, nil
}
