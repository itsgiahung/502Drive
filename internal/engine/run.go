package engine

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/itsgiahung/502Drive/internal/drive"
	"github.com/itsgiahung/502Drive/internal/state"
)

const maxTransientAttempts = 6

// run drives one job.
type run struct {
	e  *Engine
	db context.Context // never cancelled: durable writes must finish
	id string

	p          drive.Provider
	trigger    string
	collection string
	mu         sync.Mutex
	reconnect  bool // an auth failure was seen; stop claiming
	// cleanupFailures counts superseded copies a repair could not trash.
	cleanupFailures int
	// superseded collects destination ids retired by this run's re-copies
	// (RecordVersion returns them); a repair trashes them in its cleanup.
	superseded []string
}

func (r *run) log(msg string, args ...any) {
	r.e.Log.Info(msg, append([]any{"job", r.id}, args...)...)
}

func RootURL(id string) string { return "https://drive.google.com/drive/folders/" + id }

// control returns what the user wants: "run", "pause" or "cancel".
func (r *run) control() string {
	_, desired, err := r.e.Store.JobControl(r.db, r.id)
	if err != nil {
		return "run"
	}
	return desired
}

func (r *run) setPhase(phase string) {
	r.e.Store.UpdateJob(r.db, r.id, func(j *state.Job) (string, bool) {
		if j.Phase != nil && *j.Phase == phase {
			return "", false
		}
		j.Phase = &phase
		return "phase", false
	})
}

func (r *run) execute(ctx context.Context) {
	st, _, err := r.e.Store.JobControl(r.db, r.id)
	if err != nil {
		return
	}
	// A pause/cancel requested while nothing was running settles immediately.
	if r.control() == "cancel" {
		r.finishCancel()
		return
	}
	if r.control() == "pause" {
		r.finishPause()
		return
	}
	if st != state.JobRunning && st != state.JobReconciling {
		return
	}
	p, err := r.e.Provider(ctx)
	if err != nil {
		r.block("account_reconnect_required", "The Google account must be connected again.")
		return
	}
	r.p = p

	// Audit and repair are separate job families with their own phases.
	job, _ := r.e.Store.GetJob(r.db, "", r.id)
	switch job.Kind {
	case state.JobKindAudit:
		r.executeAudit(ctx, job)
		return
	case state.JobKindRepair:
		r.executeRepair(ctx, job)
		return
	}

	if !r.reconcileAll(ctx) {
		r.settle(ctx)
		return
	}
	r.setPhase("discovering")
	r.trigger = job.Trigger
	r.collection = job.CollectionID
	ok := false
	if job.Trigger == "initial" {
		ok = r.discover(ctx)
	} else {
		ok = r.discoverDelta(ctx)
	}
	if !ok {
		r.settle(ctx)
		return
	}
	r.setPhase("copying")
	r.copyAll(ctx)
	r.settle(ctx)
}

// settle moves the job to the state its items justify.
func (r *run) settle(ctx context.Context) {
	if ctx.Err() != nil { // shutting down: leave everything durable as-is
		return
	}
	switch r.control() {
	case "cancel":
		r.finishCancel()
		return
	case "pause":
		r.finishPause()
		return
	}
	if r.isReconnect() {
		r.block("account_reconnect_required", "The Google account must be connected again.")
		return
	}
	r.finalize()
}

// waitNetwork blocks while the machine is offline. It distinguishes "sleeping"
// from "failed": no attempt is consumed and no item is failed.
func (r *run) waitNetwork(ctx context.Context) bool {
	if r.e.Network == nil || r.e.Network.Online() {
		return true
	}
	r.log("waiting for the network")
	return r.e.Network.Wait(ctx)
}

// noteUnreachable tells the host that a call never left the machine.
func (r *run) noteUnreachable(err error) {
	if r.e.OnUnreachable != nil {
		r.e.OnUnreachable(err)
	}
}

func (r *run) isReconnect() bool { r.mu.Lock(); defer r.mu.Unlock(); return r.reconnect }
func (r *run) setReconnect()     { r.mu.Lock(); r.reconnect = true; r.mu.Unlock() }

func (r *run) block(code, msg string) {
	r.e.Store.UpdateJob(r.db, r.id, func(j *state.Job) (string, bool) {
		j.State, j.Phase, j.LastError = state.JobBlocked, nil, &state.Failure{Code: code, Message: msg}
		return "blocked", true
	})
}

