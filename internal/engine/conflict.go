package engine

import (
	"context"
	"path"
	"strconv"
	"strings"

	"github.com/itsgiahung/502Drive/internal/state"
)

// maxNameTries bounds the numbered candidates tried for one item.
const maxNameTries = 200

// conflictPolicy is the job's on_conflict; jobs admitted before N1 have none
// and keep the default, rename.
func (r *run) conflictPolicy() string {
	switch r.policy {
	case state.ConflictSkip, state.ConflictUpdate:
		return r.policy
	}
	return state.ConflictRename
}

// conflictName returns the n-th candidate for a taken name: "archive.tar.gz"
// becomes "archive (2).tar.gz", "document.pdf" becomes "document (2).pdf", and
// a name without an extension gets the suffix at the end.
func conflictName(name string, n int) string {
	stem, ext := name, ""
	for _, compound := range []string{".tar.gz", ".tar.bz2", ".tar.xz", ".tar.zst"} {
		if len(name) > len(compound) && strings.EqualFold(name[len(name)-len(compound):], compound) {
			stem, ext = name[:len(name)-len(compound)], name[len(name)-len(compound):]
			break
		}
	}
	if ext == "" {
		if e := path.Ext(name); e != "" && e != name {
			stem, ext = name[:len(name)-len(e)], e
		}
	}
	return stem + " (" + strconv.Itoa(n) + ")" + ext
}

// freeOnDrive asks Drive about one name. Only a top-level item (the copy root,
// or the wrapper of a single file) needs it: it sits directly in the user's
// destination folder, which may already hold objects this program did not
// create. Everything deeper lands in a folder 502Drive created, so the name
// reservations in the state DB are the whole picture there.
func (r *run) freeOnDrive(ctx context.Context, it state.Item, name string) (bool, error) {
	if it.ParentSourceID != "" {
		return true, nil
	}
	kids, err := r.p.FindChildrenByName(ctx, it.DestParentID, name)
	return len(kids) == 0, err
}

// settleName chooses the destination name of an item before its first write
// and stores the decision. It returns ok=false when the item was settled here
// (skipped by policy, or released for a retry). Once a name is stored the
// decision is never recomputed, so a restart reuses it.
func (r *run) settleName(ctx context.Context, it state.Item) (state.Item, bool) {
	if it.DestName != "" {
		return it, true
	}
	policy := r.conflictPolicy()
	if it.Kind == state.KindFolder || it.Kind == state.KindWrapper {
		// Skipping a folder would silently drop its whole subtree.
		policy = state.ConflictRename
	}

	free, err := r.freeOnDrive(ctx, it, it.Name)
	if err != nil {
		r.releaseTransient(it, err)
		return it, false
	}
	if free {
		ok, err := r.e.Store.ClaimDestName(r.db, it, state.DecisionNone, it.Name, false)
		if err != nil {
			r.releaseTransient(it, err)
			return it, false
		}
		if ok {
			it.DestName = it.Name
			return it, true
		}
	}

	// The name is taken. Same bytes already there are a duplicate, not a
	// conflict: skip without writing, whatever the policy.
	if it.Kind == state.KindFile {
		holder, found, err := r.e.Store.NameHolder(r.db, it, it.Name)
		if err != nil {
			r.releaseTransient(it, err)
			return it, false
		}
		if found && holderWillLand(holder) && state.SameContent(holder, it) {
			r.skipWithDecision(it, state.SkipConflict, state.DecisionSkip)
			return it, false
		}
	}

	switch policy {
	case state.ConflictSkip:
		r.skipWithDecision(it, state.SkipConflict, state.DecisionSkip)
		return it, false
	case state.ConflictUpdate:
		// The same name on purpose: nothing existing is removed or overwritten.
		if _, err := r.e.Store.ClaimDestName(r.db, it, state.DecisionUpdate, it.Name, true); err != nil {
			r.releaseTransient(it, err)
			return it, false
		}
		it.DestName, it.ConflictAction = it.Name, state.DecisionUpdate
		return it, true
	}

	for n := 2; n < 2+maxNameTries; n++ {
		candidate := conflictName(it.Name, n)
		free, err := r.freeOnDrive(ctx, it, candidate)
		if err != nil {
			r.releaseTransient(it, err)
			return it, false
		}
		if !free {
			continue
		}
		ok, err := r.e.Store.ClaimDestName(r.db, it, state.DecisionRename, candidate, false)
		if err != nil {
			r.releaseTransient(it, err)
			return it, false
		}
		if ok {
			it.DestName, it.ConflictAction = candidate, state.DecisionRename
			return it, true
		}
	}
	r.check(r.e.Store.ReleaseItem(r.db, it, "name_exhausted", r.e.Now().Add(r.e.backoff(it.Attempts+1)).UnixMilli()))
	return it, false
}

// holderWillLand reports whether an item holding the name is, or is about to
// be, the object at the destination. A failed or blocked holder may never be
// written, so it must not stand in for an identical file.
func holderWillLand(h state.Item) bool {
	switch h.State {
	case state.ItemDone, state.ItemPending, state.ItemExecuting, state.ItemReconciling:
		return true
	}
	return false
}

// skipWithDecision records a policy skip. A failed write is logged and the
// item is released so the decision is taken again after the next claim.
func (r *run) skipWithDecision(it state.Item, code, decision string) {
	if err := r.e.Store.SkipWithDecision(r.db, it, code, decision); err != nil {
		r.log("skip failed", "err", err)
		r.check(r.e.Store.ReleaseItem(r.db, it, "skip_write_failed", r.e.Now().Add(r.e.backoff(it.Attempts+1)).UnixMilli()))
	}
}
