package engine

import (
	"context"

	"github.com/itsgiahung/502Drive/internal/drive"
	"github.com/itsgiahung/502Drive/internal/state"
)

// executeCleanup trashes the destination root folder of a retired collection.
// It only moves to Google Drive trash (never permanent deletion), and strictly
// verifies that the folder carries 502Drive's intent marker before trashing.
func (r *run) executeCleanup(ctx context.Context, job state.Job) {
	col, err := r.e.Store.GetCollection(r.db, "", job.CollectionID)
	if err != nil {
		r.block("collection_missing", "The collection for this cleanup is missing.")
		return
	}
	r.setPhase("cleanup")

	if col.RootID == nil || *col.RootID == "" {
		// No root on Drive -> cleanup is a no-op
		r.finishCleanup(state.JobSucceeded, "", "")
		return
	}

	if err := r.e.acquire(ctx); err != nil {
		r.settleCleanup(ctx)
		return
	}
	f, err := r.p.GetFile(ctx, *col.RootID, "")
	r.e.release()

	if drive.KindOf(err) == drive.KindNotFound || (err == nil && f.Trashed) {
		// Already gone or trashed -> idempotent success
		r.finishCleanup(state.JobSucceeded, "", "")
		return
	}
	if drive.KindOf(err) == drive.KindAuth {
		r.setReconnect()
		r.block("account_reconnect_required", "The Google account must be connected again.")
		return
	}
	if err != nil {
		r.block("drive_error", err.Error())
		return
	}

	// SAFETY GATE: Verify 502Drive intent marker
	intent := f.AppProperties[drive.IntentProperty]
	if intent == "" {
		r.finishCleanup(state.JobFailed, "intent_missing", "The destination root folder does not carry a 502Drive intent marker. Refusing to delete.")
		return
	}

	if job.DryRun {
		// Dry-run: calculate plan counts and finish as succeeded
		items, _ := r.e.Store.Items(r.db, col.ID)
		var totalBytes int64
		var count int
		for _, it := range items {
			if it.DestID != "" && it.State == state.ItemDone {
				count++
				if it.Size != nil {
					totalBytes += *it.Size
				}
			}
		}
		_, _ = r.e.Store.UpdateJob(r.db, r.id, func(j *state.Job) (string, bool) {
			total := count
			j.Counts.Total, j.Counts.Bytes = &total, totalBytes
			return "plan", true
		})
		r.finishCleanup(state.JobSucceeded, "", "")
		return
	}

	// Live cleanup: move destination root folder to Drive trash
	r.setPhase("cleanup")
	if err := r.e.acquire(ctx); err != nil {
		r.settleCleanup(ctx)
		return
	}
	err = r.p.TrashFile(ctx, *col.RootID)
	r.e.release()

	if err != nil && drive.KindOf(err) != drive.KindNotFound {
		if ctx.Err() != nil {
			// Shutdown or crash mid-call: leave the job running. The next
			// engine start reconciles it with GetFile, never by trashing again.
			r.settleCleanup(ctx)
			return
		}
		// Reconcile unknown outcome: check if folder is now in trash
		if r.e.acquire(ctx) == nil {
			checkFile, checkErr := r.p.GetFile(ctx, *col.RootID, "")
			r.e.release()
			if drive.KindOf(checkErr) == drive.KindNotFound || (checkErr == nil && checkFile.Trashed) {
				r.finishCleanup(state.JobSucceeded, "", "")
				return
			}
		}
		if drive.KindOf(err) == drive.KindAuth {
			r.setReconnect()
			r.block("account_reconnect_required", "The Google account must be connected again.")
			return
		}
		r.block("trash_failed", err.Error())
		return
	}

	r.finishCleanup(state.JobSucceeded, "", "")
}

func (r *run) settleCleanup(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	switch r.control() {
	case "cancel":
		r.finishCancel()
	case "pause":
		r.finishPause()
	case "run":
		// context was cancelled from outside
	}
}

func (r *run) finishCleanup(final, code, msg string) {
	_, err := r.e.Store.FinishJob(r.db, r.id, func(j *state.Job) (string, bool) {
		j.State, j.Phase, j.LastError = final, nil, nil
		if code != "" {
			j.LastError = &state.Failure{Code: code, Message: msg}
		}
		return "finished", true
	})
	if err == nil {
		r.log("finished", "state", final)
		r.afterRun()
	}
}
