package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/filter"
	"github.com/itsgiahung/502Drive/internal/state"
)

// Copy options on the preview card: selective-sync presets (S5), the name
// conflict policy (N1) and the answer to "this source was already copied" (L2).
// All of them travel in the private callback payload, so the preview can be
// rebuilt after a rename or a folder change without asking again.

const (
	presetVideo = "novideo"
	presetDocs  = "docs"
	presetBig   = "small"

	// bigFileLimit is the "Skip files > 1 GB" preset.
	bigFileLimit int64 = 1 << 30
)

var presetOrder = []string{presetVideo, presetDocs, presetBig}

// conflictCycle is the order the policy button walks through.
var conflictCycle = []string{"rename", "skip", "update"}

// presetSpec is the filter a set of presets stands for.
func presetSpec(ids []string) filter.Spec {
	var f filter.Spec
	for _, id := range presetOrder {
		if !slices.Contains(ids, id) {
			continue
		}
		switch id {
		case presetVideo:
			f.Exclude = append(f.Exclude, "class:video")
		case presetDocs:
			f.Include = append(f.Include, "class:doc")
		case presetBig:
			f.MaxSize = bigFileLimit
		}
	}
	return f
}

// presetsOf recovers the presets from a stored filter, so the buttons always
// reflect what the plan will really do.
func presetsOf(f filter.Spec) []string {
	var ids []string
	if slices.Contains(f.Exclude, "class:video") {
		ids = append(ids, presetVideo)
	}
	if slices.Contains(f.Include, "class:doc") {
		ids = append(ids, presetDocs)
	}
	if f.MaxSize == bigFileLimit {
		ids = append(ids, presetBig)
	}
	return ids
}

func togglePreset(ids []string, id string) []string {
	var out []string
	for _, p := range presetOrder {
		on := slices.Contains(ids, p)
		if p == id {
			on = !on
		}
		if on {
			out = append(out, p)
		}
	}
	return out
}

// conflictOf normalises the stored policy: the default is "".
func conflictOf(policy string) string {
	if policy == "" || policy == "rename" {
		return ""
	}
	return policy
}

func nextConflict(policy string) string {
	cur := policy
	if cur == "" {
		cur = "rename"
	}
	i := slices.Index(conflictCycle, cur)
	return conflictOf(conflictCycle[(i+1)%len(conflictCycle)])
}

// request rebuilds the inspect request for this payload.
func (pl planPayload) request() app.PlanRequest {
	return app.PlanRequest{Source: pl.Source, DestinationID: pl.Destination, RootName: pl.Root,
		Filter: presetSpec(pl.Presets), OnConflict: pl.Conflict}
}

// optionRows are the preset and policy buttons of the preview card.
func optionRows(tx texts, view app.PlanView, base planPayload) [][]button {
	g := tx.g()
	active := presetsOf(view.Filter)
	mk := func(label, id string) button {
		next := base
		next.Presets = togglePreset(active, id)
		text := label
		if slices.Contains(active, id) {
			text = "✓ " + label
		}
		return act(text, "preview_opt", view.ID, view.Revision, planJSON(next))
	}
	policy := conflictOf(view.OnConflict)
	nextPolicy := base
	nextPolicy.Presets = active
	nextPolicy.Conflict = nextConflict(policy)
	cur := policy
	if cur == "" {
		cur = "rename"
	}
	return [][]button{
		{mk(g.presetNoVideo, presetVideo), mk(g.presetDocs, presetDocs)},
		{mk(g.presetSmall, presetBig)},
		{act(g.conflictName(cur), "preview_opt", view.ID, view.Revision, planJSON(nextPolicy))},
	}
}

// filterPreview is the "kept / excluded" block of the preview (top level only).
func filterPreview(tx texts, view app.PlanView) string {
	if !view.Filter.Active() {
		return ""
	}
	g := tx.g()
	var b strings.Builder
	fmt.Fprintf(&b, g.filterLine, view.Preview.FilterIncluded, view.Preview.FilterExcluded)
	fmt.Fprintf(&b, "\n<i>%s</i>\n", escape(g.filterScope))
	if n := len(view.Preview.ExcludedSamples); n > 0 {
		names := make([]string, 0, 3)
		for _, s := range view.Preview.ExcludedSamples[:min(n, 3)] {
			names = append(names, escape(short(s, 40)))
		}
		fmt.Fprintf(&b, "%s: %s\n", g.filterSamples, strings.Join(names, ", "))
	}
	return b.String()
}

// actPreviewOpt re-inspects with the toggled option and shows the new preview.
func (t *Telegram) actPreviewOpt(ctx context.Context, p app.Principal, st state.Callback, tx texts) error {
	var pl planPayload
	if json.Unmarshal([]byte(st.Payload), &pl) != nil || pl.Source == "" {
		return t.showError(ctx, p, st.ChatID, st.MessageID, app.Fail(409, "plan_changed", tx.confirmDest))
	}
	return t.replay(ctx, p, st.ChatID, st.MessageID, pl, "tg-cb:"+st.ID)
}

// ---- L2: copy again ----

// existingCopy reports the 409 an unanswered "copy it again" gets.
func existingCopy(err error) bool {
	ae := app.Classify(err)
	return ae != nil && ae.Code == "already_copied"
}

// copyChoiceCard asks whether to refresh the old copy or make a new one. The
// two buttons are one-shot and bound to the plan revision of the preview.
func (t *Telegram) copyChoiceCard(ctx context.Context, p app.Principal, chatID int64, messageID int, planID string, revision int, payload string) error {
	tx := t.text(ctx, p)
	g := tx.g()
	return t.showCard(ctx, p, chatID, messageID, g.alreadyCopied,
		[]button{styled(act(g.updateOld, "copy_update", planID, revision, payload), "success")},
		[]button{act(g.createNew, "copy_new", planID, revision, payload)},
		[]button{styled(act("✖ "+tx.cancel, "cancel_preview", planID, revision, ""), "danger")})
}

// actCopyMode submits with an explicit answer (update or new).
func (t *Telegram) actCopyMode(ctx context.Context, p app.Principal, st state.Callback, mode string) error {
	res, err := t.svc.SubmitMode(ctx, p, "tg-cb:"+st.ID, st.Object, st.Revision, mode)
	if err == nil {
		return t.submitted(ctx, p, st.ChatID, st.MessageID, res.Job)
	}
	if previewOnSubmitError(err) {
		var pl planPayload
		if json.Unmarshal([]byte(st.Payload), &pl) == nil && pl.Source != "" {
			return t.replay(ctx, p, st.ChatID, st.MessageID, pl, "tg-cb:"+st.ID)
		}
	}
	return t.showError(ctx, p, st.ChatID, st.MessageID, err)
}
