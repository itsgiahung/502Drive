package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/state"
)

// Several links in one message (S6): every link is its own job, all under one
// batch id, so the queue shares the workers fairly between the batch and the
// rest. The same rule as a single paste decides whether the batch starts at
// once (paste-to-copy bound to an unchanged destination) or waits on a preview
// with one button. The summary card is read from the server (GET batch), so a
// refresh always shows the truth.

const (
	// maxBatchLinks bounds one message; the rest is reported, not dropped silently.
	maxBatchLinks = 20
	// batchListLines is how many names a card shows before "+N more".
	batchListLines = 10
)

// batchItem is one inspected link waiting for the confirmation button.
type batchItem struct {
	Plan string `json:"p"`
	Rev  int    `json:"r"`
	Name string `json:"n"`
}

// batchMiss is a link that did not become a job, with the stable reason.
type batchMiss struct {
	Name string `json:"n"`
	Code string `json:"c"`
}

// batchPayload travels in the private callback payload. The batch id is
// derived from the Telegram update, so a redelivered message lands in the
// same batch.
type batchPayload struct {
	ID      string      `json:"id"`
	Items   []batchItem `json:"i,omitempty"`
	Skipped []batchMiss `json:"s,omitempty"`
	Failed  []batchMiss `json:"f,omitempty"`
	Extra   int         `json:"x,omitempty"`
}

func (bp batchPayload) json() string {
	b, _ := json.Marshal(bp)
	return string(b)
}

func decodeBatch(payload string) (batchPayload, bool) {
	var bp batchPayload
	if json.Unmarshal([]byte(payload), &bp) != nil || bp.ID == "" {
		return bp, false
	}
	return bp, true
}

// batchIDFor is a valid app batch id (8-64 of [A-Za-z0-9_-]) for one update.
func batchIDFor(base string) string { return "tgb-" + hashText(base) }

// pasteMany inspects every link of a message and either admits the batch or
// shows one preview for it.
func (t *Telegram) pasteMany(ctx context.Context, p app.Principal, chatID int64, messageID int, refs []string, base string) error {
	tx := t.text(ctx, p)
	ig := tx.ig()
	bp := batchPayload{ID: batchIDFor(base)}
	if len(refs) > maxBatchLinks {
		bp.Extra = len(refs) - maxBatchLinks
		refs = refs[:maxBatchLinks]
	}
	prefs, err := t.svc.Preferences(ctx, p)
	if err != nil {
		return t.showError(ctx, p, chatID, messageID, err)
	}
	auto := prefs.AutoCopy && prefs.DestinationID != ""
	allBound := auto && prefs.DestinationRevision != nil
	var firstDest string
	for i, ref := range refs {
		req := app.PlanRequest{Source: ref}
		if auto {
			req.DestinationID = prefs.DestinationID
		}
		view, _, ierr := t.svc.Inspect(ctx, p, fmt.Sprintf("%s:%d:plan", base, i), req)
		if ierr != nil {
			bp.Failed = append(bp.Failed, batchMiss{Name: short(ref, 40), Code: codeOf(ierr)})
			continue
		}
		bp.Items = append(bp.Items, batchItem{Plan: view.ID, Rev: view.Revision, Name: short(view.SourceName, 60)})
		if firstDest == "" {
			firstDest = view.DestinationID
		}
		if allBound && *prefs.DestinationRevision != view.DestinationRevision {
			allBound = false
		}
	}
	if len(bp.Items) == 0 {
		return t.showCard(ctx, p, chatID, messageID, t.batchBadText(tx, bp, ig.batchNone), []button{nav(tx.back, "home")})
	}
	if allBound {
		return t.admitBatch(ctx, p, chatID, messageID, base, bp)
	}
	var b strings.Builder
	fmt.Fprintf(&b, ig.batchTitle+"\n\n", len(bp.Items))
	for i, it := range bp.Items {
		if i == batchListLines {
			fmt.Fprintf(&b, ig.batchMore+"\n", len(bp.Items)-batchListLines)
			break
		}
		fmt.Fprintf(&b, "· %s\n", escape(it.Name))
	}
	fmt.Fprintf(&b, "\n%s: <b>%s</b>\n", tx.saveToLabel, escape(short(t.destinationLabel(ctx, firstDest), 60)))
	b.WriteString(t.batchMissLines(tx, bp.Failed))
	if bp.Extra > 0 {
		fmt.Fprintf(&b, "%s\n", fmt.Sprintf(ig.batchTooMany, maxBatchLinks, bp.Extra))
	}
	if prefs.AutoCopy {
		b.WriteString("\n" + tx.confirmDest + "\n")
	}
	b.WriteString("\n" + ig.batchNote)
	return t.showCard(ctx, p, chatID, messageID, b.String(),
		[]button{styled(act(fmt.Sprintf(ig.batchGo, len(bp.Items)), "links_go", bp.ID, 0, bp.json()), "success")},
		[]button{styled(act("✖ "+tx.cancel, "cancel_preview", bp.ID, 0, ""), "danger")})
}

