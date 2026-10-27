package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/state"
)

// The job management page (S10): jobs grouped by state, and one tap to pause,
// resume, retry or (after a confirmation) cancel every job of the group. Each
// job goes through the same application command as the single-job buttons, with
// its own current revision, so a job that moved on meanwhile is skipped, never
// forced. The page only ever lists the caller's own jobs.

const (
	manageJobsPerPage = 6
	manageScanPages   = 5
)

var manageFilters = []string{"all", "running", "queued", "paused", "blocked", "failed", "done"}

type managePayload struct {
	F string `json:"f,omitempty"`
	O int    `json:"o,omitempty"`
}

func (mp managePayload) json() string {
	b, _ := json.Marshal(mp)
	return string(b)
}

func decodeManage(payload string) managePayload {
	var mp managePayload
	_ = json.Unmarshal([]byte(payload), &mp)
	for _, f := range manageFilters {
		if mp.F == f {
			return mp
		}
	}
	mp.F = "all"
	return mp
}

func inGroup(group string, j state.Job) bool {
	switch group {
	case "running":
		return j.State == state.JobRunning || j.State == state.JobReconciling || j.State == state.JobPausing || j.State == state.JobCancelling
	case "queued":
		return j.State == state.JobQueued
	case "paused":
		return j.State == state.JobPaused
	case "blocked":
		return j.State == state.JobBlocked
	case "failed":
		return j.State == state.JobFailed || j.State == state.JobPartial
	case "done":
		return j.State == state.JobSucceeded
	}
	return true
}

// batchEligible says which jobs a batch action may try. Quota waits are never
// retried by hand: they resume on their own.
func batchEligible(action string, j state.Job) bool {
	switch action {
	case "pause":
		return j.State == state.JobQueued || j.State == state.JobRunning || j.State == state.JobReconciling
	case "resume":
		return j.State == state.JobPaused
	case "retry":
		return j.State == state.JobFailed || j.State == state.JobPartial || (j.State == state.JobBlocked && !quotaBlocked(j))
	case "cancel":
		return bulkStates[j.State]
	}
	return false
}

