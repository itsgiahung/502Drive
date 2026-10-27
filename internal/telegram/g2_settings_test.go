package telegram

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/state"
)

// Batch G, slice 2a: notification settings, quiet hours, daily report, error
// advice and string parity.

func (h *harness) openSettings() fakeMessage {
	h.t.Helper()
	h.say(vietnamese.kbSettings)
	return h.lastCard()
}

func (h *harness) prefs(p app.Principal) state.Preferences {
	h.t.Helper()
	got, err := h.svc.Preferences(context.Background(), p)
	if err != nil {
		h.t.Fatal(err)
	}
	return got
}

func notifLabel(g gTexts, tx texts, kind string, on bool) string {
	return "🔔 " + fmt.Sprintf(g.notifFmt, g.notifNames[kind], onOff(on, tx))
}

func TestSettingsButtonsToggleEachNotificationKind(t *testing.T) {
	h := newHarness(t, 100)
	g := vietnamese.g()
	card := h.openSettings()
	for _, kind := range []string{"completed", "failed", "action"} {
		if !hasButton(card, notifLabel(g, vietnamese, kind, true)) {
			t.Fatalf("settings lack the %s toggle (on): %v", kind, card.ButtonTexts)
		}
	}

	h.tap(card, notifLabel(g, vietnamese, "completed", true))
	if p := h.prefs(app.Owner); p.NotifyCompleted || !p.NotifyFailed || !p.NotifyActionRequired {
		t.Fatalf("only 'completed' should be off: %+v", p)
	}
	card = h.lastCard()
	if !hasButton(card, notifLabel(g, vietnamese, "completed", false)) {
		t.Fatalf("the card must show the new value: %v", card.ButtonTexts)
	}
	h.tap(card, notifLabel(g, vietnamese, "completed", false))
	if p := h.prefs(app.Owner); !p.NotifyCompleted {
		t.Fatalf("the second tap should turn it back on: %+v", p)
	}
	h.tap(h.lastCard(), notifLabel(g, vietnamese, "action", true))
	h.tap(h.lastCard(), notifLabel(g, vietnamese, "failed", true))
	if p := h.prefs(app.Owner); p.NotifyActionRequired || p.NotifyFailed || !p.NotifyCompleted {
		t.Fatalf("prefs = %+v", p)
	}
}

func TestNotificationToggleStopsTheNotifier(t *testing.T) {
	h := newHarness(t, 42)
	g := vietnamese.g()
	n := NewNotifier(NotifierOptions{Service: h.svc, OwnerID: 42, Now: h.clock})
	h.tap(h.openSettings(), notifLabel(g, vietnamese, "completed", true))

	job := admit(t, h.store, "owner", "Quiet finish")
	finish(t, h.store, job.ID, state.JobSucceeded, "")
	if err := n.JobFinished(context.Background(), job.ID); err != nil {
		t.Fatal(err)
	}
	if rows := pendingFinal(t, h.store); len(rows) != 0 {
		t.Fatalf("a muted kind was announced: %+v", rows)
	}
}

func TestNotificationTogglesAreOneShotAndPerPerson(t *testing.T) {
	h := newHarness(t, 100)
	g := vietnamese.g()
	op, _, err := h.svc.AddOperator(context.Background(), app.Owner, "op-n", 555)
	if err != nil {
		t.Fatal(err)
	}
	card := h.openSettings()
	label := notifLabel(g, vietnamese, "failed", true)
	h.tap(card, label)
	// Replaying the consumed button changes nothing.
	h.api.push(h.click(card, label))
	h.pull()
	if h.lastAnswer() != vietnamese.expired || h.prefs(app.Owner).NotifyFailed {
		t.Fatalf("a consumed toggle flipped again (answer %q)", h.lastAnswer())
	}
	// The operator keeps their own settings and cannot press the owner's button.
	opP := app.Principal{ID: op.ID, Role: app.RoleOperator}
	h.api.push(h.callbackUpdate(h.nextUpdateID(), 555, 555, card.MessageID, h.buttonData(card, notifLabel(g, vietnamese, "action", true))))
	h.pull()
	if !h.prefs(opP).NotifyActionRequired || !h.prefs(app.Owner).NotifyActionRequired {
		t.Fatalf("a foreign callback changed preferences")
	}
	// A stranger gets nothing at all.
	before, _ := h.api.messages()
	h.api.push(h.messageUpdate(h.nextUpdateID(), 777, 777, "private", vietnamese.kbSettings))
	h.pull()
	if after, _ := h.api.messages(); len(after) != len(before) {
		t.Fatalf("a stranger received a settings card")
	}
}

