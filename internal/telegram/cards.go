package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/drive"
	"github.com/itsgiahung/502Drive/internal/schedule"
	"github.com/itsgiahung/502Drive/internal/state"
)

// button is one inline-keyboard cell. nav buttons are static and reusable;
// action buttons get a one-shot callback state row.
type button struct {
	text     string
	style    string // Telegram button colour: primary (blue), success (green), danger (red)
	nav      string
	action   string
	object   string
	revision int
	payload  string
}

func nav(text, page string) button { return button{text: text, nav: page} }

// styled colours a button; clients older than Feb 2026 show it plain.
func styled(b button, style string) button {
	b.style = style
	return b
}

func act(text, action, object string, revision int, payload string) button {
	return button{text: text, action: action, object: object, revision: revision, payload: payload}
}

// planPayload is the private callback payload needed to rebuild a preview
// without asking the user for the link again.
type planPayload struct {
	Source      string `json:"source"`
	Root        string `json:"root,omitempty"`
	Destination string `json:"destination,omitempty"`
	// Presets and Conflict are the copy options picked on the preview (S5, N1).
	Presets  []string `json:"pf,omitempty"`
	Conflict string   `json:"oc,omitempty"`
}

// panel sends or edits one card and returns the message it lives in. An empty
// rows argument leaves the existing keyboard alone; pass one empty row to
// remove it.
func (t *Telegram) panel(ctx context.Context, p app.Principal, chatID int64, messageID int, text string, rows [][]button, clear bool) (int, error) {
	var callbacks []string
	keyboard := make([][]models.InlineKeyboardButton, 0, len(rows))
	for _, row := range rows {
		var keys []models.InlineKeyboardButton
		for _, b := range row {
			data := ""
			switch {
			case b.action != "":
				cb := state.Callback{ID: state.ID("cb"), Principal: p.ID, ChatID: chatID, Action: b.action,
					Object: b.object, Revision: b.revision, Payload: b.payload,
					ExpiresAt: t.now().Add(callbackTTL), CreatedAt: t.now()}
				if err := t.store.PutCallback(ctx, cb); err != nil {
					return 0, err
				}
				callbacks = append(callbacks, cb.ID)
				data = callbackPrefix + cb.ID
			case b.nav != "":
				data = navPrefix + b.nav
			default:
				continue
			}
			keys = append(keys, models.InlineKeyboardButton{Text: b.text, CallbackData: data, Style: b.style})
		}
		if len(keys) > 0 {
			keyboard = append(keyboard, keys)
		}
	}
	var markup models.ReplyMarkup
	if len(keyboard) > 0 {
		markup = &models.InlineKeyboardMarkup{InlineKeyboard: keyboard}
	} else if clear {
		markup = &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{}}
	}
	var msg *models.Message
	var err error
	if messageID > 0 {
		msg, err = t.bot.EditMessageText(ctx, &bot.EditMessageTextParams{ChatID: chatID, MessageID: messageID, Text: text,
			ParseMode: models.ParseModeHTML, ReplyMarkup: markup})
	} else {
		msg, err = t.bot.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: text,
			ParseMode: models.ParseModeHTML, ReplyMarkup: markup})
	}
	if err != nil {
		return 0, err
	}
	if msg != nil && msg.ID != 0 {
		if err := t.store.BindCallbackMessage(ctx, callbacks, msg.ID); err != nil {
			t.log.Warn("telegram: cannot bind callbacks", "err", err)
		}
		return msg.ID, nil
	}
	return messageID, nil
}

func (t *Telegram) show(ctx context.Context, p app.Principal, chatID int64, messageID int, text string, rows ...[]button) (int, error) {
	return t.panel(ctx, p, chatID, messageID, text, rows, false)
}

func (t *Telegram) answer(ctx context.Context, callbackID, toast string) error {
	_, err := t.bot.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: callbackID, Text: toast})
	return err
}