// batchBadText is the card for a message whose links all failed.
func (t *Telegram) batchBadText(tx texts, bp batchPayload, head string) string {
	var b strings.Builder
	b.WriteString(head + "\n")
	b.WriteString(t.batchMissLines(tx, bp.Failed))
	return b.String()
}

// batchMissLines renders links that were not accepted, with the reason.
func (t *Telegram) batchMissLines(tx texts, misses []batchMiss) string {
	ig := tx.ig()
	var b strings.Builder
	for i, m := range misses {
		if i == batchListLines {
			fmt.Fprintf(&b, ig.batchMore+"\n", len(misses)-batchListLines)
			break
		}
		fmt.Fprintf(&b, ig.batchBad+"\n", escape(m.Name), escape(codeText(tx, m.Code)))
	}
	return b.String()
}

// actLinksGo admits the batch the user confirmed. The button is one-shot.
func (t *Telegram) actLinksGo(ctx context.Context, p app.Principal, st state.Callback) error {
	tx := t.text(ctx, p)
	bp, ok := decodeBatch(st.Payload)
	if !ok || len(bp.Items) == 0 {
		return t.showError(ctx, p, st.ChatID, st.MessageID, app.Fail(409, "plan_changed", tx.confirmDest))
	}
	return t.admitBatch(ctx, p, st.ChatID, st.MessageID, "tg-cb:"+st.ID, bp)
}

// admitBatch submits each inspected link under the batch id. Each link has its
// own idempotency key, so a replay admits only what is still missing, and a
// refused link never stops the others.
func (t *Telegram) admitBatch(ctx context.Context, p app.Principal, chatID int64, messageID int, base string, bp batchPayload) error {
	items := bp.Items
	bp.Items = nil
	for i, it := range items {
		_, err := t.svc.SubmitModeInBatchWithSync(ctx, p, fmt.Sprintf("%s:%d:copy", base, i), it.Plan, it.Rev, "", bp.ID, "")
		switch {
		case err == nil:
		case existingCopy(err):
			bp.Skipped = append(bp.Skipped, batchMiss{Name: it.Name, Code: "already_copied"})
		default:
			bp.Failed = append(bp.Failed, batchMiss{Name: it.Name, Code: codeOf(err)})
		}
	}
	return t.batchCard(ctx, p, chatID, messageID, bp)
}

// actLinksView refreshes the summary of a batch.
func (t *Telegram) actLinksView(ctx context.Context, p app.Principal, st state.Callback) error {
	bp, ok := decodeBatch(st.Payload)
	if !ok {
		return t.page(ctx, p, st.ChatID, st.MessageID, "jobs")
	}
	return t.batchCard(ctx, p, st.ChatID, st.MessageID, bp)
}

// batchCard is the summary message of a batch: the counts, one line per link
// and the live state of every job (read from the server).
func (t *Telegram) batchCard(ctx context.Context, p app.Principal, chatID int64, messageID int, bp batchPayload) error {
	tx := t.text(ctx, p)
	ig := tx.ig()
	jobs, err := t.svc.Batch(ctx, p, bp.ID)
	if err != nil && !notFound(err) {
		return t.showError(ctx, p, chatID, messageID, err)
	}
	total := len(jobs) + len(bp.Skipped) + len(bp.Failed)
	var b strings.Builder
	fmt.Fprintf(&b, ig.batchDone+"\n\n", total, len(jobs), len(bp.Skipped), len(bp.Failed))
	for i, j := range jobs {
		if i == batchListLines {
			fmt.Fprintf(&b, ig.batchJobsMore+"\n", len(jobs)-batchListLines)
			break
		}
		fmt.Fprintf(&b, "· %s — %s%s\n", escape(short(j.RootName, 40)), stateEmoji(j.State), tx.stateLabel(j.State))
	}
	b.WriteString(t.batchMissLines(tx, append(append([]batchMiss(nil), bp.Skipped...), bp.Failed...)))
	if bp.Extra > 0 {
		fmt.Fprintf(&b, "%s\n", fmt.Sprintf(ig.batchTooMany, maxBatchLinks, bp.Extra))
	}
	bp.Items = nil
	payload := bp.json()
	line, queueRow := t.queueControls(ctx, p, tx, payload)
	if line != "" {
		b.WriteString("\n" + line)
	}
	rows := [][]button{{act(ig.batchRefresh, "links_view", bp.ID, 0, payload), nav(tx.jobs, "jobs")}}
	if queueRow != nil {
		rows = append(rows, queueRow)
	}
	rows = append(rows, []button{nav(tx.back, "home")})
	return t.showCard(ctx, p, chatID, messageID, strings.TrimRight(b.String(), "\n"), rows...)
}
