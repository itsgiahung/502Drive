package telegram

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/state"
)

// Batch G, slice 1b: copy-again (L2), selective-sync presets (S5), name
// conflict policy (N1), skipped-by-code (K1) and quota waits (Q1/S3).

func (h *harness) srcLink() string { return "https://drive.google.com/drive/folders/" + h.src.ID }

// finishCopy marks the newest job done and its collection ready, like the engine.
func (h *harness) finishCopy() state.Job {
	h.t.Helper()
	ctx := context.Background()
	jobs := h.jobs()
	if len(jobs) == 0 {
		h.t.Fatal("no job to finish")
	}
	j, err := h.store.UpdateJob(ctx, jobs[0].ID, func(j *state.Job) (string, bool) {
		j.State = state.JobSucceeded
		return "finished", true
	})
	if err != nil {
		h.t.Fatal(err)
	}
	if err = h.store.SetCollectionState(ctx, j.CollectionID, "ready"); err != nil {
		h.t.Fatal(err)
	}
	return j
}

func (h *harness) collectionCount() int {
	h.t.Helper()
	cols, err := h.svc.Collections(context.Background(), app.Owner, true)
	if err != nil {
		h.t.Fatal(err)
	}
	return len(cols)
}

// ---- L2 ----

func TestCopyingTheSameSourceAgainAsksUpdateOrNewInsteadOfDoubling(t *testing.T) {
	h := newHarness(t, 100)
	g := vietnamese.g()
	h.say(h.srcLink())
	h.tap(h.lastCard(), vietnamese.copyNow)
	first := h.finishCopy()

	h.say(h.srcLink())
	h.tap(h.lastCard(), vietnamese.copyNow)
	choice := h.lastCard()
	for _, want := range []string{g.updateOld, g.createNew} {
		if !hasButton(choice, want) {
			t.Fatalf("the choice card lacks %q: %v\n%s", want, choice.ButtonTexts, choice.Text)
		}
	}
	if len(h.jobs()) != 1 || h.collectionCount() != 1 {
		t.Fatalf("asking must not create anything: %d jobs, %d collections", len(h.jobs()), h.collectionCount())
	}

	h.tap(choice, g.updateOld)
	jobs := h.jobs()
	if len(jobs) != 2 || jobs[0].CollectionID != first.CollectionID || jobs[0].Trigger == "initial" {
		t.Fatalf("update must refresh the same collection with a delta run: %+v", jobs)
	}
	if h.collectionCount() != 1 {
		t.Fatalf("update created another collection")
	}
}

func TestCopyingAgainCanStillCreateANewCopyOnPurpose(t *testing.T) {
	h := newHarness(t, 100)
	g := vietnamese.g()
	h.say(h.srcLink())
	h.tap(h.lastCard(), vietnamese.copyNow)
	h.finishCopy()

	h.say(h.srcLink())
	h.tap(h.lastCard(), vietnamese.copyNow)
	choice := h.lastCard()
	h.tap(choice, g.createNew)
	if h.collectionCount() != 2 {
		t.Fatalf("a deliberate new copy should create a second collection, have %d", h.collectionCount())
	}
	// The choice buttons are one-shot.
	h.api.push(h.click(choice, g.createNew))
	h.pull()
	if h.collectionCount() != 2 || h.lastAnswer() != vietnamese.expired {
		t.Fatalf("a consumed choice created again (%d collections, answer %q)", h.collectionCount(), h.lastAnswer())
	}
}

func TestPasteToCopyAlsoAsksWhenTheSourceWasAlreadyCopied(t *testing.T) {
	h := newHarness(t, 100)
	g := vietnamese.g()
	h.enableAutoCopy()
	h.say(h.srcLink())
	if len(h.jobs()) != 1 {
		t.Fatalf("auto copy should have copied once, jobs = %d", len(h.jobs()))
	}
	h.finishCopy()

	h.say(h.srcLink())
	if len(h.jobs()) != 1 {
		t.Fatalf("auto copy doubled the copy: %d jobs", len(h.jobs()))
	}
	if !hasButton(h.lastCard(), g.updateOld) || !hasButton(h.lastCard(), g.createNew) {
		t.Fatalf("auto copy must ask: %v", h.lastCard().ButtonTexts)
	}
}

func TestUpdatingARetiredCopyExplainsWhatToDo(t *testing.T) {
	h := newHarness(t, 100)
	g := vietnamese.g()
	h.say(h.srcLink())
	h.tap(h.lastCard(), vietnamese.copyNow)
	first := h.finishCopy()
	c := h.collectionOf(first.CollectionID)
	if _, err := h.svc.RetireCollection(context.Background(), app.Owner, "retire-1", c.ID, c.Revision); err != nil {
		t.Fatal(err)
	}

	h.say(h.srcLink())
	h.tap(h.lastCard(), vietnamese.copyNow)
	h.tap(h.lastCard(), g.updateOld)
	text := h.lastCard().Text
	if !strings.Contains(text, g.errHint["collection_retired"]) {
		t.Fatalf("expected advice for a retired copy:\n%s", text)
	}
	if len(h.jobs()) != 1 {
		t.Fatalf("nothing may be admitted for a retired copy: %d jobs", len(h.jobs()))
	}
}

