package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/go-telegram/bot/models"

	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/drive"
	"github.com/itsgiahung/502Drive/internal/state"
)

func (t *Telegram) onMessage(ctx context.Context, updateID int64, m *models.Message) error {
	// Only private chats. A group paste gets no reply so no name or job leaks.
	if m.Chat.Type != models.ChatTypePrivate || m.From == nil {
		return nil
	}
	p, ok, err := t.principal(ctx, m.From.ID)
	if err != nil || !ok {
		return nil
	}
	t.names.Store(m.Chat.ID, m.From.FirstName)
	text := strings.TrimSpace(m.Text)
	if text == "" {
		text = strings.TrimSpace(m.Caption)
	}
	if page := keyboardPage(text); page != "" {
		t.await.Delete(m.Chat.ID)
		return t.page(ctx, p, m.Chat.ID, 0, page)
	}
	if a, ok := t.takeAwait(m.Chat.ID, text); ok {
		return t.finishAwait(ctx, p, m.Chat.ID, a, text, t.attemptKey(updateID))
	}
	if text == "" {
		return t.page(ctx, p, m.Chat.ID, 0, "home")
	}
	cmd, rest := splitCommand(text)
	switch cmd {
	case "/start", "/menu", "/help":
		t.sendKeyboard(ctx, m.Chat.ID, t.text(ctx, p))
		return t.page(ctx, p, m.Chat.ID, 0, "home")
	case "/cancel":
		return t.page(ctx, p, m.Chat.ID, 0, "home")
	case "/jobs":
		return t.page(ctx, p, m.Chat.ID, 0, "jobs")
	case "/copy", "/clone":
		if rest == "" {
			return t.page(ctx, p, m.Chat.ID, 0, "copy")
		}
		return t.paste(ctx, p, m.Chat.ID, 0, rest, t.attemptKey(updateID))
	}
	if cmd != "" { // unknown commands land on the menu; nothing to memorise
		return t.page(ctx, p, m.Chat.ID, 0, "home")
	}
	refs := driveRefs(text)
	tx := t.text(ctx, p)
	switch len(refs) {
	case 0:
		return t.showCard(ctx, p, m.Chat.ID, 0, tx.invalidLink, []button{nav(tx.back, "home")})
	case 1:
		return t.paste(ctx, p, m.Chat.ID, 0, refs[0], t.attemptKey(updateID))
	}
	return t.showCard(ctx, p, m.Chat.ID, 0, tx.oneLink, []button{nav(tx.back, "home")})
}

// attemptKey is the idempotency base for one update. A redelivered update
// reuses it, so an admission replay never creates a second job.
func (t *Telegram) attemptKey(updateID int64) string {
	return fmt.Sprintf("tg:%s:%d", t.instance, updateID)
}

func splitCommand(text string) (cmd, rest string) {
	if !strings.HasPrefix(text, "/") {
		return "", text
	}
	fields := strings.SplitN(text, " ", 2)
	cmd = strings.ToLower(fields[0])
	if i := strings.IndexByte(cmd, '@'); i > 0 {
		cmd = cmd[:i]
	}
	if len(fields) == 2 {
		rest = strings.TrimSpace(fields[1])
	}
	return cmd, rest
}

// driveRefs returns the distinct Drive references in a message. The parser is
// an allowlist: nothing else is fetched.
func driveRefs(text string) []string {
	seen := map[string]bool{}
	var out []string
	for _, field := range strings.Fields(text) {
		candidate := strings.Trim(field, "<>()[]\"',;.")
		ref, err := drive.Parse(candidate)
		if err != nil || seen[ref.ID] {
			continue
		}
		seen[ref.ID] = true
		out = append(out, candidate)
	}
	return out
}

// paste inspects a source and either confirms it or admits the copy when the
// user opted into paste-to-copy and the bound destination revision still
// matches.
func (t *Telegram) paste(ctx context.Context, p app.Principal, chatID int64, messageID int, source, base string) error {
	tx := t.text(ctx, p)
	prefs, err := t.svc.Preferences(ctx, p)
	if err != nil {
		return t.showError(ctx, p, chatID, messageID, err)
	}
	req := app.PlanRequest{Source: source}
	auto := prefs.AutoCopy && prefs.DestinationID != ""
	if auto {
		req.DestinationID = prefs.DestinationID
	}
	view, _, err := t.svc.Inspect(ctx, p, base+":plan", req)
	if err != nil {
		return t.showError(ctx, p, chatID, messageID, err)
	}
	if auto && prefs.DestinationRevision != nil && *prefs.DestinationRevision == view.DestinationRevision {
		res, err := t.svc.Submit(ctx, p, base+":copy", view.ID, view.Revision)
		if err == nil {
			return t.submitted(ctx, p, chatID, messageID, res.Job)
		}
		if !previewOnSubmitError(err) {
			return t.showError(ctx, p, chatID, messageID, err)
		}
		return t.previewCard(ctx, p, chatID, messageID, view, submitNote(err, tx), "")
	}
	note := ""
	if prefs.AutoCopy {
		// The bound revision moved, the destination changed or no auto
		// destination is set: never copy to a target the user did not confirm.
		note = tx.confirmDest
	}
	return t.previewCard(ctx, p, chatID, messageID, view, note, "")
}

