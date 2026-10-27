package telegram

import (
	"context"
	"fmt"
	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/i18n"
	"github.com/itsgiahung/502Drive/internal/present"
	"github.com/itsgiahung/502Drive/internal/state"
	"hash/fnv"
	"sort"
	"strconv"
	"strings"
	"time"
)

const batchNoticeWindow = 3 * time.Second

func (t *Telegram) queueBatchNotices(ctx context.Context) {
	pending, err := t.store.OpenBatchNotices(ctx)
	if err != nil {
		return
	}
	notifier := NewNotifier(NotifierOptions{Service: t.svc, OwnerID: t.ownerID, Now: t.now})
	for _, batch := range pending {
		if t.now().Before(batch.At.Add(batchNoticeWindow)) {
			continue
		}
		p := app.Principal{ID: batch.Principal, Role: app.RoleOperator}
		jobs, err := t.svc.Batch(ctx, p, batch.BatchID)
		if err != nil {
			continue
		}
		complete, _, _ := batchCounts(jobs)
		if !complete {
			continue
		}
		prefs, err := t.svc.Preferences(ctx, p)
		if err != nil {
			continue
		}
		kind := noticeCompleted
		for _, j := range jobs {
			if j.State == state.JobFailed || j.State == state.JobPartial {
				kind = noticeFailed
			}
		}
		for _, j := range jobs {
			if j.State == state.JobBlocked {
				kind = noticeAction
			}
		}
		enabled := notifyEnabled(prefs, kind)
		chat := int64(0)
		if enabled {
			chat, _, err = notifier.chat(ctx, p.ID)
			if err != nil {
				continue
			}
		}
		if err = t.store.FinishBatchNotice(ctx, p.ID, batch.BatchID, chat, t.now()); err != nil {
			t.log.Error("telegram: cannot queue batch summary")
		}
	}
}
func batchCounts(jobs []state.Job) (complete bool, needs, done int) {
	complete = len(jobs) > 0
	for _, j := range jobs {
		switch j.State {
		case state.JobSucceeded:
			done++
		case state.JobFailed, state.JobPartial, state.JobBlocked:
			needs++
		case state.JobCancelled:
		default:
			complete = false
		}
	}
	return
}
func batchRevision(jobs []state.Job) int {
	rows := make([]string, 0, len(jobs))
	for _, j := range jobs {
		rows = append(rows, j.ID+":"+strconv.Itoa(j.Revision))
	}
	sort.Strings(rows)
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.Join(rows, ";")))
	return int(h.Sum32() & 0x7fffffff)
}
func (t *Telegram) deliverBatchNotice(ctx context.Context, m state.Outbox) {
	if t.holdForQuiet(ctx, m) {
		return
	}
	p := app.Principal{ID: m.Principal, Role: app.RoleOperator}
	id := strings.TrimPrefix(m.Note, state.BatchNoticePrefix)
	jobs, err := t.svc.Batch(ctx, p, id)
	if err != nil {
		t.retry(ctx, m, "unavailable", 30*time.Second)
		return
	}
	complete, needs, done := batchCounts(jobs)
	if !complete {
		t.retry(ctx, m, "batch_running", batchNoticeWindow)
		return
	}
	tx := t.text(ctx, p)
	lang := string(tx.lang)
	rev := batchRevision(jobs)
	text := escape(present.BatchSummary(id, done, len(jobs), needs, lang))
	rows := [][]button{{act(i18n.T(lang, i18n.BatchViewIssues), "batch_issues", id, rev, "0")}}
	if needs > 0 {
		rows = append(rows, []button{act(i18n.T(lang, i18n.BatchRetryFailed), "batch_retry_failed", id, rev, "")})
	}
	claimed, err := t.store.FenceBatchSend(ctx, m.ID)
	if err != nil || !claimed {
		return
	}
	mid, err := t.panel(ctx, p, m.ChatID, 0, text, rows, false)
	if err == nil {
		_ = t.store.ConfirmBatchSend(ctx, m.ID, mid)
		return
	}
	// Only an explicit rate rejection proves that no message was created.
	if sendKind(err) == "rate_limited" {
		wait, _ := retryAfterOf(err)
		if wait <= 0 {
			wait = time.Second
		}
		_ = t.store.RequeueRejectedBatchSend(ctx, m.ID, t.now().Add(wait))
		return
	}
	t.log.Warn("telegram: batch summary delivery unconfirmed", "code", sendKind(err))
}
func (t *Telegram) batchNoticeAction(ctx context.Context, p app.Principal, st state.Callback) error {
	jobs, err := t.svc.Batch(ctx, p, st.Object)
	if err != nil {
		return err
	}
	if batchRevision(jobs) != st.Revision {
		return state.ErrStale
	}
	tx := t.text(ctx, p)
	lang := string(tx.lang)
	if st.Action == "batch_retry_failed" {
		for _, j := range jobs {
			if j.State == state.JobFailed || j.State == state.JobPartial || j.State == state.JobBlocked {
				if _, err = t.svc.JobAction(ctx, p, "tg-cb:"+st.ID+":"+j.ID, j.ID, "retry", j.Revision); err != nil {
					return err
				}
			}
		}
		return t.showCard(ctx, p, st.ChatID, st.MessageID, i18n.T(lang, i18n.BatchRetryAccepted), []button{nav(tx.jobs, "jobs")})
	}
	offset, _ := strconv.Atoi(st.Payload)
	offset = max(0, offset)
	end := min(len(jobs), offset+8)
	var rows [][]button
	var b strings.Builder
	b.WriteString(i18n.T(lang, i18n.BatchViewIssues) + "\n")
	for _, j := range jobs[min(offset, len(jobs)):end] {
		fmt.Fprintf(&b, "%s · %s\n", escape(short(j.SourceName, 40)), escape(present.StateLabel(j.State, lang)))
		if hasProblems(j) {
			rows = append(rows, []button{act(short(j.SourceName, 30), "job_report", j.ID, j.Revision, "")})
		}
	}
	if end < len(jobs) {
		rows = append(rows, []button{act(tx.nextBtn, "batch_issues", st.Object, st.Revision, strconv.Itoa(end))})
	}
	return t.showCard(ctx, p, st.ChatID, st.MessageID, b.String(), rows...)
}