func TestQuietHoursButtonSetsAndClearsThePreference(t *testing.T) {
	h := newHarness(t, 100)
	g := vietnamese.g()
	card := h.openSettings()
	on := fmt.Sprintf(g.quietFmt, "22:00", "07:00", vietnamese.off)
	if !hasButton(card, on) {
		t.Fatalf("quiet hours button (off) missing: %v", card.ButtonTexts)
	}
	h.tap(card, on)
	p := h.prefs(app.Owner)
	if p.QuietHoursStart == nil || *p.QuietHoursStart != "22:00" || *p.QuietHoursEnd != "07:00" {
		t.Fatalf("prefs = %+v", p)
	}
	h.tap(h.lastCard(), fmt.Sprintf(g.quietFmt, "22:00", "07:00", vietnamese.on))
	if p = h.prefs(app.Owner); p.QuietHoursStart != nil || p.QuietHoursEnd != nil {
		t.Fatalf("quiet hours should be cleared: %+v", p)
	}
}

func TestReportButtonsStoreTheLocalTimeAndCanTurnItOff(t *testing.T) {
	h := newHarness(t, 100)
	g := vietnamese.g()
	card := h.openSettings()
	h.tap(card, "📰 07:00")
	cfg := h.tg.loadReportConfig(context.Background(), "owner")
	if !cfg.Enabled || cfg.At != "07:00" {
		t.Fatalf("config = %+v", cfg)
	}
	if card = h.lastCard(); !strings.Contains(card.Text, "07:00") {
		t.Fatalf("the settings card should show the report time:\n%s", card.Text)
	}
	h.tap(card, g.reportBtnOff)
	if cfg = h.tg.loadReportConfig(context.Background(), "owner"); cfg.Enabled {
		t.Fatalf("report should be off: %+v", cfg)
	}
}

// ---- daily report ----

func (h *harness) reportAt(principal, hhmm string) {
	h.t.Helper()
	if err := h.tg.saveReportConfig(context.Background(), principal, reportConfig{Enabled: true, At: hhmm}); err != nil {
		h.t.Fatal(err)
	}
}

// clockAt moves the fake clock to the next occurrence of hh:mm UTC after the
// real now. The jobs created by a test are stamped with the real clock, so the
// report window is widened to keep them inside it.
func (h *harness) clockAt(hour, minute int) {
	h.t.Helper()
	now := time.Now().UTC()
	at := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, time.UTC)
	if !at.After(now) {
		at = at.AddDate(0, 0, 1)
	}
	h.mu.Lock()
	h.now = at
	h.mu.Unlock()
	h.tg.reportWindow = 72 * time.Hour
}

func (h *harness) utc() {
	h.t.Helper()
	tz := "UTC"
	if _, err := h.svc.PatchPreferences(context.Background(), app.Owner, "tz-utc", app.PreferencePatch{Timezone: &tz}); err != nil {
		h.t.Fatal(err)
	}
}

func reportRows(t *testing.T, store *state.Store) []state.Outbox {
	t.Helper()
	rows, err := store.DueOutbox(context.Background(), time.Now().Add(72*time.Hour), 50)
	if err != nil {
		t.Fatal(err)
	}
	var out []state.Outbox
	for _, r := range rows {
		if strings.HasPrefix(r.Note, "report:") {
			out = append(out, r)
		}
	}
	return out
}

func TestDailyReportIsSentOnceAtTheConfiguredLocalTime(t *testing.T) {
	h := newHarness(t, 100)
	ctx := context.Background()
	h.utc()
	h.reportAt("owner", "08:00")
	job := h.queuedJob("owner", "Reported")
	finish(t, h.store, job.ID, state.JobSucceeded, "")

	h.clockAt(7, 59)
	h.tg.reportTick(ctx)
	if rows := reportRows(t, h.store); len(rows) != 0 {
		t.Fatalf("the report went out before its time: %+v", rows)
	}
	h.advance(2 * time.Minute)
	h.tg.reportTick(ctx)
	h.tg.reportTick(ctx) // a second tick the same day must not send again
	rows := reportRows(t, h.store)
	if len(rows) != 1 || rows[0].ChatID != 100 || rows[0].Principal != "owner" {
		t.Fatalf("expected exactly one report for the owner: %+v", rows)
	}

	h.flush()
	sent := h.lastSent()
	if !strings.Contains(sent.Text, "Báo cáo") || !strings.Contains(sent.Text, "Xong: 1") {
		t.Fatalf("report text = %q", sent.Text)
	}

	// Restart in the middle of the day: still one report per day.
	h.tg = h.newTelegram()
	h.tg.reportWindow = 72 * time.Hour
	h.tg.reportTick(ctx)
	if rows = reportRows(t, h.store); len(rows) != 0 {
		t.Fatalf("a restart repeated the report: %d rows", len(rows))
	}
	// The next local day brings the next report.
	h.advance(24 * time.Hour)
	h.tg.reportTick(ctx)
	if rows = reportRows(t, h.store); len(rows) != 1 {
		t.Fatalf("expected a second report on the next day, have %d", len(rows))
	}
}