// previewOnSubmitError reports admission failures that belong on a fresh
// preview (retryable) rather than an error card.
func previewOnSubmitError(err error) bool {
	switch app.Classify(err).Code {
	case "capacity_exceeded", "plan_changed", "stale_revision", "plan_expired", "rate_limited", "provider_unavailable":
		return true
	}
	return false
}

func submitNote(err error, tx texts) string {
	switch app.Classify(err).Code {
	case "capacity_exceeded":
		return tx.queueFull
	}
	return tx.confirmDest
}

func (t *Telegram) onCallback(ctx context.Context, cq *models.CallbackQuery) error {
	var msg *models.Message
	if cq.Message.Message != nil {
		msg = cq.Message.Message
	}
	if msg == nil {
		return nil
	}
	if msg.Chat.Type != models.ChatTypePrivate {
		// A group message cannot be controlled from here; no data is echoed.
		return t.answer(ctx, cq.ID, "")
	}
	t.names.Store(msg.Chat.ID, cq.From.FirstName)
	p, ok, err := t.principal(ctx, cq.From.ID)
	if err != nil {
		return err
	}
	if !ok {
		return t.answer(ctx, cq.ID, "")
	}
	data := cq.Data
	if page, isNav := strings.CutPrefix(data, navPrefix); isNav {
		if err := t.answer(ctx, cq.ID, ""); err != nil {
			t.log.Warn("telegram: cannot ack navigation", "err", err)
		}
		return t.page(ctx, p, msg.Chat.ID, msg.ID, page)
	}
	id, isCB := strings.CutPrefix(data, callbackPrefix)
	if !isCB {
		return t.answer(ctx, cq.ID, "")
	}
	tx := t.text(ctx, p)
	st, err := t.store.ClaimCallback(ctx, id, p.ID, msg.Chat.ID, true, t.now())
	if errors.Is(err, state.ErrCallbackStale) {
		return t.answer(ctx, cq.ID, tx.expired)
	}
	if err != nil {
		return err
	}
	// Acknowledge before any Drive work; the ACK is not the commit.
	if err := t.answer(ctx, cq.ID, ""); err != nil {
		t.log.Warn("telegram: cannot ack callback", "err", err)
	}
	return t.act(ctx, p, st, tx)
}

func (t *Telegram) act(ctx context.Context, p app.Principal, st state.Callback, tx texts) error {
	switch st.Action {
	case "copy":
		return t.actCopy(ctx, p, st, tx)
	case "choose_dest":
		return t.actChooseDest(ctx, p, st, tx)
	case "use_dest":
		return t.actUseDest(ctx, p, st, tx)
	case "rename":
		return t.actRename(ctx, p, st, tx)
	case "back_preview":
		return t.actBackPreview(ctx, p, st, tx)
	case "pick":
		return t.actPick(ctx, p, st, tx)
	case "browse":
		return t.actBrowse(ctx, p, st, tx)
	case "savehere":
		return t.actSaveHere(ctx, p, st, tx)
	case "newdir":
		return t.actNewDir(ctx, p, st, tx)
	case "cancel_preview":
		return t.showCard(ctx, p, st.ChatID, st.MessageID, tx.cancelled, []button{nav(tx.back, "home")})
	case "job":
		j, err := t.svc.Job(ctx, p, st.Object)
		if err != nil {
			return t.showError(ctx, p, st.ChatID, st.MessageID, err)
		}
		_, err = t.jobCard(ctx, p, st.ChatID, st.MessageID, j, false)
		return err
	case "pause", "resume", "retry", "job_cancel":
		return t.actJob(ctx, p, st)
	case "collection":
		return t.collectionCard(ctx, p, st.ChatID, st.MessageID, st.Object, "")
	case "sync_presets":
		return t.syncPresetCard(ctx, p, st.ChatID, st.MessageID, st.Object)
	case "sync_set":
		return t.actSyncSet(ctx, p, st)
	case "sync_preview":
		return t.syncPreviewCard(ctx, p, st.ChatID, st.MessageID, st.Object, st.Payload)
	case "sync_run":
		return t.actSyncRun(ctx, p, st)
	case "sync_pause", "sync_resume", "sync_stop":
		return t.actSyncControl(ctx, p, st)
	case "lang":
		return t.actLanguage(ctx, p, st)
	case "auto":
		return t.actAutoCopy(ctx, p, st)
	}
	return nil
}

