package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/state"
)

// Sync after copy (S8). The default is the after_copy_sync_mode preference
// (Settings page); the preview card can override it for one copy. The choice
// travels in the private callback payload and reaches the job through
// SubmitModeInBatchWithSync, the same field POST /v1/jobs and `copy --sync`
// set, so the daemon (not the chat) applies it once the initial copy
// succeeds. "" means "use the preference".

// afterSyncCycle is the order the preview button walks through.
var afterSyncCycle = []string{"", state.AfterCopyOff, state.AfterCopyRealtime, state.AfterCopyHourly,
	state.AfterCopySixHourly, state.AfterCopyDaily}

// afterSyncModes are the choices the Settings page offers.
var afterSyncModes = afterSyncCycle[1:]

func nextAfterSync(cur string) string {
	i := slices.Index(afterSyncCycle, cur)
	return afterSyncCycle[(i+1)%len(afterSyncCycle)]
}

// afterSyncName is the chat-language name of a mode; "" reads as off.
func afterSyncName(ig igTexts, mode string) string {
	if mode == "" {
		mode = state.AfterCopyOff
	}
	if v := ig.syncAfterNames[mode]; v != "" {
		return v
	}
	return mode
}

// afterSyncOf recovers the chosen mode from a preview payload. Anything this
// build does not understand reads as "use the preference".
func afterSyncOf(payload string) string {
	var pl planPayload
	if json.Unmarshal([]byte(payload), &pl) != nil || pl.AfterSync == "" || !state.ValidAfterCopySyncMode(pl.AfterSync) {
		return ""
	}
	return pl.AfterSync
}

// submitCopy admits a copy with the sync choice of the payload.
func (t *Telegram) submitCopy(ctx context.Context, p app.Principal, key, planID string, revision int, mode, payload string) (state.Result, error) {
	return t.svc.SubmitModeInBatchWithSync(ctx, p, key, planID, revision, mode, "", afterSyncOf(payload))
}

// afterSyncRow is the preview button that cycles the mode for this copy.
func (t *Telegram) afterSyncRow(ctx context.Context, p app.Principal, tx texts, view app.PlanView, base planPayload) []button {
	ig := tx.ig()
	name := ""
	if base.AfterSync == "" {
		prefs, _ := t.svc.Preferences(ctx, p)
		name = fmt.Sprintf(ig.syncAfterDefault, afterSyncName(ig, prefs.AfterCopySyncMode))
	} else {
		name = afterSyncName(ig, base.AfterSync)
	}
	next := base
	next.AfterSync = nextAfterSync(base.AfterSync)
	return []button{act(fmt.Sprintf(ig.syncAfterBtn, name), "preview_opt", view.ID, view.Revision, planJSON(next))}
}

// afterSyncSettings is the Settings block for the default mode.
func (t *Telegram) afterSyncSettings(tx texts, prefs state.Preferences) (string, [][]button) {
	ig := tx.ig()
	cur := prefs.AfterCopySyncMode
	if cur == "" {
		cur = state.AfterCopyOff
	}
	var cells []button
	for _, mode := range afterSyncModes {
		label := afterSyncName(ig, mode)
		if mode == cur {
			label = "✓ " + label
		}
		cells = append(cells, act(label, "aftersync_set", mode, 0, ""))
	}
	text := fmt.Sprintf(ig.syncAfterTitle, afterSyncName(ig, cur)) + "\n"
	return text, [][]button{cells[:3], cells[3:]}
}

// actAfterSyncDefault saves the default through the preferences command (the
// same one the CLI and the HTTP API use).
func (t *Telegram) actAfterSyncDefault(ctx context.Context, p app.Principal, st state.Callback) error {
	mode := st.Object
	if !slices.Contains(afterSyncModes, mode) {
		return t.page(ctx, p, st.ChatID, st.MessageID, "settings")
	}
	if _, err := t.svc.PatchPreferences(ctx, p, "tg-cb:"+st.ID, app.PreferencePatch{AfterCopySyncMode: &mode}); err != nil {
		return t.showError(ctx, p, st.ChatID, st.MessageID, err)
	}
	return t.page(ctx, p, st.ChatID, st.MessageID, "settings")
}
