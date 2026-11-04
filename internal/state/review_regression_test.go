package state

import (
	"database/sql"
	"testing"
	"time"
)

func TestCancelledBootstrapCanReconcileAgain(t *testing.T) {
	for _, running := range []bool{false, true} {
		t.Run(map[bool]string{false: "queued", true: "running"}[running], func(t *testing.T) {
			s := openStore(t)
			col, _, id := bootstrappingSync(t, s, time.Now())
			if running {
				if _, err := s.ClaimQueued(t.Context()); err != nil {
					t.Fatal(err)
				}
			}
			j, _ := s.GetJob(t.Context(), "owner", id)
			if _, err := s.Action(t.Context(), "owner", "cancel", id, "cancel", j.Revision); err != nil {
				t.Fatal(err)
			}
			if running {
				if _, err := s.FinishJob(t.Context(), id, func(j *Job) (string, bool) {
					j.State = JobCancelled
					return "cancelled", true
				}); err != nil {
					t.Fatal(err)
				}
				if _, err := s.RunFinished(t.Context(), id); err != nil {
					t.Fatal(err)
				}
			}
			sub, err := s.GetSubscription(t.Context(), col.ID)
			if err != nil || sub.State != SyncNeedsReconcile {
				t.Fatalf("cancelled bootstrap stranded subscription: %+v %v", sub, err)
			}
			pending, err := s.NeedsReconcileSubs(t.Context())
			if err != nil || len(pending) != 1 || pending[0].CollectionID != col.ID {
				t.Fatalf("no recovery available: %+v %v", pending, err)
			}
		})
	}
}

func TestAuditDoesNotCountAsSuccessfulSync(t *testing.T) {
	s := openStore(t)
	col, _, id := bootstrappingSync(t, s, time.Now())
	activate(t, s, col, id)
	before, _ := s.GetSubscription(t.Context(), col.ID)
	if err := s.tx(t.Context(), func(tx *sql.Tx) error {
		sub, err := getSub(t.Context(), tx, col.ID)
		if err != nil {
			return err
		}
		sub.LastError = &Failure{Code: "source_mass_removal", Message: "Source scan stopped"}
		return putSub(t.Context(), tx, sub)
	}); err != nil {
		t.Fatal(err)
	}
	_, auditJob, _, err := s.AdmitAudit(t.Context(), "owner", "", "audit", col.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.FinishJob(t.Context(), auditJob.ID, func(j *Job) (string, bool) {
		j.State = JobSucceeded
		return "finished", true
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.RunFinished(t.Context(), auditJob.ID); err != nil {
		t.Fatal(err)
	}
	after, _ := s.GetSubscription(t.Context(), col.ID)
	if after.LastError == nil || after.LastError.Code != "source_mass_removal" || !after.LastSuccessAt.Equal(*before.LastSuccessAt) {
		t.Fatalf("read-only audit overwrote sync result: before=%+v after=%+v", before, after)
	}
}

func TestOrphanBootstrapFromOlderDaemonIsRecoverable(t *testing.T) {
	s := openStore(t)
	col, _, id := bootstrappingSync(t, s, time.Now())
	if _, err := s.AppendPendingChanges(t.Context(), col.ID, []ReplayChange{{FileID: "f1"}}, "old-buffer", time.Now()); err != nil {
		t.Fatal(err)
	}
	j, _ := s.GetJob(t.Context(), "owner", id)
	if _, err := s.Action(t.Context(), "owner", "cancel", id, "cancel", j.Revision); err != nil {
		t.Fatal(err)
	}
	// Reproduce state persisted by the old daemon, including an overflowing buffer.
	if _, err := s.db.ExecContext(t.Context(), "UPDATE sync_subscriptions SET state='bootstrapping',replay_overflow=1 WHERE collection_id=?", col.ID); err != nil {
		t.Fatal(err)
	}
	needs, err := s.NeedsReconcileSubs(t.Context())
	if err != nil || len(needs) != 1 {
		t.Fatalf("old orphan was not selected for recovery: %+v %v", needs, err)
	}
	if err := s.MarkNeedsReconcile(t.Context(), col.ID, "fresh", Failure{Code: "needs_reconcile"}); err != nil {
		t.Fatal(err)
	}
	if id, err := s.ActiveRunID(t.Context(), col.ID); err != nil || id == "" {
		t.Fatalf("no reconcile admitted for old orphan: %q %v", id, err)
	}
	sub, _ := s.GetSubscription(t.Context(), col.ID)
	if sub.Cursor != "fresh" || sub.ReplayOverflow || sub.ReplayCount != 0 || sub.ReplayBytes != 0 || pendingCount(t, s, col.ID) != 0 {
		t.Fatalf("fresh reconcile reused stale replay: %+v", sub)
	}
}