func (t *Telegram) actCopy(ctx context.Context, p app.Principal, st state.Callback, tx texts) error {
	res, err := t.svc.Submit(ctx, p, "tg-cb:"+st.ID, st.Object, st.Revision)
	if err == nil {
		return t.submitted(ctx, p, st.ChatID, st.MessageID, res.Job)
	}
	if previewOnSubmitError(err) {
		var pl planPayload
		if json.Unmarshal([]byte(st.Payload), &pl) == nil && pl.Source != "" {
			return t.paste(ctx, p, st.ChatID, st.MessageID, pl.Source, "tg-cb:"+st.ID)
		}
	}
	return t.showError(ctx, p, st.ChatID, st.MessageID, err)
}

func (t *Telegram) actChooseDest(ctx context.Context, p app.Principal, st state.Callback, tx texts) error {
	dests, err := t.svc.Destinations(ctx)
	if err != nil || len(dests) == 0 {
		return t.showCard(ctx, p, st.ChatID, st.MessageID, tx.noDestination+"\n"+tx.destHint,
			[]button{nav(tx.back, "home")})
	}
	var payload planPayload
	if json.Unmarshal([]byte(st.Payload), &payload) != nil {
		return t.showError(ctx, p, st.ChatID, st.MessageID, app.Fail(409, "plan_changed", tx.confirmDest))
	}
	rows := make([][]button, 0, len(dests)+1)
	for _, d := range dests {
		pl, _ := json.Marshal(planPayload{Source: payload.Source, Root: payload.Root, Destination: d.ID})
		label := short(d.Name, 40)
		if d.IsDefault {
			label += " ✓"
		}
		rows = append(rows, []button{act(label, "use_dest", st.Object, st.Revision, string(pl))})
	}
	rows = append(rows, []button{nav(tx.back, "home")})
	return t.showCard(ctx, p, st.ChatID, st.MessageID, "<b>"+tx.destination+"</b>", rows...)
}

func (t *Telegram) actUseDest(ctx context.Context, p app.Principal, st state.Callback, tx texts) error {
	var pl planPayload
	if err := json.Unmarshal([]byte(st.Payload), &pl); err != nil || pl.Source == "" || pl.Destination == "" {
		return t.showError(ctx, p, st.ChatID, st.MessageID, app.Fail(409, "plan_changed", tx.confirmDest))
	}
	view, _, err := t.svc.Inspect(ctx, p, "tg-cb:"+st.ID+":plan", app.PlanRequest{Source: pl.Source, DestinationID: pl.Destination, RootName: pl.Root})
	if err != nil {
		return t.showError(ctx, p, st.ChatID, st.MessageID, err)
	}
	return t.previewCard(ctx, p, st.ChatID, st.MessageID, view, "", pl.Root)
}

func (t *Telegram) actJob(ctx context.Context, p app.Principal, st state.Callback) error {
	action := st.Action
	if action == "job_cancel" {
		action = "cancel"
	}
	res, err := t.svc.JobAction(ctx, p, "tg-cb:"+st.ID, st.Object, action, st.Revision)
	if err == nil {
		_, err = t.jobCard(ctx, p, st.ChatID, st.MessageID, res.Job, false)
		return err
	}
	// Stale buttons are expected after another device acted; show the truth.
	if j, jerr := t.svc.Job(ctx, p, st.Object); jerr == nil {
		_, cardErr := t.jobCard(ctx, p, st.ChatID, st.MessageID, j, false)
		return cardErr
	}
	return t.showError(ctx, p, st.ChatID, st.MessageID, err)
}

// syncKey keeps each retry a separate idempotency key: a stale request was
// never applied, so replaying it with the same key would be a mismatch.
func syncKey(callbackID string, attempt int) string {
	key := "tg-cb:" + callbackID
	if attempt > 0 {
		key += ":" + strconv.Itoa(attempt+1)
	}
	return key
}

func staleRevision(err error) bool {
	ae := app.Classify(err)
	return ae != nil && (ae.Code == "stale_revision" || ae.Code == "plan_changed")
}

