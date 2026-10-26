package telegram

import (
	"context"
	"fmt"
	"strings"

	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/state"
)

// bulkStates are the job states /cancelall may act on. A finished job is never
// touched: the buttons of a completed card keep meaning "this job".
var bulkStates = map[string]bool{
	state.JobQueued: true, state.JobRunning: true, state.JobPausing: true, state.JobPaused: true,
	state.JobReconciling: true, state.JobCancelling: true, state.JobBlocked: true,
}

// openJobs lists the caller's own jobs in a state that can still be cancelled.
// The principal scope is the caller's: an operator never sees the owner's jobs.
func (t *Telegram) openJobs(ctx context.Context, p app.Principal) ([]state.Job, error) {
	var out []state.Job
	cursor := int64(0)
	for page := 0; page < 5; page++ {
		jobs, next, err := t.svc.Jobs(ctx, p, false, "", 100, cursor)
		if err != nil {
			return nil, err
		}
		for _, j := range jobs {
			if bulkStates[j.State] {
				out = append(out, j)
			}
		}
		if next <= cursor {
			break
		}
		cursor = next
	}
	return out, nil
}

// cancelAllAsk shows the red confirmation button. Nothing is cancelled until
// the user taps it, and the callback is one-shot.
func (t *Telegram) cancelAllAsk(ctx context.Context, p app.Principal, chatID int64, messageID int) error {
	tx := t.text(ctx, p)
	jobs, err := t.openJobs(ctx, p)
	if err != nil {
		return t.showError(ctx, p, chatID, messageID, err)
	}
	if len(jobs) == 0 {
		return t.showCard(ctx, p, chatID, messageID, "<b>"+tx.jobs+"</b>\n"+tx.cancelAllNone,
			[]button{nav(tx.back, "home")})
	}
	var b strings.Builder
	fmt.Fprintf(&b, tx.cancelAllAsk, len(jobs))
	b.WriteString("\n")
	for i, j := range jobs {
		if i >= 5 {
			fmt.Fprintf(&b, "\n+%d", len(jobs)-i)
			break
		}
		fmt.Fprintf(&b, "\n· %s — %s", escape(short(j.RootName, 32)), tx.stateLabel(j.State))
	}
	return t.showCard(ctx, p, chatID, messageID, b.String(),
		[]button{styled(act(tx.cancelAll, "cancelall_do", "", 0, ""), "danger")},
		[]button{nav(tx.back, "home")})
}

// cancelAllDo cancels each open job through the same application command as
// the CLI. A job that moved on meanwhile is skipped, not an error; the card
// always ends with the truthful counts.
func (t *Telegram) cancelAllDo(ctx context.Context, p app.Principal, st state.Callback) error {
	tx := t.text(ctx, p)
	jobs, err := t.openJobs(ctx, p)
	if err != nil {
		return t.showError(ctx, p, st.ChatID, st.MessageID, err)
	}
	canceled, skipped, failed := 0, 0, 0
	for _, j := range jobs {
		_, err := t.svc.JobAction(ctx, p, "tg-cb:"+st.ID+":"+j.ID, j.ID, "cancel", j.Revision)
		if err == nil {
			canceled++
			continue
		}
		ae := app.Classify(err)
		switch {
		case ae != nil && (ae.Status == 409 || ae.Status == 404):
			skipped++
		default:
			failed++
			t.log.Warn("telegram: cannot cancel a job", "job", j.ID, "err", err)
		}
	}
	rows := [][]button{{nav(tx.jobs, "jobs"), nav(tx.back, "home")}}
	return t.showCard(ctx, p, st.ChatID, st.MessageID, fmt.Sprintf(tx.cancelAllDone, canceled, skipped, failed), rows...)
}
