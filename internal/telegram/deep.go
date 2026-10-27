package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/present"
	"github.com/itsgiahung/502Drive/internal/state"
)

// Deep preflight (S7) on the preview card. "Deep scan" admits a second, deep
// plan for the same choices; the daemon walks the source read-only in the
// background. The card shows what is known so far and refuses to call a
// partial count a total. Nothing is written to Drive and the copy button on
// the card submits that same plan.

// defaultDeepWait is how long "Deep scan" waits for a small tree to finish
// before it shows the running card (a refresh button reads the rest).
const defaultDeepWait = 1500 * time.Millisecond

// deepWaitFor is the wait of this bot: zero means the default, a negative
// value means do not wait (tests).
func (t *Telegram) deepWaitFor() time.Duration {
	switch {
	case t.deepWait < 0:
		return 0
	case t.deepWait == 0:
		return defaultDeepWait
	}
	return t.deepWait
}

// actDeepStart starts a deep scan for the preview the button belongs to.
func (t *Telegram) actDeepStart(ctx context.Context, p app.Principal, st state.Callback, tx texts) error {
	var pl planPayload
	if json.Unmarshal([]byte(st.Payload), &pl) != nil || pl.Source == "" {
		return t.showError(ctx, p, st.ChatID, st.MessageID, app.Fail(409, "plan_changed", tx.confirmDest))
	}
	req := pl.request()
	req.Deep = true
	view, _, err := t.svc.Inspect(ctx, p, "tg-cb:"+st.ID+":deep", req)
	if err != nil {
		return t.showError(ctx, p, st.ChatID, st.MessageID, err)
	}
	// Small trees finish at once: wait a moment so the first card is the answer.
	deadline := time.Now().Add(t.deepWaitFor())
	for view.Deep != nil && view.Deep.State == "running" && time.Now().Before(deadline) {
		time.Sleep(25 * time.Millisecond)
		if v, perr := t.svc.Plan(ctx, p, view.ID); perr == nil {
			view = v
		}
	}
	return t.deepCard(ctx, p, st.ChatID, st.MessageID, view.ID, st.Payload)
}

func (t *Telegram) actDeepRefresh(ctx context.Context, p app.Principal, st state.Callback, tx texts) error {
	return t.deepCard(ctx, p, st.ChatID, st.MessageID, st.Object, st.Payload)
}

// actDeepCancel stops a running scan; the card then says it is not a total.
func (t *Telegram) actDeepCancel(ctx context.Context, p app.Principal, st state.Callback, tx texts) error {
	if _, err := t.svc.CancelPreflight(ctx, p, st.Object); err != nil {
		return t.showError(ctx, p, st.ChatID, st.MessageID, err)
	}
	return t.deepCard(ctx, p, st.ChatID, st.MessageID, st.Object, st.Payload)
}

// deepCard renders the scan state of one plan.
func (t *Telegram) deepCard(ctx context.Context, p app.Principal, chatID int64, messageID int, planID, payload string) error {
	tx := t.text(ctx, p)
	view, err := t.svc.Plan(ctx, p, planID)
	if err != nil {
		return t.showError(ctx, p, chatID, messageID, err)
	}
	return t.showCard(ctx, p, chatID, messageID, deepText(tx, view.Plan), t.deepButtons(tx, view.Plan, payload)...)
}

func (t *Telegram) deepButtons(tx texts, plan state.Plan, payload string) [][]button {
	ig := tx.ig()
	rows := [][]button{{styled(act(tx.copyNow, "copy", plan.ID, plan.Revision, payload), "success")}}
	if plan.Deep != nil && plan.Deep.State == "running" {
		rows = append(rows, []button{act(ig.deepRefresh, "deep_refresh", plan.ID, plan.Revision, payload),
			styled(act(ig.deepCancel, "deep_cancel", plan.ID, plan.Revision, payload), "danger")})
	} else {
		rows = append(rows, []button{act(ig.deepAgain, "deep_start", plan.ID, plan.Revision, payload)})
	}
	return append(rows, []button{act(tx.backBtn, "back_preview", plan.ID, plan.Revision, payload)})
}

// deepText is the body of the scan card. A scan that did not complete shows
// its counts as "so far", never as the total.
func deepText(tx texts, plan state.Plan) string {
	ig := tx.ig()
	var b strings.Builder
	fmt.Fprintf(&b, ig.deepTitle+"\n", escape(short(plan.SourceName, 60)))
	d := plan.Deep
	if d == nil {
		return b.String()
	}
	if d.State == "running" {
		fmt.Fprintf(&b, "\n"+ig.deepRunning+"\n", d.Scanned, d.Files, d.Folders)
		return strings.TrimRight(b.String(), "\n")
	}
	if line := ig.deepStates[d.State]; line != "" {
		fmt.Fprintf(&b, "\n%s\n", line)
	}
	label := ig.deepTotals
	if !d.Complete {
		label = ig.deepPartial
	}
	fmt.Fprintf(&b, "\n"+label+"\n", d.Files, d.Folders, present.HumanBytes(d.Bytes))
	if d.Native > 0 {
		fmt.Fprintf(&b, ig.deepNative+"\n", d.Native)
	}
	if d.Shortcuts > 0 {
		fmt.Fprintf(&b, ig.deepShortcuts+"\n", d.Shortcuts)
	}
	if d.CannotCopy > 0 {
		fmt.Fprintf(&b, ig.deepCannot+"\n", d.CannotCopy)
	}
	if d.Unreadable > 0 {
		fmt.Fprintf(&b, ig.deepUnreadable+"\n", d.Unreadable)
	}
	if d.Complete {
		if d.EstimateSecs != nil {
			fmt.Fprintf(&b, ig.deepEstimate+"\n", present.HumanDuration(time.Duration(*d.EstimateSecs)*time.Second, string(tx.lang)))
		}
		if d.StorageFree != nil {
			fmt.Fprintf(&b, ig.deepFree+"\n", present.HumanBytes(max(*d.StorageFree, 0)))
		}
		if d.DailyLeft != nil {
			fmt.Fprintf(&b, ig.deepDaily+"\n", present.HumanBytes(max(*d.DailyLeft, 0)))
		}
		for _, w := range d.Warnings {
			line := ig.deepWarn[w]
			if line == "" {
				line = "⚠️ " + w
			}
			fmt.Fprintf(&b, "%s\n", escape(line))
		}
	}
	return strings.TrimRight(b.String(), "\n")
}
