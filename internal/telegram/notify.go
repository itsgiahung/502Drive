package telegram

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-telegram/bot"

	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/state"
)

// notifyLoop delivers queued progress/final messages. It is the only writer of
// panel edits, so a job gets at most one edit per EditInterval.
func (t *Telegram) notifyLoop(ctx context.Context) {
	interval := t.editEvery
	if interval > time.Second {
		interval = time.Second
	}
	if interval < 50*time.Millisecond {
		interval = 50 * time.Millisecond
	}
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			t.flush(ctx)
		}
	}
}

func (t *Telegram) flush(ctx context.Context) {
	due, err := t.store.DueOutbox(ctx, t.now(), 10)
	if err != nil {
		t.log.Error("telegram: cannot read the outbox", "err", err)
		return
	}
	for _, m := range due {
		if ctx.Err() != nil {
			return
		}
		t.deliver(ctx, m)
	}
}

func (t *Telegram) deliver(ctx context.Context, m state.Outbox) {
	if m.Kind == "final" {
		t.deliverFinal(ctx, m)
		return
	}
	callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	p := app.Principal{ID: m.Principal, Role: app.RoleOperator}
	job, err := t.svc.Job(callCtx, p, m.JobID)
	switch {
	case errors.Is(err, state.ErrNotFound):
		t.dead(callCtx, m, "not_found")
		return
	case err != nil:
		t.retry(callCtx, m, "unavailable", 30*time.Second)
		return
	}
	tx := t.textFor(callCtx, m.Principal)
	text := jobTextFrame(tx, job, int(t.now().Unix()/3))
	hash := hashText(text)
	if m.MessageID != 0 && hash == m.LastHash {
		// Nothing to say: a repeated edit would only burn the rate budget.
		t.finishOrDefer(callCtx, m, job, hash, m.MessageID)
		return
	}
	var rows [][]button
	if m.MessageID == 0 || job.State != m.LastState {
		rows = t.jobButtons(tx, job)
	}
	mid, err := t.panel(callCtx, p, m.ChatID, m.MessageID, text, rows, false)
	if err == nil {
		t.finishOrDefer(callCtx, m, job, hash, mid)
		return
	}
	switch sendKind(err) {
	case "not_modified":
		t.finishOrDefer(callCtx, m, job, hash, m.MessageID)
	case "gone":
		// The panel was deleted: one replacement message, then remember it.
		mid, sendErr := t.panel(callCtx, p, m.ChatID, 0, text, t.jobButtons(tx, job), false)
		if sendErr != nil {
			t.storeFailure(callCtx, m, sendErr)
			return
		}
		t.finishOrDefer(callCtx, m, job, hash, mid)
	case "forbidden":
		// Blocked bot: undeliverable, but the job is untouched.
		t.dead(callCtx, m, "forbidden")
	case "rate_limited":
		wait, _ := retryAfterOf(err)
		if wait <= 0 {
			wait = time.Second
		}
		t.retry(callCtx, m, "rate_limited", wait)
	default:
		t.storeFailure(callCtx, m, err)
	}
}

// deliverFinal sends one notification about work the user did not start in a
// chat. It never edits a panel and never touches the job.
func (t *Telegram) deliverFinal(ctx context.Context, m state.Outbox) {
	callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	p := app.Principal{ID: m.Principal, Role: app.RoleOperator}
	tx := t.textFor(callCtx, m.Principal)
	var text string
	if m.JobID != "" {
		job, err := t.svc.Job(callCtx, p, m.JobID)
		switch {
		case errors.Is(err, state.ErrNotFound):
			t.dead(callCtx, m, "not_found")
			return
		case err != nil:
			t.retry(callCtx, m, "unavailable", 30*time.Second)
			return
		}
		text = finalNoticeText(tx, job)
	} else {
		text = fmt.Sprintf(tx.syncErrorNotice, escape(short(m.Note, 120)))
	}
	rows := [][]button{{nav(tx.jobs, "jobs"), nav(tx.back, "home")}}
	mid, err := t.panel(callCtx, p, m.ChatID, 0, text, rows, false)
	if err == nil {
		if err = t.store.SentOutbox(callCtx, m.ID, mid); err != nil {
			t.log.Error("telegram: cannot finish a notification", "err", err)
		}
		return
	}
	switch sendKind(err) {
	case "not_modified":
		// Sending, not editing: treat as delivered so it is not retried.
		_ = t.store.SentOutbox(callCtx, m.ID, 0)
	case "forbidden":
		t.dead(callCtx, m, "forbidden")
	case "rate_limited":
		wait, _ := retryAfterOf(err)
		if wait <= 0 {
			wait = time.Second
		}
		t.retry(callCtx, m, "rate_limited", wait)
	default:
		t.storeFailure(callCtx, m, err)
	}
}

// finalNoticeText is the message body for a finished job.
func finalNoticeText(tx texts, job state.Job) string {
	header := tx.noticeCompleted
	switch job.State {
	case state.JobPartial, state.JobFailed:
		header = tx.noticeFailed
	case state.JobBlocked:
		header = tx.noticeAction
	}
	return "<b>" + header + "</b>\n" + jobText(tx, job)
}

func (t *Telegram) finishOrDefer(ctx context.Context, m state.Outbox, job state.Job, hash string, messageID int) {
	if state.IsTerminal(job.State) {
		if err := t.store.SentOutbox(ctx, m.ID, messageID); err != nil {
			t.log.Error("telegram: cannot finish notification", "err", err)
		}
		return
	}
	if err := t.store.DeferOutbox(ctx, m.ID, t.now().Add(t.editEvery), messageID, hash, job.State); err != nil {
		t.log.Error("telegram: cannot reschedule notification", "err", err)
	}
}

func (t *Telegram) storeFailure(ctx context.Context, m state.Outbox, err error) {
	if sendKind(err) == "bad_request" {
		t.dead(ctx, m, "bad_request")
		return
	}
	wait := time.Duration(min(m.Attempts+1, 5)) * 15 * time.Second
	t.retry(ctx, m, "unavailable", wait)
}

func (t *Telegram) retry(ctx context.Context, m state.Outbox, code string, wait time.Duration) {
	if err := t.store.RetryOutbox(ctx, m.ID, t.now().Add(wait), code); err != nil {
		t.log.Error("telegram: cannot retry notification", "err", err)
	}
}

func (t *Telegram) dead(ctx context.Context, m state.Outbox, code string) {
	t.log.Warn("telegram: notification undeliverable", "job", m.JobID, "code", code)
	if err := t.store.DeadOutbox(ctx, m.ID, code); err != nil {
		t.log.Error("telegram: cannot mark notification", "err", err)
	}
}

// sendKind classifies a Bot API send/edit failure.
func sendKind(err error) string {
	var tm *bot.TooManyRequestsError
	if errors.As(err, &tm) {
		return "rate_limited"
	}
	switch {
	case errors.Is(err, bot.ErrorForbidden):
		return "forbidden"
	case errors.Is(err, bot.ErrorBadRequest):
		msg := strings.ToLower(err.Error())
		switch {
		case strings.Contains(msg, "message is not modified"):
			return "not_modified"
		case strings.Contains(msg, "message to edit not found"),
			strings.Contains(msg, "message can't be edited"),
			strings.Contains(msg, "message_id_invalid"):
			return "gone"
		}
		return "bad_request"
	}
	return "unavailable"
}
