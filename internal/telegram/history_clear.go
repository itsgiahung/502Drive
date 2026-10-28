package telegram

import (
	"context"
	"fmt"
	"strconv"

	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/state"
)

// Clear history from chat: a range picker, a dry-run preview, then one
// confirmation. The owner clears by age or everything finished; an operator
// only their own finished jobs. Collections, mappings and Drive are never
// touched, and every tap is a one-shot callback.

var historyDays = []int{7, 30, 90}

// historyRequest turns the picked scope into a service request. scope is a
// number of days, "all" (owner) or "own" (the caller's finished jobs).
func (t *Telegram) historyRequest(ctx context.Context, p app.Principal, scope string, dry bool) (app.HistoryClearRequest, bool) {
	req := app.HistoryClearRequest{DryRun: &dry}
	switch {
	case scope == "own":
		var ids []string
		for _, st := range []string{state.JobSucceeded, state.JobPartial, state.JobFailed, state.JobCancelled} {
			jobs, _, err := t.jobsWhere(ctx, p, false, st, "", 500-len(ids))
			if err != nil {
				return req, false
			}
			for _, j := range jobs {
				ids = append(ids, j.ID)
			}
		}
		req.JobIDs = ids
		return req, len(ids) > 0
	case scope == "all":
		zero := 0
		req.OlderThanDays = &zero
		return req, true
	}
	n, err := strconv.Atoi(scope)
	if err != nil || n < 1 || n > 36500 {
		return req, false
	}
	req.OlderThanDays = &n
	return req, true
}

func (t *Telegram) historyPick(ctx context.Context, p app.Principal, chatID int64, messageID int) error {
	tx := t.text(ctx, p)
	ht := tx.hist()
	if p.Role != app.RoleOwner {
		return t.showCard(ctx, p, chatID, messageID, ht.title+"\n"+ht.pickOwn,
			[]button{act(ht.own, "hist_ask", "", 0, "own")},
			[]button{nav(ht.back, "jobs")})
	}
	row := make([]button, 0, len(historyDays))
	for _, d := range historyDays {
		row = append(row, act(fmt.Sprintf(ht.days, d), "hist_ask", "", 0, strconv.Itoa(d)))
	}
	return t.showCard(ctx, p, chatID, messageID, ht.title+"\n"+ht.pick,
		row,
		[]button{act(ht.all, "hist_ask", "", 0, "all")},
		[]button{nav(ht.back, "jobs")})
}

// historyAsk previews (dry run) and offers the one confirmation button.
func (t *Telegram) historyAsk(ctx context.Context, p app.Principal, st state.Callback) error {
	tx := t.text(ctx, p)
	ht := tx.hist()
	req, ok := t.historyRequest(ctx, p, st.Payload, true)
	if !ok {
		return t.showCard(ctx, p, st.ChatID, st.MessageID, ht.title+"\n"+ht.nothing, []button{nav(ht.back, "jobs")})
	}
	res, err := t.svc.ClearHistory(ctx, p, req)
	if err != nil {
		return t.showError(ctx, p, st.ChatID, st.MessageID, err)
	}
	if res.Hidden == 0 && res.Deleted.JobsPruned == 0 {
		return t.showCard(ctx, p, st.ChatID, st.MessageID, ht.title+"\n"+ht.nothing, []button{nav(ht.back, "jobs")})
	}
	return t.showCard(ctx, p, st.ChatID, st.MessageID,
		ht.title+"\n"+fmt.Sprintf(ht.preview, res.Hidden)+"\n"+ht.keep,
		[]button{styled(act(ht.confirmBtn, "hist_do", "", 0, st.Payload), "danger")},
		[]button{nav(ht.back, "jobs")})
}

// historyDo applies the clear the preview showed.
func (t *Telegram) historyDo(ctx context.Context, p app.Principal, st state.Callback) error {
	tx := t.text(ctx, p)
	ht := tx.hist()
	req, ok := t.historyRequest(ctx, p, st.Payload, false)
	if !ok {
		return t.showCard(ctx, p, st.ChatID, st.MessageID, ht.title+"\n"+ht.nothing, []button{nav(ht.back, "jobs")})
	}
	res, err := t.svc.ClearHistory(ctx, p, req)
	if err != nil {
		return t.showError(ctx, p, st.ChatID, st.MessageID, err)
	}
	return t.showCard(ctx, p, st.ChatID, st.MessageID, ht.title+"\n"+fmt.Sprintf(ht.done, res.Hidden)+"\n"+ht.keep,
		[]button{nav(ht.back, "jobs")})
}
