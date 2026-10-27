package engine

import (
	"context"
	"fmt"

	"github.com/itsgiahung/502Drive/internal/drive"
	"github.com/itsgiahung/502Drive/internal/state"
)

// Content identity excludes names and parents: our own moves must not invalidate
// a restore point. An external content edit still makes the point unavailable.
func snapshotIdentity(f drive.File) string {
	if f.IsFolder() {
		return "folder"
	}
	if f.IsShortcut() {
		return "shortcut:" + f.ShortcutTargetID
	}
	if f.MD5 != "" {
		return "md5:" + f.MD5
	}
	if !f.Modified.IsZero() {
		return fmt.Sprintf("modified:%d", f.Modified.UnixMilli())
	}
	if f.Version != "" {
		return "version:" + f.Version
	}
	return ""
}

func (r *run) captureSnapshot(ctx context.Context) error {
	r.setPhase("snapshot")
	snap, err := r.e.Store.BeginSnapshot(r.db, r.id)
	if err != nil {
		return err
	}
	after := ""
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if r.control() != "run" {
			return context.Canceled
		}
		page, err := r.e.Store.SnapshotItems(r.db, snap.ID, 200, after)
		if err != nil {
			return err
		}
		for _, v := range page {
			if v.Identity != "" {
				continue
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if r.control() != "run" {
				return context.Canceled
			}
			if err = r.e.acquire(ctx); err != nil {
				return err
			}
			f, e := r.p.GetFile(ctx, v.DestID, "")
			r.e.release()
			if e != nil {
				return e
			}
			if f.Trashed || !ownsGeneration(f, v.Item) {
				return errNotOurs
			}
			if f.Name != v.Name || len(f.Parents) != 1 || f.Parents[0] != v.ParentID {
				return fmt.Errorf("snapshot destination mapping changed")
			}
			identity := snapshotIdentity(f)
			if identity == "" {
				return fmt.Errorf("snapshot content identity unavailable")
			}
			v.Item.IntentKey = f.AppProperties[drive.IntentProperty]
			if err = r.e.Store.RecordSnapshotItem(r.db, snap.ID, v.Item, identity); err != nil {
				return err
			}
		}
		if len(page) < 200 {
			break
		}
		after = page[len(page)-1].SourceID
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if r.control() != "run" {
		return context.Canceled
	}
	return nil
}

func (r *run) executeSnapshot(ctx context.Context, job state.Job) {
	if err := r.captureSnapshot(ctx); err != nil {
		r.stopProtection(ctx, "snapshot_unavailable", err)
		return
	}
	r.finishProtection(state.JobSucceeded, "", "")
	if col, err := r.e.Store.GetCollection(r.db, "", job.CollectionID); err == nil {
		r.check(r.e.Store.PruneSnapshots(r.db, col.ID, col.Protection))
	}
}

func (r *run) finishProtection(status, code, message string) {
	_, err := r.e.Store.FinishJob(r.db, r.id, func(j *state.Job) (string, bool) {
		j.State, j.Phase, j.LastError = status, nil, nil
		if code != "" {
			j.LastError = &state.Failure{Code: code, Message: message}
		}
		return "finished", true
	})
	if err == nil {
		r.afterRun()
	} else {
		r.log("protection finalization failed", "err", err)
	}
}

func (r *run) stopProtection(ctx context.Context, code string, err error) {
	if ctx.Err() != nil {
		return
	}
	switch r.control() {
	case "pause":
		r.finishPause()
		return
	case "cancel":
		r.finishCancel()
		return
	}
	if drive.KindOf(err) == drive.KindQuota {
		r.blockQuota(nil, err)
		return
	}
	if drive.KindOf(err) == drive.KindAuth {
		r.setReconnect()
		code = "account_reconnect_required"
	}
	r.block(code, "Protection stopped safely; no further destination changes were made: "+codeFor(err))
}

func (r *run) mirror(ctx context.Context) error {
	if err := r.e.Store.FreezeMirrorActions(r.db, r.id, r.collection); err != nil {
		return err
	}
	return r.applyDestinationActions(ctx, false)
}

// archiveFolder has its own durable create intent. A lost create response is
// looked up on every restart and never followed by another blind create.
func (r *run) archiveFolder(ctx context.Context) (string, error) {
	col, err := r.e.Store.GetCollection(r.db, "", r.collection)
	if err != nil {
		return "", err
	}
	if col.RootID == nil {
		return "", state.ErrNeedsAction
	}
	root, err := r.p.GetFile(ctx, *col.RootID, "")
	if err != nil {
		return "", err
	}
	if len(root.Parents) != 1 {
		return "", state.ErrNeedsAction
	}
	a, err := r.e.Store.Archive(r.db, col.ID, root.Parents[0])
	if err != nil {
		return "", err
	}
	if a.DestID != "" {
		f, e := r.p.GetFile(ctx, a.DestID, "")
		if e != nil {
			return "", e
		}
		if f.Trashed || !stillOurs(f, a.Intent) {
			return "", errNotOurs
		}
		return f.ID, nil
	}
	found, err := r.p.FindByIntent(ctx, a.ParentID, a.Intent)
	if err != nil {
		return "", err
	}
	if len(found) > 1 {
		return "", state.ErrNeedsAction
	}
	if len(found) == 1 {
		if err = r.e.Store.RecordArchive(r.db, col.ID, found[0].ID); err != nil {
			return "", err
		}
		return found[0].ID, nil
	}
	if a.Attempted {
		return "", fmt.Errorf("archive create outcome is unknown")
	}
	if err = r.e.Store.RecordArchive(r.db, col.ID, ""); err != nil {
		return "", err
	}
	f, err := r.p.CreateFolder(ctx, ".502drive-versions-"+col.ID, a.ParentID, a.Intent)
	if err != nil {
		return "", err
	}
	if err = r.e.Store.RecordArchive(r.db, col.ID, f.ID); err != nil {
		return "", err
	}
	return f.ID, nil
}

// removeProtected acts only on an exactly tagged object. Folder emptiness is
// checked after child actions, so user additions cannot be carried into trash.
func (r *run) removeProtected(ctx context.Context, it state.Item) error {
	f, err := r.p.GetFile(ctx, it.DestID, "")
	if drive.KindOf(err) == drive.KindNotFound {
		return nil
	}
	if err != nil {
		return err
	}
	if f.Trashed {
		return nil
	}
	if !ownsGeneration(f, it) {
		return errNotOurs
	}
	if f.IsFolder() {
		kids, next, err := r.p.ListChildren(ctx, f.ID, "", "")
		if err != nil {
			return err
		}
		if len(kids) > 0 || next != "" {
			return fmt.Errorf("destination folder contains retained or foreign children")
		}
	}
	keep, err := r.e.Store.SnapshotReferences(r.db, f.ID)
	if err != nil {
		return err
	}
	if keep {
		parent, err := r.archiveFolder(ctx)
		if err != nil {
			return err
		}
		if len(f.Parents) != 1 {
			return state.ErrNeedsAction
		}
		if f.Parents[0] == parent {
			return nil
		}
		return r.p.MoveFile(ctx, f.ID, f.Parents[0], parent)
	}
	return r.p.TrashFile(ctx, f.ID)
}

func (r *run) applyDestinationActions(ctx context.Context, restore bool) error {
	after := 0
	for {
		actions, err := r.e.Store.DestinationActionPage(r.db, r.id, after, 200)
		if err != nil {
			return err
		}
		for _, a := range actions {
			if a.State == "done" {
				continue
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if r.control() != "run" {
				return context.Canceled
			}
			if !r.waitNetwork(ctx) {
				return ctx.Err()
			}
			if err = r.e.Store.SetDestinationAction(r.db, r.id, a.Ordinal, "executing"); err != nil {
				return err
			}
			for {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				if r.control() != "run" {
					return context.Canceled
				}
				if err = r.e.acquire(ctx); err != nil {
					return err
				}
				if a.Action == "restore" {
					err = r.restoreDestination(ctx, a.Target)
				} else {
					err = r.removeProtected(ctx, a.Target.Item)
				}
				r.e.release()
				if drive.KindOf(err) == drive.KindThrottled {
					if !r.e.sleep(ctx, max(r.e.throttleWait(), drive.RetryAfterOf(err))) {
						return ctx.Err()
					}
					continue
				}
				if drive.KindOf(err) == drive.KindUnreachable {
					r.noteUnreachable(err)
					if !r.waitNetwork(ctx) {
						return ctx.Err()
					}
					if !r.e.sleep(ctx, r.e.BackoffBase) {
						return ctx.Err()
					}
					continue
				}
				break
			}
			if err != nil {
				return err
			}
			if err = r.e.Store.CompleteDestinationAction(r.db, r.id, a, !restore); err != nil {
				return err
			}
		}
		if len(actions) < 200 {
			return nil
		}
		after = actions[len(actions)-1].Ordinal + 1
	}
}

func (r *run) restoreDestination(ctx context.Context, v state.SnapshotItem) error {
	cur, err := r.p.GetFile(ctx, v.DestID, "")
	if err != nil {
		return err
	}
	if !stillOurs(cur, v.Item.IntentKey) {
		return errNotOurs
	}
	if snapshotIdentity(cur) != v.Identity {
		return fmt.Errorf("snapshot content changed")
	}
	if cur.Trashed {
		if err = r.p.RestoreFile(ctx, cur.ID); err != nil {
			return err
		}
	}
	if cur.Name != v.Name {
		if err = r.p.RenameFile(ctx, cur.ID, v.Name); err != nil {
			return err
		}
	}
	if len(cur.Parents) != 1 {
		return state.ErrNeedsAction
	}
	if cur.Parents[0] != v.ParentID {
		return r.p.MoveFile(ctx, cur.ID, cur.Parents[0], v.ParentID)
	}
	return nil
}

// Validate every target before the first write. A restore is refused in full
// when any saved physical version is missing, edited, or no longer ours.
func (r *run) validateRestore(ctx context.Context, snapshot string) error {
	after := ""
	count := 0
	for {
		page, err := r.e.Store.SnapshotItems(r.db, snapshot, 200, after)
		if err != nil {
			return err
		}
		for _, v := range page {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if r.control() != "run" {
				return context.Canceled
			}
			if err = r.e.acquire(ctx); err != nil {
				return err
			}
			cur, e := r.p.GetFile(ctx, v.DestID, "")
			r.e.release()
			if e != nil {
				return e
			}
			if !stillOurs(cur, v.Item.IntentKey) {
				return errNotOurs
			}
			if v.Identity == "" || snapshotIdentity(cur) != v.Identity {
				return fmt.Errorf("snapshot content changed or unavailable")
			}
			count++
		}
		if len(page) < 200 {
			break
		}
		after = page[len(page)-1].SourceID
	}
	if count == 0 {
		return state.ErrNeedsAction
	}
	return nil
}

func (r *run) executeRestore(ctx context.Context, job state.Job) {
	r.setPhase("planning")
	err := r.validateRestore(ctx, job.SnapshotID)
	if err == nil {
		err = r.e.Store.FreezeRestoreActions(r.db, r.id, r.collection, job.SnapshotID)
	}
	if err != nil {
		r.stopProtection(ctx, "snapshot_unavailable", err)
		return
	}
	if job.DryRun {
		if ctx.Err() != nil || r.control() != "run" {
			r.stopProtection(ctx, "restore_incomplete", context.Canceled)
			return
		}
		r.finishProtection(state.JobSucceeded, "", "")
		return
	}
	r.setPhase("restoring")
	if err = r.applyDestinationActions(ctx, true); err != nil {
		r.stopProtection(ctx, "restore_incomplete", err)
		return
	}
	if err = r.e.Store.ApplyRestoredMapping(r.db, r.collection, r.id, job.SnapshotID); err != nil {
		r.stopProtection(ctx, "restore_incomplete", err)
		return
	}
	if ctx.Err() != nil || r.control() != "run" {
		r.stopProtection(ctx, "restore_incomplete", context.Canceled)
		return
	}
	r.finishProtection(state.JobSucceeded, "", "")
}

func (r *run) protectSuccessfulRun(ctx context.Context, job state.Job) bool {
	if _, err := r.e.Store.SkipOrphans(r.db, r.id); err != nil {
		r.stopProtection(ctx, "snapshot_unavailable", err)
		return false
	}
	pr, err := r.e.Store.JobProgress(r.db, r.id)
	if err != nil {
		return false
	}
	if !pr.RootDone || pr.Failed > 0 || pr.Blocked > 0 || pr.Pending > 0 || pr.Active > 0 || pr.Reconciling > 0 || pr.ListingLeft > 0 || pr.ListingFailed > 0 {
		return true
	}
	if job.Protection.Mirror && job.Trigger != "initial" {
		r.setPhase("mirror")
		if err = r.mirror(ctx); err != nil {
			r.stopProtection(ctx, "mirror_incomplete", err)
			return false
		}
	}
	if job.Protection.Snapshots {
		if err = r.captureSnapshot(ctx); err != nil {
			r.stopProtection(ctx, "snapshot_unavailable", err)
			return false
		}
	}
	return true
}
