package state

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigration010AddsConflictActionAndDestName(t *testing.T) {
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
		if _, err = db.Exec("INSERT INTO collections(id, principal, plan_id, body) VALUES(?,?,?,?)", "col1", "owner", "plan1", []byte(`{"id":"col1","source_id":"src","source_kind":"folder","state":"ready"}`)); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec("INSERT INTO jobs VALUES(?,?,?,?,?)", "job1", "owner", "col1", "succeeded", []byte(`{"id":"job1","principal":"owner"}`)); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(`INSERT INTO items(collection_id,source_id,job_id,parent_source_id,kind,name,mime_type,resource_key,size,fingerprint,state,dest_parent_id,list_state)
			VALUES('col1','src1','job1','','file','test.txt','text/plain','',10,'v1|md5|test.txt','done','root','done')`); err != nil {
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
	if err = s.db.QueryRowContext(context.Background(), "PRAGMA user_version").Scan(&ver); err != nil || ver < 10 {
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

	// Verify existing row has default empty conflict_action and dest_name
	var action, destName string
	err = s.db.QueryRowContext(context.Background(), "SELECT conflict_action, dest_name FROM items WHERE collection_id='col1' AND source_id='src1'").Scan(&action, &destName)
	if err != nil {
		t.Fatalf("query conflict cols: %v", err)
	}
	if action != "" || destName != "" {
		t.Fatalf("expected empty defaults, got action=%q destName=%q", action, destName)
	}

	// Verify we can update conflict_action and dest_name via RecordConflictDecision
	ok, err := s.ClaimDestName(context.Background(), Item{CollectionID: "col1", SourceID: "src1", DestParentID: "root"}, DecisionRename, "test (2).txt", false)
	if err != nil || !ok {
		t.Fatalf("ClaimDestName failed: ok=%v err=%v", ok, err)
	}
	err = s.db.QueryRowContext(context.Background(), "SELECT conflict_action, dest_name FROM items WHERE collection_id='col1' AND source_id='src1'").Scan(&action, &destName)
	if err != nil {
		t.Fatalf("query conflict cols after update: %v", err)
	}
	if action != "rename" || destName != "test (2).txt" {
		t.Fatalf("expected rename / test (2).txt, got action=%q destName=%q", action, destName)
	}
}

// A database that already holds mapped objects keeps their names reserved
// after migration 010, so a new copy in the same folder cannot take them.
func TestMigration010BackfillsReservedNamesAndIndexesThem(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "state-v2.db"))
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
	if _, err = db.Exec("INSERT INTO collections(id, principal, plan_id, body) VALUES(?,?,?,?)", "col1", "owner", "plan1", []byte(`{"id":"col1","source_id":"src","source_kind":"folder","state":"ready"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("INSERT INTO jobs VALUES(?,?,?,?,?)", "job1", "owner", "col1", "succeeded", []byte(`{"id":"job1","principal":"owner"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO items(collection_id,source_id,job_id,parent_source_id,kind,name,mime_type,resource_key,size,fingerprint,state,dest_parent_id,dest_id,list_state)
		VALUES('col1','mapped','job1','','file','report.txt','text/plain','',10,'v1|md5|report.txt','done','folderD','destMapped','done')`); err != nil {
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

	var reserved string
	if err := s.db.QueryRowContext(ctx, "SELECT dest_name FROM items WHERE source_id='mapped'").Scan(&reserved); err != nil || reserved != "report.txt" {
		t.Fatalf("backfilled dest_name = %q err=%v", reserved, err)
	}

	// The reservation is what a new item in the same folder checks.
	newcomer := Item{CollectionID: "col1", SourceID: "new", DestParentID: "folderD"}
	ok, err := s.ClaimDestName(ctx, newcomer, DecisionNone, "report.txt", false)
	if err != nil || ok {
		t.Fatalf("claim of a reserved name: ok=%v err=%v", ok, err)
	}
	ok, err = s.ClaimDestName(ctx, newcomer, DecisionRename, "report (2).txt", false)
	if err != nil || !ok {
		t.Fatalf("claim of a free name: ok=%v err=%v", ok, err)
	}

	rows, err := s.db.QueryContext(ctx, "EXPLAIN QUERY PLAN SELECT source_id FROM items WHERE dest_parent_id=? AND dest_name=?", "folderD", "x")
	if err != nil {
		t.Fatal(err)
	}
	var plan strings.Builder
	for rows.Next() {
		var id, parent, notused int
		var detail string
		if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(detail + "\n")
	}
	rows.Close()
	t.Logf("name lookup plan:\n%s", plan.String())
	if !strings.Contains(plan.String(), "items_dest_name") {
		t.Fatalf("name lookup does not use the index:\n%s", plan.String())
	}
}

// A decision written before a restart is still the one in force after it.
func TestDecisionSurvivesReopen(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := s.db.ExecContext(ctx, "INSERT INTO plans VALUES(?,?,?,NULL)", "plan1", "owner", []byte(`{"id":"plan1"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, "INSERT INTO collections(id, principal, plan_id, body) VALUES(?,?,?,?)", "col1", "owner", "plan1", []byte(`{"id":"col1","state":"ready"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, "INSERT INTO jobs VALUES(?,?,?,?,?)", "job1", "owner", "col1", "running", []byte(`{"id":"job1","principal":"owner"}`)); err != nil {
		t.Fatal(err)
	}
	for _, src := range []string{"a", "b"} {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO items(collection_id,source_id,job_id,parent_source_id,kind,name,mime_type,resource_key,size,fingerprint,state,dest_parent_id,list_state)
			VALUES('col1',?,'job1','','file','same.txt','text/plain','',1,'v1|m|same.txt','pending','D','done')`, src); err != nil {
			t.Fatal(err)
		}
	}
	a := Item{CollectionID: "col1", SourceID: "a", DestParentID: "D"}
	b := Item{CollectionID: "col1", SourceID: "b", DestParentID: "D"}
	if ok, err := s.ClaimDestName(ctx, a, DecisionNone, "same.txt", false); err != nil || !ok {
		t.Fatalf("first claim: %v %v", ok, err)
	}
	if ok, err := s.ClaimDestName(ctx, b, DecisionRename, "same (2).txt", false); err != nil || !ok {
		t.Fatalf("second claim: %v %v", ok, err)
	}
	s.Close()

	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var action, name string
	if err := s.db.QueryRowContext(ctx, "SELECT conflict_action, dest_name FROM items WHERE source_id='b'").Scan(&action, &name); err != nil {
		t.Fatal(err)
	}
	if action != DecisionRename || name != "same (2).txt" {
		t.Fatalf("after reopen: action=%q name=%q", action, name)
	}
	// Skipped items reserve nothing, so the name is free again for others.
	if err := s.SkipWithDecision(ctx, b, SkipConflict, DecisionSkip); err != nil {
		t.Fatal(err)
	}
	c := Item{CollectionID: "col1", SourceID: "c", DestParentID: "D"}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO items(collection_id,source_id,job_id,parent_source_id,kind,name,mime_type,resource_key,size,fingerprint,state,dest_parent_id,list_state)
		VALUES('col1','c','job1','','file','z.txt','text/plain','',1,'v1|m|z.txt','pending','D','done')`); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.ClaimDestName(ctx, c, DecisionNone, "same (2).txt", false); err != nil || !ok {
		t.Fatalf("name of a skipped item should be free: %v %v", ok, err)
	}
}

// Migration 011 backfills rel_path for trees copied before S5, and the pruning
// statement uses indexes rather than scanning the collection.
func TestMigration011BackfillsRelPathsAndPlansPruneWithIndexes(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "state-v2.db"))
	if err != nil {
		t.Fatal(err)
	}
	ms, err := loadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range ms {
		if m.version > 10 {
			break
		}
		if _, err = db.Exec(m.sql); err != nil {
			t.Fatalf("migration %d: %v", m.version, err)
		}
	}
	if _, err = db.Exec("PRAGMA user_version=10; PRAGMA application_id=502002"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("INSERT INTO plans VALUES(?,?,?,NULL)", "plan1", "owner", []byte(`{"id":"plan1"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("INSERT INTO collections(id, principal, plan_id, body) VALUES(?,?,?,?)", "col1", "owner", "plan1", []byte(`{"id":"col1","state":"ready"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("INSERT INTO jobs VALUES(?,?,?,?,?)", "job1", "owner", "col1", "succeeded", []byte(`{"id":"job1","principal":"owner"}`)); err != nil {
		t.Fatal(err)
	}
	rows := [][]any{
		{"root", "", "folder", "Proj"},
		{"sub", "root", "folder", "sub"},
		{"deep", "sub", "file", "d.txt"},
		{"top", "root", "file", "t.txt"},
	}
	for _, r := range rows {
		if _, err = db.Exec(`INSERT INTO items(collection_id,source_id,job_id,parent_source_id,kind,name,mime_type,resource_key,size,fingerprint,state,dest_parent_id,list_state)
			VALUES('col1',?,'job1',?,?,?,'','',1,'','done','D','done')`, r[0], r[1], r[2], r[3]); err != nil {
			t.Fatal(err)
		}
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
	want := map[string]string{"root": "", "sub": "sub", "deep": "sub/d.txt", "top": "t.txt"}
	for id, rel := range want {
		var got string
		if err := s.db.QueryRowContext(ctx, "SELECT rel_path FROM items WHERE source_id=?", id).Scan(&got); err != nil || got != rel {
			t.Fatalf("rel_path of %s = %q (err %v), want %q", id, got, err, rel)
		}
	}

	rs, err := s.db.QueryContext(ctx, "EXPLAIN QUERY PLAN "+pruneEmptySQL, "skipped", "col1")
	if err != nil {
		t.Fatal(err)
	}
	var plan strings.Builder
	for rs.Next() {
		var id, parent, notused int
		var detail string
		if err := rs.Scan(&id, &parent, &notused, &detail); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(detail + "\n")
	}
	rs.Close()
	t.Logf("prune plan:\n%s", plan.String())
	if !strings.Contains(plan.String(), "items_collection_kind_state") || !strings.Contains(plan.String(), "items_parent") {
		t.Fatalf("prune plan does not use the indexes:\n%s", plan.String())
	}
}

func TestSameSourceUpgradesLegacyFingerprintsOnce(t *testing.T) {
	cases := []struct {
		stored, current string
		want            bool
	}{
		{"m|abc|a.txt", "m|abc|a.txt", true},
		{"m|abc|a.txt", "m|def|a.txt", false},
		{"v3|abc|a.txt", "m|abc|a.txt", true},      // legacy binary: md5 and name
		{"v3|abc|a.txt", "m|def|a.txt", false},     // content changed
		{"v3|abc|a.txt", "m|abc|b.txt", false},     // renamed
		{"v3||note", "t1700000000000||note", true}, // legacy native: name only
		{"v3||note", "t1700000000000||other", false},
		{"v3|abc|a.txt", "v4|abc|a.txt", false}, // both legacy: exact only
		{"t1||note", "t2||note", false},
	}
	for _, c := range cases {
		if got := SameSource(c.stored, c.current); got != c.want {
			t.Fatalf("SameSource(%q, %q) = %v, want %v", c.stored, c.current, got, c.want)
		}
	}
}
