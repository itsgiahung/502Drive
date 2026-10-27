package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/itsgiahung/502Drive/internal/drive"
	"github.com/itsgiahung/502Drive/internal/state"
)

var errStopped = errors.New("audit stopped")

// executeAudit scans the source and destination trees read-only and records
// every divergence as a finding. It never writes to Drive.
func (r *run) executeAudit(ctx context.Context, job state.Job) {
	audit, err := r.e.Store.AuditByJob(r.db, r.id)
	if err != nil {
		r.block("audit_missing", "The audit record for this job is missing.")
		return
	}
	col, err := r.e.Store.GetCollection(r.db, "", job.CollectionID)
	if err != nil {
		r.block("audit_missing", "The collection for this audit is missing.")
		return
	}
	r.setPhase("scanning")
	counts := map[string]int{}
	findings, stopped := r.scanAudit(ctx, col, counts)
	if stopped {
		r.settle(ctx)
		return
	}
	scanned := counts["__scanned"]
	if err := r.e.Store.RecordFindings(r.db, audit.ID, findings); err != nil {
		r.log("cannot record findings", "err", err)
		return
	}
	if err := r.e.Store.AuditScanProgress(r.db, audit.ID, scanned, findingCounts(counts)); err != nil {
		r.log("cannot record audit progress", "err", err)
	}
	final, code, msg := state.JobSucceeded, "", ""
	if counts[state.FindingUnreadable] > 0 {
		final, code, msg = state.JobPartial, "scan_incomplete", "Some source folders could not be read; the report covers the rest."
	}
	if _, err := r.e.Store.CompleteAudit(r.db, audit.ID, state.AuditCompleted, findingCounts(counts), nil); err != nil {
		r.log("cannot complete the audit", "err", err)
		return
	}
	r.finishAudit(final, code, msg)
}

// findingCounts drops the scan-progress meta key from the per-kind totals.
func findingCounts(counts map[string]int) map[string]int {
	out := make(map[string]int, len(counts))
	for k, v := range counts {
		if k != "__scanned" {
			out[k] = v
		}
	}
	return out
}

// finishAudit settles an audit job without consulting item counts (an audit
// owns no items, so finalize() would lie). Like every terminal write it goes
// through FinishJob, so the bookkeeping marker survives a crash before afterRun.
func (r *run) finishAudit(final, code, msg string) {
	_, err := r.e.Store.FinishJob(r.db, r.id, func(j *state.Job) (string, bool) {
		j.State, j.Phase, j.LastError = final, nil, nil
		if code != "" {
			j.LastError = &state.Failure{Code: code, Message: msg}
		}
		return "finished", true
	})
	if err == nil {
		r.log("audit finished", "state", final)
		r.afterRun()
	}
}

