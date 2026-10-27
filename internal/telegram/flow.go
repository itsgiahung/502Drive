package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/state"
)

// The link-first copy flow: paste a link, see a preview that already has sane
// defaults (the source's own name, saved in My Drive), then change the name or
// the place only if you want to.

const (
	awaitTTL       = 10 * time.Minute
	foldersPerPage = 8
)

// awaitState is the one free-text answer a chat owes the bot: a new output
// name or the name of a new folder. It lives in memory only; losing it on a
// restart just means tapping the button again.
type awaitState struct {
	kind      string // "rename" or "mkdir"
	plan      planPayload
	folder    string
	messageID int
	until     time.Time
}

// browsePayload travels in the callback of every folder button.
type browsePayload struct {
	Plan   planPayload `json:"p"`
	Folder string      `json:"f"`
	Offset int         `json:"o,omitempty"`
}

func (t *Telegram) setAwait(chatID int64, a awaitState) {
	a.until = t.now().Add(awaitTTL)
	t.await.Store(chatID, a)
}

func validFolderName(s string) bool {
	if s == "" || utf8.RuneCountInString(s) > 255 || strings.TrimSpace(s) != s {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func planKey(base string) string { return base + ":plan" }

// takeAwait consumes a pending free-text answer. A slash command or a Drive
// link cancels it instead (and is then handled normally).
func (t *Telegram) takeAwait(chatID int64, text string) (awaitState, bool) {
	v, ok := t.await.Load(chatID)
	if !ok {
		return awaitState{}, false
	}
	t.await.Delete(chatID)
	a := v.(awaitState)
	if t.now().After(a.until) || strings.HasPrefix(text, "/") || len(driveRefs(text)) > 0 {
		return awaitState{}, false
	}
	return a, true
}

// finishAwait applies the typed text.
func (t *Telegram) finishAwait(ctx context.Context, p app.Principal, chatID int64, a awaitState, text, base string) error {
	tx := t.text(ctx, p)
	name := strings.TrimSpace(text)
	if !validFolderName(name) {
		t.setAwait(chatID, a)
		return t.showCard(ctx, p, chatID, 0, "⚠️ "+tx.errMsg["invalid_request"]+"\n"+map[string]string{"rename": tx.askName, "mkdir": tx.askFolder}[a.kind])
	}
	switch a.kind {
	case "rename":
		pl := a.plan
		pl.Root = name
		view, _, err := t.svc.Inspect(ctx, p, planKey(base), pl.request())
		if err != nil {
			return t.showError(ctx, p, chatID, 0, err)
		}
		return t.previewCard(ctx, p, chatID, 0, view, "✏️ "+tx.renamed, name)
	case "mkdir":
		made, _, err := t.svc.Mkdir(ctx, p, base+":mkdir", a.folder, name)
		if err != nil {
			return t.showError(ctx, p, chatID, 0, err)
		}
		return t.browseCard(ctx, p, chatID, 0, tx, browsePayload{Plan: a.plan, Folder: made.ID})
	}
	return nil
}

func decodeBrowse(payload string) (browsePayload, bool) {
	var bp browsePayload
	if json.Unmarshal([]byte(payload), &bp) != nil || bp.Plan.Source == "" {
		return bp, false
	}
	return bp, true
}

func browsePL(bp browsePayload) string {
	b, _ := json.Marshal(bp)
	return string(b)
}

// actRename asks for the new output name.
func (t *Telegram) actRename(ctx context.Context, p app.Principal, st state.Callback, tx texts) error {
	var pl planPayload
	if json.Unmarshal([]byte(st.Payload), &pl) != nil || pl.Source == "" {
		return t.showError(ctx, p, st.ChatID, st.MessageID, app.Fail(409, "plan_changed", tx.confirmDest))
	}
	t.setAwait(st.ChatID, awaitState{kind: "rename", plan: pl, messageID: st.MessageID})
	return t.showCard(ctx, p, st.ChatID, st.MessageID, tx.askName,
		[]button{act(tx.backBtn, "back_preview", st.Object, st.Revision, st.Payload)})
}

// actBackPreview rebuilds the preview from the payload.
func (t *Telegram) actBackPreview(ctx context.Context, p app.Principal, st state.Callback, tx texts) error {
	t.await.Delete(st.ChatID)
	var pl planPayload
	if json.Unmarshal([]byte(st.Payload), &pl) != nil || pl.Source == "" {
		return t.page(ctx, p, st.ChatID, st.MessageID, "home")
	}
	return t.replay(ctx, p, st.ChatID, st.MessageID, pl, "tg-cb:"+st.ID)
}

// replay inspects again with the choices made so far and shows the preview.
func (t *Telegram) replay(ctx context.Context, p app.Principal, chatID int64, messageID int, pl planPayload, base string) error {
	view, _, err := t.svc.Inspect(ctx, p, planKey(base), pl.request())
	if err != nil {
		return t.showError(ctx, p, chatID, messageID, err)
	}
	return t.previewCard(ctx, p, chatID, messageID, view, "", pl.Root)
}

// actPick opens the folder chooser at My Drive.
func (t *Telegram) actPick(ctx context.Context, p app.Principal, st state.Callback, tx texts) error {
	var pl planPayload
	if json.Unmarshal([]byte(st.Payload), &pl) != nil || pl.Source == "" {
		return t.showError(ctx, p, st.ChatID, st.MessageID, app.Fail(409, "plan_changed", tx.confirmDest))
	}
	return t.browseCard(ctx, p, st.ChatID, st.MessageID, tx, browsePayload{Plan: pl, Folder: app.RootFolder})
}

func (t *Telegram) actBrowse(ctx context.Context, p app.Principal, st state.Callback, tx texts) error {
	bp, ok := decodeBrowse(st.Payload)
	if !ok {
		return t.showError(ctx, p, st.ChatID, st.MessageID, app.Fail(409, "plan_changed", tx.confirmDest))
	}
	return t.browseCard(ctx, p, st.ChatID, st.MessageID, tx, bp)
}

// browseCard lists the sub-folders of one folder as buttons.
func (t *Telegram) browseCard(ctx context.Context, p app.Principal, chatID int64, messageID int, tx texts, bp browsePayload) error {
	view, err := t.svc.Browse(ctx, p, bp.Folder, "", "")
	if err != nil {
		return t.showError(ctx, p, chatID, messageID, err)
	}
	var folders []string
	byID := map[string]string{}
	for _, it := range view.Children {
		if it.IsFolder() {
			folders = append(folders, it.ID)
			byID[it.ID] = it.Name
		}
	}
	offset := min(max(bp.Offset, 0), len(folders))
	end := min(offset+foldersPerPage, len(folders))
	var b strings.Builder
	b.WriteString(tx.pickTitle + "\n")
	names := make([]string, 0, len(view.Path))
	for _, f := range view.Path {
		names = append(names, escape(short(f.Name, 24)))
	}
	fmt.Fprintf(&b, "📍 %s\n", strings.Join(names, " › "))
	if len(folders) == 0 {
		b.WriteString("\n" + tx.noFolders)
	}
	var rows [][]button
	for _, id := range folders[offset:end] {
		child := browsePayload{Plan: bp.Plan, Folder: id}
		rows = append(rows, []button{act("📁 "+short(byID[id], 40), "browse", id, 0, browsePL(child))})
	}
	nav := []button{}
	if n := len(view.Path); n >= 2 {
		up := browsePayload{Plan: bp.Plan, Folder: view.Path[n-2].ID}
		nav = append(nav, act(tx.upBtn, "browse", up.Folder, 0, browsePL(up)))
	}
	if end < len(folders) {
		next := browsePayload{Plan: bp.Plan, Folder: bp.Folder, Offset: end}
		nav = append(nav, act(tx.nextBtn, "browse", bp.Folder, 0, browsePL(next)))
	}
	if len(nav) > 0 {
		rows = append(rows, nav)
	}
	here := browsePayload{Plan: bp.Plan, Folder: view.Folder.ID}
	if !view.Folder.CanAddChildren {
		rows = append(rows, []button{act(tx.backBtn, "back_preview", "", 0, planJSON(bp.Plan))})
		return t.showCard(ctx, p, chatID, messageID, b.String(), rows...)
	}
	rows = append(rows,
		[]button{styled(act(tx.saveHere, "savehere", view.Folder.ID, 0, browsePL(here)), "success"), act(tx.newFolderBtn, "newdir", view.Folder.ID, 0, browsePL(here))},
		[]button{act(tx.backBtn, "back_preview", "", 0, planJSON(bp.Plan))})
	return t.showCard(ctx, p, chatID, messageID, b.String(), rows...)
}

func planJSON(pl planPayload) string {
	b, _ := json.Marshal(pl)
	return string(b)
}

// actSaveHere makes the open folder the destination of this copy.
func (t *Telegram) actSaveHere(ctx context.Context, p app.Principal, st state.Callback, tx texts) error {
	bp, ok := decodeBrowse(st.Payload)
	if !ok {
		return t.showError(ctx, p, st.ChatID, st.MessageID, app.Fail(409, "plan_changed", tx.confirmDest))
	}
	dest, err := t.svc.EnsureDestination(ctx, p, bp.Folder)
	if err != nil {
		return t.showError(ctx, p, st.ChatID, st.MessageID, err)
	}
	pl := bp.Plan
	pl.Destination = dest.ID
	return t.replay(ctx, p, st.ChatID, st.MessageID, pl, "tg-cb:"+st.ID)
}

// actNewDir asks for the name of a folder to create in the open folder.
func (t *Telegram) actNewDir(ctx context.Context, p app.Principal, st state.Callback, tx texts) error {
	bp, ok := decodeBrowse(st.Payload)
	if !ok {
		return t.showError(ctx, p, st.ChatID, st.MessageID, app.Fail(409, "plan_changed", tx.confirmDest))
	}
	t.setAwait(st.ChatID, awaitState{kind: "mkdir", plan: bp.Plan, folder: bp.Folder, messageID: st.MessageID})
	return t.showCard(ctx, p, st.ChatID, st.MessageID, tx.askFolder,
		[]button{act(tx.backBtn, "browse", bp.Folder, 0, browsePL(bp))})
}