func (r *run) finishPause() {
	r.e.Store.UpdateJob(r.db, r.id, func(j *state.Job) (string, bool) {
		j.State, j.Phase = state.JobPaused, nil
		return "paused", true
	})
}

func (r *run) finishCancel() {
	r.e.Store.CancelPending(r.db, r.id)
	j, err := r.e.Store.UpdateJob(r.db, r.id, func(j *state.Job) (string, bool) {
		j.State, j.Phase = state.JobCancelled, nil
		return "cancelled", true
	})
	if err == nil {
		// Only the baseline copy defines the collection's state; a cancelled
		// sync run must not make the collection look broken.
		if j.Trigger == "initial" {
			r.e.Store.SetCollectionState(r.db, j.CollectionID, "cancelled")
		}
		r.afterRun()
	}
}

// finalize picks succeeded / partial / blocked / failed from item evidence.
func (r *run) finalize() {
	r.e.Store.SkipOrphans(r.db, r.id)
	pr, err := r.e.Store.JobProgress(r.db, r.id)
	if err != nil {
		return
	}
	final, code, msg := state.JobSucceeded, "", ""
	switch {
	case !pr.RootDone && pr.Blocked > 0:
		final, code, msg = state.JobBlocked, "unknown_remote_outcome", "A write may have happened but could not be confirmed; retry to confirm."
	case !pr.RootDone:
		final, code, msg = state.JobFailed, "root_not_created", "The destination folder could not be created."
	case pr.Blocked > 0:
		final, code, msg = state.JobBlocked, "unknown_remote_outcome", "Some writes could not be confirmed; retry to re-check them."
	case pr.Failed > 0 || pr.ListingFailed > 0:
		final, code, msg = state.JobPartial, "items_failed", "Some items could not be copied."
	}
	j, err := r.e.Store.UpdateJob(r.db, r.id, func(j *state.Job) (string, bool) {
		j.State, j.Phase, j.LastError = final, nil, nil
		if code != "" {
			j.LastError = &state.Failure{Code: code, Message: msg}
		}
		return "finished", true
	})
	if err == nil {
		if j.Trigger == "initial" {
			cs := "ready"
			if final != state.JobSucceeded {
				cs = final
			}
			r.e.Store.SetCollectionState(r.db, j.CollectionID, cs)
		}
		r.log("finished", "state", final)
		r.afterRun()
	}
}

// ---- discovery ----

// discover lists every folder page by page. It returns false when the job
// should stop (control request, shutdown, fatal provider state).
func (r *run) discover(ctx context.Context) bool {
	for {
		if ctx.Err() != nil || r.control() != "run" {
			return false
		}
		folder, err := r.e.Store.NextToList(r.db, r.id)
		if errors.Is(err, state.ErrNotFound) {
			return true
		}
		if err != nil {
			r.log("list lookup failed", "err", err)
			return false
		}
		files, next, err := r.listPage(ctx, folder)
		if err != nil {
			switch drive.KindOf(err) {
			case drive.KindAuth:
				r.setReconnect()
				return false
			case drive.KindCancelled:
				return false
			}
			if ctx.Err() != nil {
				return false
			}
			r.e.Store.FailListing(r.db, folder, codeFor(err))
			continue
		}
		children := make([]state.Item, 0, len(files))
		for _, f := range files {
			children = append(children, toItem(f))
		}
		if err = r.e.Store.CommitPage(r.db, folder, children, next); err != nil {
			r.log("commit page failed", "err", err)
			return false
		}
	}
}

func toItem(f drive.File) state.Item {
	it := state.Item{SourceID: f.ID, Name: f.Name, MimeType: f.MimeType, ResourceKey: f.ResourceKey, Size: f.Size, Fingerprint: f.Fingerprint(), Kind: state.KindFile}
	switch {
	case f.IsFolder():
		it.Kind = state.KindFolder
	case f.IsShortcut():
		it.Kind, it.ShortcutTarget, it.ShortcutTargetMim, it.ResourceKey = state.KindShortcut, f.ShortcutTargetID, f.ShortcutTargetMime, f.ShortcutTargetKey
	}
	return it
}