func (t *Telegram) text(ctx context.Context, p app.Principal) texts { return t.textFor(ctx, p.ID) }

func (t *Telegram) textFor(ctx context.Context, principalID string) texts {
	prefs, err := t.svc.Preferences(ctx, app.Principal{ID: principalID, Role: app.RoleOperator})
	if err != nil {
		return vietnamese
	}
	return textsFor(langOf(prefs.Language))
}

// page renders one menu page into an existing panel (messageID > 0) or a new
// message.
func (t *Telegram) page(ctx context.Context, p app.Principal, chatID int64, messageID int, name string) error {
	tx := t.text(ctx, p)
	switch name {
	case "copy":
		return t.showCard(ctx, p, chatID, messageID, tx.copyHint, []button{nav(tx.back, "home")})
	case "jobs":
		return t.jobsPage(ctx, p, chatID, messageID, tx)
	case "library":
		return t.libraryPage(ctx, p, chatID, messageID, tx)
	case "dest":
		return t.destPage(ctx, p, chatID, messageID, tx)
	case "account":
		return t.accountPage(ctx, p, chatID, messageID, tx)
	case "settings":
		return t.settingsPage(ctx, p, chatID, messageID, tx)
	}
	return t.homePage(ctx, p, chatID, messageID, tx)
}

func (t *Telegram) showCard(ctx context.Context, p app.Principal, chatID int64, messageID int, text string, rows ...[]button) error {
	_, err := t.show(ctx, p, chatID, messageID, text, rows...)
	return err
}

func (t *Telegram) homePage(ctx context.Context, p app.Principal, chatID int64, messageID int, tx texts) error {
	st, err := t.svc.Status(ctx)
	if err != nil {
		return t.showCard(ctx, p, chatID, messageID, fmt.Sprintf("⚠️ %s: %s", tx.problemLabel, escape(app.Classify(err).Message)),
			[]button{nav(tx.back, "home")})
	}
	prefs, _ := t.svc.Preferences(ctx, p)
	var b strings.Builder
	b.WriteString(tx.welcomeTitle + "\n\n")
	if name, ok := t.names.Load(chatID); ok && name.(string) != "" {
		fmt.Fprintf(&b, tx.welcomeHello+"\n\n", escape(short(name.(string), 40)))
	}
	b.WriteString("<blockquote>" + tx.welcomeFeatures + "</blockquote>\n\n")
	fmt.Fprintf(&b, "☁️ %s: %s %s\n", tx.google, googleEmoji(st.Account.Status), escape(googleState(st.Account.Status, tx)))
	dest := destName(ctx, t.svc)
	if dest == "" {
		dest = tx.noDestination
	}
	fmt.Fprintf(&b, "📂 %s: <b>%s</b>\n", tx.destLabel, escape(short(dest, 50)))
	fmt.Fprintf(&b, "⏳ %s %d · 🕒 %s %d · ⚠️ %s %d\n", tx.running, st.Running, tx.queued, st.Queued, tx.needAction, st.Blocked)
	fmt.Fprintf(&b, "🤖 %s: %s\n\n%s", tx.autoCopy, onOff(prefs.AutoCopy, tx), tx.welcomeTail)
	return t.showCard(ctx, p, chatID, messageID, b.String(),
		[]button{styled(nav(tx.kbCopy, "copy"), "primary")},
		[]button{nav(tx.kbJobs, "jobs"), nav(tx.kbSync, "library")},
		[]button{nav(tx.kbDest, "dest"), nav(tx.kbSettings, "settings")},
		[]button{nav(tx.kbAccount, "account")})
}

func googleEmoji(status string) string {
	switch status {
	case "connected":
		return "✅"
	case "reconnect_required":
		return "⚠️"
	}
	return "❌"
}

