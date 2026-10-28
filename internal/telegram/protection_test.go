package telegram

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/state"
)

// rootCopy completes the root item of the baseline copy, which is what gives a
// collection a destination root (the engine normally does this).
func (h *harness) rootCopy(collectionID string) {
	h.t.Helper()
	ctx := context.Background()
	for _, j := range h.jobs() {
		if j.CollectionID != collectionID || j.Kind != "" {
			continue
		}
		it, err := h.store.ClaimItem(ctx, j.ID, h.clock().UnixMilli())
		if err != nil {
			h.t.Fatal(err)
		}
		link := func(id string) string { return "https://drive.google.com/drive/folders/" + id }
		if err = h.store.CompleteItem(ctx, it, "dest-root-"+j.ID, link); err != nil {
			h.t.Fatal(err)
		}
		return
	}
	h.t.Fatal("no baseline job")
}

// readySnapshot captures a restore point the way the engine leaves one: the
// mapping is frozen by BeginSnapshot and the job finishing makes it ready.
func (h *harness) readySnapshot(collectionID string) state.Snapshot {
	h.t.Helper()
	ctx := context.Background()
	h.rootCopy(collectionID)
	c := h.collectionOf(collectionID)
	job, _, err := h.svc.RequestSnapshot(ctx, app.Owner, fmt.Sprintf("snap-key-%d", time.Now().UnixNano()), collectionID, c.Revision)
	if err != nil {
		h.t.Fatal(err)
	}
	snap, err := h.store.BeginSnapshot(ctx, job.ID)
	if err != nil {
		h.t.Fatal(err)
	}
	// The engine records a content identity for every frozen item; the job
	// cannot succeed with unfilled identities.
	items, err := h.store.SnapshotItems(ctx, snap.ID, 100, "")
	if err != nil {
		h.t.Fatal(err)
	}
	for _, it := range items {
		if err = h.store.RecordSnapshotItem(ctx, snap.ID, it.Item, "identity:"+it.SourceID); err != nil {
			h.t.Fatal(err)
		}
	}
	if _, err = h.store.FinishJob(ctx, job.ID, func(j *state.Job) (string, bool) {
		j.State = state.JobSucceeded
		return "", true
	}); err != nil {
		h.t.Fatal(err)
	}
	snaps, _, err := h.svc.Snapshots(ctx, app.Owner, collectionID, 5, 0)
	if err != nil || len(snaps) == 0 {
		h.t.Fatalf("no snapshot: %v", err)
	}
	return snaps[0]
}

// forgeProtection stores a callback nobody offered (to drive a stale or
// expired confirmation) and delivers it.
func (h *harness) forgeProtection(seq int, action, object string, revision int, payload string, age time.Duration, messageID int) {
	h.t.Helper()
	cb := state.Callback{ID: fmt.Sprintf("cb_forged_%d", seq), Principal: "owner", ChatID: h.ownerID, MessageID: messageID,
		Action: action, Object: object, Revision: revision, Payload: payload,
		CreatedAt: h.clock().Add(-age), ExpiresAt: h.clock().Add(callbackTTL)}
	if err := h.store.PutCallback(context.Background(), cb); err != nil {
		h.t.Fatal(err)
	}
	h.api.push(h.callbackUpdate(h.nextUpdateID(), h.ownerID, h.ownerID, messageID, callbackPrefix+cb.ID))
	h.pull()
}

func TestProtectionCardOffersOptInMirrorAndSnapshots(t *testing.T) {
	h := newHarness(t, 100)
	pt := vietnamese.prot()
	id := h.readyCollection("Protect me")

	card := h.collectionCardFor(id)
	if !hasButton(card, pt.btn) {
		t.Fatalf("the collection card has no protection button: %v", card.ButtonTexts)
	}
	h.tap(card, pt.btn)
	card = h.lastCard()
	for _, want := range []string{pt.mirrorLabel, pt.snapLabel, "30 ngày", "10 bản", pt.mirrorAskBtn, pt.snapToggleOn, pt.snapsBtn, pt.captureBtn} {
		if !strings.Contains(card.Text, want) && !hasButton(card, want) {
			t.Fatalf("the protection card misses %q:\n%s\n%v", want, card.Text, card.ButtonTexts)
		}
	}
	if c := h.collectionOf(id); c.Protection.Mirror || c.Protection.Snapshots {
		t.Fatalf("opening the card changed the options: %+v", c.Protection)
	}
}

