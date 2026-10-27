package state

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// A database at schema 007 with data upgrades to 008 with a clean
// foreign_key_check and a usable ledger.
func TestMigration008AddsTheCopyLedger(t *testing.T) {
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
			if m.version > 7 {
				break
			}
			if _, err = db.Exec(m.sql); err != nil {
				t.Fatalf("migration %d: %v", m.version, err)
			}
		}
		if _, err = db.Exec("PRAGMA user_version=7; PRAGMA application_id=502002"); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec("INSERT INTO plans VALUES(?,?,?,NULL)", "plan1", "owner", []byte(`{"id":"plan1"}`)); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec("INSERT INTO collections(id, principal, plan_id, body) VALUES(?,?,?,?)", "col1", "owner", "plan1", []byte(`{"id":"col1","source_id":"src","source_kind":"folder","state":"ready"}`)); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec("INSERT INTO jobs VALUES(?,?,?,?,?)", "job1", "owner", "col1", "succeeded", []byte(`{"id":"job1","principal":"owner"}`)); err != nil {
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
	if err = s.db.QueryRowContext(context.Background(), "PRAGMA user_version").Scan(&ver); err != nil || ver < 8 {
		t.Fatalf("user_version = %d err=%v", ver, err)
	}
	fk, err := s.db.QueryContext(context.Background(), "PRAGMA foreign_key_check")
	if err != nil {
		t.Fatal(err)
	}
	if fk.Next() {
		t.Fatal("foreign_key_check is not clean")
	}
	fk.Close()
	if _, err = s.db.ExecContext(context.Background(), "INSERT INTO copy_ledger(principal,hour,bytes) VALUES('owner',1,42)"); err != nil {
		t.Fatalf("ledger insert on upgraded db: %v", err)
	}
	if got, err := s.CopiedBytes24h(context.Background(), "owner", time.Unix(1*3600+30, 0)); err != nil || got != 42 {
		t.Fatalf("ledger = %d err=%v", got, err)
	}
}

func TestResumeDueBlockedOnlyMovesJobsPastTheirMark(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	col, _ := auditFixture(t, s)
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	resume := now.Add(time.Hour)
	if _, err := s.UpdateJob(ctx, col.BaselineJobID, func(j *Job) (string, bool) {
		j.State, j.DesiredState = JobBlocked, "run"
		j.LastError = &Failure{Code: "quota_exceeded", Message: "fixture"}
		j.ResumeAfter = &resume
		return "blocked", true
	}); err != nil {
		t.Fatal(err)
	}

	if n, err := s.ResumeDueBlocked(ctx, now); err != nil || n != 0 {
		t.Fatalf("early resume: n=%d err=%v", n, err)
	}
	if j, _ := s.GetJob(ctx, "", col.BaselineJobID); j.State != JobBlocked || j.ResumeAfter == nil {
		t.Fatalf("job changed before the mark: %+v", j)
	}
	if n, err := s.ResumeDueBlocked(ctx, resume.Add(time.Second)); err != nil || n != 1 {
		t.Fatalf("due resume: n=%d err=%v", n, err)
	}
	j, err := s.GetJob(ctx, "", col.BaselineJobID)
	if err != nil {
		t.Fatal(err)
	}
	if j.State != JobQueued || j.ResumeAfter != nil || j.LastError != nil || j.DesiredState != "run" {
		t.Fatalf("resumed job = %+v", j)
	}

	// A block without a mark is a human decision and stays put.
	if _, err = s.UpdateJob(ctx, col.BaselineJobID, func(j *Job) (string, bool) {
		j.State, j.LastError, j.ResumeAfter = JobBlocked, &Failure{Code: "unknown_remote_outcome", Message: "human"}, nil
		return "blocked", true
	}); err != nil {
		t.Fatal(err)
	}
	if n, err := s.ResumeDueBlocked(ctx, resume.Add(100*time.Hour)); err != nil || n != 0 {
		t.Fatalf("a manual block was auto-resumed: n=%d err=%v", n, err)
	}
}