// listPage retries transient failures in place; the page checkpoint is only
// committed once a page has been read successfully.
func (r *run) listPage(ctx context.Context, folder state.Item) ([]drive.File, string, error) {
	var lastErr error
	for attempt := 0; attempt < maxTransientAttempts; attempt++ {
		if !r.waitNetwork(ctx) {
			return nil, "", ctx.Err()
		}
		if err := r.e.acquire(ctx); err != nil {
			return nil, "", err
		}
		files, next, err := r.p.ListChildren(ctx, folder.SourceID, folder.ResourceKey, folder.PageToken)
		r.e.release()
		if err == nil {
			return files, next, nil
		}
		lastErr = err
		switch drive.KindOf(err) {
		case drive.KindUnreachable:
			// No page was read. When the host has a network gate, wait for it
			// and re-read the same page without spending the retry budget; a
			// host without a gate still gets the normal backoff (never a spin).
			r.noteUnreachable(err)
			if r.e.Network != nil && !r.e.Network.Online() {
				if !r.waitNetwork(ctx) {
					return nil, "", ctx.Err()
				}
				attempt--
				continue
			}
			if !r.e.sleep(ctx, max(r.e.backoff(attempt), drive.RetryAfterOf(err))) {
				return nil, "", ctx.Err()
			}
		case drive.KindThrottled, drive.KindUnavailable:
			d := max(r.e.backoff(attempt), drive.RetryAfterOf(err))
			if !r.e.sleep(ctx, d) {
				return nil, "", ctx.Err()
			}
		default:
			return nil, "", err
		}
	}
	return nil, "", lastErr
}

func codeFor(err error) string {
	switch drive.KindOf(err) {
	case drive.KindNotFound:
		return "source_not_found"
	case drive.KindPermission:
		return "permission_denied"
	case drive.KindQuota:
		return "quota_exceeded"
	case drive.KindThrottled:
		return "rate_limited"
	case drive.KindAuth:
		return "account_reconnect_required"
	case drive.KindInvalid:
		return "request_rejected"
	}
	return "provider_unavailable"
}

// ---- copying ----

func (r *run) copyAll(ctx context.Context) {
	var wg sync.WaitGroup
	for range r.e.Workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.worker(ctx)
		}()
	}
	wg.Wait()
}

func (r *run) worker(ctx context.Context) {
	for {
		if ctx.Err() != nil || r.control() != "run" || r.isReconnect() {
			return
		}
		// Offline: stop claiming work. In-flight writes are never cancelled:
		// they either finish or reconcile.
		if !r.waitNetwork(ctx) {
			return
		}
		it, err := r.e.Store.ClaimItem(r.db, r.id, r.e.Now().UnixMilli())
		if errors.Is(err, state.ErrNotFound) {
			pr, perr := r.e.Store.JobProgress(r.db, r.id)
			if perr != nil {
				return
			}
			if pr.Active == 0 && pr.Reconciling == 0 && pr.Pending == 0 {
				return
			}
			if pr.Active == 0 && pr.Reconciling == 0 && pr.NextRetryAt == 0 {
				return // pending items whose parents can never be mapped
			}
			wait := 100 * time.Millisecond
			if pr.Active == 0 && pr.NextRetryAt > 0 { // cooling down
				wait = min(time.Until(time.UnixMilli(pr.NextRetryAt)), time.Second)
				wait = max(wait, 10*time.Millisecond)
			}
			if !r.e.sleep(ctx, wait) {
				return
			}
			continue
		}
		if err != nil {
			r.log("claim item failed", "err", err)
			return
		}
		r.process(ctx, it)
	}
}