func TestMirrorNeedsItsOwnConfirmationAndIsOneShot(t *testing.T) {
	h := newHarness(t, 100)
	pt := vietnamese.prot()
	id := h.readyCollection("Mirror me")
	h.tap(h.collectionCardFor(id), pt.btn)
	h.tap(h.lastCard(), pt.mirrorAskBtn)
	ask := h.lastCard()
	if !strings.Contains(ask.Text, "thùng rác Drive") || !strings.Contains(ask.Text, "30 ngày") {
		t.Fatalf("the explanation does not mention the trash:\n%s", ask.Text)
	}
	if h.collectionOf(id).Protection.Mirror {
		t.Fatalf("the explanation itself enabled mirror")
	}
	if styleOf(ask, pt.mirrorConfirm) != "danger" {
		t.Fatalf("the confirmation must be the red button: %v / %v", ask.ButtonTexts, ask.Styles)
	}
	h.tap(ask, pt.mirrorConfirm)
	if !h.collectionOf(id).Protection.Mirror {
		t.Fatalf("mirror was not enabled after the confirmation")
	}
	rev := h.collectionOf(id).Revision
	h.api.push(h.click(ask, pt.mirrorConfirm))
	h.pull()
	if h.lastAnswer() != vietnamese.expired {
		t.Fatalf("the second tap answered %q", h.lastAnswer())
	}
	if h.collectionOf(id).Revision != rev {
		t.Fatalf("a consumed confirmation changed the collection again")
	}
}

func TestMirrorConfirmationExpiresAndAChangedCollectionIsRefused(t *testing.T) {
	h := newHarness(t, 100)
	pt := vietnamese.prot()
	id := h.readyCollection("Expiring mirror")
	h.tap(h.collectionCardFor(id), pt.btn)
	h.tap(h.lastCard(), pt.mirrorAskBtn)
	ask := h.lastCard()

	h.advance(lifecycleConfirmTTL + time.Minute)
	h.tap(ask, pt.mirrorConfirm)
	if h.collectionOf(id).Protection.Mirror {
		t.Fatalf("an expired confirmation enabled mirror")
	}
	if !strings.Contains(h.lastCard().Text, vietnamese.g().confirmExpired) {
		t.Fatalf("expected the expiry notice:\n%s", h.lastCard().Text)
	}

	// A fresh confirmation, then the collection changes under it.
	h.tap(h.collectionCardFor(id), pt.btn)
	h.tap(h.lastCard(), pt.mirrorAskBtn)
	ask = h.lastCard()
	c := h.collectionOf(id)
	if _, err := h.svc.SetCollectionFilter(context.Background(), app.Owner, id, c.Filter, c.Revision); err != nil {
		t.Fatal(err)
	}
	h.tap(ask, pt.mirrorConfirm)
	if h.collectionOf(id).Protection.Mirror {
		t.Fatalf("a stale confirmation enabled mirror")
	}
}

func TestRetentionComesFromTheFixedChoicesOnly(t *testing.T) {
	h := newHarness(t, 100)
	pt := vietnamese.prot()
	id := h.readyCollection("Retention")
	h.tap(h.collectionCardFor(id), pt.btn)
	h.tap(h.lastCard(), pt.keepBtn)
	keep := h.lastCard()
	for _, label := range []string{"7 ngày", "30 ngày", "90 ngày", "5 bản", "10 bản", "20 bản"} {
		if !hasButton(keep, label) {
			t.Fatalf("the fixed choice %q is missing: %v", label, keep.ButtonTexts)
		}
	}
	h.tap(keep, "90 ngày")
	if got := h.collectionOf(id).Protection.KeepDays; got != 90 {
		t.Fatalf("keep days = %d, want 90", got)
	}
	h.tap(h.lastCard(), pt.keepBtn)
	h.tap(h.lastCard(), "20 bản")
	opts := h.collectionOf(id).Protection
	if opts.KeepCount != 20 || opts.KeepDays != 90 {
		t.Fatalf("retention after the second pick = %+v", opts)
	}
	// No free text: a value outside the list is refused even if forged.
	h.forgeProtection(1, "protect_days", id, h.collectionOf(id).Revision, "45", 0, h.lastCard().MessageID)
	if got := h.collectionOf(id).Protection.KeepDays; got != 90 {
		t.Fatalf("a typed retention value was accepted: %d", got)
	}
}