// ---- S5 ----

func (h *harness) addMixedFiles() {
	h.drive.Put("clip.mp4", "video/mp4", h.src.ID, 100)
	h.drive.Put("report.pdf", "application/pdf", h.src.ID, 100)
	h.drive.Put("disk.img", "application/octet-stream", h.src.ID, 2<<30)
}

func TestVideoPresetShowsKeptAndExcludedAndIsSubmitted(t *testing.T) {
	h := newHarness(t, 100)
	g := vietnamese.g()
	h.addMixedFiles()
	h.say(h.srcLink())
	h.tap(h.lastCard(), g.presetNoVideo)
	card := h.lastCard()
	if !strings.Contains(card.Text, "clip.mp4") {
		t.Fatalf("the excluded sample must be listed:\n%s", card.Text)
	}
	if !strings.Contains(card.Text, strings.Split(g.filterLine, "%d")[0]) || !strings.Contains(card.Text, g.filterScope) {
		t.Fatalf("the kept/excluded line and its scope note are missing:\n%s", card.Text)
	}
	if !strings.Contains(card.Text, "1") || !hasButton(card, "✓ "+g.presetNoVideo) {
		t.Fatalf("the active preset must be marked: %v", card.ButtonTexts)
	}

	h.tap(card, vietnamese.copyNow)
	jobs := h.jobs()
	if len(jobs) != 1 || len(jobs[0].Filter.Exclude) != 1 || jobs[0].Filter.Exclude[0] != "class:video" {
		t.Fatalf("the job did not get the preset: %+v", jobs)
	}
}

func TestDocumentsOnlyAndSizePresetsMapToTheFilterSpec(t *testing.T) {
	h := newHarness(t, 100)
	g := vietnamese.g()
	h.addMixedFiles()
	h.say(h.srcLink())
	h.tap(h.lastCard(), g.presetDocs)
	h.tap(h.lastCard(), g.presetSmall)
	card := h.lastCard()
	if !strings.Contains(card.Text, "disk.img") || !strings.Contains(card.Text, "clip.mp4") {
		t.Fatalf("both excluded files should be sampled:\n%s", card.Text)
	}
	h.tap(card, vietnamese.copyNow)
	f := h.jobs()[0].Filter
	if len(f.Include) != 1 || f.Include[0] != "class:doc" || f.MaxSize != 1<<30 {
		t.Fatalf("filter = %+v", f)
	}
}

func TestPresetsToggleOffAndSurviveARename(t *testing.T) {
	h := newHarness(t, 100)
	g := vietnamese.g()
	h.addMixedFiles()
	h.say(h.srcLink())
	h.tap(h.lastCard(), g.presetNoVideo)
	h.tap(h.lastCard(), vietnamese.renameBtn)
	h.say("Named copy")
	card := h.lastCard()
	if !hasButton(card, "✓ "+g.presetNoVideo) || !strings.Contains(card.Text, "Named copy") {
		t.Fatalf("the preset must survive a rename: %v\n%s", card.ButtonTexts, card.Text)
	}
	h.tap(card, "✓ "+g.presetNoVideo) // toggle off
	card = h.lastCard()
	if hasButton(card, "✓ ") || strings.Contains(card.Text, "clip.mp4") {
		t.Fatalf("toggling off must clear the filter: %v\n%s", card.ButtonTexts, card.Text)
	}
	h.tap(card, vietnamese.copyNow)
	if f := h.jobs()[0].Filter; f.Active() {
		t.Fatalf("the job still carries a filter: %+v", f)
	}
}

// ---- N1 ----

func TestNameConflictPolicyCyclesOnThePreviewAndIsSubmitted(t *testing.T) {
	h := newHarness(t, 100)
	g := vietnamese.g()
	h.say(h.srcLink())
	card := h.lastCard()
	if !strings.Contains(strings.Join(card.ButtonTexts, "|"), g.conflictName("rename")) {
		t.Fatalf("the default policy is rename: %v", card.ButtonTexts)
	}
	h.tap(card, g.conflictName("rename"))
	card = h.lastCard()
	if !hasButton(card, g.conflictName("skip")) {
		t.Fatalf("the next policy is skip: %v", card.ButtonTexts)
	}
	h.tap(card, g.conflictName("skip"))
	card = h.lastCard()
	if !hasButton(card, g.conflictName("update")) {
		t.Fatalf("the third policy is update: %v", card.ButtonTexts)
	}
	h.tap(card, vietnamese.copyNow)
	if got := h.jobs()[0].OnConflict; got != "update" {
		t.Fatalf("job on_conflict = %q", got)
	}
}