func (r *run) process(ctx context.Context, it state.Item) {
	if it.Op == state.OpAlign {
		r.align(ctx, it)
		return
	}
	src := drive.File{ID: it.SourceID, Name: it.Name, ResourceKey: it.ResourceKey, MimeType: it.MimeType}
	var out drive.File
	var err error
	if err = r.e.acquire(ctx); err != nil {
		// Shutdown before the request was sent: nothing to reconcile.
		r.e.Store.SetItemState(r.db, it, state.ItemPending, "")
		return
	}
	switch it.Kind {
	case state.KindWrapper, state.KindFolder:
		out, err = r.p.CreateFolder(ctx, it.Name, it.DestParentID, it.IntentKey)
	case state.KindFile:
		out, err = r.p.CopyFile(ctx, src, it.DestParentID, it.IntentKey)
	case state.KindShortcut:
		out, err = r.p.CreateShortcut(ctx, it.Name, it.DestParentID, it.ShortcutTarget, it.ResourceKey, it.IntentKey)
	default:
		err = &drive.Error{Kind: drive.KindInvalid, Reason: "unsupported_kind"}
	}
	r.e.release()
	if err == nil {
		r.complete(ctx, it, out.ID)
		return
	}
	switch k := drive.KindOf(err); {
	case k == drive.KindUnreachable:
		// The request never reached Google: the item did not fail, the machine
		// was offline. Give the attempt back and let the gate stop new claims.
		r.noteUnreachable(err)
		r.e.Store.ReleaseItemNoPenalty(r.db, it, "waiting_for_network")
	case drive.NotExecuted(err):
		switch k {
		case drive.KindAuth:
			r.setReconnect()
			r.e.Store.ReleaseItem(r.db, it, "account_reconnect_required", 1)
		case drive.KindThrottled, drive.KindQuota:
			if it.Attempts >= maxTransientAttempts {
				r.e.Store.ReleaseItem(r.db, it, "retry_exhausted", 0)
				return
			}
			d := max(r.e.backoff(it.Attempts), drive.RetryAfterOf(err))
			if k == drive.KindQuota {
				d = max(d, 10*time.Minute)
			}
			r.e.Store.ReleaseItem(r.db, it, codeFor(err), r.e.Now().Add(d).UnixMilli())
		default:
			r.e.Store.ReleaseItem(r.db, it, codeFor(err), 0)
		}
	default:
		// The write may have happened. Never resend before checking.
		r.e.Store.SetItemState(r.db, it, state.ItemReconciling, "")
		if ctx.Err() == nil {
			r.reconcile(ctx, it)
		}
	}
}

// ---- reconciliation ----

// reconcileAll resolves every item left in an unknown state by a crash or by
// an earlier lost response. It returns false if the job must stop.
func (r *run) reconcileAll(ctx context.Context) bool {
	items, err := r.e.Store.ItemsInState(r.db, r.id, state.ItemReconciling)
	if err != nil {
		return false
	}
	for _, it := range items {
		if ctx.Err() != nil || r.control() != "run" {
			return false
		}
		r.reconcile(ctx, it)
	}
	return !r.isReconnect()
}

const reconcileTries = 3

// reconcile looks for our own tagged output. One match is adopted; several or
// none-after-settling block the item for a human decision — except when the
// user already confirmed a resend by retrying.
func (r *run) reconcile(ctx context.Context, it state.Item) {
	for try := 0; try < reconcileTries; try++ {
		if err := r.e.acquire(ctx); err != nil {
			return
		}
		found, err := r.p.FindByIntent(ctx, it.DestParentID, it.IntentKey)
		r.e.release()
		if err != nil {
			switch drive.KindOf(err) {
			case drive.KindAuth:
				r.setReconnect()
				return
			case drive.KindUnreachable:
				// Offline: never conclude "unknown outcome" from a request that
				// never left the machine. Wait, then look again.
				r.noteUnreachable(err)
				if !r.waitNetwork(ctx) {
					return
				}
				try--
				continue
			}
			if !r.e.sleep(ctx, r.e.backoff(try)) {
				return
			}
			continue
		}
		switch len(found) {
		case 1:
			r.complete(ctx, it, found[0].ID)
			return
		case 0:
			if !r.e.sleep(ctx, r.e.Settle) {
				return
			}
		default:
			r.e.Store.SetItemState(r.db, it, state.ItemBlocked, "multiple_matches")
			return
		}
	}
	if ctx.Err() != nil {
		return
	}
	if it.ErrorCode == "retry_confirmed" {
		r.e.Store.SetItemState(r.db, it, state.ItemPending, "")
		return
	}
	r.e.Store.SetItemState(r.db, it, state.ItemBlocked, "unknown_remote_outcome")
}

// complete records the destination object, keeps the version history and, under
// replace_copy, retires the superseded copy only after the new one is mapped.
func (r *run) complete(ctx context.Context, it state.Item, destID string) {
	if e := r.e.Store.CompleteItem(r.db, it, destID, RootURL); e != nil {
		r.log("complete failed", "err", e)
		return
	}
	if it.Kind == state.KindWrapper || it.Kind == state.KindFolder {
		return
	}
	old, e := r.e.Store.RecordVersion(r.db, it, destID)
	if e != nil {
		r.log("record version failed", "err", e)
		return
	}
	if old != "" {
		r.mu.Lock()
		r.superseded = append(r.superseded, old)
		r.mu.Unlock()
	}
}