// actSyncSet stores a preset through the same application command as the CLI
// and the HTTP API. Revisions are read at click time.
func (t *Telegram) actSyncSet(ctx context.Context, p app.Principal, st state.Callback) error {
	var pl syncPayload
	if err := json.Unmarshal([]byte(st.Payload), &pl); err != nil || pl.Mode == "" {
		return t.showCard(ctx, p, st.ChatID, st.MessageID, t.text(ctx, p).expired, []button{nav(t.text(ctx, p).back, "home")})
	}
	var lastErr error
	for attempt := range 2 {
		c, err := t.svc.Collection(ctx, p, st.Object)
		if err != nil {
			return t.showError(ctx, p, st.ChatID, st.MessageID, err)
		}
		view, configured, err := t.svc.SyncStatus(ctx, st.Object)
		if err != nil {
			return t.showError(ctx, p, st.ChatID, st.MessageID, err)
		}
		req := app.SyncRequest{Mode: pl.Mode, Schedule: pl.Schedule, ContentPolicy: state.PolicyVersioned,
			ExpectedCollection: c.Revision}
		if configured {
			revision := view.Subscription.Revision
			req.ExpectedSubscription = &revision
		}
		if _, _, err = t.svc.ConfigureSync(ctx, p, syncKey(st.ID, attempt), st.Object, req); err == nil {
			return t.collectionCard(ctx, p, st.ChatID, st.MessageID, st.Object, "")
		}
		lastErr = err
		if !staleRevision(err) {
			break
		}
	}
	return t.showError(ctx, p, st.ChatID, st.MessageID, lastErr)
}

// actSyncRun admits a delta run now, or joins the active one.
func (t *Telegram) actSyncRun(ctx context.Context, p app.Principal, st state.Callback) error {
	var lastErr error
	for attempt := range 2 {
		c, err := t.svc.Collection(ctx, p, st.Object)
		if err != nil {
			return t.showError(ctx, p, st.ChatID, st.MessageID, err)
		}
		if _, _, err = t.svc.RunNow(ctx, p, syncKey(st.ID, attempt), st.Object, c.Revision); err == nil {
			return t.collectionCard(ctx, p, st.ChatID, st.MessageID, st.Object, "")
		}
		lastErr = err
		if !staleRevision(err) {
			break
		}
	}
	return t.showError(ctx, p, st.ChatID, st.MessageID, lastErr)
}

// actSyncControl pauses, resumes or stops the subscription.
func (t *Telegram) actSyncControl(ctx context.Context, p app.Principal, st state.Callback) error {
	action := strings.TrimPrefix(st.Action, "sync_")
	view, configured, err := t.svc.SyncStatus(ctx, st.Object)
	if err != nil {
		return t.showError(ctx, p, st.ChatID, st.MessageID, err)
	}
	if !configured {
		return t.collectionCard(ctx, p, st.ChatID, st.MessageID, st.Object, "")
	}
	if _, _, err = t.svc.SyncAction(ctx, p, "tg-cb:"+st.ID, st.Object, action, view.Subscription.Revision); err != nil {
		return t.showError(ctx, p, st.ChatID, st.MessageID, err)
	}
	return t.collectionCard(ctx, p, st.ChatID, st.MessageID, st.Object, "")
}

func (t *Telegram) actLanguage(ctx context.Context, p app.Principal, st state.Callback) error {
	lang := st.Object
	if lang != string(langEN) {
		lang = string(langVI)
	}
	if _, err := t.svc.PatchPreferences(ctx, p, "tg-cb:"+st.ID, app.PreferencePatch{Language: &lang}); err != nil {
		return t.showError(ctx, p, st.ChatID, st.MessageID, err)
	}
	return t.page(ctx, p, st.ChatID, st.MessageID, "settings")
}

func (t *Telegram) actAutoCopy(ctx context.Context, p app.Principal, st state.Callback) error {
	on := st.Object == "on"
	tx := t.text(ctx, p)
	patch := app.PreferencePatch{AutoCopy: &on}
	if on {
		dests, err := t.svc.Destinations(ctx)
		if err != nil {
			return t.showError(ctx, p, st.ChatID, st.MessageID, err)
		}
		found := false
		for _, d := range dests {
			if !d.IsDefault {
				continue
			}
			id, rev := d.ID, d.Revision
			patch.DestinationID, patch.DestinationRevision = &id, &rev
			found = true
			break
		}
		if !found {
			return t.showCard(ctx, p, st.ChatID, st.MessageID, tx.autoCopyNeedsDestination,
				[]button{nav(tx.back, "home")})
		}
	}
	if _, err := t.svc.PatchPreferences(ctx, p, "tg-cb:"+st.ID, patch); err != nil {
		return t.showError(ctx, p, st.ChatID, st.MessageID, err)
	}
	return t.page(ctx, p, st.ChatID, st.MessageID, "settings")
}
