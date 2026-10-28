package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/state"
)

// Protection from chat: the opt-in mirror, automatic snapshots with fixed
// retention choices, manual capture, pinning and restore. Every button is one
// one-shot callback bound to the revision it showed. Enabling mirror and
// applying a restore each get their own confirmation card; a restore is always
// admitted as a dry run first, and apply only appears once that dry run
// succeeded and its plan was read.

const restorePreviewRows = 100

// protectionCard is the protection menu of one collection.
func (t *Telegram) protectionCard(ctx context.Context, p app.Principal, chatID int64, messageID int, id, note string) error {
	c, err := t.svc.Collection(ctx, p, id)
	if err != nil {
		return t.showError(ctx, p, chatID, messageID, err)
	}
	tx := t.text(ctx, p)
	pt := tx.prot()
	opts := c.Protection.Defaults()
	var b strings.Builder
	fmt.Fprintf(&b, "<b>%s</b>\n%s\n\n", escape(short(c.SourceName, 60)), pt.title)
	fmt.Fprintf(&b, "%s: <b>%s</b>\n", pt.mirrorLabel, onOff(opts.Mirror, tx))
	fmt.Fprintf(&b, "%s: <b>%s</b> · %s\n", pt.snapLabel, onOff(opts.Snapshots, tx), fmt.Sprintf(pt.keepFmt, opts.KeepDays, opts.KeepCount))
	if note != "" {
		b.WriteString("\n" + note)
	}
	mirror := act(pt.mirrorAskBtn, "protect_mirror_ask", c.ID, c.Revision, "")
	if opts.Mirror {
		mirror = act(pt.mirrorOffBtn, "protect_mirror_off", c.ID, c.Revision, "")
	}
	snapToggle := act(pt.snapToggleOn, "protect_snap_toggle", c.ID, c.Revision, "")
	if opts.Snapshots {
		snapToggle = act(pt.snapToggleOff, "protect_snap_toggle", c.ID, c.Revision, "")
	}
	return t.showCard(ctx, p, chatID, messageID, b.String(),
		[]button{mirror, snapToggle},
		[]button{act(pt.keepBtn, "protect_keep", c.ID, c.Revision, "")},
		[]button{act(pt.snapsBtn, "protect_snaps", c.ID, c.Revision, ""), act(pt.captureBtn, "protect_capture", c.ID, c.Revision, "")},
		[]button{act("↩️ "+tx.library, "collection", c.ID, 0, ""), nav(tx.back, "home")})
}

// protectKeepCard offers only the fixed retention values.
func (t *Telegram) protectKeepCard(ctx context.Context, p app.Principal, chatID int64, messageID int, id string, revision int) error {
	tx := t.text(ctx, p)
	pt := tx.prot()
	days := make([]button, 0, len(protectDays))
	for _, d := range protectDays {
		days = append(days, act(fmt.Sprintf(pt.daysValue, d), "protect_days", id, revision, strconv.Itoa(d)))
	}
	counts := make([]button, 0, len(protectCounts))
	for _, n := range protectCounts {
		counts = append(counts, act(fmt.Sprintf(pt.countValue, n), "protect_counts", id, revision, strconv.Itoa(n)))
	}
	return t.showCard(ctx, p, chatID, messageID, pt.keepTitle+"\n"+pt.keepHint,
		days, counts,
		[]button{act("↩️ "+pt.snapsBack, "protect", id, 0, ""), nav(tx.back, "home")})
}

// The fixed retention choices; typing a number is not possible.
var (
	protectDays   = []int{7, 30, 90}
	protectCounts = []int{5, 10, 20}
)

func containsChoice(values []int, want int) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

// protectSnapshotsCard lists the newest snapshots of one collection.
func (t *Telegram) protectSnapshotsCard(ctx context.Context, p app.Principal, chatID int64, messageID int, id string, revision int) error {
	tx := t.text(ctx, p)
	pt := tx.prot()
	snaps, _, err := t.svc.Snapshots(ctx, p, id, 8, 0)
	if err != nil {
		return t.showError(ctx, p, chatID, messageID, err)
	}
	var rows [][]button
	for _, s := range snaps {
		pin := ""
		if s.Pinned {
			pin = " 📌"
		}
		label := fmt.Sprintf(pt.snapRow, short(s.ID, 18), s.CreatedAt.Local().Format("2006-01-02"), s.Items) + pin
		rows = append(rows, []button{act(label, "protect_snap", s.ID, s.Revision, "")})
	}
	text := pt.snapsTitle
	if len(rows) == 0 {
		text += "\n" + pt.snapsEmpty
	}
	rows = append(rows, []button{act("↩️ "+pt.snapsBack, "protect", id, revision, ""), nav(tx.back, "home")})
	return t.showCard(ctx, p, chatID, messageID, text, rows...)
}

