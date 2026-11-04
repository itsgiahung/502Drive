package daemon

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/itsgiahung/502Drive/internal/state"
)

func TestReplayKeepsCursorAndBufferWhenRelevanceReadFails(t *testing.T) {
	rig := newSyncRig(t, time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC))
	c := rig.ready("Replay")
	sub, job := rig.bootstrapping(c)
	if _, err := rig.store.AppendPendingChanges(t.Context(), c.ID, []state.ReplayChange{{FileID: "synthetic-change", Parents: []string{c.SourceID}}}, "next", time.Now()); err != nil {
		t.Fatal(err)
	}
	rig.finish(job)
	db, err := sql.Open("sqlite", filepath.Join(rig.dir, "state-v2.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// Inject a metadata-query failure while leaving the subscription/buffer writable.
	if _, err = db.Exec("ALTER TABLE items RENAME TO unavailable_items"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := db.Exec("ALTER TABLE unavailable_items RENAME TO items"); err != nil {
			t.Error(err)
		}
	}()
	rig.sy.replayPending(t.Context(), time.Now())
	after := rig.subscription(c.ID)
	if after.Cursor != sub.Cursor || rig.pending(c.ID) != 1 {
		t.Fatalf("failed relevance query consumed changes: cursor=%q->%q pending=%d", sub.Cursor, after.Cursor, rig.pending(c.ID))
	}
}