// replyKeyboard is the persistent two-column menu above the input field.
func replyKeyboard(tx texts) *models.ReplyKeyboardMarkup {
	key := func(s string) models.KeyboardButton { return models.KeyboardButton{Text: s} }
	return &models.ReplyKeyboardMarkup{
		Keyboard: [][]models.KeyboardButton{
			{key(tx.kbCopy), key(tx.kbJobs)},
			{key(tx.kbSync), key(tx.kbDest)},
			{key(tx.kbAccount), key(tx.kbSettings)},
		},
		IsPersistent: true, ResizeKeyboard: true, InputFieldPlaceholder: tx.kbPlaceholder,
	}
}

// keyboardPage maps a tap on the persistent keyboard (in either language) to a
// menu page; "" means the text was not a keyboard label.
func keyboardPage(text string) string {
	for _, tx := range []texts{vietnamese, english} {
		switch text {
		case tx.kbCopy:
			return "copy"
		case tx.kbJobs:
			return "jobs"
		case tx.kbSync:
			return "library"
		case tx.kbDest:
			return "dest"
		case tx.kbAccount:
			return "account"
		case tx.kbSettings:
			return "settings"
		}
	}
	return ""
}

// sendKeyboard installs the persistent keyboard with a short, silent message.
func (t *Telegram) sendKeyboard(ctx context.Context, chatID int64, tx texts) {
	_, err := t.bot.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: tx.kbReady,
		ReplyMarkup: replyKeyboard(tx), DisableNotification: true})
	if err != nil {
		t.log.Warn("telegram: cannot install the keyboard", "err", err)
	}
}

func googleState(status string, tx texts) string {
	switch status {
	case "connected":
		return tx.connected
	case "reconnect_required":
		return tx.reconnect
	}
	return tx.disconnected
}

func onOff(v bool, tx texts) string {
	if v {
		return tx.on
	}
	return tx.off
}

// destName reads the default destination label; empty when none is set.
func destName(ctx context.Context, svc *app.Service) string {
	dests, err := svc.Destinations(ctx)
	if err != nil {
		return ""
	}
	for _, d := range dests {
		if d.IsDefault {
			return d.Name
		}
	}
	return ""
}

func (t *Telegram) jobsPage(ctx context.Context, p app.Principal, chatID int64, messageID int, tx texts) error {
	jobs, _, err := t.svc.Jobs(ctx, p, false, "", "", 5, 0)
	if err != nil {
		return t.showCard(ctx, p, chatID, messageID, tx.jobsEmpty, []button{nav(tx.back, "home")})
	}
	if len(jobs) == 0 {
		return t.showCard(ctx, p, chatID, messageID, "<b>"+tx.jobs+"</b>\n"+tx.jobsEmpty, []button{nav(tx.back, "home")})
	}
	var b strings.Builder
	b.WriteString("<b>" + tx.jobs + "</b>\n")
	for _, j := range jobs {
		fmt.Fprintf(&b, "· %s — %s\n", escape(short(j.RootName, 40)), tx.stateLabel(j.State))
	}
	rows := make([][]button, 0, len(jobs)+2)
	for _, j := range jobs {
		rows = append(rows, []button{act(short(j.RootName, 24)+" · "+tx.stateLabel(j.State), "job", j.ID, j.Revision, "")})
	}
	for _, j := range jobs {
		if bulkStates[j.State] {
			rows = append(rows, []button{styled(act(tx.cancelAll, "cancelall_ask", "", 0, ""), "danger")})
			break
		}
	}
	rows = append(rows, []button{act(tx.g().manageBtn, "jobs_mgmt", "", 0, ""), nav(tx.back, "home")})
	return t.showCard(ctx, p, chatID, messageID, b.String(), rows...)
}

