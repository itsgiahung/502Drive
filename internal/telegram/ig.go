package telegram

import (
	"context"
	"errors"

	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/state"
)

// Batch IG wires the backend API of batches C2 and H into the chat: several
// links in one message (S6), the queue pause and order (S6), the deep scan on
// the preview card (S7), the after-copy sync choice (S8), the problem report
// file (X1) and the search-backed /find. Every action below goes through the
// same app.Service call the CLI and the HTTP API use, so authorization,
// idempotency and revisions are the service's, not the chat's.

// actIG routes the callback actions that batch IG added. act() falls through
// to it, so flow.go and handle.go carry no batch-IG logic.
func (t *Telegram) actIG(ctx context.Context, p app.Principal, st state.Callback, tx texts) error {
	switch st.Action {
	case "links_go":
		return t.actLinksGo(ctx, p, st)
	case "links_view":
		return t.actLinksView(ctx, p, st)
	case "queue_pause", "queue_resume":
		return t.actQueue(ctx, p, st)
	case "job_move":
		return t.actJobMove(ctx, p, st)
	case "job_report":
		return t.actJobReport(ctx, p, st)
	case "deep_start":
		return t.actDeepStart(ctx, p, st, tx)
	case "deep_refresh":
		return t.actDeepRefresh(ctx, p, st, tx)
	case "deep_cancel":
		return t.actDeepCancel(ctx, p, st, tx)
	case "aftersync_set":
		return t.actAfterSyncDefault(ctx, p, st)
	case "col_jobs":
		return t.collectionJobsCard(ctx, p, st.ChatID, st.MessageID, st.Object)
	}
	return nil
}

// codeOf is the stable error code of err, "internal_error" when unknown.
func codeOf(err error) string {
	if ae := app.Classify(err); ae != nil && ae.Code != "" {
		return ae.Code
	}
	return "internal_error"
}

// codeText is the chat-language sentence for a stable error code, falling back
// to the code itself so a new server code is never hidden.
func codeText(tx texts, code string) string {
	if v := tx.errMsg[code]; v != "" {
		return v
	}
	if v := tx.g().errMsg[code]; v != "" {
		return v
	}
	return code
}

func notFound(err error) bool { return errors.Is(err, state.ErrNotFound) }

// igPageSize is the page size of the server-side job queries.
const igPageSize = 100

// jobsWhere pages the caller's jobs through the server-side filters (state and
// collection, "" for any) with a cursor, newest first, up to max rows.
// truncated says the cap cut the list short.
func (t *Telegram) jobsWhere(ctx context.Context, p app.Principal, all bool, st, collectionID string, max int) (jobs []state.Job, truncated bool, err error) {
	cursor := int64(0)
	for len(jobs) < max {
		page, next, perr := t.svc.Jobs(ctx, p, all, st, collectionID, min(igPageSize, max-len(jobs)), cursor)
		if perr != nil {
			return nil, false, perr
		}
		jobs = append(jobs, page...)
		if next == 0 { // ListJobs cursors fall; 0 means the last page
			return jobs, false, nil
		}
		cursor = next
	}
	return jobs, true, nil
}
