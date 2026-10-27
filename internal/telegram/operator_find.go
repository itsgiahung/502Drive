package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/i18n"
	"github.com/itsgiahung/502Drive/internal/present"
	"github.com/itsgiahung/502Drive/internal/state"
	"strings"
)

type libraryFindPayload struct {
	Query  string `json:"query"`
	Cursor int64  `json:"cursor"`
}

func (t *Telegram) findLibrary(ctx context.Context, p app.Principal, chat int64, mid int, q libraryFindPayload) error {
	tx := t.text(ctx, p)
	lang := string(tx.lang)
	cols, next, err := t.svc.FindLibrary(ctx, p, q.Query, 10, q.Cursor)
	if err != nil {
		return t.showError(ctx, p, chat, mid, err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, i18n.T(lang, i18n.LibraryFindTitle)+"\n", escape(q.Query))
	fmt.Fprintln(&b, i18n.T(lang, i18n.LibraryFindScope))
	if len(cols) == 0 {
		fmt.Fprintln(&b, tx.g().findNone)
	}
	var rows [][]button
	for _, c := range cols {
		fmt.Fprintf(&b, "· %s\n", escape(short(c.SourceName, 60)))
		rows = append(rows, []button{act(short(c.SourceName, 40), "find_library_collection", c.ID, c.Revision, "")})
	}
	if next != 0 {
		raw, _ := json.Marshal(libraryFindPayload{q.Query, next})
		rows = append(rows, []button{act(tx.nextBtn, "find_library", "", 0, string(raw))})
	}
	return t.showCard(ctx, p, chat, mid, b.String(), append(rows, []button{nav(tx.back, "home")})...)
}
func (t *Telegram) libraryFindAction(ctx context.Context, p app.Principal, st state.Callback) error {
	if st.Action == "find_library" {
		var q libraryFindPayload
		if json.Unmarshal([]byte(st.Payload), &q) != nil {
			return app.Fail(400, "invalid_request", "Invalid library search.")
		}
		return t.findLibrary(ctx, p, st.ChatID, st.MessageID, q)
	}
	c, err := t.svc.Collection(ctx, p, st.Object)
	if err != nil {
		return t.showError(ctx, p, st.ChatID, st.MessageID, err)
	}
	if c.Revision != st.Revision {
		return t.showError(ctx, p, st.ChatID, st.MessageID, state.ErrStale)
	}
	tx := t.text(ctx, p)
	lines := []string{i18n.T(string(tx.lang), i18n.LibraryFindScope), escape(c.SourceName), escape(present.StateLabel(c.State, string(tx.lang)))}
	if c.ResultURL != nil {
		lines = append(lines, escape(*c.ResultURL))
	}
	return t.showCard(ctx, p, st.ChatID, st.MessageID, strings.Join(lines, "\n"), []button{nav(tx.back, "home")})
}