func (t *Telegram) libraryPage(ctx context.Context, p app.Principal, chatID int64, messageID int, tx texts) error {
	cols, err := t.svc.Collections(ctx, p, false)
	if err != nil {
		return t.showCard(ctx, p, chatID, messageID, tx.libraryEmpty, []button{nav(tx.back, "home")})
	}
	retired := t.retiredCount(ctx, p)
	var retiredRow []button
	if retired > 0 {
		retiredRow = []button{act(fmt.Sprintf(tx.g().retiredButton, retired), "retired_list", "", 0, "")}
	}
	if len(cols) == 0 {
		rows := [][]button{}
		if retiredRow != nil {
			rows = append(rows, retiredRow)
		}
		rows = append(rows, []button{nav(tx.back, "home")})
		return t.showCard(ctx, p, chatID, messageID, "<b>"+tx.library+"</b>\n"+tx.libraryEmpty, rows...)
	}
	var b strings.Builder
	b.WriteString("<b>" + tx.library + "</b>\n")
	var rows [][]button
	for i, c := range cols {
		if i == 5 {
			break
		}
		status := tx.syncOff
		if view, configured, err := t.svc.SyncStatus(ctx, c.ID); err == nil && configured {
			status = tx.syncStateLabel(view.Subscription.State)
		}
		fmt.Fprintf(&b, "· %s — %s\n", escape(short(c.SourceName, 40)), escape(status))
		rows = append(rows, []button{act(short(c.SourceName, 24)+" · "+status, "collection", c.ID, c.Revision, "")})
	}
	if retiredRow != nil {
		rows = append(rows, retiredRow)
	}
	rows = append(rows, []button{nav(tx.destination, "dest"), nav(tx.back, "home")})
	return t.showCard(ctx, p, chatID, messageID, b.String(), rows...)
}

// collectionCard is the library detail: copy state, real sync state and the
// actions that are valid in that state.
func (t *Telegram) collectionCard(ctx context.Context, p app.Principal, chatID int64, messageID int, id, note string) error {
	c, err := t.svc.Collection(ctx, p, id)
	if err != nil {
		return t.showError(ctx, p, chatID, messageID, err)
	}
	view, configured, err := t.svc.SyncStatus(ctx, id)
	if err != nil {
		return t.showError(ctx, p, chatID, messageID, err)
	}
	tx := t.text(ctx, p)
	if c.State == state.CollectionRetired {
		return t.retiredCard(ctx, p, chatID, messageID, c, note)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "<b>%s</b>\n", escape(short(c.SourceName, 60)))
	fmt.Fprintf(&b, "%s: %s\n", tx.copyStateLabel, escape(c.State))
	if !configured {
		fmt.Fprintf(&b, "%s: %s\n", tx.syncLabel, tx.syncOff)
	} else {
		sub := view.Subscription
		fmt.Fprintf(&b, "%s: %s · %s\n", tx.syncLabel, escape(sub.Mode), escape(tx.syncStateLabel(sub.State)))
		fmt.Fprintf(&b, "%s: %s\n", tx.scheduleLabel, escape(tx.scheduleSummary(sub.Schedule)))
		if sub.NextDue != nil {
			fmt.Fprintf(&b, "%s: %s\n", tx.nextRun, sub.NextDue.UTC().Format("2006-01-02 15:04 MST"))
		}
		if sub.LastSuccessAt != nil {
			fmt.Fprintf(&b, "%s: %s\n", tx.lastSuccess, sub.LastSuccessAt.UTC().Format("2006-01-02 15:04 MST"))
		}
		if sub.LastError != nil {
			fmt.Fprintf(&b, "%s: %s\n", tx.problemLabel, escape(short(sub.LastError.Message, 160)))
		}
	}
	if view.ActiveJobID != "" {
		fmt.Fprintf(&b, "%s: <code>%s</code>\n", tx.activeJob, escape(view.ActiveJobID))
	}
	b.WriteString("\n" + tx.keepCopies)
	if note != "" {
		b.WriteString("\n\n" + note)
	}
	if summary, repairRow := t.auditSummary(ctx, p, tx, id); summary != "" {
		b.WriteString("\n" + escape(summary))
		rows := repairRow
		rows = append(rows, t.collectionButtons(tx, id, view, configured)...)
		rows = append(rows, t.lifecycleRows(tx, c)...)
		rows = append(rows, []button{nav(tx.library, "library"), nav(tx.back, "home")})
		return t.showCard(ctx, p, chatID, messageID, b.String(), rows...)
	}
	rows := t.collectionButtons(tx, id, view, configured)
	rows = append(rows, t.lifecycleRows(tx, c)...)
	rows = append(rows, []button{nav(tx.library, "library"), nav(tx.back, "home")})
	return t.showCard(ctx, p, chatID, messageID, b.String(), rows...)
}