// scanAudit walks both trees and returns the findings. counts carries the
// per-kind totals plus "__scanned" (folders visited) for the progress row.
func (r *run) scanAudit(ctx context.Context, col state.Collection, counts map[string]int) ([]state.Finding, bool) {
	var findings []state.Finding
	add := func(kind, sourceID, destID string, repairable bool, detail string) {
		counts[kind]++
		findings = append(findings, state.Finding{Kind: kind, SourceID: sourceID, DestID: destID, Repairable: repairable, Detail: detail})
	}

	// Source walk. The root itself is fetched so the root mapping can be checked.
	srcFiles := map[string]drive.File{}
	scanned := 0
	if code, stopped := r.probe(ctx, col.SourceID, col.ResourceKey, srcFiles); stopped {
		return nil, true
	} else if code != "" {
		add(state.FindingUnreadable, col.SourceID, "", false, code)
	}
	if col.SourceKind != state.KindFile {
		n, bad, err := r.walkTree(ctx, col.SourceID, col.ResourceKey, srcFiles)
		scanned += n
		if err != nil {
			counts["__scanned"] = scanned
			return nil, true
		}
		for _, b := range bad {
			add(state.FindingUnreadable, b.id, "", false, b.code)
		}
	}
	counts["__scanned"] = scanned

	// Destination walk.
	destFiles := map[string]drive.File{}
	if col.RootID != nil {
		if code, stopped := r.probe(ctx, *col.RootID, "", destFiles); stopped {
			return nil, true
		} else if code != "" {
			// Without the destination tree only the source side is reported.
			r.log("destination root unreadable", "code", code)
		}
		if _, _, err := r.walkTree(ctx, *col.RootID, "", destFiles); err != nil {
			r.log("destination walk stopped", "err", err)
			return nil, true
		}
	}

	items, err := r.e.Store.Items(r.db, col.ID)
	if err != nil {
		r.log("cannot read the mapping", "err", err)
		return findings, false
	}
	mappedDest := map[string]bool{}
	byIntent := map[string]state.Item{}
	for _, it := range items {
		if it.State != state.ItemDone || it.DestID == "" {
			continue
		}
		mappedDest[it.DestID] = true
		if it.IntentKey != "" {
			byIntent[it.IntentKey] = it
		}
	}

	for _, it := range items {
		if it.Kind == state.KindWrapper || it.State != state.ItemDone || it.DestID == "" {
			continue
		}
		src, sok := srcFiles[it.SourceID]
		dst, dok := destFiles[it.DestID]
		switch {
		case !sok:
			add(state.FindingRemoved, it.SourceID, it.DestID, false, "the source object is gone")
			continue
		case !dok:
			add(state.FindingMissing, it.SourceID, "", true, "the mapped copy is no longer in the destination tree")
			continue
		}
		ours := it.IntentKey != "" && dst.AppProperties[drive.IntentProperty] == it.IntentKey
		if dst.Name != it.TargetName() {
			add(state.FindingName, it.SourceID, it.DestID, ours, fmt.Sprintf("destination is named %q", dst.Name))
		}
		if len(dst.Parents) > 0 && dst.Parents[0] != it.DestParentID {
			add(state.FindingParent, it.SourceID, it.DestID, ours, "the copy left its mapped folder")
		}
		if it.Kind == state.KindFile && src.Size != nil && dst.Size != nil &&
			(*src.Size != *dst.Size || (src.MD5 != "" && dst.MD5 != "" && src.MD5 != dst.MD5)) {
			add(state.FindingSize, it.SourceID, it.DestID, ours, "size or checksum differs from the source")
		}
	}
	// Extra copies carrying our intent, and objects nobody mapped.
	intents := map[string][]string{}
	for _, f := range destFiles {
		if in := f.AppProperties[drive.IntentProperty]; in != "" {
			intents[in] = append(intents[in], f.ID)
		}
	}
	for intent, ids := range intents {
		it, ok := byIntent[intent]
		if !ok {
			continue
		}
		for _, id := range ids {
			if id != it.DestID {
				add(state.FindingDuplicate, it.SourceID, id, true, "another live copy with the same intent")
			}
		}
	}
	for id, f := range destFiles {
		if mappedDest[id] || f.AppProperties[drive.IntentProperty] != "" {
			continue
		}
		add(state.FindingExtra, "", id, false, fmt.Sprintf("unmapped object %q", f.Name))
	}
	return findings, false
}

// probe fetches one object into the map. It returns "" on success, an error
// code for a readable failure, and stopped=true when the audit must stop
// (shutdown, pause/cancel, broken auth).
func (r *run) probe(ctx context.Context, id, key string, into map[string]drive.File) (code string, stopped bool) {
	f, err := r.p.GetFile(ctx, id, key)
	switch {
	case err == nil:
		into[f.ID] = f
		return "", false
	case drive.KindOf(err) == drive.KindAuth:
		r.setReconnect()
		return "", true
	case drive.KindOf(err) == drive.KindNotFound:
		return "", false // the comparison below reports it
	default:
		return codeFor(err), false
	}
}

// walkBad is one folder that could not be listed.
type walkBad struct {
	id   string
	code string
}

// walkTree lists every folder below root breadth-first, read-only. It reuses
// the copy engine's page reader (backoff, Retry-After, network gate). Folders
// that cannot be listed are reported and the walk continues; the caller turns
// them into unreadable_source findings.
func (r *run) walkTree(ctx context.Context, rootID, resourceKey string, into map[string]drive.File) (int, []walkBad, error) {
	queue := []state.Item{{SourceID: rootID, ResourceKey: resourceKey, ListState: "none"}}
	scanned := 0
	var bad []walkBad
	for len(queue) > 0 {
		if ctx.Err() != nil || r.control() != "run" {
			return scanned, bad, errStopped
		}
		folder := queue[0]
		queue = queue[1:]
		// Every page of a folder, not just the first: a short page would hide
		// children and report them missing or extra.
		ok := false
		for {
			files, next, err := r.listPage(ctx, folder)
			if err != nil {
				if drive.KindOf(err) == drive.KindAuth {
					r.setReconnect()
					return scanned, bad, errStopped
				}
				bad = append(bad, walkBad{folder.SourceID, codeFor(err)})
				break
			}
			ok = true
			for _, f := range files {
				into[f.ID] = f
				if f.IsFolder() {
					queue = append(queue, state.Item{SourceID: f.ID, ResourceKey: f.ResourceKey})
				}
			}
			if next == "" {
				break
			}
			folder.PageToken = next
		}
		if ok {
			scanned++
		}
	}
	return scanned, bad, nil
}