// protectSnapshotCard is one restore point: pin/unpin and the restore entry.
func (t *Telegram) protectSnapshotCard(ctx context.Context, p app.Principal, chatID int64, messageID int, snapshotID string, note string) error {
	s, err := t.svc.Snapshot(ctx, p, snapshotID)
	if err != nil {
		return t.showError(ctx, p, chatID, messageID, err)
	}
	tx := t.text(ctx, p)
	pt := tx.prot()
	pin := pt.pinBtn
	if s.Pinned {
		pin = pt.unpinBtn
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n<code>%s</code>\n", pt.snapCardTitle, escape(s.ID))
	fmt.Fprintf(&b, "%s\n", escape(s.CreatedAt.Local().Format("2006-01-02 15:04")))
	fmt.Fprintf(&b, "%s · %d\n", fmt.Sprintf(pt.snapshotState, escape(s.State)), s.Items)
	if note != "" {
		b.WriteString("\n" + note)
	}
	pinAction := "protect_pin"
	if s.Pinned {
		pinAction = "protect_unpin"
	}
	return t.showCard(ctx, p, chatID, messageID, b.String(),
		[]button{act(pin, pinAction, s.ID, s.Revision, "")},
		[]button{styled(act(pt.restoreBtn, "protect_restore", s.ID, s.Revision, ""), "primary")},
		[]button{act("↩️ "+pt.snapsBack, "protect_snaps", s.CollectionID, s.Revision, ""), nav(tx.back, "home")})
}

// restorePreviewCard shows what the dry run would do. apply exists only after
// that job succeeded and the captured plan is not empty.
func (t *Telegram) restorePreviewCard(ctx context.Context, p app.Principal, chatID int64, messageID int, snapshotID string, revision int, jobID, note string) error {
	job, err := t.svc.Job(ctx, p, jobID)
	if err != nil {
		return t.showError(ctx, p, chatID, messageID, err)
	}
	tx := t.text(ctx, p)
	pt := tx.prot()
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n<code>%s</code>\n", pt.restoreTitle, escape(snapshotID))
	fmt.Fprintf(&b, "%s: <b>%s</b>\n", tx.jobLabel, escape(job.State))
	back := []button{act("↩️ "+pt.restoreBack, "protect_snap", snapshotID, revision, ""), nav(tx.back, "home")}
	switch job.State {
	case state.JobSucceeded:
		actions, err := t.svc.RestorePlan(ctx, p, jobID, 0, restorePreviewRows+1)
		if err != nil {
			return t.showError(ctx, p, chatID, messageID, err)
		}
		more := len(actions) > restorePreviewRows
		if more {
			actions = actions[:restorePreviewRows]
		}
		if len(actions) == 0 {
			b.WriteString("\n" + pt.restoreEmpty)
			return t.showCard(ctx, p, chatID, messageID, b.String(), back)
		}
		if more {
			fmt.Fprintf(&b, "\n%s\n", fmt.Sprintf(pt.restoreCountMore, len(actions)))
		} else {
			fmt.Fprintf(&b, "\n%s\n", fmt.Sprintf(pt.restoreCount, len(actions)))
		}
		for i, a := range actions {
			if i >= 8 {
				break
			}
			fmt.Fprintf(&b, "· %s · %s\n", escape(a.Action), escape(short(a.Target.Path+" "+a.Target.Name, 60)))
		}
		if note != "" {
			b.WriteString("\n" + note)
		}
		return t.showCard(ctx, p, chatID, messageID, b.String(),
			[]button{styled(act(pt.applyBtn, "protect_restore_ask", snapshotID, revision, jobID), "primary")},
			back)
	case state.JobFailed, state.JobBlocked, state.JobPartial, state.JobCancelled:
		b.WriteString("\n" + fmt.Sprintf(pt.restoreFailed, escape(job.State)))
		return t.showCard(ctx, p, chatID, messageID, b.String(), back)
	}
	b.WriteString("\n" + pt.restoreRunning)
	if note != "" {
		b.WriteString("\n" + note)
	}
	return t.showCard(ctx, p, chatID, messageID, b.String(),
		[]button{act(pt.restoreRead, "protect_restore_read", snapshotID, revision, jobID)},
		back)
}

// ---- callback handlers ----

func (t *Telegram) actProtect(ctx context.Context, p app.Principal, st state.Callback, tx texts) error {
	return t.protectionCard(ctx, p, st.ChatID, st.MessageID, st.Object, "")
}

func (t *Telegram) actProtectMirrorAsk(ctx context.Context, p app.Principal, st state.Callback, tx texts) error {
	c, err := t.svc.Collection(ctx, p, st.Object)
	if err != nil {
		return t.showError(ctx, p, st.ChatID, st.MessageID, err)
	}
	pt := tx.prot()
	return t.showCard(ctx, p, st.ChatID, st.MessageID, pt.mirrorAsk,
		[]button{styled(act(pt.mirrorConfirm, "protect_mirror_on", c.ID, c.Revision, ""), "danger")},
		[]button{act("↩️ "+pt.btn, "protect", c.ID, 0, "")})
}

func (t *Telegram) actProtectMirrorOn(ctx context.Context, p app.Principal, st state.Callback, tx texts) error {
	if t.confirmStale(st) {
		return t.expiredCard(ctx, p, st)
	}
	c, err := t.svc.Collection(ctx, p, st.Object)
	if err != nil {
		return t.showError(ctx, p, st.ChatID, st.MessageID, err)
	}
	opts := c.Protection.Defaults()
	opts.Mirror = true
	if _, _, err = t.svc.SetProtection(ctx, p, "tg-cb:"+st.ID, c.ID, st.Revision, opts); err != nil {
		return t.showError(ctx, p, st.ChatID, st.MessageID, err)
	}
	return t.protectionCard(ctx, p, st.ChatID, st.MessageID, c.ID, tx.prot().mirrorOnDone)
}

func (t *Telegram) actProtectOptions(ctx context.Context, p app.Principal, st state.Callback, tx texts, change func(*state.ProtectionOptions), note string) error {
	c, err := t.svc.Collection(ctx, p, st.Object)
	if err != nil {
		return t.showError(ctx, p, st.ChatID, st.MessageID, err)
	}
	opts := c.Protection.Defaults()
	change(&opts)
	if _, _, err = t.svc.SetProtection(ctx, p, "tg-cb:"+st.ID, c.ID, st.Revision, opts); err != nil {
		return t.showError(ctx, p, st.ChatID, st.MessageID, err)
	}
	return t.protectionCard(ctx, p, st.ChatID, st.MessageID, c.ID, note)
}

func (t *Telegram) actProtectMirrorOff(ctx context.Context, p app.Principal, st state.Callback, tx texts) error {
	return t.actProtectOptions(ctx, p, st, tx, func(o *state.ProtectionOptions) { o.Mirror = false }, tx.prot().mirrorOffDone)
}

func (t *Telegram) actProtectSnapToggle(ctx context.Context, p app.Principal, st state.Callback, tx texts) error {
	pt := tx.prot()
	note := pt.snapOnDone
	return t.actProtectOptions(ctx, p, st, tx, func(o *state.ProtectionOptions) {
		o.Snapshots = !o.Snapshots
		if !o.Snapshots {
			note = pt.snapOffDone
		}
	}, note)
}

func (t *Telegram) actProtectKeep(ctx context.Context, p app.Principal, st state.Callback, tx texts) error {
	if _, err := t.svc.Collection(ctx, p, st.Object); err != nil {
		return t.showError(ctx, p, st.ChatID, st.MessageID, err)
	}
	return t.protectKeepCard(ctx, p, st.ChatID, st.MessageID, st.Object, st.Revision)
}

func (t *Telegram) actProtectRetention(ctx context.Context, p app.Principal, st state.Callback, tx texts, days bool) error {
	n, err := strconv.Atoi(st.Payload)
	choices := protectCounts
	if days {
		choices = protectDays
	}
	if err != nil || !containsChoice(choices, n) {
		return t.showError(ctx, p, st.ChatID, st.MessageID, app.Fail(400, "invalid_request", tx.prot().invalidChoice))
	}
	return t.actProtectOptions(ctx, p, st, tx, func(o *state.ProtectionOptions) {
		if days {
			o.KeepDays = n
		} else {
			o.KeepCount = n
		}
	}, tx.prot().saved)
}

func (t *Telegram) actProtectCapture(ctx context.Context, p app.Principal, st state.Callback, tx texts) error {
	c, err := t.svc.Collection(ctx, p, st.Object)
	if err != nil {
		return t.showError(ctx, p, st.ChatID, st.MessageID, err)
	}
	job, _, err := t.svc.RequestSnapshot(ctx, p, "tg-cb:"+st.ID, c.ID, st.Revision)
	if err != nil {
		return t.showError(ctx, p, st.ChatID, st.MessageID, err)
	}
	return t.protectionCard(ctx, p, st.ChatID, st.MessageID, c.ID, fmt.Sprintf(tx.prot().captureDone, escape(job.ID)))
}

func (t *Telegram) actProtectSnaps(ctx context.Context, p app.Principal, st state.Callback, tx texts) error {
	return t.protectSnapshotsCard(ctx, p, st.ChatID, st.MessageID, st.Object, st.Revision)
}

func (t *Telegram) actProtectSnap(ctx context.Context, p app.Principal, st state.Callback, tx texts) error {
	return t.protectSnapshotCard(ctx, p, st.ChatID, st.MessageID, st.Object, "")
}

func (t *Telegram) actProtectPin(ctx context.Context, p app.Principal, st state.Callback, tx texts, pin bool) error {
	if _, _, err := t.svc.PinSnapshot(ctx, p, "tg-cb:"+st.ID, st.Object, st.Revision, pin); err != nil {
		return t.showError(ctx, p, st.ChatID, st.MessageID, err)
	}
	return t.protectSnapshotCard(ctx, p, st.ChatID, st.MessageID, st.Object, tx.prot().pinDone)
}

// actProtectRestore admits the dry run (nothing is written on Drive).
func (t *Telegram) actProtectRestore(ctx context.Context, p app.Principal, st state.Callback, tx texts) error {
	job, _, err := t.svc.RequestRestore(ctx, p, "tg-cb:"+st.ID, st.Object, app.RestoreRequest{ExpectedRevision: st.Revision})
	if err != nil {
		return t.showError(ctx, p, st.ChatID, st.MessageID, err)
	}
	return t.restorePreviewCard(ctx, p, st.ChatID, st.MessageID, st.Object, st.Revision, job.ID, "")
}

func (t *Telegram) actProtectRestoreRead(ctx context.Context, p app.Principal, st state.Callback, tx texts) error {
	if st.Payload == "" {
		return t.showError(ctx, p, st.ChatID, st.MessageID, app.Fail(409, "plan_changed", tx.expired))
	}
	return t.restorePreviewCard(ctx, p, st.ChatID, st.MessageID, st.Object, st.Revision, st.Payload, "")
}

// actProtectRestoreAsk is the last card before a restore: it names the point
// and says sync pauses. The apply button carries the same snapshot revision.
func (t *Telegram) actProtectRestoreAsk(ctx context.Context, p app.Principal, st state.Callback, tx texts) error {
	pt := tx.prot()
	return t.showCard(ctx, p, st.ChatID, st.MessageID, fmt.Sprintf(pt.applyAsk, escape(st.Object)),
		[]button{styled(act(pt.applyConfirm, "protect_restore_do", st.Object, st.Revision, st.Payload), "danger")},
		[]button{act("↩️ "+pt.restoreBack, "protect_snap", st.Object, st.Revision, "")})
}

func (t *Telegram) actProtectRestoreDo(ctx context.Context, p app.Principal, st state.Callback, tx texts) error {
	if t.confirmStale(st) {
		return t.expiredCard(ctx, p, st)
	}
	dry := false
	job, _, err := t.svc.RequestRestore(ctx, p, "tg-cb:"+st.ID, st.Object, app.RestoreRequest{ExpectedRevision: st.Revision, DryRun: &dry})
	if err != nil {
		return t.showError(ctx, p, st.ChatID, st.MessageID, err)
	}
	return t.showCard(ctx, p, st.ChatID, st.MessageID, fmt.Sprintf(tx.prot().applyDone, escape(job.ID)),
		[]button{act(tx.details, "job", job.ID, job.Revision, ""), nav(tx.library, "library")})
}