// collectionButtons lists only actions that can succeed in the current state.
func (t *Telegram) collectionButtons(tx texts, id string, view app.SyncView, configured bool) [][]button {
	var rows [][]button
	primary := []button{act(tx.runNow, "sync_run", id, 0, "")}
	if !configured {
		primary = append([]button{act(tx.enableSync, "sync_presets", id, 0, "")}, primary...)
		return [][]button{primary, {act(tx.auditCheck, "audit", id, 0, "")}}
	}
	sub := view.Subscription
	switch sub.State {
	case "active", "bootstrapping", "needs_reconcile":
		primary = append(primary, act(tx.pauseSync, "sync_pause", id, sub.Revision, ""))
	case "paused", "stopped", "error":
		primary = append(primary, act(tx.resumeSync, "sync_resume", id, sub.Revision, ""))
	}
	rows = append(rows, primary)
	secondary := []button{act(tx.changeSchedule, "sync_presets", id, 0, "")}
	if sub.State != "stopped" && sub.State != "stopping" {
		secondary = append(secondary, act(tx.stopSync, "sync_stop", id, sub.Revision, ""))
	}
	rows = append(rows, secondary)
	return append(rows, []button{act(tx.auditCheck, "audit", id, 0, "")})
}

// syncPayload is the private intent of a preset button; revisions are read
// again when the button is clicked.
type syncPayload struct {
	Mode     string         `json:"mode"`
	Schedule *schedule.Spec `json:"schedule,omitempty"`
}

// syncPreviewCard shows a schedule and its next three runs before saving, so a
// bot user sees what will happen without cron syntax.
func (t *Telegram) syncPreviewCard(ctx context.Context, p app.Principal, chatID int64, messageID int, collectionID, payload string) error {
	tx := t.text(ctx, p)
	var pl syncPayload
	if err := json.Unmarshal([]byte(payload), &pl); err != nil || pl.Schedule == nil {
		return t.showError(ctx, p, chatID, messageID, app.Fail(409, "plan_changed", tx.expired))
	}
	slots, err := pl.Schedule.Preview(t.now().UTC(), 3)
	if err != nil {
		return t.showError(ctx, p, chatID, messageID, app.Fail(400, "invalid_request", err.Error()))
	}
	var b strings.Builder
	fmt.Fprintf(&b, "<b>%s</b>\n", tx.pickPreset)
	fmt.Fprintf(&b, "%s: %s\n", tx.scheduleLabel, escape(tx.scheduleSummary(pl.Schedule)))
	fmt.Fprintf(&b, "\n%s:\n", tx.nextRuns)
	for _, slot := range slots {
		fmt.Fprintf(&b, "· %s\n", slotTime(pl.Schedule, slot.At))
	}
	return t.showCard(ctx, p, chatID, messageID, b.String(),
		[]button{act(tx.save, "sync_set", collectionID, 0, payload), act(tx.cancel, "collection", collectionID, 0, "")})
}

// slotTime renders a run time in the schedule's own zone, so the user sees the
// wall clock they configured instead of a UTC instant.
func slotTime(sp *schedule.Spec, at time.Time) string {
	if sp != nil && sp.Timezone != "" {
		if loc, err := time.LoadLocation(sp.Timezone); err == nil {
			return at.In(loc).Format("2006-01-02 15:04 MST")
		}
	}
	return at.UTC().Format("2006-01-02 15:04 MST")
}

