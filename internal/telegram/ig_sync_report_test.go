package telegram

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/state"
)

// Batch IG, slice 3: sync after copy (S8), the problem report file (X1), the
// search-backed /find, server-side job filters and the daily report settings.

// ---- S8 ----

func syncLabel(ig igTexts, mode string) string {
	return fmt.Sprintf(ig.syncAfterBtn, ig.syncAfterNames[mode])
}

func defaultSyncLabel(ig igTexts, pref string) string {
	return fmt.Sprintf(ig.syncAfterBtn, fmt.Sprintf(ig.syncAfterDefault, ig.syncAfterNames[pref]))
}

func TestPreviewCyclesTheSyncAfterCopyModeAndSubmitsIt(t *testing.T) {
	h := newHarness(t, 100)
	ig := vietnamese.ig()
	h.say(h.srcLink())
	card := h.lastCard()
	if !hasButton(card, defaultSyncLabel(ig, "off")) {
		t.Fatalf("the preview starts on the preference (off): %v", card.ButtonTexts)
	}
	// default -> off -> realtime -> hourly
	for _, next := range []string{"off", "realtime", "hourly"} {
		h.tap(card, "🔄")
		card = h.lastCard()
		if !hasButton(card, syncLabel(ig, next)) {
			t.Fatalf("expected %q after the tap: %v", syncLabel(ig, next), card.ButtonTexts)
		}
	}
	h.tap(card, vietnamese.copyNow)
	if got := h.jobs()[0].AfterCopySync; got != state.AfterCopyHourly {
		t.Fatalf("the job asked for %q, want hourly", got)
	}
}

func TestSyncChoiceSurvivesRenameAndOtherOptions(t *testing.T) {
	h := newHarness(t, 100)
	ig := vietnamese.ig()
	h.say(h.srcLink())
	h.tap(h.lastCard(), "🔄") // off
	h.tap(h.lastCard(), "🔄") // realtime
	h.tap(h.lastCard(), vietnamese.renameBtn)
	h.say("Renamed")
	card := h.lastCard()
	if !hasButton(card, syncLabel(ig, "realtime")) || !strings.Contains(card.Text, "Renamed") {
		t.Fatalf("a rename must keep the sync choice: %v\n%s", card.ButtonTexts, card.Text)
	}
	h.tap(card, vietnamese.g().presetNoVideo)
	if !hasButton(h.lastCard(), syncLabel(ig, "realtime")) {
		t.Fatalf("a preset must keep the sync choice: %v", h.lastCard().ButtonTexts)
	}
	h.tap(h.lastCard(), vietnamese.copyNow)
	if got := h.jobs()[0].AfterCopySync; got != state.AfterCopyRealtime {
		t.Fatalf("job after-copy sync = %q", got)
	}
}

func TestSettingsSetTheDefaultSyncAfterCopyThroughThePreferences(t *testing.T) {
	h := newHarness(t, 100)
	ig := vietnamese.ig()
	op := h.addOperator(555)
	card := h.openSettings()
	if !strings.Contains(card.Text, fmt.Sprintf(ig.syncAfterTitle, ig.syncAfterNames["off"])) {
		t.Fatalf("settings show the current default:\n%s", card.Text)
	}
	h.tap(card, ig.syncAfterNames["daily"])
	if got := h.prefs(app.Owner).AfterCopySyncMode; got != state.AfterCopyDaily {
		t.Fatalf("the preference = %q, want daily", got)
	}
	card = h.lastCard()
	if !hasButton(card, "✓ "+ig.syncAfterNames["daily"]) || !strings.Contains(card.Text, fmt.Sprintf(ig.syncAfterTitle, ig.syncAfterNames["daily"])) {
		t.Fatalf("the new default must be marked: %v\n%s", card.ButtonTexts, card.Text)
	}
	if got := h.prefs(op).AfterCopySyncMode; got != state.AfterCopyOff {
		t.Fatalf("the operator's preference changed to %q", got)
	}

	// The preview now shows the preference as the default and a job inherits it.
	h.say(h.srcLink())
	if !hasButton(h.lastCard(), defaultSyncLabel(ig, "daily")) {
		t.Fatalf("the preview must show the preference: %v", h.lastCard().ButtonTexts)
	}
	h.tap(h.lastCard(), vietnamese.copyNow)
	job := h.jobs()[0]
	if job.AfterCopySync != "" {
		t.Fatalf("an untouched choice leaves the job on the preference, got %q", job.AfterCopySync)
	}
	// The daemon applies it once the copy has finished.
	ctx := context.Background()
	h.finishCopy()
	if _, err := h.store.RunFinished(ctx, job.ID); err != nil {
		t.Fatal(err)
	}
	h.advance(time.Minute)
	if applied, failed, err := h.svc.ApplyAfterCopySync(ctx); err != nil || applied != 1 || failed != 0 {
		t.Fatalf("apply = %d/%d, %v", applied, failed, err)
	}
	sub, err := h.store.GetSubscription(ctx, job.CollectionID)
	if err != nil || sub.Schedule == nil || sub.State != state.SyncActive {
		t.Fatalf("the daily default did not become a subscription: %+v, %v", sub, err)
	}
}