func TestDailyReportIsSkippedWhenThereIsNothingToSay(t *testing.T) {
	h := newHarness(t, 100)
	ctx := context.Background()
	h.utc()
	h.reportAt("owner", "08:00")
	h.clockAt(8, 1)
	h.tg.reportTick(ctx)
	if rows := reportRows(t, h.store); len(rows) != 0 {
		t.Fatalf("an empty day produced a report: %+v", rows)
	}
	// A no-op sync run (nothing touched) is not worth a report either.
	h.tg.reportTick(ctx)
	h.flush()
	if sent, _ := h.api.messages(); len(sent) != 0 {
		t.Fatalf("nothing should be sent: %+v", sent)
	}
}

func TestDailyReportCanBeTurnedOff(t *testing.T) {
	h := newHarness(t, 100)
	ctx := context.Background()
	h.utc()
	job := h.queuedJob("owner", "Busy")
	finish(t, h.store, job.ID, state.JobSucceeded, "")
	if err := h.tg.saveReportConfig(ctx, "owner", reportConfig{Enabled: false, At: "08:00"}); err != nil {
		t.Fatal(err)
	}
	h.clockAt(8, 1)
	h.tg.reportTick(ctx)
	if rows := reportRows(t, h.store); len(rows) != 0 {
		t.Fatalf("a disabled report was sent: %+v", rows)
	}
}

func TestDailyReportCountsOnlyThatPersonsJobs(t *testing.T) {
	h := newHarness(t, 100)
	ctx := context.Background()
	op, _, err := h.svc.AddOperator(ctx, app.Owner, "op-r", 555)
	if err != nil {
		t.Fatal(err)
	}
	h.utc()
	tz := "UTC"
	if _, err = h.svc.PatchPreferences(ctx, app.Principal{ID: op.ID, Role: app.RoleOperator}, "tz-op", app.PreferencePatch{Timezone: &tz}); err != nil {
		t.Fatal(err)
	}
	h.reportAt(op.ID, "08:00")
	ownerJob := h.queuedJob("owner", "Owner only")
	finish(t, h.store, ownerJob.ID, state.JobSucceeded, "")
	h.clockAt(8, 1)
	h.tg.reportTick(ctx)
	if rows := reportRows(t, h.store); len(rows) != 0 {
		t.Fatalf("the operator's report included the owner's job: %+v", rows)
	}
	opJob := h.queuedJob(op.ID, "Mine")
	finish(t, h.store, opJob.ID, state.JobFailed, "x")
	h.advance(24 * time.Hour)
	h.tg.reportTick(ctx)
	rows := reportRows(t, h.store)
	if len(rows) != 1 || rows[0].ChatID != 555 || rows[0].Principal != op.ID {
		t.Fatalf("the operator should get one report in their own chat: %+v", rows)
	}
	h.flush()
	text := h.lastSent().Text
	if !strings.Contains(text, "Lỗi hoặc xong một phần: 1") || strings.Contains(text, "Xong: 1") {
		t.Fatalf("operator report = %q", text)
	}
}

func TestDailyReportDueIsExactlyOncePerLocalDayAcrossDST(t *testing.T) {
	for _, tc := range []struct{ zone, day string }{
		{"America/New_York", "2026-03-08"}, // 02:30 does not exist
		{"America/New_York", "2026-11-01"}, // 01:30 happens twice
		{"Europe/Berlin", "2026-03-29"},
		{"Asia/Ho_Chi_Minh", "2026-06-15"},
	} {
		loc, err := time.LoadLocation(tc.zone)
		if err != nil {
			t.Skipf("zone data missing: %v", err)
		}
		start, _ := time.ParseInLocation("2006-01-02", tc.day, loc)
		for _, at := range []string{"01:30", "02:30", "08:00"} {
			perDay := map[string]int{}
			last := ""
			for m := -120; m < 24*60+240; m++ { // walk across the whole local day
				now := start.Add(time.Duration(m) * time.Minute)
				if due, day := reportDue(now, loc, at, last); due {
					perDay[day]++
					last = day
				}
			}
			if perDay[tc.day] != 1 {
				t.Fatalf("%s %s at %s: %d sends on the day itself", tc.zone, tc.day, at, perDay[tc.day])
			}
			for day, n := range perDay {
				if n != 1 {
					t.Fatalf("%s %s at %s: %d sends on %s", tc.zone, tc.day, at, n, day)
				}
			}
		}
	}
}