// syncPresetCard offers structured presets instead of cron syntax.
func (t *Telegram) syncPresetCard(ctx context.Context, p app.Principal, chatID int64, messageID int, id string) error {
	tx := t.text(ctx, p)
	prefs, err := t.svc.Preferences(ctx, p)
	if err != nil {
		return t.showError(ctx, p, chatID, messageID, err)
	}
	tz := prefs.Timezone
	if tz == "" {
		tz = syncTimezone
	}
	spec := func(kind string, minutes int, local string, weekdays []int) string {
		sp := schedule.Spec{Kind: kind, IntervalMinutes: minutes, LocalTime: local, Timezone: tz}
		if kind == schedule.KindInterval {
			sp.AnchorAt = t.now().UTC()
		}
		sp.Weekdays = weekdays
		pl, _ := json.Marshal(syncPayload{Mode: "scheduled", Schedule: &sp})
		return string(pl)
	}
	realtime, _ := json.Marshal(syncPayload{Mode: "realtime"})
	return t.showCard(ctx, p, chatID, messageID, "<b>"+tx.pickPreset+"</b>",
		[]button{act(tx.every30m, "sync_preview", id, 0, spec(schedule.KindInterval, 30, "", nil)),
			act(tx.everyHour, "sync_preview", id, 0, spec(schedule.KindInterval, 60, "", nil))},
		[]button{act(tx.every6h, "sync_preview", id, 0, spec(schedule.KindInterval, 360, "", nil)),
			act(tx.daily2200, "sync_preview", id, 0, spec(schedule.KindDaily, 0, "22:00", nil))},
		[]button{act(tx.weeklyMWF, "sync_preview", id, 0, spec(schedule.KindWeekly, 0, "22:00", []int{1, 3, 5})),
			act(tx.realtimePreset, "sync_set", id, 0, string(realtime))},
		[]button{act(tx.back, "collection", id, 0, "")})
}

func (t *Telegram) destPage(ctx context.Context, p app.Principal, chatID int64, messageID int, tx texts) error {
	dests, err := t.svc.Destinations(ctx)
	if err != nil || len(dests) == 0 {
		return t.showCard(ctx, p, chatID, messageID, "<b>"+tx.destination+"</b>\n"+tx.noDestination+"\n\n"+tx.destHint,
			[]button{nav(tx.back, "home")})
	}
	var b strings.Builder
	b.WriteString("<b>" + tx.destination + "</b>\n")
	for _, d := range dests {
		mark := ""
		if d.IsDefault {
			mark = " (" + tx.destLabel + ")"
		}
		fmt.Fprintf(&b, "· %s%s\n", escape(short(d.Name, 48)), mark)
	}
	b.WriteString("\n" + tx.destHint)
	return t.showCard(ctx, p, chatID, messageID, b.String(),
		[]button{nav(tx.jobs, "jobs"), nav(tx.back, "home")})
}

func (t *Telegram) accountPage(ctx context.Context, p app.Principal, chatID int64, messageID int, tx texts) error {
	acc, err := t.svc.Account(ctx)
	if err != nil {
		return t.showCard(ctx, p, chatID, messageID, tx.accountHint, []button{nav(tx.back, "home")})
	}
	var b strings.Builder
	b.WriteString("<b>" + tx.account + "</b>\n")
	fmt.Fprintf(&b, "%s: %s\n", tx.google, googleState(acc.Status, tx))
	if acc.Label != nil && *acc.Label != "" {
		fmt.Fprintf(&b, "%s\n", escape(*acc.Label))
	}
	if acc.LastError != nil {
		fmt.Fprintf(&b, "%s: %s\n", tx.problemLabel, escape(short(acc.LastError.Message, 160)))
	}
	b.WriteString("\n" + tx.accountHint)
	return t.showCard(ctx, p, chatID, messageID, b.String(),
		[]button{nav(tx.settings, "settings"), nav(tx.back, "home")})
}