// repairAction is one verified step of a repair plan.
type repairAction struct {
	finding state.Finding
	what    string // recopy | align | trash-old | trash-duplicate
}

// executeRepair applies the repairable findings of a completed audit. It plans
// (re-checking every finding), then — unless dry-run — re-arms the mapping
// items through the normal copy path and trashes only our own superseded
// objects.
func (r *run) executeRepair(ctx context.Context, job state.Job) {
	audit, err := r.e.Store.GetAudit(r.db, "", job.AuditID)
	if err != nil {
		r.block("audit_missing", "The audit for this repair is missing.")
		return
	}
	col, err := r.e.Store.GetCollection(r.db, "", job.CollectionID)
	if err != nil {
		r.block("audit_missing", "The collection for this repair is missing.")
		return
	}
	// A previous repair may have died between a remote write and the mapping
	// commit. Resolve those items first: preparing a new generation would
	// forget the intent that can still find the copy it made.
	if !r.reconcileAll(ctx) {
		r.settleRepair(ctx)
		return
	}
	findings, err := r.e.Store.FindingsByAudit(r.db, audit.ID, true)
	if err != nil {
		r.log("cannot read findings", "err", err)
		return
	}
	items, err := r.e.Store.Items(r.db, col.ID)
	if err != nil {
		r.log("cannot read the mapping", "err", err)
		return
	}
	bySource := map[string]state.Item{}
	for _, it := range items {
		bySource[it.SourceID] = it
	}

	// Plan phase: drop stale findings, keep what still needs work. Every
	// finding is re-checked, including the intent marker: a copy that is gone,
	// trashed, unreadable or no longer ours is never written through.
	var plan []repairAction
	var alreadyFixed []state.Finding
	unverified := 0
	actions := []map[string]string{}
	for _, f := range findings {
		it, ok := bySource[f.SourceID]
		if f.Kind != state.FindingDuplicate && !ok {
			continue // the mapping row vanished; nothing to act on
		}
		if ok && unresolvedItem(it) {
			// A blocked or in-flight item needs a reconciliation or a human
			// decision first; re-arming it from a stale finding would forget
			// the intent of a write that may have happened.
			unverified++
			continue
		}
		switch f.Kind {
		case state.FindingMissing:
			if it.DestID != "" {
				cur, gerr := r.p.GetFile(ctx, it.DestID, "")
				if drive.KindOf(gerr) == drive.KindAuth {
					r.setReconnect()
					r.settleRepair(ctx)
					return
				}
				switch {
				case gerr == nil && !cur.Trashed && atMappedFolder(cur, it):
					alreadyFixed = append(alreadyFixed, f) // the object is back
					continue
				case gerr != nil && drive.KindOf(gerr) != drive.KindNotFound:
					// A read that failed proves nothing about the copy.
					unverified++
					continue
				}
			}
			plan = append(plan, repairAction{f, "recopy"})
		case state.FindingName, state.FindingParent:
			cur, gerr := r.p.GetFile(ctx, it.DestID, "")
			if drive.KindOf(gerr) == drive.KindAuth {
				r.setReconnect()
				r.settleRepair(ctx)
				return
			}
			if gerr != nil || cur.Trashed || !ownsGeneration(cur, it) {
				// Not provably alive and ours: never rename or move it.
				unverified++
				continue
			}
			fixed := (f.Kind == state.FindingName && cur.Name == it.TargetName()) ||
				(f.Kind == state.FindingParent && atMappedFolder(cur, it))
			if fixed {
				alreadyFixed = append(alreadyFixed, f)
				continue
			}
			plan = append(plan, repairAction{f, "align"})
		case state.FindingSize:
			cur, gerr := r.p.GetFile(ctx, it.DestID, "")
			src, serr := r.p.GetFile(ctx, it.SourceID, it.ResourceKey)
			for _, k := range []error{gerr, serr} {
				if drive.KindOf(k) == drive.KindAuth {
					r.setReconnect()
					r.settleRepair(ctx)
					return
				}
			}
			switch {
			case gerr != nil && drive.KindOf(gerr) != drive.KindNotFound,
				serr != nil:
				unverified++ // cannot compare without both objects
				continue
			case gerr == nil && !cur.Trashed && sizeMatches(cur, src):
				alreadyFixed = append(alreadyFixed, f)
				continue
			}
			plan = append(plan, repairAction{f, "recopy"})
		case state.FindingDuplicate:
			cur, gerr := r.p.GetFile(ctx, f.DestID, "")
			if drive.KindOf(gerr) == drive.KindAuth {
				r.setReconnect()
				r.settleRepair(ctx)
				return
			}
			switch {
			case drive.KindOf(gerr) == drive.KindNotFound, gerr == nil && cur.Trashed:
				alreadyFixed = append(alreadyFixed, f) // the extra copy is already gone
				continue
			case gerr != nil:
				unverified++
				continue
			}
			mapped, merr := r.p.GetFile(ctx, it.DestID, "")
			src, serr := r.p.GetFile(ctx, it.SourceID, it.ResourceKey)
			if merr != nil || serr != nil || mapped.Trashed || mapped.Name != it.Name || !sizeMatches(mapped, src) {
				unverified++
				continue // cannot prove the remaining copy is right: leave both
			}
			plan = append(plan, repairAction{f, "trash-duplicate"})
		}
	}
	for _, a := range plan {
		actions = append(actions, map[string]string{"kind": a.finding.Kind, "source_id": a.finding.SourceID, "dest_id": a.finding.DestID, "action": a.what})
	}
	pb, _ := json.Marshal(map[string]any{"dry_run": job.DryRun, "actions": actions})
	if err := r.e.Store.SaveRepairPlan(r.db, audit.ID, pb); err != nil {
		r.log("cannot save the repair plan", "err", err)
	}

	if job.DryRun {
		if err := r.e.Store.MarkFindingsRepaired(r.db, audit.ID, alreadyFixed); err != nil {
			r.log("cannot mark findings repaired", "err", err)
		}
		r.finishRepair(state.JobSucceeded, "", "")
		return
	}

	if err := r.applyRepair(ctx, col, plan); err != nil {
		r.log("repair apply stopped", "err", err)
		r.settleRepair(ctx)
		return
	}
	if ctx.Err() != nil {
		return // shutdown: durable state is consistent, the job resumes later
	}

	// Verify what actually happened and mark the findings honestly.
	var repaired []state.Finding
	itemsNow, err := r.e.Store.Items(r.db, col.ID)
	if err == nil {
		now := map[string]state.Item{}
		for _, it := range itemsNow {
			now[it.SourceID] = it
		}
		for _, a := range plan {
			switch a.what {
			case "recopy", "align":
				it, ok := now[a.finding.SourceID]
				if ok && it.State == state.ItemDone && it.DestID != "" {
					repaired = append(repaired, a.finding)
				}
			case "trash-duplicate", "trash-old":
				if f, err := r.p.GetFile(ctx, a.finding.DestID, ""); err != nil || f.Trashed {
					repaired = append(repaired, a.finding)
				}
			}
		}
	}
	if err := r.e.Store.MarkFindingsRepaired(r.db, audit.ID, append(alreadyFixed, repaired...)); err != nil {
		r.log("cannot mark findings repaired", "err", err)
	}

	pr, perr := r.e.Store.JobProgress(r.db, r.id)
	final, code, msg := state.JobSucceeded, "", ""
	switch {
	case perr == nil && pr.Blocked > 0:
		final, code, msg = state.JobBlocked, "unknown_remote_outcome", "Some writes may have happened but could not be confirmed; retry to re-check them."
	case perr == nil && pr.Failed > 0:
		final, code, msg = state.JobPartial, "items_failed", "Some repairs could not be applied; run the repair again."
	case unverified > 0:
		final, code, msg = state.JobPartial, "findings_unverified", "Some findings could not be verified against Drive; run the repair again."
	case r.cleanupFailures > 0:
		final, code, msg = state.JobPartial, "cleanup_failed", "Some superseded copies could not be trashed; run the repair again."
	}
	r.finishRepair(final, code, msg)
}

