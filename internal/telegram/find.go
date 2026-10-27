package telegram

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/drive"
	"github.com/itsgiahung/502Drive/internal/state"
)

// /find NAME (S10): list the items of My Drive whose name contains NAME and
// let the user pick one as the source of a copy. It only uses the read-only
// Browse command (owner only, like the folder chooser), so it searches the top
// level of My Drive; a recursive Drive search needs a backend search API (see
// the batch-G report, "Can backend").

const (
	findMaxResults = 8
	findScanPages  = 5
	findKeep       = 100
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
	matches, partial, err := t.findInRoot(ctx, p, q)
	if err != nil {
		return t.showError(ctx, p, chatID, 0, err)
	}
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
		rows = append(rows, []button{act(icon+" "+short(f.Name, 40), "find_pick", f.ID, 0, "")})
	}
	if partial {
		b.WriteString(g.findPartial + "\n")
	}
	b.WriteString("\n<i>" + g.findScope + "</i>")
	rows = append(rows, []button{nav(tx.back, "home")})
	return t.showCard(ctx, p, chatID, 0, b.String(), rows...)
}

// findInRoot scans at most findScanPages pages of My Drive for names that
// contain q (case-insensitive). partial reports a listing that was cut short.
func (t *Telegram) findInRoot(ctx context.Context, p app.Principal, q string) (matches []drive.File, partial bool, err error) {
	needle := strings.ToLower(q)
	token := ""
	for page := 0; page < findScanPages; page++ {
		view, berr := t.svc.Browse(ctx, p, app.RootFolder, "", token)
		if berr != nil {
			return nil, false, berr
		}
		for _, f := range view.Children {
			if strings.Contains(strings.ToLower(f.Name), needle) && len(matches) < findKeep {
				matches = append(matches, f)
			}
		}
		token = view.NextPageToken
		if token == "" {
			break
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].IsFolder() != matches[j].IsFolder() {
			return matches[i].IsFolder()
		}
		return strings.ToLower(matches[i].Name) < strings.ToLower(matches[j].Name)
	})
	return matches, token != "", nil
}

// actFindPick makes the chosen item the source of a copy preview.
func (t *Telegram) actFindPick(ctx context.Context, p app.Principal, st state.Callback) error {
	return t.paste(ctx, p, st.ChatID, st.MessageID, st.Object, "tg-cb:"+st.ID)
}