func TestExplicitOffOverridesADailyDefault(t *testing.T) {
	h := newHarness(t, 100)
	ig := vietnamese.ig()
	ctx := context.Background()
	daily := state.AfterCopyDaily
	if _, err := h.svc.PatchPreferences(ctx, app.Owner, "pref-daily", app.PreferencePatch{AfterCopySyncMode: &daily}); err != nil {
		t.Fatal(err)
	}
	h.say(h.srcLink())
	h.tap(h.lastCard(), "🔄") // default -> off
	if !hasButton(h.lastCard(), syncLabel(ig, "off")) {
		t.Fatalf("labels: %v", h.lastCard().ButtonTexts)
	}
	h.tap(h.lastCard(), vietnamese.copyNow)
	job := h.jobs()[0]
	if job.AfterCopySync != state.AfterCopyOff {
		t.Fatalf("explicit off was stored as %q", job.AfterCopySync)
	}
	h.finishCopy()
	if _, err := h.store.RunFinished(ctx, job.ID); err != nil {
		t.Fatal(err)
	}
	h.advance(time.Minute)
	if applied, _, err := h.svc.ApplyAfterCopySync(ctx); err != nil || applied != 0 {
		t.Fatalf("off must not subscribe: applied %d, %v", applied, err)
	}
}

func TestForgedSyncModeIsIgnored(t *testing.T) {
	h := newHarness(t, 100)
	h.say(h.srcLink())
	card := h.lastCard()
	h.pushCallback(h.ownerID, card.MessageID, h.forge("owner", h.ownerID, "aftersync_set", "every_second", 0, ""))
	if got := h.prefs(app.Owner).AfterCopySyncMode; got != state.AfterCopyOff {
		t.Fatalf("an unknown mode was stored: %q", got)
	}
	if afterSyncOf(`{"source":"x","as":"every_second"}`) != "" || afterSyncOf("") != "" || afterSyncOf("{") != "" {
		t.Fatal("unknown or malformed payloads must read as 'use the preference'")
	}
}

// ---- X1 ----

