package state

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestPruneEnforcesRetentionAndProtectsReferences(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	now := time.Now().UTC()
	oldTime := now.Add(-100 * 24 * time.Hour)
	recentTime := now.Add(-10 * 24 * time.Hour)

	// 1. Setup Collection C1 and Plan P1
	p1 := Plan{
		ID:        "plan_old_1",
		Principal: "owner",
		ExpiresAt: oldTime,
		Revision:  1,
	}
	p1Bytes, _ := json.Marshal(p1)
	if _, err := s.db.ExecContext(ctx, "INSERT INTO plans(id, principal, body, consumed_job) VALUES(?, ?, ?, ?)", p1.ID, p1.Principal, p1Bytes, "job_referenced"); err != nil {
		t.Fatal(err)
	}

	c1 := Collection{
		ID:       "col_1",
		SourceID: "src_1",
		RootName: "Root1",
	}
	c1Bytes, _ := json.Marshal(c1)
	if _, err := s.db.ExecContext(ctx, "INSERT INTO collections(id, principal, plan_id, body) VALUES(?, ?, ?, ?)", c1.ID, "owner", p1.ID, c1Bytes); err != nil {
		t.Fatal(err)
	}

	// 2. Setup Jobs:
	// Job A: Old, done, but referenced by items (MUST NOT BE PRUNED)
	jobA := Job{
		ID:           "job_referenced",
		Principal:    "owner",
		CollectionID: c1.ID,
		State:        "succeeded",
		CreatedAt:    oldTime,
		UpdatedAt:    oldTime,
	}
	jobABytes, _ := json.Marshal(jobA)
	if _, err := s.db.ExecContext(ctx, "INSERT INTO jobs(id, principal, collection_id, state, body) VALUES(?, ?, ?, ?, ?)", jobA.ID, jobA.Principal, jobA.CollectionID, jobA.State, jobABytes); err != nil {
		t.Fatal(err)
	}

	// Add an item referencing Job A (the mapping)
	if _, err := s.db.ExecContext(ctx, `INSERT INTO items(collection_id, source_id, job_id, parent_source_id, kind, name, state, dest_id, intent_key)
		VALUES(?, 'src_item_1', ?, '', 'file', 'doc1.pdf', 'done', 'dest_item_1', 'intent_123')`, c1.ID, jobA.ID); err != nil {
		t.Fatal(err)
	}

	// Add an event for Job A
	if _, err := s.db.ExecContext(ctx, "INSERT INTO events(job_id, body) VALUES(?, ?)", jobA.ID, []byte(`{"kind":"status"}`)); err != nil {
		t.Fatal(err)
	}

	// Job B: Old, done, unreferenced (MUST BE PRUNED)
	jobB := Job{
		ID:           "job_unreferenced_old",
		Principal:    "owner",
		CollectionID: c1.ID,
		State:        "succeeded",
		CreatedAt:    oldTime,
		UpdatedAt:    oldTime,
	}
	jobBBytes, _ := json.Marshal(jobB)
	if _, err := s.db.ExecContext(ctx, "INSERT INTO jobs(id, principal, collection_id, state, body) VALUES(?, ?, ?, ?, ?)", jobB.ID, jobB.Principal, jobB.CollectionID, jobB.State, jobBBytes); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, "INSERT INTO events(job_id, body) VALUES(?, ?)", jobB.ID, []byte(`{"kind":"status"}`)); err != nil {
		t.Fatal(err)
	}

	// Job C: Recent, done, unreferenced (MUST NOT BE PRUNED: newer than cutoff)
	jobC := Job{
		ID:           "job_unreferenced_recent",
		Principal:    "owner",
		CollectionID: c1.ID,
		State:        "succeeded",
		CreatedAt:    recentTime,
		UpdatedAt:    recentTime,
	}
	jobCBytes, _ := json.Marshal(jobC)
	if _, err := s.db.ExecContext(ctx, "INSERT INTO jobs(id, principal, collection_id, state, body) VALUES(?, ?, ?, ?, ?)", jobC.ID, jobC.Principal, jobC.CollectionID, jobC.State, jobCBytes); err != nil {
		t.Fatal(err)
	}

	// Job D: Old, running, unreferenced (MUST NOT BE PRUNED: still running)
	jobD := Job{
		ID:           "job_running_old",
		Principal:    "owner",
		CollectionID: c1.ID,
		State:        "running",
		CreatedAt:    oldTime,
		UpdatedAt:    oldTime,
	}
	jobDBytes, _ := json.Marshal(jobD)
	if _, err := s.db.ExecContext(ctx, "INSERT INTO jobs(id, principal, collection_id, state, body) VALUES(?, ?, ?, ?, ?)", jobD.ID, jobD.Principal, jobD.CollectionID, jobD.State, jobDBytes); err != nil {
		t.Fatal(err)
	}

	// Telegram inbox entries
	// Old processed inbox (MUST BE PRUNED)
	if _, err := s.db.ExecContext(ctx, "INSERT INTO telegram_inbox(bot, update_id, body, state, received_at) VALUES('bot', 1, X'01', 'processed', ?)", oldTime.UnixMilli()); err != nil {
		t.Fatal(err)
	}
	// Recent processed inbox (MUST NOT BE PRUNED)
	if _, err := s.db.ExecContext(ctx, "INSERT INTO telegram_inbox(bot, update_id, body, state, received_at) VALUES('bot', 2, X'01', 'processed', ?)", recentTime.UnixMilli()); err != nil {
		t.Fatal(err)
	}
	// Old pending inbox (MUST NOT BE PRUNED: still pending)
	if _, err := s.db.ExecContext(ctx, "INSERT INTO telegram_inbox(bot, update_id, body, state, received_at) VALUES('bot', 3, X'01', 'pending', ?)", oldTime.UnixMilli()); err != nil {
		t.Fatal(err)
	}

	// Notification outbox
	// Old sent outbox (MUST BE PRUNED)
	if _, err := s.db.ExecContext(ctx, `INSERT INTO notification_outbox(id, kind, principal, chat_id, state, created_at, updated_at)
		VALUES('out1', 'progress', 'owner', 123, 'sent', ?, ?)`, oldTime.UnixMilli(), oldTime.UnixMilli()); err != nil {
		t.Fatal(err)
	}

	// 3. Test Dry-Run: should report what will be pruned without deleting anything
	reportDry, err := s.Prune(ctx, PruneOptions{OlderThan: 90 * 24 * time.Hour, DryRun: true})
	if err != nil {
		t.Fatalf("Prune dry-run failed: %v", err)
	}
	if reportDry.JobsPruned != 1 {
		t.Fatalf("Dry run expected 1 job pruned, got %d", reportDry.JobsPruned)
	}
	if reportDry.TelegramInboxPruned != 1 {
		t.Fatalf("Dry run expected 1 telegram inbox pruned, got %d", reportDry.TelegramInboxPruned)
	}
	if reportDry.TelegramOutboxPruned != 1 {
		t.Fatalf("Dry run expected 1 telegram outbox pruned, got %d", reportDry.TelegramOutboxPruned)
	}

	// Verify nothing was actually deleted in dry run
	var count int
	_ = s.db.QueryRowContext(ctx, "SELECT count(*) FROM jobs").Scan(&count)
	if count != 4 {
		t.Fatalf("Dry run deleted jobs: count = %d, expected 4", count)
	}

	// 4. Test Apply Prune:
	reportApply, err := s.Prune(ctx, PruneOptions{OlderThan: 90 * 24 * time.Hour, DryRun: false})
	if err != nil {
		t.Fatalf("Prune apply failed: %v", err)
	}
	if reportApply.JobsPruned != reportDry.JobsPruned {
		t.Fatalf("Apply jobs pruned %d != dry-run %d", reportApply.JobsPruned, reportDry.JobsPruned)
	}
	if reportApply.TelegramInboxPruned != reportDry.TelegramInboxPruned {
		t.Fatalf("Apply inbox pruned %d != dry-run %d", reportApply.TelegramInboxPruned, reportDry.TelegramInboxPruned)
	}

	// 5. Verify Database State:
	// Job A (referenced) must still exist
	var exists bool
	_ = s.db.QueryRowContext(ctx, "SELECT count(*)>0 FROM jobs WHERE id=?", jobA.ID).Scan(&exists)
	if !exists {
		t.Fatal("Referenced Job A was wrongly pruned")
	}

	// Job B (unreferenced old) must be gone
	_ = s.db.QueryRowContext(ctx, "SELECT count(*)>0 FROM jobs WHERE id=?", jobB.ID).Scan(&exists)
	if exists {
		t.Fatal("Unreferenced old Job B was not pruned")
	}

	// Job C (recent) must still exist
	_ = s.db.QueryRowContext(ctx, "SELECT count(*)>0 FROM jobs WHERE id=?", jobC.ID).Scan(&exists)
	if !exists {
		t.Fatal("Recent Job C was wrongly pruned")
	}

	// Job D (running) must still exist
	_ = s.db.QueryRowContext(ctx, "SELECT count(*)>0 FROM jobs WHERE id=?", jobD.ID).Scan(&exists)
	if !exists {
		t.Fatal("Running Job D was wrongly pruned")
	}

	// Events for Job B must be gone
	var eventCount int
	_ = s.db.QueryRowContext(ctx, "SELECT count(*) FROM events WHERE job_id=?", jobB.ID).Scan(&eventCount)
	if eventCount != 0 {
		t.Fatalf("Events for Job B were not pruned, count = %d", eventCount)
	}

	// Collections must remain intact (C1 not deleted)
	var colCount int
	_ = s.db.QueryRowContext(ctx, "SELECT count(*) FROM collections WHERE id=?", c1.ID).Scan(&colCount)
	if colCount != 1 {
		t.Fatal("Collection C1 was wrongly pruned")
	}

	// Mapping must remain intact (item not deleted)
	var itemCount int
	_ = s.db.QueryRowContext(ctx, "SELECT count(*) FROM items WHERE collection_id=? AND source_id='src_item_1'", c1.ID).Scan(&itemCount)
	if itemCount != 1 {
		t.Fatal("Item mapping was wrongly pruned")
	}

	// Write intent on completed item should be cleared
	var intentKey string
	_ = s.db.QueryRowContext(ctx, "SELECT intent_key FROM items WHERE collection_id=? AND source_id='src_item_1'", c1.ID).Scan(&intentKey)
	if intentKey != "" {
		t.Fatalf("Expected intent_key to be cleared, got %q", intentKey)
	}

	// 6. Foreign Key Integrity Check (CRITICAL)
	rows, err := s.db.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		t.Fatalf("foreign_key_check query failed: %v", err)
	}
	defer rows.Close()
	if rows.Next() {
		var table, fkid, parent string
		var rowid int64
		_ = rows.Scan(&table, &rowid, &parent, &fkid)
		t.Fatalf("Foreign key violation detected: table=%s, rowid=%d, parent=%s, fkid=%s", table, rowid, parent, fkid)
	}

	// 7. Test Retention = 0 (keep forever): prune with 0 duration should do nothing
	reportZero, err := s.Prune(ctx, PruneOptions{OlderThan: 0, DryRun: false})
	if err != nil {
		t.Fatalf("Prune with 0 failed: %v", err)
	}
	if reportZero.JobsPruned != 0 || reportZero.TelegramInboxPruned != 0 {
		t.Fatalf("Prune with olderThan=0 should prune nothing, got %+v", reportZero)
	}
}
