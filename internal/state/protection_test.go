package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
)

func TestProtectionOwnershipBusyAndIdempotency(t *testing.T) {
	s := openStore(t)
	c, _ := auditFixture(t, s)
	ctx := context.Background()
	opts := ProtectionOptions{Mirror: true, Snapshots: true}
	if _, _, err := s.SetProtection(ctx, "operator", "operator", "k", c.ID, c.Revision, opts); !errors.Is(err, ErrNotFound) {
		t.Fatalf("scope %v", err)
	}
	got, replay, err := s.SetProtection(ctx, "owner", "owner", "k", c.ID, c.Revision, opts)
	if err != nil || replay || !got.Protection.Mirror || got.Protection.KeepDays != 30 {
		t.Fatalf("options %+v %v %v", got, replay, err)
	}
	if _, replay, err = s.SetProtection(ctx, "owner", "owner", "k", c.ID, c.Revision, opts); err != nil || !replay {
		t.Fatalf("replay %v %v", replay, err)
	}
	if _, _, err = s.SetProtection(ctx, "owner", "owner", "stale", c.ID, c.Revision, opts); !errors.Is(err, ErrStale) {
		t.Fatalf("stale %v", err)
	}
	if _, _, _, err = s.AdmitAudit(ctx, "owner", "owner", "audit", c.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.SetProtection(ctx, "owner", "owner", "busy", c.ID, got.Revision, opts); err == nil {
		t.Fatal("changed options during active writer")
	}
}

func TestSnapshotsCaptureRetentionPinsAndRestorePause(t *testing.T) {
	s := openStore(t)
	c, _ := auditFixture(t, s)
	ctx := t.Context()
	if _, err := s.db.ExecContext(ctx, "UPDATE collections SET body=json_set(body,'$.destination_root_id','destroot') WHERE id=?", c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, "INSERT INTO sync_subscriptions(collection_id,id,mode,state) VALUES(?,?,'scheduled','active')", c.ID, "sub_test"); err != nil {
		t.Fatal(err)
	}
	var first Snapshot
	for n := 0; n < 3; n++ {
		j, _, err := s.AdmitProtectionJob(ctx, "owner", "owner", fmt.Sprint("snap", n), c.ID, "", c.Revision, false)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.ClaimQueued(ctx); err != nil {
			t.Fatal(err)
		}
		snap, err := s.BeginSnapshot(ctx, j.ID)
		if err != nil {
			t.Fatal(err)
		}
		snap2, err := s.BeginSnapshot(ctx, j.ID)
		if err != nil || snap2.ID != snap.ID {
			t.Fatal("capture restart changed snapshot")
		}
		items, err := s.SnapshotItems(ctx, snap.ID, 100, "")
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range items {
			if err = s.RecordSnapshotItem(ctx, snap.ID, v.Item, "synthetic-identity"); err != nil {
				t.Fatal(err)
			}
		}
		if _, err = s.FinishJob(ctx, j.ID, func(j *Job) (string, bool) { j.State = JobSucceeded; return "finished", true }); err != nil {
			t.Fatal(err)
		}
		snap, err = s.Snapshot(ctx, "owner", snap.ID)
		if err != nil || snap.State != "ready" {
			t.Fatalf("ready %+v %v", snap, err)
		}
		if n == 0 {
			first, _, err = s.PinSnapshot(ctx, "owner", "owner", "pin", snap.ID, snap.Revision, true)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := s.PruneSnapshots(ctx, c.ID, ProtectionOptions{KeepDays: 30, KeepCount: 1}); err != nil {
		t.Fatal(err)
	}
	list, next, err := s.Snapshots(ctx, "owner", c.ID, 1, 0)
	if err != nil || len(list) != 1 || next == 0 {
		t.Fatalf("page %v %d %v", list, next, err)
	}
	more, _, err := s.Snapshots(ctx, "owner", c.ID, 1, next)
	if err != nil || len(more) != 1 || more[0].ID != first.ID {
		t.Fatalf("pin retained %v %v", more, err)
	}
	if _, err = s.Snapshot(ctx, "operator", first.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("scope leaked snapshot")
	}
	j, _, err := s.AdmitProtectionJob(ctx, "owner", "owner", "preview", c.ID, first.ID, first.Revision, true)
	if err != nil {
		t.Fatal(err)
	}
	sub, _ := s.GetSubscription(ctx, c.ID)
	if sub.State != SyncActive {
		t.Fatal("preview paused sync")
	}
	if _, err = s.FinishJob(ctx, j.ID, func(j *Job) (string, bool) { j.State = JobSucceeded; return "finished", true }); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.AdmitProtectionJob(ctx, "owner", "owner", "restore", c.ID, first.ID, first.Revision, false); err != nil {
		t.Fatal(err)
	}
	sub, _ = s.GetSubscription(ctx, c.ID)
	if sub.State != SyncPaused {
		t.Fatal("restore did not pause sync")
	}
}

func TestMigration027Upgrades026WithoutChangingCollections(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", dir+"/state-v2.db")
	if err != nil {
		t.Fatal(err)
	}
	migrations, err := loadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range migrations {
		if m.version > 26 {
			break
		}
		if _, err = db.Exec(m.sql); err != nil {
			t.Fatal(err)
		}
	}
	for _, stmt := range []string{
		`PRAGMA user_version=26`, `PRAGMA application_id=502002`,
		`INSERT INTO plans(id,principal,body) VALUES('plan','owner','{}')`,
		`INSERT INTO collections(id,principal,plan_id,body) VALUES('col','owner','plan','{"id":"col","state":"ready","revision":7}')`,
	} {
		if _, err = db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c, err := s.GetCollection(t.Context(), "owner", "col")
	if err != nil || c.Revision != 7 || c.Protection.Mirror || c.Protection.Snapshots {
		t.Fatalf("upgrade %+v %v", c, err)
	}
	var n int
	if err = s.db.QueryRowContext(t.Context(), "SELECT count(*) FROM pragma_foreign_key_check").Scan(&n); err != nil || n != 0 {
		t.Fatalf("FK %d %v", n, err)
	}
}