// problemJob makes a job with one failed, one blocked and one skipped item.
func (h *harness) problemJob(principal string, extra int) state.Job {
	h.t.Helper()
	ctx := context.Background()
	job := h.queuedJob(principal, "Problems")
	roots, err := h.store.Items(ctx, job.CollectionID)
	if err != nil || len(roots) == 0 {
		h.t.Fatalf("items: %v %v", roots, err)
	}
	children := []state.Item{
		{SourceID: "s-fail", Kind: state.KindFile, Name: "a.txt", RelPath: "dir/a.txt", State: state.ItemFailed, ErrorCode: "retry_exhausted"},
		{SourceID: "s-block", Kind: state.KindFile, Name: "b.txt", RelPath: "dir/b.txt", State: state.ItemBlocked, ErrorCode: "quota_exceeded"},
		{SourceID: "s-skip", Kind: state.KindFile, Name: "=HYPERLINK(\"http://x\")", RelPath: "=HYPERLINK(\"http://x\")", State: state.ItemSkipped, ErrorCode: state.SkipCannotCopy},
	}
	for i := range extra {
		children = append(children, state.Item{SourceID: fmt.Sprintf("s-extra-%05d", i), Kind: state.KindFile, Name: "x", RelPath: fmt.Sprintf("many/%05d.txt", i),
			State: state.ItemSkipped, ErrorCode: state.SkipFiltered})
	}
	if err = h.store.CommitPage(ctx, roots[0], children, ""); err != nil {
		h.t.Fatal(err)
	}
	if _, err = h.store.UpdateJob(ctx, job.ID, func(j *state.Job) (string, bool) {
		j.State = state.JobPartial
		j.Counts.Failed, j.Counts.Blocked, j.Counts.Skipped = 1, 1, 1+extra
		j.LastError = &state.Failure{Code: "retry_exhausted", Message: "open /home/admin/.config/502drive/state.db: permission denied"}
		return "finished", true
	}); err != nil {
		h.t.Fatal(err)
	}
	got, err := h.svc.Job(ctx, app.Owner, job.ID)
	if err != nil {
		h.t.Fatal(err)
	}
	return got
}

func readCSV(t *testing.T, content []byte) [][]string {
	t.Helper()
	rows, err := csv.NewReader(strings.NewReader(string(content))).ReadAll()
	if err != nil {
		t.Fatalf("not valid CSV: %v\n%s", err, content)
	}
	return rows
}

func TestProblemReportSendsACSVWithReasonsAndAdvice(t *testing.T) {
	h := newHarness(t, 100)
	ig := vietnamese.ig()
	job := h.problemJob("owner", 0)

	card := h.openJobCard(job.ID)
	if !hasButton(card, ig.reportBtn) {
		t.Fatalf("a job with problems offers the report: %v", card.ButtonTexts)
	}
	h.tap(card, ig.reportBtn)

	docs := h.api.documents()
	if len(docs) != 1 {
		t.Fatalf("want one file, got %d", len(docs))
	}
	doc := docs[0]
	if doc.ChatID != h.ownerID || doc.Filename != "report-"+job.ID+".csv" {
		t.Fatalf("document = %+v", doc)
	}
	rows := readCSV(t, doc.Content)
	if len(rows) != 4 || strings.Join(rows[0], ",") != "state,code,reason,path,attempts,source_id,destination_id" {
		t.Fatalf("the file has the CLI columns and one row per problem: %v", rows)
	}
	got := map[string]string{}
	for _, r := range rows[1:] {
		got[r[5]] = r[1]
	}
	if got["s-fail"] != "retry_exhausted" || got["s-block"] != "quota_exceeded" || got["s-skip"] != state.SkipCannotCopy {
		t.Fatalf("codes = %v", got)
	}
	// A cell a spreadsheet would run as a formula is neutralised.
	for _, r := range rows[1:] {
		if r[5] == "s-skip" && !strings.HasPrefix(r[3], "'=") {
			t.Fatalf("formula cell is not guarded: %q", r[3])
		}
	}
	// The advice names the reason and what to do for each kind of problem.
	g := vietnamese.g()
	for _, want := range []string{
		fmt.Sprintf(ig.reportCaption, "Problems", 3), ig.reportHow,
		g.skipLabels[state.SkipCannotCopy], ig.itemHint[state.SkipCannotCopy],
		ig.itemHint["quota_exceeded"],
	} {
		if !strings.Contains(doc.Caption, want) {
			t.Fatalf("the caption lacks %q:\n%s", want, doc.Caption)
		}
	}
	if len([]rune(doc.Caption)) > 1024 {
		t.Fatalf("caption is %d runes, over Telegram's limit", len([]rune(doc.Caption)))
	}
	// Nothing about the daemon's machine or the bot leaks into the file.
	whole := string(doc.Content) + doc.Caption
	for _, secret := range []string{"/home/", "state.db", "permission denied", testBotToken, "master.key", ".config"} {
		if strings.Contains(whole, secret) {
			t.Fatalf("the report leaks %q:\n%s", secret, whole)
		}
	}
}