// ---- K1 ----

func TestFinishedJobCardCountsSkippedItemsByCode(t *testing.T) {
	h := newHarness(t, 100)
	ctx := context.Background()
	g := vietnamese.g()
	job := admit(t, h.store, "owner", "Skippy")
	items, err := h.store.Items(ctx, job.CollectionID)
	if err != nil || len(items) == 0 {
		t.Fatalf("items: %v %v", items, err)
	}
	if err = h.store.SkipItem(ctx, items[0], state.SkipCannotCopy); err != nil {
		t.Fatal(err)
	}
	finish(t, h.store, job.ID, state.JobSucceeded, "")
	got, err := h.svc.Job(ctx, app.Owner, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	text := finalNoticeText(vietnamese, got)
	if !strings.Contains(text, g.skipLabel(state.SkipCannotCopy)) || !strings.Contains(text, "×1") {
		t.Fatalf("the result must count skipped items by reason:\n%s", text)
	}
	en := finalNoticeText(english, got)
	if !strings.Contains(en, english.g().skipLabel(state.SkipCannotCopy)) {
		t.Fatalf("english result lacks the reason:\n%s", en)
	}
}

func TestSkippedLineIsSortedAndTruncatesUnknownCodes(t *testing.T) {
	j := state.Job{ID: "job_x", RootName: "R", State: state.JobSucceeded,
		Counts: state.Counts{Skipped: 6, SkippedByCode: map[string]int{
			state.SkipFiltered: 3, state.SkipCannotCopy: 2, "brand_new_code": 1}}}
	text := jobText(vietnamese, j)
	a := strings.Index(text, vietnamese.g().skipLabel(state.SkipFiltered))
	b := strings.Index(text, vietnamese.g().skipLabel(state.SkipCannotCopy))
	if a < 0 || b < 0 || a > b {
		t.Fatalf("most frequent reason should come first:\n%s", text)
	}
	if !strings.Contains(text, "brand_new_code") {
		t.Fatalf("an unknown code must stay visible:\n%s", text)
	}
}

// ---- Q1 / S3 ----

func TestQuotaBlockedJobShowsTheWaitAndHasNoBlindRetry(t *testing.T) {
	h := newHarness(t, 100)
	g := vietnamese.g()
	ctx := context.Background()
	tz := "Asia/Ho_Chi_Minh"
	if _, err := h.svc.PatchPreferences(ctx, app.Owner, "tz-q", app.PreferencePatch{Timezone: &tz}); err != nil {
		t.Fatal(err)
	}
	job := h.queuedJob("owner", "Quota")
	resume := h.clock().Add(3 * time.Hour)
	if _, err := h.store.UpdateJob(ctx, job.ID, func(j *state.Job) (string, bool) {
		j.State = state.JobBlocked
		j.LastError = &state.Failure{Code: "quota_exceeded", Message: "daily limit"}
		j.ResumeAfter = &resume
		return "blocked", true
	}); err != nil {
		t.Fatal(err)
	}
	j, _ := h.svc.Job(ctx, app.Owner, job.ID)
	if _, err := h.tg.jobCard(ctx, app.Owner, h.ownerID, 12, j, false); err != nil {
		t.Fatal(err)
	}
	card := h.lastCard()
	loc, _ := time.LoadLocation(tz)
	want := resume.In(loc).Format("15:04")
	if !strings.Contains(card.Text, want) || !strings.Contains(card.Text, strings.Split(g.quotaWait, "%s")[0]) {
		t.Fatalf("the card must say when the quota wait ends (%s):\n%s", want, card.Text)
	}
	if hasButton(card, vietnamese.retry) {
		t.Fatalf("a quota wait resumes by itself; no retry button: %v", card.ButtonTexts)
	}
	if !hasButton(card, vietnamese.cancel) {
		t.Fatalf("the user can still cancel: %v", card.ButtonTexts)
	}
}

func TestBlockedJobWithoutAQuotaWaitKeepsItsRetryButton(t *testing.T) {
	h := newHarness(t, 100)
	ctx := context.Background()
	job := h.queuedJob("owner", "Needs you")
	if _, err := h.store.UpdateJob(ctx, job.ID, func(j *state.Job) (string, bool) {
		j.State = state.JobBlocked
		j.LastError = &state.Failure{Code: "destination_not_writable", Message: "no write access"}
		return "blocked", true
	}); err != nil {
		t.Fatal(err)
	}
	j, _ := h.svc.Job(ctx, app.Owner, job.ID)
	if _, err := h.tg.jobCard(ctx, app.Owner, h.ownerID, 12, j, false); err != nil {
		t.Fatal(err)
	}
	if !hasButton(h.lastCard(), vietnamese.retry) {
		t.Fatalf("a blocked job that needs action keeps Retry: %v", h.lastCard().ButtonTexts)
	}
}