// applyRepair re-arms the mapping items and runs the normal copy workers, then
// trashes the superseded objects. Only objects tagged with our intent are ever
// touched.
func (r *run) applyRepair(ctx context.Context, col state.Collection, plan []repairAction) error {
	var ops []state.RepairOp
	var trash []repairAction
	for _, a := range plan {
		switch a.what {
		case "recopy":
			ops = append(ops, state.RepairOp{SourceID: a.finding.SourceID, Action: "recopy"})
		case "align":
			ops = append(ops, state.RepairOp{SourceID: a.finding.SourceID, Action: "align"})
		case "trash-old", "trash-duplicate":
			trash = append(trash, a)
		}
	}
	if len(ops) > 0 {
		if err := r.e.Store.PrepareRepair(r.db, r.id, col.ID, ops); err != nil {
			return err
		}
		r.setPhase("copying")
		r.copyAll(ctx)
		if ctx.Err() != nil || r.control() != "run" || r.isReconnect() {
			return errStopped
		}
	}
	// Cleanup: trash the duplicates named by the plan and the superseded
	// copies this run's re-copies retired. Every candidate must still carry
	// one of our intents; a failed trash is retried by the next repair run
	// (the leftover shows up as duplicate_at_destination) — nothing is ever
	// deleted.
	r.setPhase("cleanup")
	items, err := r.e.Store.Items(r.db, col.ID)
	if err != nil {
		r.cleanupFailures += len(trash)
		return err
	}
	intents := map[string]string{}
	for _, it := range items {
		if it.IntentKey != "" {
			intents[it.SourceID] = it.IntentKey
		}
	}
	for _, a := range trash {
		if ctx.Err() != nil || r.control() != "run" {
			return errStopped
		}
		r.trashOwned(ctx, a.finding.DestID, intents[a.finding.SourceID])
	}
	// Superseded versions are durable: a crashed repair finds them again on
	// the next run. They carry an older intent of ours, so any 502drive
	// intent is accepted here; the id itself comes from our own records.
	// Under versioned_copy the older generations are the user's history: only
	// the copies this repair's own re-copies replaced are trashed. replace_copy
	// trashes every superseded copy, as a sync run would.
	sups, err := r.e.Store.SupersededVersions(r.db, col.ID)
	if err != nil {
		r.cleanupFailures++
		return err
	}
	replace := false
	if sub, e := r.e.Store.GetSubscription(r.db, col.ID); e == nil && sub.ContentPolicy == state.PolicyReplace {
		replace = true
	}
	mine := map[string]bool{}
	r.mu.Lock()
	for _, id := range r.superseded {
		mine[id] = true
	}
	r.mu.Unlock()
	for _, v := range sups {
		if ctx.Err() != nil || r.control() != "run" {
			return errStopped
		}
		if !replace && !mine[v.DestID] {
			continue
		}
		before := r.cleanupFailures
		r.trashOwned(ctx, v.DestID, "")
		if r.cleanupFailures == before {
			if err := r.e.Store.MarkTrashed(r.db, v); err != nil {
				r.log("cannot mark the version trashed", "err", err)
			}
		}
	}
	return nil
}

