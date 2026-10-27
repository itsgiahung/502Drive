package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"
)

// A terminal job commit records the collection state and a durable bookkeeping
// marker in one transaction. If the process dies before the bookkeeping
// transaction (the after-copy request), the startup replay must still record it.
func TestFinishJobReplaysBookkeepingAfterACrash(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	col := Collection{ID: "col_bk", SourceID: "src_bk", SourceName: "P", SourceKind: KindFolder,
		DestinationID: "dest", RootName: "P", State: CollectionCopying, Revision: 1, BaselineJobID: "job_bk"}
	cb, _ := json.Marshal(col)
	j := Job{ID: col.BaselineJobID, Principal: "owner", CollectionID: col.ID, Trigger: "initial",
		SourceName: "P", DestinationID: "dest", RootName: "P", State: JobRunning, DesiredState: "run",
		AfterCopySync: "hourly", Revision: 1, CreatedAt: now, UpdatedAt: now}
	jb, _ := json.Marshal(j)
	if err := s.tx(ctx, func(tx *sql.Tx) error {
		if _, e := tx.ExecContext(ctx, "INSERT INTO plans VALUES(?,?,?,NULL)", "plan_"+col.ID, "owner", []byte(`{}`)); e != nil {
			return e
		}
		if _, e := tx.ExecContext(ctx, "INSERT INTO collections(id, principal, plan_id, body) VALUES(?,?,?,?)", col.ID, "owner", "plan_"+col.ID, cb); e != nil {
			return e
		}
		_, e := tx.ExecContext(ctx, "INSERT INTO jobs VALUES(?,?,?,?,?)", j.ID, "owner", col.ID, j.State, jb)
		return e
	}); err != nil {
		t.Fatal(err)
	}

	// The terminal commit succeeds; the process dies before RunFinished.
	if _, err := s.FinishJob(ctx, j.ID, func(x *Job) (string, bool) {
		x.State = JobSucceeded
		return "finished", true
	}); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetCollection(ctx, "", col.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != CollectionReady {
		t.Fatalf("collection state = %q, want ready (committed with the job)", got.State)
	}
	if _, err := s.AfterCopySyncOf(ctx, col.ID); err != ErrNotFound {
		t.Fatalf("after-copy row exists before bookkeeping: err=%v", err)
	}

	// Startup replay applies the bookkeeping exactly once.
	n, err := s.FinishPendingBookkeeping(ctx)
	if err != nil || n != 1 {
		t.Fatalf("replay = %d err=%v, want 1", n, err)
	}
	if _, err := s.AfterCopySyncOf(ctx, col.ID); err != nil {
		t.Fatalf("after-copy request lost after replay: %v", err)
	}
	if n, err = s.FinishPendingBookkeeping(ctx); err != nil || n != 0 {
		t.Fatalf("second replay = %d err=%v, want 0", n, err)
	}
}

// The new table is additive: a database at schema 25 with data upgrades to 26
// with the data intact and a clean foreign key check.
func TestMigration026AddsBookkeepingTable(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", dir+"/state-v2.db")
	if err != nil {
		t.Fatal(err)
	}
	ms, err := loadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range ms {
		if m.version > 25 {
			break
		}
		if _, err = db.Exec(m.sql); err != nil {
			t.Fatalf("migration %d: %v", m.version, err)
		}
	}
	if _, err = db.Exec("PRAGMA user_version=25; PRAGMA application_id=502002"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("INSERT INTO plans VALUES(?,?,?,NULL)", "plan1", "owner", []byte(`{"id":"plan1"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("INSERT INTO collections(id,principal,plan_id,body) VALUES(?,?,?,?)", "col1", "owner", "plan1", []byte(`{"id":"col1","state":"copying"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("INSERT INTO jobs VALUES(?,?,?,?,?)", "job1", "owner", "col1", "running", []byte(`{"id":"job1","collection_id":"col1","trigger":"initial","state":"running"}`)); err != nil {
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
	ctx := context.Background()
	var ver int
	if err = s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&ver); err != nil || ver < 26 {
		t.Fatalf("user_version = %d err=%v", ver, err)
	}
	if _, err = s.FinishJob(ctx, "job1", func(j *Job) (string, bool) {
		j.State = JobSucceeded
		return "finished", true
	}); err != nil {
		t.Fatalf("FinishJob on upgraded database: %v", err)
	}
	rows, err := s.db.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("foreign key violation after upgrade")
	}
}