// retireSuperseded trashes old copies when the collection uses replace_copy.
// A failure keeps both copies and is reported by the caller as partial.
func (r *run) retireSuperseded(ctx context.Context) (failed int) {
	sub, err := r.e.Store.GetSubscription(r.db, r.collection)
	if err != nil || sub.ContentPolicy != state.PolicyReplace {
		return 0
	}
	olds, err := r.e.Store.SupersededVersions(r.db, r.collection)
	if err != nil {
		return 0
	}
	for _, v := range olds {
		if ctx.Err() != nil {
			return failed
		}
		if r.e.acquire(ctx) != nil {
			return failed
		}
		err := r.p.TrashFile(ctx, v.DestID)
		r.e.release()
		if err == nil || drive.KindOf(err) == drive.KindNotFound {
			r.e.Store.MarkTrashed(r.db, v)
		} else {
			failed++
		}
	}
	return failed
}

// align renames/moves an object we already own so it matches the source.
// Both steps are idempotent, so an unknown outcome is simply repeated later.
func (r *run) align(ctx context.Context, it state.Item) {
	retry := func(err error) {
		if drive.KindOf(err) == drive.KindAuth {
			r.setReconnect()
		}
		if drive.NotExecuted(err) && drive.KindOf(err) != drive.KindThrottled && drive.KindOf(err) != drive.KindUnreachable {
			r.e.Store.ReleaseItem(r.db, it, codeFor(err), 0)
			return
		}
		if it.Attempts >= maxTransientAttempts {
			r.e.Store.ReleaseItem(r.db, it, "retry_exhausted", 0)
			return
		}
		r.e.Store.ReleaseItem(r.db, it, codeFor(err), r.e.Now().Add(max(r.e.backoff(it.Attempts), drive.RetryAfterOf(err))).UnixMilli())
	}
	if err := r.e.acquire(ctx); err != nil {
		r.e.Store.SetItemState(r.db, it, state.ItemPending, "")
		return
	}
	cur, err := r.p.GetFile(ctx, it.DestID, "")
	if err == nil && cur.Name != it.Name {
		err = r.p.RenameFile(ctx, it.DestID, it.Name)
	}
	if err == nil && len(cur.Parents) > 0 && cur.Parents[0] != it.DestParentID {
		err = r.p.MoveFile(ctx, it.DestID, cur.Parents[0], it.DestParentID)
	}
	r.e.release()
	if err != nil {
		retry(err)
		return
	}
	r.e.Store.CompleteItem(r.db, it, it.DestID, RootURL)
}

func (r *run) afterRun() {
	if r.e.AfterRun != nil {
		r.e.AfterRun(r.id)
	}
}

// ---- delta discovery (sync runs) ----

// discoverDelta rescans the source against the existing mapping. It never
// creates a new root; only differences become work.
func (r *run) discoverDelta(ctx context.Context) bool {
	coll, err := r.e.Store.GetCollection(r.db, "", r.collection)
	if err != nil {
		return false
	}
	r.retireSuperseded(ctx)
	if coll.SourceKind == state.KindFile {
		f, err := r.p.GetFile(ctx, coll.SourceID, coll.ResourceKey)
		found := err == nil && !f.Trashed
		if err != nil && drive.KindOf(err) != drive.KindNotFound {
			if drive.KindOf(err) == drive.KindAuth {
				r.setReconnect()
			}
			return false
		}
		item := toItem(f)
		item.SourceID = coll.SourceID
		_, err = r.e.Store.ApplySingleFile(r.db, r.id, coll.ID, item, found)
		return err == nil
	}
	for {
		if ctx.Err() != nil || r.control() != "run" {
			return false
		}
		folder, err := r.e.Store.NextToList(r.db, r.id)
		if errors.Is(err, state.ErrNotFound) {
			break
		}
		if err != nil {
			return false
		}
		files, next, err := r.listPage(ctx, folder)
		if err != nil {
			switch drive.KindOf(err) {
			case drive.KindAuth:
				r.setReconnect()
				return false
			case drive.KindCancelled:
				return false
			}
			if ctx.Err() != nil {
				return false
			}
			r.e.Store.FailListing(r.db, folder, codeFor(err))
			continue
		}
		children := make([]state.Item, 0, len(files))
		for _, f := range files {
			children = append(children, toItem(f))
		}
		if _, err = r.e.Store.CommitDeltaPage(r.db, r.id, folder, children, next); err != nil {
			return false
		}
	}
	if pr, err := r.e.Store.JobProgress(r.db, r.id); err == nil && pr.ListingFailed == 0 {
		r.e.Store.FinishScan(r.db, r.id, r.collection)
	}
	return true
}