func TestProblemReportButtonOnlyAppearsWhenThereAreProblems(t *testing.T) {
	h := newHarness(t, 100)
	ig := vietnamese.ig()
	clean := h.queuedJob("owner", "Clean")
	if hasButton(h.openJobCard(clean.ID), ig.reportBtn) {
		t.Fatal("a clean job must not offer a report")
	}
	// A stale button for a job that has no problem items answers politely.
	h.pushCallback(h.ownerID, h.lastCard().MessageID, h.forge("owner", h.ownerID, "job_report", clean.ID, clean.Revision, ""))
	if len(h.api.documents()) != 0 || !strings.Contains(h.lastCard().Text, ig.reportNone) {
		t.Fatalf("no problem items means no file:\n%s", h.lastCard().Text)
	}
}

func TestProblemReportIsCappedAndSaysSo(t *testing.T) {
	h := newHarness(t, 100)
	ig := vietnamese.ig()
	job := h.problemJob("owner", reportMaxRows+50)
	h.tap(h.openJobCard(job.ID), ig.reportBtn)
	docs := h.api.documents()
	if len(docs) != 1 {
		t.Fatalf("want one file, got %d", len(docs))
	}
	rows := readCSV(t, docs[0].Content)
	if len(rows) != reportMaxRows+1 {
		t.Fatalf("the file has %d rows, want %d (+header)", len(rows), reportMaxRows)
	}
	if !strings.Contains(docs[0].Caption, fmt.Sprintf(ig.reportTruncated, reportMaxRows)) {
		t.Fatalf("the cut must be announced:\n%s", docs[0].Caption)
	}
	if len(docs[0].Content) > reportMaxBytes+1<<16 {
		t.Fatalf("file is %d bytes", len(docs[0].Content))
	}
}

func TestProblemReportIsScopedToTheCaller(t *testing.T) {
	h := newHarness(t, 100)
	op := h.addOperator(555)
	mine := h.problemJob(op.ID, 0)
	owners := h.problemJob("owner", 0)

	// The operator gets a report of their own job, in their own chat.
	h.sayAs(555, vietnamese.kbJobs)
	h.pushCallback(555, h.lastCard().MessageID, h.forge(op.ID, 555, "job_report", mine.ID, mine.Revision, ""))
	docs := h.api.documents()
	if len(docs) != 1 || docs[0].ChatID != 555 {
		t.Fatalf("operator report = %+v", docs)
	}
	// ...but not of the owner's: it answers not-found and sends nothing.
	h.pushCallback(555, h.lastCard().MessageID, h.forge(op.ID, 555, "job_report", owners.ID, owners.Revision, ""))
	if len(h.api.documents()) != 1 {
		t.Fatalf("an operator received the owner's report")
	}
	// The owner can read any job's report; the owner's button is one-shot.
	card := h.openJobCard(owners.ID)
	h.tap(card, vietnamese.ig().reportBtn)
	if len(h.api.documents()) != 2 {
		t.Fatalf("the owner's report was not sent")
	}
	h.api.push(h.click(card, vietnamese.ig().reportBtn))
	h.pull()
	if len(h.api.documents()) != 2 || h.lastAnswer() != vietnamese.expired {
		t.Fatalf("the report button is one-shot (%d files, %q)", len(h.api.documents()), h.lastAnswer())
	}
	// A stranger and a group chat can press nothing and get nothing.
	h.api.push(h.callbackUpdate(h.nextUpdateID(), 777, 777, 1, h.forge("owner", 777, "job_report", owners.ID, 0, "")))
	h.api.push(h.groupCallbackUpdate(h.nextUpdateID(), 100, -100, 1, h.forge("owner", -100, "job_report", owners.ID, 0, "")))
	h.pull()
	if len(h.api.documents()) != 2 {
		t.Fatalf("a stranger or a group got a report")
	}
}