func (t *Telegram) settingsPage(ctx context.Context, p app.Principal, chatID int64, messageID int, tx texts) error {
	prefs, err := t.svc.Preferences(ctx, p)
	if err != nil {
		return t.showCard(ctx, p, chatID, messageID, tx.settingsHint, []button{nav(tx.back, "home")})
	}
	var b strings.Builder
	b.WriteString("<b>" + tx.settings + "</b>\n")
	fmt.Fprintf(&b, "%s: %s\n", tx.language, escape(string(langOf(prefs.Language))))
	fmt.Fprintf(&b, "%s: %s\n", tx.autoCopy, onOff(prefs.AutoCopy, tx))
	block, extra := t.notifySettings(ctx, p, tx, prefs)
	fmt.Fprintf(&b, "%s\n%s", block, tx.settingsHint)
	rows := [][]button{
		{act("Tiếng Việt", "lang", string(langVI), 0, ""), act("English", "lang", string(langEN), 0, "")},
		{act(tx.on, "auto", "on", 0, ""), act(tx.off, "auto", "off", 0, "")},
	}
	rows = append(rows, extra...)
	rows = append(rows, []button{nav(tx.back, "home")})
	return t.showCard(ctx, p, chatID, messageID, b.String(), rows...)
}

func (t *Telegram) previewCard(ctx context.Context, p app.Principal, chatID int64, messageID int, view app.PlanView, note, root string) error {
	tx := t.text(ctx, p)
	var b strings.Builder
	fmt.Fprintf(&b, "📥 <b>%s</b>\n\n", escape(fmt.Sprintf(tx.previewTitle, short(view.SourceName, 60))))
	seen := fmt.Sprintf("%d files, %d folders", view.Preview.ObservedFiles, view.Preview.ObservedFolders)
	if !view.Preview.ScanComplete {
		seen += " " + tx.scanIncomplete
	}
	fmt.Fprintf(&b, "📎 %s: %s <i>(%s)</i>\n", tx.sourceLabel, escape(short(view.SourceName, 80)), escape(seen))
	fmt.Fprintf(&b, "%s: <b>%s</b>\n", tx.saveToLabel, escape(short(t.destinationLabel(ctx, view.DestinationID), 60)))
	fmt.Fprintf(&b, "%s: <b>%s</b>\n", tx.nameLabel, escape(short(view.RootName, 80)))
	for _, w := range view.Warnings {
		fmt.Fprintf(&b, "⚠️ %s\n", escape(short(w, 160)))
	}
	if note != "" {
		fmt.Fprintf(&b, "\n%s\n", note)
	}
	if block := filterPreview(tx, view); block != "" {
		b.WriteString("\n" + block)
	}
	pl := planPayload{Source: refOf(view.Plan), Root: root, Destination: view.DestinationID,
		Presets: presetsOf(view.Filter), Conflict: conflictOf(view.OnConflict)}
	payload, _ := json.Marshal(pl)
	rows := [][]button{
		{styled(act(tx.copyNow, "copy", view.ID, view.Revision, string(payload)), "success")},
		{act(tx.renameBtn, "rename", view.ID, view.Revision, string(payload)), act(tx.pickBtn, "pick", view.ID, view.Revision, string(payload))},
	}
	rows = append(rows, optionRows(tx, view, pl)...)
	rows = append(rows, []button{styled(act("✖ "+tx.cancel, "cancel_preview", view.ID, view.Revision, ""), "danger")})
	return t.showCard(ctx, p, chatID, messageID, b.String(), rows...)
}

// destinationLabel is the name shown for a destination id.
func (t *Telegram) destinationLabel(ctx context.Context, id string) string {
	if dests, err := t.svc.Destinations(ctx); err == nil {
		for _, d := range dests {
			if d.ID == id {
				return d.Name
			}
		}
	}
	return id
}

// syncTimezone is the zone used by chat presets; schedules never use the host
// timezone.
const syncTimezone = "Asia/Ho_Chi_Minh"