func TestReportDueWaitsForTheLocalClockNotUTC(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Ho_Chi_Minh")
	// 08:00 in Ho Chi Minh is 01:00 UTC.
	before := time.Date(2026, 6, 15, 0, 59, 0, 0, time.UTC)
	after := time.Date(2026, 6, 15, 1, 1, 0, 0, time.UTC)
	if due, _ := reportDue(before, loc, "08:00", ""); due {
		t.Fatalf("due one minute early")
	}
	if due, day := reportDue(after, loc, "08:00", ""); !due || day != "2026-06-15" {
		t.Fatalf("not due after the local time (day %q)", day)
	}
	if due, _ := reportDue(after, loc, "08:00", "2026-06-15"); due {
		t.Fatalf("due twice on the same local day")
	}
}

// ---- quiet hours ----

func TestQuietHoursHoldTelegramNoticesUntilTheyEnd(t *testing.T) {
	h := newHarness(t, 42)
	ctx := context.Background()
	n := NewNotifier(NotifierOptions{Service: h.svc, OwnerID: 42, Now: h.clock})
	tz := "UTC"
	now := h.clock().UTC()
	start := now.Add(-time.Hour).Format("15:04")
	end := now.Add(time.Hour).Format("15:04")
	if _, err := h.svc.PatchPreferences(ctx, app.Owner, "quiet-1", app.PreferencePatch{Timezone: &tz, QuietHoursStart: &start, QuietHoursEnd: &end}); err != nil {
		t.Fatal(err)
	}

	job := admit(t, h.store, "owner", "Night copy")
	finish(t, h.store, job.ID, state.JobSucceeded, "")
	if err := n.JobFinished(ctx, job.ID); err != nil {
		t.Fatal(err)
	}
	h.flush()
	if sent, _ := h.api.messages(); len(sent) != 0 {
		t.Fatalf("a notice was delivered during quiet hours: %+v", sent)
	}
	rows := pendingFinal(t, h.store)
	if len(rows) != 1 {
		t.Fatalf("the notice must stay queued, not be dropped: %+v", rows)
	}
	if !rows[0].NextSend.After(h.clock()) {
		t.Fatalf("the notice must be rescheduled after the quiet window, next_send=%v now=%v", rows[0].NextSend, h.clock())
	}

	// After the window the very same notice goes out.
	h.advance(2*time.Hour + time.Minute)
	h.flush()
	sent, _ := h.api.messages()
	if len(sent) != 1 || !strings.Contains(sent[0].Text, "Night copy") {
		t.Fatalf("expected the held notice after quiet hours: %+v", sent)
	}
}

func TestQuietHoursDoNotBlockTheAnswerToYourOwnAction(t *testing.T) {
	h := newHarness(t, 100)
	ctx := context.Background()
	tz := "UTC"
	now := h.clock().UTC()
	start, end := now.Add(-time.Hour).Format("15:04"), now.Add(time.Hour).Format("15:04")
	if _, err := h.svc.PatchPreferences(ctx, app.Owner, "quiet-2", app.PreferencePatch{Timezone: &tz, QuietHoursStart: &start, QuietHoursEnd: &end}); err != nil {
		t.Fatal(err)
	}
	h.say(h.srcLink())
	h.tap(h.lastCard(), vietnamese.copyNow)
	if len(h.jobs()) != 1 {
		t.Fatalf("a chat copy must work during quiet hours")
	}
	h.flush()
	if !strings.Contains(h.lastCard().Text, "Project Alpha") {
		t.Fatalf("the progress panel of a chat job must still update: %q", h.lastCard().Text)
	}
}

func TestQuietEndHandlesOvernightSpans(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Ho_Chi_Minh")
	s, e := "22:00", "07:00"
	p := state.Preferences{Timezone: "Asia/Ho_Chi_Minh", QuietHoursStart: &s, QuietHoursEnd: &e}
	night := time.Date(2026, 6, 15, 23, 30, 0, 0, loc)
	if got := quietEnd(p, night); !got.Equal(time.Date(2026, 6, 16, 7, 0, 0, 0, loc)) {
		t.Fatalf("overnight end = %v", got)
	}
	early := time.Date(2026, 6, 16, 3, 0, 0, 0, loc)
	if got := quietEnd(p, early); !got.Equal(time.Date(2026, 6, 16, 7, 0, 0, 0, loc)) {
		t.Fatalf("early-morning end = %v", got)
	}
}