// groupCallbackUpdate is a button press inside a group chat.
func (h *harness) groupCallbackUpdate(updateID, userID, chatID int64, messageID int, data string) []byte {
	var u map[string]any
	if err := json.Unmarshal(h.callbackUpdate(updateID, userID, chatID, messageID, data), &u); err != nil {
		h.t.Fatal(err)
	}
	u["callback_query"].(map[string]any)["message"].(map[string]any)["chat"] = map[string]any{"id": chatID, "type": "supergroup"}
	b, _ := json.Marshal(u)
	return b
}

// ---- /find through the Drive search API ----

func TestFindSearchesTheWholeDriveNotJustTheTopLevel(t *testing.T) {
	h := newHarness(t, 100)
	g := vietnamese.g()
	deep := h.drive.Folder("Level one", "root")
	nested := h.drive.Folder("Quarterly Budget", deep.ID)
	h.drive.File("budget-2026.xlsx", nested.ID, 10)
	listsBefore := h.drive.CallCount("list")

	h.say("/find budget")
	card := h.lastCard()
	if !hasButton(card, "Quarterly Budget") || !hasButton(card, "budget-2026.xlsx") {
		t.Fatalf("nested items must be found: %v\n%s", card.ButtonTexts, card.Text)
	}
	if h.drive.CallCount("search") != 1 || h.drive.CallCount("list") != listsBefore {
		t.Fatalf("/find must use the search API, not walk folders (search %d, list %d->%d)",
			h.drive.CallCount("search"), listsBefore, h.drive.CallCount("list"))
	}
	if !strings.Contains(card.Text, g.findScope) {
		t.Fatalf("the card states what is searched:\n%s", card.Text)
	}
	h.tap(card, "Quarterly Budget")
	if !strings.Contains(h.lastCard().Text, "Quarterly Budget") || !hasButton(h.lastCard(), vietnamese.copyNow) {
		t.Fatalf("a deep result must open the copy preview:\n%s", h.lastCard().Text)
	}
}

func TestFindForAnOperatorIsRefusedByTheServiceWithoutASearch(t *testing.T) {
	h := newHarness(t, 100)
	h.addOperator(555)
	h.drive.Folder("Owner secrets", "root")
	h.sayAs(555, "/find secrets")
	if strings.Contains(h.lastCard().Text, "Owner secrets") || h.drive.CallCount("search") != 0 {
		t.Fatalf("an operator searched the owner's Drive:\n%s", h.lastCard().Text)
	}
	if !strings.Contains(h.lastCard().Text, vietnamese.errMsg["forbidden"]) {
		t.Fatalf("the refusal should be explained:\n%s", h.lastCard().Text)
	}
}

// ---- job pages read through the server-side filter ----

func TestManageGroupFindsOldJobsBeyondTheNewestHundreds(t *testing.T) {
	h := newHarness(t, 100)
	ctx := context.Background()
	oldFailed := h.queuedJob("owner", "Ancient failure")
	h.setJobState(oldFailed.ID, state.JobFailed)
	for i := range 260 { // paused jobs do not count against the active-job cap
		h.setJobState(h.queuedJob("owner", fmt.Sprintf("Newer %03d", i)).ID, state.JobPaused)
	}
	failed, err := h.tg.manageJobs(ctx, app.Owner, "failed")
	if err != nil || len(failed) != 1 || failed[0].ID != oldFailed.ID {
		t.Fatalf("the failed group = %d jobs, %v", len(failed), err)
	}
	paused, err := h.tg.manageJobs(ctx, app.Owner, "paused")
	if err != nil || len(paused) != 260 {
		t.Fatalf("the paused group = %d jobs, %v (every page must be read)", len(paused), err)
	}
	all, err := h.tg.manageJobs(ctx, app.Owner, "all")
	if err != nil || len(all) != 261 {
		t.Fatalf("the all group = %d jobs, %v", len(all), err)
	}
	// A group of several states keeps the newest first.
	other := h.queuedJob("owner", "Newest partial")
	h.setJobState(other.ID, state.JobPartial)
	both, _ := h.tg.manageJobs(ctx, app.Owner, "failed")
	if len(both) != 2 || both[0].ID != other.ID {
		t.Fatalf("failed+partial = %+v", both)
	}
}