// refOf rebuilds a Drive reference for a plan without showing it to the user.
func refOf(p state.Plan) string {
	if p.ResourceKey == "" {
		return p.SourceID
	}
	ref := drive.Reference{ID: p.SourceID, Key: p.ResourceKey}
	return "https://drive.google.com/open?id=" + ref.ID + "&resourcekey=" + ref.Key
}

func (t *Telegram) jobCard(ctx context.Context, p app.Principal, chatID int64, messageID int, j state.Job, fresh bool) (int, error) {
	tx := t.text(ctx, p)
	text := jobTextView(tx, j, 0, t.jobViewFor(ctx, p.ID))
	if fresh {
		text = tx.copyStarted + "\n" + text
	}
	return t.show(ctx, p, chatID, messageID, text, t.jobButtons(tx, j)...)
}

// submitted shows the new job card and starts its durable progress panel. The
// panel edits that same message instead of sending another one.
func (t *Telegram) submitted(ctx context.Context, p app.Principal, chatID int64, messageID int, j state.Job) error {
	if err := t.store.EnqueueProgress(ctx, p.ID, j.ID, chatID, t.now().Add(t.editEvery)); err != nil {
		t.log.Error("telegram: cannot queue the progress panel", "err", err)
	}
	mid, err := t.jobCard(ctx, p, chatID, messageID, j, true)
	if err != nil {
		return err
	}
	tx := t.text(ctx, p)
	hash := hashText(jobText(tx, j))
	if err := t.store.BindProgress(ctx, p.ID, j.ID, mid, hash, j.State); err != nil {
		t.log.Warn("telegram: cannot bind the progress panel", "err", err)
	}
	return nil
}

func (t *Telegram) jobButtons(tx texts, j state.Job) [][]button {
	control := func(label, action string) button { return act(label, action, j.ID, j.Revision, "") }
	var rows [][]button
	switch j.State {
	case state.JobQueued, state.JobRunning, state.JobReconciling, state.JobPausing, state.JobCancelling:
		rows = append(rows, []button{control(tx.pause, "pause"), styled(control(tx.cancel, "job_cancel"), "danger")})
	case state.JobPaused:
		rows = append(rows, []button{styled(control(tx.resume, "resume"), "success"), styled(control(tx.cancel, "job_cancel"), "danger")})
	case state.JobBlocked, state.JobFailed, state.JobPartial:
		if quotaBlocked(j) {
			// A quota wait resumes by itself (Q1): retrying blindly would only
			// hit the same limit.
			rows = append(rows, []button{styled(control(tx.cancel, "job_cancel"), "danger")})
			break
		}
		rows = append(rows, []button{styled(control(tx.retry, "retry"), "success")})
	}
	rows = append(rows, []button{act(tx.details, "job", j.ID, j.Revision, ""), nav(tx.jobs, "jobs")})
	return rows
}

func (t *Telegram) showError(ctx context.Context, p app.Principal, chatID int64, messageID int, err error) error {
	tx := t.text(ctx, p)
	g := tx.g()
	ae := app.Classify(err)
	if ae == nil {
		ae = app.Fail(500, "internal_error", "Internal error.")
	}
	msg := ae.Message
	if local := tx.errMsg[ae.Code]; local != "" {
		msg = local
	} else if local := g.errMsg[ae.Code]; local != "" {
		msg = local
	}
	var busy state.ErrCollectionBusy
	if errors.As(err, &busy) {
		msg = fmt.Sprintf(g.busyMsg, busy.ActiveJobID)
	}
	hint := g.errHint[ae.Code]
	if hint == "" {
		hint = g.errHintDefault
	}
	return t.showCard(ctx, p, chatID, messageID, fmt.Sprintf("⚠️ %s: %s\n💡 %s", tx.problemLabel, escape(msg), escape(hint)),
		[]button{nav(tx.back, "home")})
}
