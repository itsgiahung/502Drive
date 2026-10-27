package telegram

import (
	"context"
	"fmt"
	"strings"

	"github.com/itsgiahung/502Drive/internal/app"
)

// collectionJobsMax is how many of a source's jobs the card lists.
const collectionJobsMax = 8

// collectionJobsCard lists the newest jobs of one source, read through the
// server-side collection filter (GET /v1/jobs?collection_id=). The collection
// is read in the caller's scope first, so another person's source answers
// "not found" and no job of it is listed.
func (t *Telegram) collectionJobsCard(ctx context.Context, p app.Principal, chatID int64, messageID int, collectionID string) error {
	tx := t.text(ctx, p)
	ig := tx.ig()
	c, err := t.svc.Collection(ctx, p, collectionID)
	if err != nil {
		return t.showError(ctx, p, chatID, messageID, err)
	}
	jobs, _, err := t.jobsWhere(ctx, p, false, "", collectionID, collectionJobsMax)
	if err != nil {
		return t.showError(ctx, p, chatID, messageID, err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, ig.colJobsTitle+"\n", escape(short(c.SourceName, 50)), len(jobs))
	if len(jobs) == 0 {
		b.WriteString(ig.colJobsNone)
	}
	rows := make([][]button, 0, len(jobs)+1)
	for _, j := range jobs {
		fmt.Fprintf(&b, "· %s — %s\n", escape(short(j.RootName, 40)), tx.stateLabel(j.State))
		rows = append(rows, []button{act(short(j.RootName, 24)+" · "+tx.stateLabel(j.State), "job", j.ID, j.Revision, "")})
	}
	rows = append(rows, []button{act(tx.backBtn, "collection", collectionID, 0, ""), nav(tx.jobs, "jobs")})
	return t.showCard(ctx, p, chatID, messageID, strings.TrimRight(b.String(), "\n"), rows...)
}
