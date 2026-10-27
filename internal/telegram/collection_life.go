package telegram

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/state"
)

// Collection lifecycle from chat (L1): stop following, follow again, forget the
// local records, and move the destination copy to the Drive trash. Every
// destructive step is a preview card followed by one red confirmation. The
// confirmation is a one-shot callback bound to the collection revision that the
// preview showed, and it expires after lifecycleConfirmTTL.

const lifecycleConfirmTTL = 5 * time.Minute

// confirmStale reports a confirmation that was shown too long ago.
func (t *Telegram) confirmStale(st state.Callback) bool {
	return t.now().Sub(st.CreatedAt) > lifecycleConfirmTTL
}

// expiredCard tells the user nothing changed; the collection card is one tap away.
func (t *Telegram) expiredCard(ctx context.Context, p app.Principal, st state.Callback) error {
	tx := t.text(ctx, p)
	return t.showCard(ctx, p, st.ChatID, st.MessageID, tx.g().confirmExpired,
		[]button{act(tx.library, "collection", st.Object, 0, ""), nav(tx.back, "home")})
}

// lifecycleRows is the live collection's lifecycle button (only Stop is
// offered while it is followed).
func (t *Telegram) lifecycleRows(tx texts, c state.Collection) [][]button {
	return [][]button{{act(tx.g().retireBtn, "retire_ask", c.ID, c.Revision, "")}}
}

// retiredCard is the card of a collection that is no longer followed.
func (t *Telegram) retiredCard(ctx context.Context, p app.Principal, chatID int64, messageID int, c state.Collection, note string) error {
	tx := t.text(ctx, p)
	g := tx.g()
	var b strings.Builder
	fmt.Fprintf(&b, "<b>%s</b>\n%s\n", escape(short(c.SourceName, 60)), g.retiredTag)
	if c.RetiredAt != nil {
		fmt.Fprintf(&b, "%s\n", c.RetiredAt.UTC().Format("2006-01-02 15:04 MST"))
	}
	if note != "" {
		b.WriteString("\n" + note)
	}
	return t.showCard(ctx, p, chatID, messageID, b.String(),
		[]button{act(g.resumeBtn, "col_resume", c.ID, c.Revision, "")},
		[]button{act(g.cleanupBtn, "cleanup_ask", c.ID, c.Revision, ""), act(g.forgetBtn, "forget_ask", c.ID, c.Revision, "")},
		[]button{nav(tx.library, "library"), nav(tx.back, "home")})
}

// retiredButton is the library link to the stopped sources.
func (t *Telegram) retiredCount(ctx context.Context, p app.Principal) int {
	cols, err := t.svc.Collections(ctx, p, true)
	if err != nil {
		return 0
	}
	n := 0
	for _, c := range cols {
		if c.State == state.CollectionRetired {
			n++
		}
	}
	return n
}

// retiredListCard lists the collections that are no longer followed.
func (t *Telegram) retiredListCard(ctx context.Context, p app.Principal, chatID int64, messageID int) error {
	tx := t.text(ctx, p)
	g := tx.g()
	cols, err := t.svc.Collections(ctx, p, true)
	if err != nil {
		return t.showError(ctx, p, chatID, messageID, err)
	}
	var rows [][]button
	for _, c := range cols {
		if c.State != state.CollectionRetired || len(rows) >= 8 {
			continue
		}
		rows = append(rows, []button{act(short(c.SourceName, 40), "collection", c.ID, c.Revision, "")})
	}
	text := "<b>" + g.retiredList + "</b>"
	if len(rows) == 0 {
		text += "\n" + g.retiredEmpty
	}
	rows = append(rows, []button{nav(tx.library, "library"), nav(tx.back, "home")})
	return t.showCard(ctx, p, chatID, messageID, text, rows...)
}

// actRetireAsk shows what stopping does, then waits for the red button.
func (t *Telegram) actRetireAsk(ctx context.Context, p app.Principal, st state.Callback, tx texts) error {
	c, err := t.svc.Collection(ctx, p, st.Object)
	if err != nil {
		return t.showError(ctx, p, st.ChatID, st.MessageID, err)
	}
	g := tx.g()
	return t.showCard(ctx, p, st.ChatID, st.MessageID, fmt.Sprintf(g.retireAsk, escape(short(c.SourceName, 60))),
		[]button{styled(act(g.retireConfirm, "retire_do", c.ID, c.Revision, ""), "danger")},
		[]button{act("↩️ "+tx.cancel, "collection", c.ID, 0, "")})
}