// ---- error advice and string parity ----

func TestErrorCardsExplainWhatToDoInBothLanguages(t *testing.T) {
	h := newHarness(t, 100)
	ctx := context.Background()
	for _, tx := range []texts{vietnamese, english} {
		lang := string(tx.lang)
		if _, err := h.svc.PatchPreferences(ctx, app.Owner, "lang-"+lang, app.PreferencePatch{Language: &lang}); err != nil {
			t.Fatal(err)
		}
		g := tx.g()
		for _, code := range []string{"stale_revision", "plan_expired", "collection_busy", "account_reconnect_required", "rate_limited", "forbidden", "not_found"} {
			if err := h.tg.showError(ctx, app.Owner, h.ownerID, 0, app.Fail(409, code, "raw "+code)); err != nil {
				t.Fatal(err)
			}
			text := h.lastCard().Text
			if hint := g.errHint[code]; hint == "" || !strings.Contains(text, hint) {
				t.Fatalf("%s/%s: missing the advice %q in %q", lang, code, hint, text)
			}
		}
		// An unknown code still ends with a next step, never a bare error.
		_ = h.tg.showError(ctx, app.Owner, h.ownerID, 0, app.Fail(500, "brand_new", "raw"))
		if !strings.Contains(h.lastCard().Text, g.errHintDefault) {
			t.Fatalf("%s: unknown errors need the default advice: %q", lang, h.lastCard().Text)
		}
	}
}

func TestBatchGStringsExistInBothLanguages(t *testing.T) {
	var check func(name string, vi, en reflect.Value)
	check = func(name string, vi, en reflect.Value) {
		for i := 0; i < vi.NumField(); i++ {
			field := vi.Type().Field(i).Name
			a, b := vi.Field(i), en.Field(i)
			switch a.Kind() {
			case reflect.String:
				if a.String() == "" || b.String() == "" {
					t.Errorf("%s.%s is empty (vi=%q en=%q)", name, field, a.String(), b.String())
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
			case reflect.Struct:
				check(name+"."+field, a, b)
			}
		}
	}
	check("gTexts", reflect.ValueOf(gVI), reflect.ValueOf(gEN))
}

func TestFailedJobNoticeSaysWhatToDoNext(t *testing.T) {
	j := state.Job{ID: "job_h", RootName: "Hinted", State: state.JobPartial,
		LastError: &state.Failure{Code: "retry_exhausted", Message: "gave up"}}
	for _, tx := range []texts{vietnamese, english} {
		g := tx.g()
		if text := finalNoticeText(tx, j); !strings.Contains(text, g.errHint["retry_exhausted"]) {
			t.Fatalf("%s: the failure notice has no advice:\n%s", tx.lang, text)
		}
	}
	// A quota wait explains itself; it must not also suggest retrying.
	resume := time.Now().Add(time.Hour)
	q := state.Job{ID: "job_q", RootName: "Waiting", State: state.JobBlocked, ResumeAfter: &resume,
		LastError: &state.Failure{Code: "quota_exceeded", Message: "daily"}}
	if text := jobText(vietnamese, q); strings.Contains(text, "💡") {
		t.Fatalf("a quota wait should not add retry advice:\n%s", text)
	}
}

func TestReportShowsTheAverageSpeedAndOmitsEmptyLines(t *testing.T) {
	s := reportStats{Done: 2, Bytes: 3 << 30, Seconds: 60, Sync: 1}
	text := renderReport(vietnamese, s, time.Date(2026, 6, 15, 8, 0, 0, 0, time.UTC))
	for _, want := range []string{"Xong: 2", "Lần đồng bộ: 1", "3.0 GB", "/s", "2026-06-15 08:00"} {
		if !strings.Contains(text, want) {
			t.Fatalf("report lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "Lỗi") || strings.Contains(text, "Cần xử lý") {
		t.Fatalf("zero-count lines must be omitted:\n%s", text)
	}
	if en := renderReport(english, reportStats{Failed: 1}, time.Now()); !strings.Contains(en, "Failed or partly done: 1") || !strings.Contains(en, english.g().reportBody.footer) {
		t.Fatalf("english report = %q", en)
	}
}
