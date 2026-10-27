package state

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestRepeatedQueryPlans records the query plans of the queries that run on
// every sync tick or every claim. Plans are logged (and asserted to avoid a
// full scan of items/jobs) so a schema change that drops an index fails here.
func TestRepeatedQueryPlans(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	col, _ := auditFixture(t, s)
	if _, _, err := s.ConfigureSync(ctx, "owner", "", "plan-cfg", col.ID, SyncChange{
		Mode: ModeRealtime, ContentPolicy: PolicyVersioned, ExpectedCollection: col.Revision,
		Cursor: "7", Now: time.Now(), FullReconcileEvery: 86400,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendPendingChanges(ctx, col.ID, []ReplayChange{{FileID: "f1"}}, "8", time.Now()); err != nil {
		t.Fatal(err)
	}

	queries := []struct {
		name  string
		sql   string
		args  []any
		index string
	}{
		{"pending-batch", "SELECT change_json,cursor_after FROM sync_pending_changes WHERE collection_id=? ORDER BY seq LIMIT ?", []any{col.ID, 10}, "sqlite_autoindex_sync_pending_changes_1"},
		{"pending-last", "SELECT cursor_after FROM sync_pending_changes WHERE collection_id=? ORDER BY seq DESC LIMIT 1", []any{col.ID}, "sqlite_autoindex_sync_pending_changes_1"},
		{"pending-collections", "SELECT DISTINCT collection_id FROM sync_pending_changes ORDER BY collection_id", nil, ""},
		{"due-polls", "SELECT " + subCols + " FROM sync_subscriptions WHERE mode='realtime' AND state IN ('active','bootstrapping') AND next_poll_at<=? ORDER BY next_poll_at LIMIT 50", []any{time.Now().UnixMilli()}, "sync_due_poll"},
		{"full-reconcile", "SELECT collection_id FROM sync_subscriptions WHERE mode='realtime' AND state='active' AND full_reconcile_every>0 AND (next_full_reconcile_at=0 OR next_full_reconcile_at<=?) ORDER BY next_full_reconcile_at LIMIT 50", []any{time.Now().UnixMilli()}, "sync_due_full"},
		{"next-batch", "SELECT " + itemCols + " FROM items WHERE kind='folder' AND list_state='none' AND (job_id=? OR scan_job=?) ORDER BY rowid LIMIT ?", []any{col.BaselineJobID, col.BaselineJobID, 40}, "items_job_list"},
		{"next-partial", "SELECT " + itemCols + " FROM items WHERE kind='folder' AND list_state='partial' AND (job_id=? OR scan_job=?) ORDER BY rowid LIMIT 1", []any{col.BaselineJobID, col.BaselineJobID}, "items_job_list"},
		{"item-totals", "SELECT count(*), coalesce(sum(state='done'),0) FROM items WHERE collection_id=? AND NOT (parent_source_id='' AND (kind='folder' OR kind='wrapper'))", []any{col.ID}, ""},
		{"copied-24h", "SELECT coalesce(sum(bytes),0) FROM copy_ledger WHERE principal=? AND hour>=?", []any{"owner", 0}, "sqlite_autoindex_copy_ledger_1"},
	}
	for _, q := range queries {
		rows, err := s.db.QueryContext(ctx, "EXPLAIN QUERY PLAN "+q.sql, q.args...)
		if err != nil {
			t.Fatalf("%s: %v", q.name, err)
		}
		var plan []string
		for rows.Next() {
			var id, parent, notused int
			var detail string
			if err = rows.Scan(&id, &parent, &notused, &detail); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			plan = append(plan, detail)
		}
		rows.Close()
		got := strings.Join(plan, " | ")
		t.Logf("%s: %s", q.name, got)
		if q.index != "" && !strings.Contains(got, q.index) {
			t.Errorf("%s does not use %s: %s", q.name, q.index, got)
		}
	}
}