func (t *Telegram) actRetireDo(ctx context.Context, p app.Principal, st state.Callback, tx texts) error {
	if t.confirmStale(st) {
		return t.expiredCard(ctx, p, st)
	}
	if _, err := t.svc.RetireCollection(ctx, p, "tg-cb:"+st.ID, st.Object, st.Revision); err != nil {
		return t.showError(ctx, p, st.ChatID, st.MessageID, err)
	}
	return t.collectionCard(ctx, p, st.ChatID, st.MessageID, st.Object, tx.g().retireDone)
}

func (t *Telegram) actResumeCollection(ctx context.Context, p app.Principal, st state.Callback, tx texts) error {
	if _, err := t.svc.ResumeCollection(ctx, p, "tg-cb:"+st.ID, st.Object, st.Revision); err != nil {
		return t.showError(ctx, p, st.ChatID, st.MessageID, err)
	}
	return t.collectionCard(ctx, p, st.ChatID, st.MessageID, st.Object, tx.g().resumeDone)
}

func (t *Telegram) actForgetAsk(ctx context.Context, p app.Principal, st state.Callback, tx texts) error {
	c, err := t.svc.Collection(ctx, p, st.Object)
	if err != nil {
		return t.showError(ctx, p, st.ChatID, st.MessageID, err)
	}
	if c.State != state.CollectionRetired {
		return t.showError(ctx, p, st.ChatID, st.MessageID, state.ErrCollectionNotRetired)
	}
	g := tx.g()
	return t.showCard(ctx, p, st.ChatID, st.MessageID, fmt.Sprintf(g.forgetAsk, escape(short(c.SourceName, 60))),
		[]button{styled(act(g.forgetConfirm, "forget_do", c.ID, c.Revision, ""), "danger")},
		[]button{act("↩️ "+tx.cancel, "collection", c.ID, 0, "")})
}

func (t *Telegram) actForgetDo(ctx context.Context, p app.Principal, st state.Callback, tx texts) error {
	if t.confirmStale(st) {
		return t.expiredCard(ctx, p, st)
	}
	if err := t.svc.ForgetCollection(ctx, p, "tg-cb:"+st.ID, st.Object, st.Revision); err != nil {
		return t.showError(ctx, p, st.ChatID, st.MessageID, err)
	}
	return t.showCard(ctx, p, st.ChatID, st.MessageID, tx.g().forgetDone,
		[]button{nav(tx.library, "library"), nav(tx.back, "home")})
}

// actCleanupAsk is the preview of the trash move: it admits the dry-run job
// (nothing on Drive changes) and then waits for the red button.
func (t *Telegram) actCleanupAsk(ctx context.Context, p app.Principal, st state.Callback, tx texts) error {
	c, err := t.svc.Collection(ctx, p, st.Object)
	if err != nil {
		return t.showError(ctx, p, st.ChatID, st.MessageID, err)
	}
	dry, err := t.svc.CleanupCollection(ctx, p, "tg-cb:"+st.ID, c.ID, app.CleanupRequest{ExpectedRevision: c.Revision, DryRun: true})
	if err != nil {
		return t.showError(ctx, p, st.ChatID, st.MessageID, err)
	}
	g := tx.g()
	return t.showCard(ctx, p, st.ChatID, st.MessageID, fmt.Sprintf(g.cleanupAsk, escape(short(c.RootName, 60)), escape(dry.ID)),
		[]button{styled(act(g.cleanupConfirm, "cleanup_do", c.ID, c.Revision, ""), "danger")},
		[]button{act(tx.details, "job", dry.ID, dry.Revision, ""), act("↩️ "+tx.cancel, "collection", c.ID, 0, "")})
}

func (t *Telegram) actCleanupDo(ctx context.Context, p app.Principal, st state.Callback, tx texts) error {
	if t.confirmStale(st) {
		return t.expiredCard(ctx, p, st)
	}
	c, err := t.svc.Collection(ctx, p, st.Object)
	if err != nil {
		return t.showError(ctx, p, st.ChatID, st.MessageID, err)
	}
	job, err := t.svc.CleanupCollection(ctx, p, "tg-cb:"+st.ID, c.ID,
		app.CleanupRequest{ExpectedRevision: st.Revision, ConfirmName: c.RootName})
	if err != nil {
		return t.showError(ctx, p, st.ChatID, st.MessageID, err)
	}
	return t.showCard(ctx, p, st.ChatID, st.MessageID, fmt.Sprintf(tx.g().cleanupBegan, escape(job.ID)),
		[]button{act(tx.details, "job", job.ID, job.Revision, ""), nav(tx.library, "library")})
}