func TestRestoreIsPreviewedAndApplyHasItsOwnConfirmation(t *testing.T) {
	h := newHarness(t, 100)
	pt := vietnamese.prot()
	id := h.readyCollection("Restore me")
	snap := h.readySnapshot(id)

	h.tap(h.collectionCardFor(id), pt.btn)
	h.tap(h.lastCard(), pt.snapsBtn)
	list := h.lastCard()
	if !hasButton(list, short(snap.ID, 18)) {
		t.Fatalf("the snapshot is not listed: %v", list.ButtonTexts)
	}
	h.tap(list, short(snap.ID, 18))
	snapCard := h.lastCard()
	if !strings.Contains(snapCard.Text, snap.ID) || !hasButton(snapCard, pt.pinBtn) {
		t.Fatalf("the snapshot card is wrong:\n%s\n%v", snapCard.Text, snapCard.ButtonTexts)
	}
	h.tap(snapCard, pt.restoreBtn)

	preview := h.lastCard()
	if !strings.Contains(preview.Text, pt.restoreRunning) {
		t.Fatalf("a queued dry run must say it is running:\n%s", preview.Text)
	}
	if hasButton(preview, pt.applyBtn) {
		t.Fatalf("apply is offered before the dry run finished: %v", preview.ButtonTexts)
	}
	jobs := h.jobs()
	if len(jobs) == 0 || jobs[0].Kind != state.JobKindRestore || !jobs[0].DryRun {
		t.Fatalf("the first restore is not a dry run: %+v", jobs)
	}
	dry := jobs[0]
	ctx := context.Background()
	if err := h.store.FreezeRestoreActions(ctx, dry.ID, id, snap.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.FinishJob(ctx, dry.ID, func(j *state.Job) (string, bool) {
		j.State = state.JobSucceeded
		return "", true
	}); err != nil {
		t.Fatal(err)
	}
	h.tap(preview, pt.restoreRead)
	ready := h.lastCard()
	if !strings.Contains(ready.Text, fmt.Sprintf(pt.restoreCount, 1)) {
		t.Fatalf("the plan count is missing:\n%s", ready.Text)
	}
	if !hasButton(ready, pt.applyBtn) {
		t.Fatalf("apply is not offered after the preview: %v", ready.ButtonTexts)
	}

	// The apply step is its own card and carries the snapshot revision.
	h.tap(ready, pt.applyBtn)
	ask := h.lastCard()
	if !strings.Contains(ask.Text, "tạm dừng") || styleOf(ask, pt.applyConfirm) != "danger" {
		t.Fatalf("apply must explain the sync pause in a red button:\n%s\n%v", ask.Text, ask.ButtonTexts)
	}
	h.tap(ask, pt.applyConfirm)
	jobs = h.jobs()
	if len(jobs) == 0 || jobs[0].Kind != state.JobKindRestore || jobs[0].DryRun {
		t.Fatalf("apply did not admit a real restore: %+v", jobs)
	}
}

func TestRestoreApplyRefusesAnExpiredConfirmation(t *testing.T) {
	h := newHarness(t, 100)
	id := h.readyCollection("Expired apply")
	snap := h.readySnapshot(id)
	before := len(h.jobs())
	h.forgeProtection(3, "protect_restore_do", snap.ID, snap.Revision, "", lifecycleConfirmTTL+time.Minute, 10)
	if got := len(h.jobs()); got != before {
		t.Fatalf("an expired apply admitted a job (%d -> %d)", before, got)
	}
	if !strings.Contains(h.lastCard().Text, vietnamese.g().confirmExpired) {
		t.Fatalf("expected the expiry notice:\n%s", h.lastCard().Text)
	}
}

func TestRestoreApplyRefusesAStaleSnapshotRevision(t *testing.T) {
	h := newHarness(t, 100)
	id := h.readyCollection("Stale apply")
	snap := h.readySnapshot(id)
	// Pin the snapshot: its revision moves past the one the card showed.
	if _, _, err := h.svc.PinSnapshot(context.Background(), app.Owner, "pin-key", snap.ID, snap.Revision, true); err != nil {
		t.Fatal(err)
	}
	before := len(h.jobs())
	h.forgeProtection(4, "protect_restore_do", snap.ID, snap.Revision, "", 0, 10)
	if got := len(h.jobs()); got != before {
		t.Fatalf("a stale apply admitted a job (%d -> %d)", before, got)
	}
}

func TestProtectionStringsExistInBothLanguages(t *testing.T) {
	var check func(name string, vi, en reflect.Value)
	check = func(name string, vi, en reflect.Value) {
		for i := range vi.NumField() {
			field := vi.Type().Field(i).Name
			a, b := vi.Field(i), en.Field(i)
			switch a.Kind() {
			case reflect.String:
				if a.String() == "" || b.String() == "" {
					t.Errorf("%s.%s is empty (vi=%q en=%q)", name, field, a.String(), b.String())
				}
			case reflect.Struct:
				check(name+"."+field, a, b)
			}
		}
	}
	check("protectTexts", reflect.ValueOf(protectVI), reflect.ValueOf(protectEN))
}
