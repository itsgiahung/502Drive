package telegram

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/state"
)

// /find NAME (S10, IG): search the whole Drive by name through the same
// application command as GET /v1/drive/search (up to 20 results; owner only,
// because it exposes the owner's Drive names) and let the user pick one as the
// source of a copy. Google decides what the account may see.

const (
	findMaxResults = 10
	findMaxQuery   = 80
)

func (t *Telegram) findCmd(ctx context.Context, p app.Principal, chatID int64, query string) error {
	tx := t.text(ctx, p)
	g := tx.g()
	q := strings.TrimSpace(query)
	if q == "" {
		return t.showCard(ctx, p, chatID, 0, g.findUsage, []button{nav(tx.back, "home")})
	}
	if len([]rune(q)) > findMaxQuery {
		q = string([]rune(q)[:findMaxQuery])
	}
	matches, err := t.svc.Search(ctx, p, q)
	if err != nil {
		return t.showError(ctx, p, chatID, 0, err)
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].IsFolder() != matches[j].IsFolder() {
			return matches[i].IsFolder()
		}
		return strings.ToLower(matches[i].Name) < strings.ToLower(matches[j].Name)
	})
	var b strings.Builder
	fmt.Fprintf(&b, g.findTitle+"\n", escape(q))
	if len(matches) == 0 {
		b.WriteString(g.findNone + "\n")
	}
	var rows [][]button
	for i, f := range matches {
		if i == findMaxResults {
			fmt.Fprintf(&b, "\n"+g.findMore+"\n", len(matches)-findMaxResults)
			break
		}
		icon := "📄"
		if f.IsFolder() {
			icon = "📁"
		}
		fmt.Fprintf(&b, "· %s %s\n", icon, escape(short(f.Name, 60)))
		ref := ""
		if f.ResourceKey != "" { // shared-by-link items need their key to open
			ref = "https://drive.google.com/open?id=" + f.ID + "&resourcekey=" + f.ResourceKey
		}
		rows = append(rows, []button{act(icon+" "+short(f.Name, 40), "find_pick", f.ID, 0, ref)})
	}
	b.WriteString("\n<i>" + g.findScope + "</i>")
	rows = append(rows, []button{nav(tx.back, "home")})
	return t.showCard(ctx, p, chatID, 0, b.String(), rows...)
}

// actFindPick makes the chosen item the source of a copy preview.
func (t *Telegram) actFindPick(ctx context.Context, p app.Principal, st state.Callback) error {
	source := st.Object
	if st.Payload != "" {
		source = st.Payload
	}
	return t.paste(ctx, p, st.ChatID, st.MessageID, source, "tg-cb:"+st.ID)
}
