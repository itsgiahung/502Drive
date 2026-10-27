package engine

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/itsgiahung/502Drive/internal/drive"
	"github.com/itsgiahung/502Drive/internal/state"
)

const maxTransientAttempts = 6

// The mass-removal guard: a delta scan that no longer finds more than 30% of
// the source (with at least 10 mapped items, so tiny trees do not trip it)
// stops before copying and asks for an integrity audit instead of syncing a
// suspect tree silently.
const (
	massRemovalPercent = 30
	massRemovalFloor   = 10
)

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
	// quota is set when an account-wide quota error blocked the job; workers
	// stop instead of failing every remaining item.
	quota bool
	// massRemoval is set when the scan found that most of the source is gone.
	massRemoval bool
	massRemoved int
	massTotal   int
	// cleanupFailures counts superseded copies a repair could not trash.
	cleanupFailures int
	// superseded collects destination ids retired by this run's re-copies
	// (CompleteVersionedItem returns them); a repair trashes them in its cleanup.
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
	case state.JobKindCleanup:
		r.executeCleanup(ctx, job)
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
	if r.massRemoval {
		// Never copy a tree that the scan says mostly disappeared.
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
	if r.isQuota() {
		return // the job is already blocked with resume_after
	}
	if r.isReconnect() {
		r.block("account_reconnect_required", "The Google account must be connected again.")
		return
	}
	if r.massRemoval {
		r.finishMassRemoval()
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

// finishMassRemoval records that the scan lost most of the source and stops
// before copying. The removed items are only flagged (never deleted); the
// partial run carries source_removed so the sync bookkeeping queues a
// read-only integrity audit instead of syncing a suspect tree silently.
func (r *run) finishMassRemoval() {
	msg := fmt.Sprintf("The source scan no longer found %d of %d mapped items. The sync stopped before copying and an integrity audit was queued.", r.massRemoved, r.massTotal)
	j, err := r.e.Store.UpdateJob(r.db, r.id, func(j *state.Job) (string, bool) {
		j.State, j.Phase, j.LastError = state.JobPartial, nil, &state.Failure{Code: "source_mass_removal", Message: msg}
		j.SourceRemoved = r.massRemoved
		return "finished", true
	})
	if err == nil {
		r.log("stopped: most of the source disappeared", "removed", r.massRemoved, "total", r.massTotal, "state", j.State)
		r.afterRun()
	}
}

// ---- discovery ----

// pageCommit stores one folder's freshly read children and its next page
// checkpoint. Initial discovery and delta scanning differ only here.
type pageCommit func(folder state.Item, children []state.Item, next string) error

// maxBatchFiles bounds one combined listing. A bigger batch falls back to
// per-folder pages instead of holding an unbounded slice in memory.
const maxBatchFiles = 20000

// discover lists every folder page by page. It returns false when the job
// should stop (control request, shutdown, fatal provider state).
func (r *run) discover(ctx context.Context) bool {
	return r.scanTree(ctx, func(folder state.Item, children []state.Item, next string) error {
		return r.e.Store.CommitPage(r.db, folder, children, next)
	})
}

// scanTree walks the folders that still need listing. Interrupted pages are
// resumed one by one (their token belongs to one query); fresh folders are
// combined into as few files.list calls as the provider allows.
func (r *run) scanTree(ctx context.Context, commit pageCommit) bool {
	bl, batch := r.p.(drive.BatchLister)
	for {
		if ctx.Err() != nil || r.control() != "run" {
			return false
		}
		folder, err := r.e.Store.NextToListPartial(r.db, r.id)
		if err == nil {
			if !r.scanOne(ctx, folder, commit) {
				return false
			}
			continue
		}
		if !errors.Is(err, state.ErrNotFound) {
			r.log("list lookup failed", "err", err)
			return false
		}
		if !batch {
			folder, err = r.e.Store.NextToList(r.db, r.id)
			if errors.Is(err, state.ErrNotFound) {
				return true
			}
			if err != nil {
				r.log("list lookup failed", "err", err)
				return false
			}
			if !r.scanOne(ctx, folder, commit) {
				return false
			}
			continue
		}
		folders, err := r.e.Store.NextToListBatch(r.db, r.id, drive.MaxBatchParents)
		if err != nil {
			r.log("list lookup failed", "err", err)
			return false
		}
		if len(folders) == 0 {
			return true
		}
		if len(folders) == 1 {
			if !r.scanOne(ctx, folders[0], commit) {
				return false
			}
			continue
		}
		ok, handled := r.scanBatch(ctx, bl, folders, commit)
		if !ok {
			return false
		}
		if handled {
			continue
		}
		// The combined query failed or was too large: isolate folders one by
		// one, so a permission error on one does not fail the whole batch.
		for _, folder := range folders {
			if !r.scanOne(ctx, folder, commit) {
				return false
			}
		}
	}
}

// scanOne reads and commits one folder, following its page checkpoint.
func (r *run) scanOne(ctx context.Context, folder state.Item, commit pageCommit) bool {
	files, next, err := r.listPage(ctx, folder)
	if err != nil {
		switch drive.KindOf(err) {
		case drive.KindAuth:
			r.setReconnect()
			return false
		case drive.KindCancelled:
			return false
		case drive.KindQuota:
			r.blockQuota(nil, err)
			return false
		}
		if ctx.Err() != nil {
			return false
		}
		r.e.Store.FailListing(r.db, folder, codeFor(err))
		return true
	}
	children := make([]state.Item, 0, len(files))
	for _, f := range files {
		children = append(children, toItem(f))
	}
	if err = commit(folder, children, next); err != nil {
		r.log("commit page failed", "err", err)
		return false
	}
	return true
}

// scanBatch drains one combined query and commits each folder in order. It
// returns handled=false when the caller must fall back to per-folder listing.
func (r *run) scanBatch(ctx context.Context, bl drive.BatchLister, folders []state.Item, commit pageCommit) (ok, handled bool) {
	parents := make([]drive.Parent, len(folders))
	wanted := make(map[string]bool, len(folders))
	for i, f := range folders {
		parents[i] = drive.Parent{ID: f.SourceID, ResourceKey: f.ResourceKey}
		wanted[f.SourceID] = true
	}
	grouped := make(map[string][]state.Item, len(folders))
	total, token := 0, ""
	for {
		files, next, err := r.listBatch(ctx, bl, parents, token)
		if err != nil {
			switch drive.KindOf(err) {
			case drive.KindAuth:
				r.setReconnect()
				return false, true
			case drive.KindCancelled:
				return false, true
			case drive.KindQuota:
				r.blockQuota(nil, err)
				return false, true
			}
			if ctx.Err() != nil {
				return false, true
			}
			r.log("batched list failed; falling back", "err", err, "folders", len(folders))
			return true, false
		}
		for _, f := range files {
			for _, parent := range f.Parents {
				if wanted[parent] {
					grouped[parent] = append(grouped[parent], toItem(f))
					total++
					break
				}
			}
		}
		if total > maxBatchFiles {
			r.log("batched list too large; falling back", "files", total)
			return true, false
		}
		if next == "" {
			break
		}
		token = next
	}
	for _, folder := range folders {
		if err := commit(folder, grouped[folder.SourceID], ""); err != nil {
			r.log("commit page failed", "err", err)
			return false, true
		}
	}
	return true, true
}

// listBatch reads one page of a combined query with the same transient-retry
// rules as listPage: a throttle never spends an attempt.
func (r *run) listBatch(ctx context.Context, bl drive.BatchLister, parents []drive.Parent, token string) ([]drive.File, string, error) {
	var lastErr error
	for attempt := 0; attempt < maxTransientAttempts; attempt++ {
		if ctx.Err() != nil {
			return nil, "", ctx.Err()
		}
		if r.control() != "run" {
			// Stop, but never report an empty successful page: the caller
			// would commit it as "folder fully listed".
			return nil, "", context.Canceled
		}
		if !r.waitNetwork(ctx) {
			return nil, "", ctx.Err()
		}
		if err := r.e.acquire(ctx); err != nil {
			return nil, "", err
		}
		files, next, err := bl.ListChildrenBatch(ctx, parents, token)
		r.e.release()
		if err == nil {
			r.e.throttleClear()
			return files, next, nil
		}
		lastErr = err
		switch drive.KindOf(err) {
		case drive.KindUnreachable:
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
		case drive.KindThrottled:
			if !r.e.sleep(ctx, max(r.e.throttleWait(), drive.RetryAfterOf(err))) {
				return nil, "", ctx.Err()
			}
			attempt--
		case drive.KindUnavailable, drive.KindServer:
			if !r.e.sleep(ctx, max(r.e.backoff(attempt), drive.RetryAfterOf(err))) {
				return nil, "", ctx.Err()
			}
		default:
			return nil, "", err
		}
	}
	return nil, "", lastErr
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
		if ctx.Err() != nil {
			return nil, "", ctx.Err()
		}
		if r.control() != "run" {
			// Stop, but never report an empty successful page: the caller
			// would commit it as "folder fully listed".
			return nil, "", context.Canceled
		}
		if !r.waitNetwork(ctx) {
			return nil, "", ctx.Err()
		}
		if err := r.e.acquire(ctx); err != nil {
			return nil, "", err
		}
		files, next, err := r.p.ListChildren(ctx, folder.SourceID, folder.ResourceKey, folder.PageToken)
		r.e.release()
		if err == nil {
			r.e.throttleClear()
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
		case drive.KindThrottled:
			// Not a failed page read: keep the attempt and grow the account-wide
			// throttle backoff instead.
			d := max(r.e.throttleWait(), drive.RetryAfterOf(err))
			if !r.e.sleep(ctx, d) {
				return nil, "", ctx.Err()
			}
			attempt--
		case drive.KindUnavailable, drive.KindServer:
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
	// Spawn the AIMD ceiling; the shared throttle decides how many of these
	// workers may hold a provider call at once (the account-wide limit).
	for range r.e.MaxWorkers {
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
		if ctx.Err() != nil || r.control() != "run" || r.isReconnect() || r.isQuota() {
			return
		}
		// Offline: stop claiming work. In-flight writes are never cancelled:
		// they either finish or reconcile.
		if !r.waitNetwork(ctx) {
			return
		}
		// The permit covers claim → provider call → result, so no more items
		// are marked executing than the account-wide limit allows.
		if err := r.e.acquire(ctx); err != nil {
			return
		}
		// Re-check the gate after waiting: a worker queued behind a failing
		// call must not send the next write into a network that just dropped.
		if !r.waitNetwork(ctx) {
			r.e.release()
			return
		}
		it, err := r.e.Store.ClaimItem(r.db, r.id, r.e.Now().UnixMilli())
		if errors.Is(err, state.ErrNotFound) {
			r.e.release()
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
			r.e.release()
			r.log("claim item failed", "err", err)
			return
		}
		r.process(ctx, it)
		r.e.release()
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
	if err == nil {
		r.e.throttleClear()
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
		case drive.KindThrottled:
			// A throttle is not this item's failure: give the attempt back and
			// wait with an account-wide, jittered backoff (plus Retry-After).
			d := max(r.e.throttleWait(), drive.RetryAfterOf(err))
			r.e.Store.ReleaseItemThrottled(r.db, it, codeFor(err), r.e.Now().Add(d).UnixMilli())
		case drive.KindQuota:
			// Account-wide: stop the run, give the attempt back, and let the
			// engine continue after the documented reset.
			item := it
			r.blockQuota(&item, err)
		default:
			r.e.Store.ReleaseItem(r.db, it, codeFor(err), 0)
		}
	default:
		// The write may have happened. Never resend before checking.
		r.e.Store.SetItemState(r.db, it, state.ItemReconciling, "")
		if ctx.Err() == nil {
			r.reconcile(ctx, it, true)
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
		r.reconcile(ctx, it, false)
	}
	return !r.isReconnect()
}

const reconcileTries = 3

// reconcile looks for our own tagged output. One match is adopted; several or
// none-after-settling block the item for a human decision — except when the
// user already confirmed a resend by retrying.
func (r *run) reconcile(ctx context.Context, it state.Item, held bool) {
	if !held {
		if err := r.e.acquire(ctx); err != nil {
			return
		}
		defer r.e.release()
	}
	for try := 0; try < reconcileTries; try++ {
		if ctx.Err() != nil || r.control() != "run" {
			return
		}
		found, err := r.p.FindByIntent(ctx, it.DestParentID, it.IntentKey)
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
			case drive.KindThrottled:
				// Not a failed reconciliation attempt.
				try--
			}
			if !r.e.sleep(ctx, max(r.e.throttleWait(), drive.RetryAfterOf(err))) {
				return
			}
			continue
		}
		r.e.throttleClear()
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
	if it.Kind == state.KindWrapper || it.Kind == state.KindFolder {
		if e := r.e.Store.CompleteItem(r.db, it, destID, RootURL); e != nil {
			r.log("complete failed", "err", e)
		}
		return
	}
	old, e := r.e.Store.CompleteVersionedItem(r.db, it, destID)
	if e != nil {
		r.log("complete failed", "err", e)
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
		k := drive.KindOf(err)
		if k == drive.KindAuth {
			r.setReconnect()
		}
		switch k {
		case drive.KindQuota:
			item := it
			r.blockQuota(&item, err)
			return
		case drive.KindThrottled:
			// A throttle is not this item's failure: no attempt is spent.
			r.e.Store.ReleaseItemThrottled(r.db, it, codeFor(err), r.e.Now().Add(max(r.e.throttleWait(), drive.RetryAfterOf(err))).UnixMilli())
			return
		case drive.KindUnreachable:
			// Neither is an offline machine; the worker gate handles the wait.
			r.e.Store.ReleaseItemNoPenalty(r.db, it, "waiting_for_network")
			return
		}
		if drive.NotExecuted(err) {
			r.e.Store.ReleaseItem(r.db, it, codeFor(err), 0)
			return
		}
		if it.Attempts >= maxTransientAttempts {
			r.e.Store.ReleaseItem(r.db, it, "retry_exhausted", 0)
			return
		}
		r.e.Store.ReleaseItem(r.db, it, codeFor(err), r.e.Now().Add(max(r.e.backoff(it.Attempts), drive.RetryAfterOf(err))).UnixMilli())
	}
	cur, err := r.p.GetFile(ctx, it.DestID, "")
	if err == nil && cur.Name != it.Name {
		err = r.p.RenameFile(ctx, it.DestID, it.Name)
	}
	if err == nil && len(cur.Parents) > 0 && cur.Parents[0] != it.DestParentID {
		err = r.p.MoveFile(ctx, it.DestID, cur.Parents[0], it.DestParentID)
	}
	if err != nil {
		retry(err)
		return
	}
	r.e.throttleClear()
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
		if err := r.e.acquire(ctx); err != nil {
			return false
		}
		f, err := r.p.GetFile(ctx, coll.SourceID, coll.ResourceKey)
		r.e.release()
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
	if !r.scanTree(ctx, func(folder state.Item, children []state.Item, next string) error {
		_, err := r.e.Store.CommitDeltaPage(r.db, r.id, folder, children, next)
		return err
	}) {
		return false
	}
	if pr, err := r.e.Store.JobProgress(r.db, r.id); err == nil && pr.ListingFailed == 0 {
		total, _, terr := r.e.Store.ItemTotals(r.db, r.collection)
		if terr != nil {
			return false
		}
		removed, err := r.e.Store.FinishScan(r.db, r.id, r.collection)
		if err != nil {
			return false
		}
		if total >= massRemovalFloor && removed*100 > total*massRemovalPercent {
			r.massRemoval, r.massRemoved, r.massTotal = true, removed, total
		}
	}
	return true
}