func TestManageGroupNeverShowsAnotherPersonsJobs(t *testing.T) {
	h := newHarness(t, 100)
	op := h.addOperator(555)
	h.queuedJob(op.ID, "Mine")
	h.queuedJob("owner", "Theirs")
	mine, err := h.tg.manageJobs(context.Background(), op, "queued")
	if err != nil || len(mine) != 1 || mine[0].RootName != "Mine" {
		t.Fatalf("operator group = %+v, %v", mine, err)
	}
}

// ---- the daily report lives in the preferences ----

func TestDailyReportTimeIsStoredOnlyInThePreferences(t *testing.T) {
	h := newHarness(t, 100)
	g := vietnamese.g()
	ctx := context.Background()
	card := h.openSettings()
	h.tap(card, "📰 07:00")
	p := h.prefs(app.Owner)
	if !p.DailyReport || p.DailyReportAt != "07:00" {
		t.Fatalf("prefs = %+v", p)
	}
	for _, key := range []string{"tg_report:owner", "tg_report_cfg:owner", "tg_report"} {
		if v, ok, _ := h.store.Setting(ctx, key); ok {
			t.Fatalf("the schedule is also stored in settings[%s] = %q", key, v)
		}
	}
	if !strings.Contains(h.lastCard().Text, "07:00") {
		t.Fatalf("settings show the time:\n%s", h.lastCard().Text)
	}
	h.tap(h.lastCard(), g.reportBtnOff)
	if p = h.prefs(app.Owner); p.DailyReport {
		t.Fatalf("the report did not turn off: %+v", p)
	}
	// What the API and CLI wrote is what the chat uses.
	on, at := true, "21:00"
	if _, err := h.svc.PatchPreferences(ctx, app.Owner, "api-report", app.PreferencePatch{DailyReport: &on, DailyReportAt: &at}); err != nil {
		t.Fatal(err)
	}
	if cfg := h.tg.loadReportConfig(ctx, "owner"); !cfg.Enabled || cfg.At != "21:00" {
		t.Fatalf("the chat did not read the API's schedule: %+v", cfg)
	}
	if !hasButton(h.openSettings(), "✓ 📰 21:00") {
		t.Fatalf("settings must mark the API's time: %v", h.lastCard().ButtonTexts)
	}
	if err := h.tg.saveReportConfig(ctx, "owner", reportConfig{Enabled: true, At: "25:99"}); err == nil {
		t.Fatal("an invalid time was accepted")
	}
}