// trashOwned moves one of our own objects to the trash. exactIntent pins the
// expected 502drive intent; empty accepts any intent of ours. A missing object
// is already as good as trashed; anything without our intent is left alone.
func (r *run) trashOwned(ctx context.Context, id, exactIntent string) {
	err := r.trashOurs(ctx, id, exactIntent)
	if err == nil || drive.KindOf(err) == drive.KindNotFound {
		return
	}
	if drive.KindOf(err) == drive.KindAuth {
		r.setReconnect()
	}
	r.cleanupFailures++
}

// unresolvedItem reports whether an item's last remote write may still have
// happened and has not been reconciled: executing, reconciling, or blocked
// (which includes multiple matches and an unknown outcome).
func unresolvedItem(it state.Item) bool {
	switch it.State {
	case state.ItemExecuting, state.ItemReconciling, state.ItemBlocked:
		return true
	}
	return false
}

// atMappedFolder reports whether the object still sits in the folder the
// mapping expects, i.e. it was not moved out of the copied tree.
func atMappedFolder(cur drive.File, it state.Item) bool {
	return len(cur.Parents) > 0 && cur.Parents[0] == it.DestParentID
}

// settleRepair stops a repair the way its control state asks. It never calls
// finalize(): item counts belong to the baseline, not to a repair.
func (r *run) settleRepair(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	switch r.control() {
	case "cancel":
		r.finishCancel()
	case "pause":
		r.finishPause()
	case "run":
		if r.isReconnect() {
			r.block("account_reconnect_required", "The Google account must be connected again.")
		}
	}
}

func (r *run) finishRepair(final, code, msg string) {
	_, err := r.e.Store.FinishJob(r.db, r.id, func(j *state.Job) (string, bool) {
		j.State, j.Phase, j.LastError = final, nil, nil
		if code != "" {
			j.LastError = &state.Failure{Code: code, Message: msg}
		}
		return "finished", true
	})
	if err == nil {
		r.log("repair finished", "state", final)
		r.afterRun()
	}
}

func sizeMatches(dst, src drive.File) bool {
	if src.Size == nil || dst.Size == nil {
		return false
	}
	if *src.Size != *dst.Size {
		return false
	}
	return src.MD5 == "" || dst.MD5 == "" || src.MD5 == dst.MD5
}