// manageJobs lists the caller's jobs of one group (the newest 500 at most).
func (t *Telegram) manageJobs(ctx context.Context, p app.Principal, group string) ([]state.Job, error) {
	var out []state.Job
	cursor := int64(0)
	for page := 0; page < manageScanPages; page++ {
		jobs, next, err := t.svc.Jobs(ctx, p, false, "", 100, cursor)
		if err != nil {
			return nil, err
		}
		for _, j := range jobs {
			if inGroup(group, j) {
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

func countEligible(action string, jobs []state.Job) int {
	n := 0
	for _, j := range jobs {
		if batchEligible(action, j) {
			n++
		}
	}
	return n
}

func (t *Telegram) manageCard(ctx context.Context, p app.Principal, chatID int64, messageID int, mp managePayload) error {
	tx := t.text(ctx, p)
	g := tx.g()
	jobs, err := t.manageJobs(ctx, p, mp.F)
	if err != nil {
		return t.showError(ctx, p, chatID, messageID, err)
	}
	offset := min(max(mp.O, 0), max(len(jobs)-1, 0)/manageJobsPerPage*manageJobsPerPage)
	end := min(offset+manageJobsPerPage, len(jobs))

	var b strings.Builder
	fmt.Fprintf(&b, g.manageTitle+"\n", g.filterNames[mp.F], len(jobs))
	if len(jobs) == 0 {
		b.WriteString(g.manageEmpty)
	}
	var rows [][]button
	// Filter chips first: they are the page's main control.
	var chips []button
	for _, f := range manageFilters {
		label := g.filterNames[f]
		if f == mp.F {
			label = "✓ " + label
		}
		chips = append(chips, act(label, "jobs_mgmt", "", 0, managePayload{F: f}.json()))
	}
	rows = append(rows, chips[:4], chips[4:])
	for _, j := range jobs[offset:end] {
		fmt.Fprintf(&b, "· %s — %s\n", escape(short(j.RootName, 40)), tx.stateLabel(j.State))
		rows = append(rows, []button{act(short(j.RootName, 24)+" · "+tx.stateLabel(j.State), "job", j.ID, j.Revision, "")})
	}
	var pager []button
	if offset > 0 {
		pager = append(pager, act(g.pagePrev, "jobs_mgmt", "", 0, managePayload{F: mp.F, O: max(offset-manageJobsPerPage, 0)}.json()))
	}
	if end < len(jobs) {
		pager = append(pager, act(tx.nextBtn, "jobs_mgmt", "", 0, managePayload{F: mp.F, O: end}.json()))
	}
	if len(pager) > 0 {
		rows = append(rows, pager)
	}
	var batch []button
	if n := countEligible("pause", jobs); n > 0 {
		batch = append(batch, act(fmt.Sprintf(g.batchPause, n), "batch_do", "pause", 0, mp.json()))
	}
	if n := countEligible("resume", jobs); n > 0 {
		batch = append(batch, styled(act(fmt.Sprintf(g.batchResume, n), "batch_do", "resume", 0, mp.json()), "success"))
	}
	if n := countEligible("retry", jobs); n > 0 {
		batch = append(batch, styled(act(fmt.Sprintf(g.batchRetry, n), "batch_do", "retry", 0, mp.json()), "success"))
	}
	if len(batch) > 0 {
		rows = append(rows, batch)
	}
	if n := countEligible("cancel", jobs); n > 0 {
		rows = append(rows, []button{styled(act(fmt.Sprintf(g.batchCancel, n), "batch_cancel_ask", "", 0, mp.json()), "danger")})
	}
	rows = append(rows, []button{nav(tx.jobs, "jobs"), nav(tx.back, "home")})
	return t.showCard(ctx, p, chatID, messageID, b.String(), rows...)
}

// runBatch applies one action to every eligible job of the group and answers
// with the truthful counts.
func (t *Telegram) runBatch(ctx context.Context, p app.Principal, st state.Callback, action string, mp managePayload) error {
	tx := t.text(ctx, p)
	g := tx.g()
	jobs, err := t.manageJobs(ctx, p, mp.F)
	if err != nil {
		return t.showError(ctx, p, st.ChatID, st.MessageID, err)
	}
	done, skipped, failed := 0, 0, 0
	for _, j := range jobs {
		if !batchEligible(action, j) {
			continue
		}
		_, err := t.svc.JobAction(ctx, p, "tg-cb:"+st.ID+":"+j.ID, j.ID, action, j.Revision)
		if err == nil {
			done++
			continue
		}
		if ae := app.Classify(err); ae != nil && (ae.Status == 409 || ae.Status == 404) {
			skipped++
			continue
		}
		failed++
		t.log.Warn("telegram: batch action failed", "job", j.ID, "action", action, "err", err)
	}
	return t.showCard(ctx, p, st.ChatID, st.MessageID,
		fmt.Sprintf(g.batchResult, g.batchActions[action], done, skipped, failed),
		[]button{act(g.manageBtn, "jobs_mgmt", "", 0, mp.json())},
		[]button{nav(tx.jobs, "jobs"), nav(tx.back, "home")})
}

func (t *Telegram) actBatchDo(ctx context.Context, p app.Principal, st state.Callback) error {
	switch st.Object {
	case "pause", "resume", "retry":
		return t.runBatch(ctx, p, st, st.Object, decodeManage(st.Payload))
	}
	return nil
}

// actBatchCancelAsk counts what would be cancelled and waits for the red button.
func (t *Telegram) actBatchCancelAsk(ctx context.Context, p app.Principal, st state.Callback) error {
	tx := t.text(ctx, p)
	g := tx.g()
	mp := decodeManage(st.Payload)
	jobs, err := t.manageJobs(ctx, p, mp.F)
	if err != nil {
		return t.showError(ctx, p, st.ChatID, st.MessageID, err)
	}
	n := countEligible("cancel", jobs)
	if n == 0 {
		return t.showCard(ctx, p, st.ChatID, st.MessageID, g.batchCancelNo,
			[]button{act(g.manageBtn, "jobs_mgmt", "", 0, mp.json())}, []button{nav(tx.back, "home")})
	}
	return t.showCard(ctx, p, st.ChatID, st.MessageID, fmt.Sprintf(g.batchCancelAsk, n, g.filterNames[mp.F]),
		[]button{styled(act(fmt.Sprintf(g.batchCancel, n), "batch_cancel_do", "", 0, mp.json()), "danger")},
		[]button{act("↩️ "+tx.cancel, "jobs_mgmt", "", 0, mp.json())})
}

func (t *Telegram) actBatchCancelDo(ctx context.Context, p app.Principal, st state.Callback) error {
	if t.confirmStale(st) {
		tx := t.text(ctx, p)
		return t.showCard(ctx, p, st.ChatID, st.MessageID, tx.g().confirmExpired,
			[]button{act(tx.g().manageBtn, "jobs_mgmt", "", 0, decodeManage(st.Payload).json())}, []button{nav(tx.back, "home")})
	}
	return t.runBatch(ctx, p, st, "cancel", decodeManage(st.Payload))
}