func TestDailyReportReadsOldFinishedJobsThroughTheHistoryWindow(t *testing.T) {
	h := newHarness(t, 100)
	ctx := context.Background()
	done := h.queuedJob("owner", "Finished long ago")
	h.setJobState(done.ID, state.JobSucceeded)
	blocked := h.queuedJob("owner", "Blocked long ago")
	h.setJobState(blocked.ID, state.JobBlocked)
	for i := range 520 { // newer, unfinished work must not hide the finished job
		h.setJobState(h.queuedJob("owner", fmt.Sprintf("Waiting %03d", i)).ID, state.JobPaused)
	}
	stats, err := h.tg.reportStats(ctx, app.Owner, h.clock().Add(-time.Hour), h.clock().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if stats.Done != 1 || stats.Blocked != 1 {
		t.Fatalf("stats = %+v; an old job must not hide behind 500 newer ones", stats)
	}
	// Jobs outside the window are not counted.
	out, err := h.tg.reportStats(ctx, app.Owner, h.clock().Add(time.Hour), h.clock().Add(2*time.Hour))
	if err != nil || out.Done != 0 {
		t.Fatalf("a later window counted a job: %+v, %v", out, err)
	}
}

// ---- strings ----

func TestBatchIGStringsExistInBothLanguages(t *testing.T) {
	check := func(name string, vi, en reflect.Value) {
		for i := 0; i < vi.NumField(); i++ {
			field := vi.Type().Field(i).Name
			a, b := vi.Field(i), en.Field(i)
			switch a.Kind() {
			case reflect.String:
				if a.String() == "" || b.String() == "" {
					t.Errorf("%s.%s is empty (vi=%q en=%q)", name, field, a.String(), b.String())
				}
				if strings.Count(a.String(), "%") != strings.Count(b.String(), "%") {
					t.Errorf("%s.%s has a different number of verbs in the two languages", name, field)
				}
			case reflect.Map:
				if a.Len() == 0 || a.Len() != b.Len() {
					t.Errorf("%s.%s: vi has %d keys, en has %d", name, field, a.Len(), b.Len())
				}
				for _, k := range a.MapKeys() {
					if !b.MapIndex(k).IsValid() || b.MapIndex(k).String() == "" || a.MapIndex(k).String() == "" {
						t.Errorf("%s.%s[%v] is missing or empty in one language", name, field, k)
					}
				}
			}
		}
	}
	check("igTexts", reflect.ValueOf(igVI), reflect.ValueOf(igEN))
}

func TestEveryDeepWarningAndSyncModeHasAText(t *testing.T) {
	for _, ig := range []igTexts{igVI, igEN} {
		for _, w := range []string{"estimate_unknown", "destination_storage_short", "may_hit_daily_limit"} {
			if ig.deepWarn[w] == "" {
				t.Errorf("no text for the warning %q", w)
			}
		}
		for _, mode := range afterSyncModes {
			if ig.syncAfterNames[mode] == "" {
				t.Errorf("no name for the mode %q", mode)
			}
		}
		for _, code := range []string{state.SkipFiltered, state.SkipCannotCopy, state.SkipConflict} {
			if ig.itemHint[code] == "" {
				t.Errorf("no advice for the skip code %q", code)
			}
		}
	}
}

func TestBulkOrderCoversExactlyTheBulkStates(t *testing.T) {
	if len(bulkOrder) != len(bulkStates) {
		t.Fatalf("bulkOrder has %d states, bulkStates %d", len(bulkOrder), len(bulkStates))
	}
	for _, st := range bulkOrder {
		if !bulkStates[st] {
			t.Errorf("%s is in bulkOrder but not in bulkStates", st)
		}
	}
}

// ---- the jobs of one source ----

func TestSourceCardListsOnlyThatSourcesJobs(t *testing.T) {
	h := newHarness(t, 100)
	ig := vietnamese.ig()
	op := h.addOperator(555)
	a := h.readyCollection("Source A")
	h.readyCollection("Source B")
	if _, _, err := h.svc.RunNow(context.Background(), app.Owner, "again-a", a, h.collectionOf(a).Revision); err != nil {
		t.Fatal(err)
	}

	card := h.collectionCardFor(a)
	if !hasButton(card, ig.colJobsBtn) {
		t.Fatalf("the source card needs the jobs button: %v", card.ButtonTexts)
	}
	h.tap(card, ig.colJobsBtn)
	list := h.lastCard()
	if !strings.Contains(list.Text, fmt.Sprintf(ig.colJobsTitle, "Source A", 2)) || strings.Contains(list.Text, "Source B") {
		t.Fatalf("only the jobs of Source A belong here:\n%s", list.Text)
	}
	h.tap(list, vietnamese.stateLabel(state.JobSucceeded)) // a job button opens its card
	if !strings.Contains(h.lastCard().Text, "Source A") {
		t.Fatalf("a job button must open the job:\n%s", h.lastCard().Text)
	}

	// An operator cannot list the jobs of the owner's source.
	h.pushCallback(555, list.MessageID, h.forge(op.ID, 555, "col_jobs", a, 0, ""))
	if strings.Contains(h.lastCard().Text, "Source A") {
		t.Fatalf("an operator saw the owner's source:\n%s", h.lastCard().Text)
	}
}
