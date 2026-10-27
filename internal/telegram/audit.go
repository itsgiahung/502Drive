package telegram

import (
	"context"
	"fmt"

	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/state"
)

// auditStart admits a read-only integrity audit for a collection (the 🔍
// button on the collection card). The callback is one-shot, so its id is a
// safe idempotency key: a double tap replays instead of creating two jobs.
func (t *Telegram) auditStart(ctx context.Context, p app.Principal, st state.Callback) error {
	tx := t.text(ctx, p)
	_, j, _, err := t.svc.RequestAudit(ctx, p, "tg-cb:"+st.ID, st.Object)
	if err != nil {
		return t.showError(ctx, p, st.ChatID, st.MessageID, err)
	}
	body := fmt.Sprintf(tx.auditStarted, escape(j.ID))
	return t.showCard(ctx, p, st.ChatID, st.MessageID, body, []button{nav(tx.library, "library")})
}

// auditRepair admits the repair job for the audit named by the button.
func (t *Telegram) auditRepair(ctx context.Context, p app.Principal, st state.Callback) error {
	tx := t.text(ctx, p)
	rev := st.Revision
	j, _, err := t.svc.RequestRepair(ctx, p, "tg-cb:"+st.ID, st.Object, app.RepairRequest{ExpectedRevision: &rev})
	if err != nil {
		return t.showError(ctx, p, st.ChatID, st.MessageID, err)
	}
	body := fmt.Sprintf(tx.auditRepairStarted, escape(j.ID))
	return t.showCard(ctx, p, st.ChatID, st.MessageID, body, []button{nav(tx.library, "library")})
}

// findingKindsRepairable are the kinds a repair can act on.
func repairableCount(counts map[string]int) int {
	return counts[state.FindingMissing] + counts[state.FindingName] +
		counts[state.FindingSize] + counts[state.FindingParent] + counts[state.FindingDuplicate]
}

func totalFindings(counts map[string]int) int {
	n := 0
	for _, k := range []string{state.FindingMissing, state.FindingExtra, state.FindingName,
		state.FindingSize, state.FindingParent, state.FindingDuplicate, state.FindingRemoved, state.FindingUnreadable} {
		n += counts[k]
	}
	return n
}

// auditSummary is the audit line for the collection card plus, when there is
// something safe to fix, the Sửa row. Empty when no finished audit exists.
func (t *Telegram) auditSummary(ctx context.Context, p app.Principal, tx texts, collectionID string) (string, [][]button) {
	a, ok, err := t.svc.LastAudit(ctx, p, collectionID)
	if err != nil || !ok || a.State != state.AuditCompleted {
		return "", nil
	}
	total := totalFindings(a.Counts)
	if total == 0 {
		return tx.auditClean, nil
	}
	repairable := repairableCount(a.Counts)
	summary := fmt.Sprintf(tx.auditDone, total, repairable)
	if repairable == 0 {
		return summary, nil
	}
	return summary, [][]button{{act(tx.auditRepair, "audit_repair", a.ID, a.Revision, "")}}
}
