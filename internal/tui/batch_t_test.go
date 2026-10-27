package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// ---- helpers for the batch T screens ----

var tNow = time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)

// tFake is the drive fake plus the actions of the new screens. Everything the
// model asks is recorded as "name:arg:arg" so a test reads like the scenario.
type tFake struct {
	*driveFake
	audit      AuditReport
	auditErr   error
	existing   bool // SubmitMode answers ExistingCopyError while no mode is given
	planFilter *PlanFilter
	prefErr    error
	dryPlans   int
}

// set mutates the fake daemon's preferences the way the real one would, so the
// refresh after a change shows the new value.
func (f *tFake) set(fn func(p *Prefs)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(&f.snap.Prefs)
}

func newTFake() *tFake {
	f := &tFake{driveFake: newDriveFake()}
	f.audit = AuditReport{ID: "aud_1", CollectionID: "col_1", JobID: "job_aud", State: "running", Revision: 1}
	f.snap.Prefs = Prefs{Language: "en", Timezone: "UTC", NotifyCompleted: true, RetentionDays: 90,
		Autostart: false, AutostartKnown: true}
	return f
}

func (f *tFake) acts() Actions {
	a := f.driveFake.acts()
	a.InspectWith = func(_ context.Context, source, dest, name string, o CopyOptions) (Plan, error) {
		f.record(fmt.Sprintf("inspect:%s:%s:%s:%s", o.FilterPreset, o.OnConflict, o.Shortcuts, name))
		root := name
		if root == "" {
			root = "Big Folder"
		}
		return Plan{ID: "plan_1", Revision: 1, SourceID: source, SourceName: "Big Folder", SourceKind: "folder",
			DestinationID: firstNonEmpty(dest, "dst_1"), RootName: root, Files: 10, Folders: 2, Complete: true,
			Filter: f.planFilter}, nil
	}
	a.SubmitMode = func(_ context.Context, p Plan, mode string) (string, string, error) {
		f.record("submit:" + p.ID + ":" + mode)
		if f.existing && mode == "" {
			return "", "", &ExistingCopyError{Message: "This source is already copied to this destination (collection col_1, ready)."}
		}
		return "job_new", "col_1", nil
	}
	a.StartAudit = func(_ context.Context, id string) (AuditReport, error) {
		f.record("audit:" + id)
		return AuditReport{ID: "aud_1", CollectionID: id, JobID: "job_aud", State: "running", Revision: 1}, f.auditErr
	}
	a.GetAudit = func(_ context.Context, id string) (AuditReport, error) {
		f.record("getaudit:" + id)
		return f.audit, f.auditErr
	}
	a.RepairAudit = func(_ context.Context, id string, rev int, dry bool) (string, error) {
		f.record(fmt.Sprintf("repair:%s:%d:%v", id, rev, dry))
		return "job_fix", nil
	}
	a.CollectionLifecycle = func(_ context.Context, id, action string, rev int) error {
		f.record(fmt.Sprintf("life:%s:%s:%d", action, id, rev))
		return nil
	}
	a.Cleanup = func(_ context.Context, id string, rev int, dry bool, name string) (string, error) {
		f.record(fmt.Sprintf("cleanup:%s:%d:%v:%s", id, rev, dry, name))
		if !dry {
			return "job_clean_run", nil
		}
		f.dryPlans++
		if f.dryPlans == 1 {
			return "job_clean", nil
		}
		return fmt.Sprintf("job_clean_%d", f.dryPlans), nil
	}
	a.SetFilterPreset = func(_ context.Context, id, preset string, rev int) error {
		f.record(fmt.Sprintf("filter:%s:%s:%d", id, preset, rev))
		return nil
	}
	a.ShowRetired = func(on bool) { f.record(fmt.Sprintf("retired:%v", on)) }
	a.SetPreference = func(_ context.Context, field string, v any) error {
		f.record(fmt.Sprintf("pref:%s:%v", field, v))
		if f.prefErr != nil {
			return f.prefErr
		}
		f.set(func(p *Prefs) {
			switch field {
			case "notify_completed":
				p.NotifyCompleted = v.(bool)
			case "notify_failed":
				p.NotifyFailed = v.(bool)
			case "notify_action_required":
				p.NotifyAction = v.(bool)
			case "retention_days":
				p.RetentionDays = v.(int)
			case "quiet_hours":
				p.QuietStart, p.QuietEnd, _ = strings.Cut(v.(string), "-")
			}
		})
		return nil
	}
	a.SetAutostart = func(on bool) error {
		f.record(fmt.Sprintf("autostart:%v", on))
		f.set(func(p *Prefs) { p.Autostart = on })
		return nil
	}
	a.SetLanguage = func(_ context.Context, lang string) error {
		f.record("lang:" + lang)
		f.set(func(p *Prefs) { p.Language = lang })
		return nil
	}
	return a
}

// tModel builds a model in the given language and size; opts tweak the options.
func tModel(t *testing.T, f *tFake, lang string, w, h int, tweak ...func(*Options)) Model {
	t.Helper()
	opt := Options{Width: w, Height: h, Language: lang, NoMotion: true, Now: func() time.Time { return tNow }}
	for _, fn := range tweak {
		fn(&opt)
	}
	m := New(f.acts(), opt)
	m = run(t, m, m.fetch())
	m, _ = update(t, m, tea.WindowSizeMsg{Width: w, Height: h})
	return m
}

// settle runs a command and, like the Bubble Tea loop, the commands that its
// messages return, until none is left. Tests use NoMotion so no timer is among
// them.
func settle(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	pending := []tea.Cmd{cmd}
	for round := 0; round < 8 && len(pending) > 0; round++ {
		var next []tea.Cmd
		for _, c := range pending {
			if c == nil {
				continue
			}
			msg := callCmd(c)
			if batch, ok := msg.(tea.BatchMsg); ok {
				next = append(next, batch...)
				continue
			}
			if msg == nil {
				continue
			}
			var more tea.Cmd
			m, more = update(t, m, msg)
			next = append(next, more)
		}
		pending = next
	}
	return m
}

// callCmd runs a command but gives up on one that waits (a cursor blink or a
// timer): those only matter to a real terminal.
func callCmd(c tea.Cmd) tea.Msg {
	ch := make(chan tea.Msg, 1)
	go func() { ch <- c() }()
	select {
	case msg := <-ch:
		return msg
	case <-time.After(80 * time.Millisecond):
		return nil
	}
}

func refresh(t *testing.T, m Model) Model {
	t.Helper()
	return settle(t, m, m.fetch())
}

func (f *tFake) has(prefix string) bool {
	for _, c := range f.recorded() {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}

func (f *tFake) count(prefix string) int {
	n := 0
	for _, c := range f.recorded() {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}

// fitsEverywhere renders the model at the usual terminal widths and checks the
// lines never run past the edge, the height is exact, and a model without
// colour prints no escape codes. It returns the views by width.
func fitsEverywhere(t *testing.T, m Model, h int) map[int]string {
	t.Helper()
	out := map[int]string{}
	for _, w := range []int{40, 60, 80, 100, 120} {
		m2, _ := update(t, m, tea.WindowSizeMsg{Width: w, Height: h})
		v := m2.View()
		out[w] = v
		ls := lines(v)
		if len(ls) != h {
			t.Errorf("width %d: %d lines, want %d", w, len(ls), h)
		}
		for i, l := range ls {
			if got := ansi.StringWidth(l); got > w {
				t.Errorf("width %d: line %d is %d columns wide: %q", w, i, got, l)
			}
		}
		if !m2.opt.Color && strings.Contains(v, "\x1b[") {
			t.Errorf("width %d: colour codes in a no-colour view", w)
		}
	}
	return out
}

func sendKeys(t *testing.T, m Model, keys ...string) Model {
	t.Helper()
	for _, k := range keys {
		var cmd tea.Cmd
		m, cmd = update(t, m, key(k))
		m = settle(t, m, cmd)
	}
	return m
}

// ---- daemon lifecycle labels ----

func TestLifecycleBannerNamesSleepResumeAndSignIn(t *testing.T) {
	cases := []struct {
		name        string
		lang        string
		state       string
		online      bool
		gap         int
		wantAll     []string
		wantNoneFor string
	}{
		{"sleeping en", "en", "sleeping", false, 0, []string{"Sleeping", "when the computer wakes"}, ""},
		{"sleeping vi", "vi", "sleeping", false, 0, []string{"Đang ngủ", "khi máy dậy"}, ""},
		{"resuming after a long sleep en", "en", "resuming", false, 3600, []string{"Reconnecting…", "1h00m"}, ""},
		{"resuming after a long sleep vi", "vi", "resuming", false, 3600, []string{"Đang kết nối lại…", "1g00p"}, ""},
		{"short gap is not mentioned", "en", "resuming", false, 30, []string{"Reconnecting…"}, "asleep"},
		{"sign in again vi", "vi", "reconnect_required", false, 0, []string{"Cần đăng nhập lại Google", "nhấn l"}, ""},
		{"offline while running", "en", "running", false, 0, []string{"Reconnecting…"}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newTFake()
			f.snap.Status.Lifecycle, f.snap.Status.LifecycleOnline, f.snap.Status.GapSeconds = c.state, c.online, c.gap
			m := tModel(t, f, c.lang, 100, 30)
			for _, tab := range []string{"1", "2", "3"} {
				m, _ = update(t, m, key(tab))
				v := m.View()
				for _, want := range c.wantAll {
					if !strings.Contains(v, want) {
						t.Errorf("tab %s lacks %q:\n%s", tab, want, v)
					}
				}
				if c.wantNoneFor != "" && strings.Contains(v, c.wantNoneFor) {
					t.Errorf("tab %s must not mention %q:\n%s", tab, c.wantNoneFor, v)
				}
			}
			for w, v := range fitsEverywhere(t, m, 30) {
				for _, want := range c.wantAll[:1] {
					if w >= 60 && !strings.Contains(v, strings.TrimSuffix(want, "…")) {
						t.Errorf("width %d lost the lifecycle label %q:\n%s", w, want, v)
					}
				}
			}
		})
	}
}

func TestLifecycleIsSilentWhenAllIsWell(t *testing.T) {
	f := newTFake()
	f.snap.Status.Lifecycle, f.snap.Status.LifecycleOnline = "running", true
	m := tModel(t, f, "en", 100, 30)
	for _, tab := range []string{"1", "2", "3"} {
		m, _ = update(t, m, key(tab))
		if v := m.View(); strings.Contains(v, "Reconnecting") || strings.Contains(v, "Sleeping") || strings.Contains(v, "zZ") {
			t.Fatalf("tab %s shows a lifecycle notice while all is well:\n%s", tab, v)
		}
	}
	f.snap.Status.Lifecycle = "" // an older daemon reports nothing
	m = refresh(t, m)
	if m.lifeShown() {
		t.Fatal("no lifecycle reported means no banner")
	}
}

func TestSignInAgainKeyWorksWhereTheBannerShows(t *testing.T) {
	f := newTFake()
	f.snap.Status.Lifecycle = "reconnect_required"
	f.snap.Collections = []Collection{libCollection(f)}
	acts := f.acts()
	logins := 0
	acts.Login = func(context.Context) (LoginSession, error) {
		logins++
		return LoginSession{ID: "s1", URL: "https://example.test/auth"}, nil
	}
	m := New(acts, Options{Width: 100, Height: 30, Language: "en", NoMotion: true, Now: func() time.Time { return tNow }})
	m = run(t, m, m.fetch())
	for _, tab := range []string{"2", "3", "4"} {
		m, _ = update(t, m, key(tab))
		var cmd tea.Cmd
		m, cmd = update(t, m, key("l"))
		m = run(t, m, cmd)
		if m.screen != screenAccount || m.login == nil {
			t.Fatalf("tab %s: l must start the sign-in and show it on Account (screen %v)", tab, m.screen)
		}
	}
	if logins != 3 {
		t.Fatalf("logins = %d", logins)
	}
	// Without that state, l does nothing on these tabs.
	f.snap.Status.Lifecycle = "running"
	m = refresh(t, m)
	m, _ = update(t, m, key("2"))
	if _, cmd := update(t, m, key("l")); cmd != nil {
		t.Fatal("l is not a Jobs key when no sign-in is needed")
	}
}

func TestLifecycleChipOnTheTabBar(t *testing.T) {
	f := newTFake()
	f.snap.Status.Lifecycle = "sleeping"
	m := tModel(t, f, "en", 100, 30)
	if bar := lines(m.View())[0]; !strings.Contains(bar, "zZ") {
		t.Fatalf("a sleeping machine must show on the bar: %q", bar)
	}
	f.snap.Status.Lifecycle, f.snap.Status.LifecycleOnline = "resuming", false
	m = refresh(t, m)
	if bar := lines(m.View())[0]; !strings.Contains(bar, "~") {
		t.Fatalf("reconnecting must show on the bar: %q", bar)
	}
}

// ---- Jobs: quota wait, kind, skipped reasons ----

func waitingJob() Job {
	return Job{ID: "job_w", CollectionID: "col_1", SourceName: "Big Folder", State: "blocked", Revision: 5, Completed: 10,
		ResumeAfter: tNow.Add(2*time.Hour + 5*time.Minute)}
}

func TestBlockedJobShowsWhenQuotaReturnsAndOffersNoRetry(t *testing.T) {
	f := newTFake()
	f.snap.Jobs = []Job{waitingJob()}
	f.snap.Status.Blocked = 1
	m := tModel(t, f, "en", 100, 30)
	m, _ = update(t, m, key("2"))
	if v := m.View(); !strings.Contains(v, "Waiting for quota until 14:05") {
		t.Fatalf("the Jobs list must say when the job continues:\n%s", v)
	}
	// Retry from the list: nothing is sent, the daemon resumes it by itself.
	m = sendKeys(t, m, "t")
	if f.has("job:retry") {
		t.Fatalf("a job waiting for quota must not be retried: %v", f.recorded())
	}
	// Detail: the line, the note, and a footer without the retry key.
	m, _ = update(t, m, key("enter"))
	v := m.View()
	for _, want := range []string{"Waiting for quota until 14:05", "resumes by itself, no retry needed"} {
		if !strings.Contains(v, want) {
			t.Fatalf("job detail lacks %q:\n%s", want, v)
		}
	}
	if strings.Contains(lines(v)[len(lines(v))-1], "retry") {
		t.Fatalf("the footer must not offer retry:\n%s", v)
	}
	m, cmd := update(t, m, key("t"))
	m = run(t, m, cmd)
	if f.has("job:retry") || !strings.Contains(m.View(), "14:05") {
		t.Fatalf("t on a waiting job explains instead of retrying: %v / %q", f.recorded(), m.status)
	}
	// Pausing and cancelling stay available.
	m, cmd = update(t, m, key("p"))
	_ = run(t, m, cmd)
	if !f.has("job:pause:job_w") {
		t.Fatalf("pause must still work: %v", f.recorded())
	}
}

func TestBlockedJobWithoutAFutureTimeCanStillBeRetried(t *testing.T) {
	f := newTFake()
	j := waitingJob()
	j.ResumeAfter = tNow.Add(-time.Minute) // already due: nothing to wait for
	f.snap.Jobs = []Job{j}
	m := tModel(t, f, "en", 100, 30)
	m, _ = update(t, m, key("2"))
	if strings.Contains(m.View(), "Waiting for quota") {
		t.Fatal("a time in the past is not a wait")
	}
	_ = sendKeys(t, m, "t")
	if !f.has("job:retry:job_w") {
		t.Fatalf("an ordinary blocked job keeps its retry: %v", f.recorded())
	}
}

func TestQuotaWaitInVietnameseHomeAndNarrowWidths(t *testing.T) {
	f := newTFake()
	f.snap.Jobs = []Job{waitingJob()}
	f.snap.Status.Blocked = 1
	m := tModel(t, f, "vi", 100, 30)
	if v := m.View(); !strings.Contains(v, "chờ tới 14:05") {
		t.Fatalf("Home's recent list must show the wait:\n%s", v)
	}
	m, _ = update(t, m, key("2"))
	for w, v := range fitsEverywhere(t, m, 30) {
		if w >= 60 && !strings.Contains(v, "Chờ quota tới 14:05") {
			t.Errorf("width %d: %s", w, v)
		}
	}
	m, _ = update(t, m, key("enter"))
	fitsEverywhere(t, m, 30)
}

func TestJobDetailListsSkippedItemsByReason(t *testing.T) {
	f := newTFake()
	total := 40
	f.snap.Jobs = []Job{{ID: "job_s", SourceName: "Docs", State: "partial", Revision: 2, Completed: 20, Skipped: 16, Total: &total,
		Kind: "", Bytes: 3 << 20,
		SkippedByCode: map[string]int{"filtered": 12, "conflict": 3, "future_code": 1, "cannot_copy": 0}}}
	m := tModel(t, f, "en", 100, 30)
	m, _ = update(t, m, key("2"))
	m, _ = update(t, m, key("enter"))
	v := m.View()
	for _, want := range []string{"Skipped", "12  excluded by the filter", "3  name taken, skipped", "1  future_code", "3.0 MB"} {
		if !strings.Contains(v, want) {
			t.Fatalf("job detail lacks %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "copying not allowed") {
		t.Fatal("a reason with a zero count is not listed")
	}
	if strings.Index(v, "excluded by the filter") > strings.Index(v, "name taken") {
		t.Fatal("the biggest reason comes first")
	}
	vi := tModel(t, f, "vi", 60, 30)
	vi, _ = update(t, vi, key("2"))
	vi, _ = update(t, vi, key("enter"))
	if !strings.Contains(vi.View(), "bị bộ lọc loại") || !strings.Contains(vi.View(), "Bỏ qua") {
		t.Fatalf("Vietnamese labels missing:\n%s", vi.View())
	}
	fitsEverywhere(t, vi, 30)
}

func TestEverySkipCodeHasALabelInBothLanguages(t *testing.T) {
	codes := []string{"filtered", "conflict", "shortcut_unresolved", "shortcut_loop", "cannot_copy",
		"google_native_export_only", "too_large_for_quota", "unreadable", "trashed_source"}
	for _, lang := range []string{"en", "vi"} {
		m := New(Actions{}, Options{Language: lang})
		for _, c := range codes {
			if got := m.skipLabel(c); got == c || got == "" {
				t.Errorf("%s: code %s has no label (%q)", lang, c, got)
			}
		}
	}
}

func TestAuditAndCleanupJobsShowTheirKindAndBytes(t *testing.T) {
	f := newTFake()
	total := 7
	f.snap.Jobs = []Job{{ID: "job_c", SourceName: "Archive", State: "succeeded", Kind: "cleanup", Revision: 3, Total: &total, Completed: 0, Bytes: 1536}}
	m := tModel(t, f, "en", 100, 30)
	m, _ = update(t, m, key("2"))
	m, _ = update(t, m, key("enter"))
	v := m.View()
	if !strings.Contains(v, "destination cleanup") || !strings.Contains(v, "1.5 KB") {
		t.Fatalf("kind and size missing:\n%s", v)
	}
}

// ---- Copy dialog: filter, name clash, shortcuts, existing copy ----

func openCopy(t *testing.T, f *tFake, lang string, w, h int) Model {
	t.Helper()
	m := tModel(t, f, lang, w, h)
	m, cmd := m.startCopy("https://drive.google.com/drive/folders/SRC", screenHome)
	return run(t, m, cmd)
}

func TestCopyDialogOptionsTravelWithThePreview(t *testing.T) {
	f := newTFake()
	f.planFilter = &PlanFilter{Included: 7, Excluded: 3, Scope: "top_level",
		ExcludedSamples: []string{"clip1.mp4", "clip2.mp4", "clip3.mp4", "clip4.mp4"}}
	m := openCopy(t, f, "en", 100, 34)
	if !f.has("inspect::::") {
		t.Fatalf("the first preview uses the daemon defaults: %v", f.recorded())
	}
	v := m.View()
	for _, want := range []string{"Filter: no filter", "Name clash: rename", "Shortcuts: follow", "Kept: 7 · Excluded: 3", "top level only", "clip1.mp4", "clip3.mp4"} {
		if !strings.Contains(v, want) {
			t.Fatalf("the dialog lacks %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "clip4.mp4") {
		t.Fatal("only a few samples are shown")
	}
	m = sendKeys(t, m, "f")
	if !f.has("inspect:no_video::") || !strings.Contains(m.View(), "Filter: no video") {
		t.Fatalf("f must re-run the preview with the filter: %v\n%s", f.recorded(), m.View())
	}
	m = sendKeys(t, m, "f", "f")
	if !f.has("inspect:under_1gb::") {
		t.Fatalf("f cycles none, no video, documents only, under 1 GB: %v", f.recorded())
	}
	m = sendKeys(t, m, "o")
	if !f.has("inspect:under_1gb:skip:") || !strings.Contains(m.View(), "Name clash: skip") {
		t.Fatalf("o cycles the name clash decision: %v", f.recorded())
	}
	m = sendKeys(t, m, "o", "k")
	if !f.has("inspect:under_1gb:update:skip:") || !strings.Contains(m.View(), "Shortcuts: skip") {
		t.Fatalf("k toggles shortcuts: %v", f.recorded())
	}
	// Starting the copy submits the plan that the options produced.
	m = sendKeys(t, m, "enter")
	if !f.has("submit:plan_1:") {
		t.Fatalf("submit missing: %v", f.recorded())
	}
	if m.screen != screenJobs {
		t.Fatalf("a started copy goes to the Jobs tab, got %v", m.screen)
	}
}

func TestCopyDialogOptionsReadInVietnameseAtEveryWidth(t *testing.T) {
	f := newTFake()
	f.planFilter = &PlanFilter{Included: 7, Excluded: 3, Scope: "top_level", ExcludedSamples: []string{"phim gia đình.mp4"}}
	m := openCopy(t, f, "vi", 100, 34)
	m = sendKeys(t, m, "f", "o")
	for w, v := range fitsEverywhere(t, m, 34) {
		if w >= 60 {
			for _, want := range []string{"Bộ lọc", "bỏ video", "Trùng tên", "bỏ qua", "Đạt: 7 · Bị loại: 3"} {
				if !strings.Contains(v, want) {
					t.Errorf("width %d lacks %q:\n%s", w, want, v)
				}
			}
		}
	}
}

func TestCopyDialogOptionKeysAreIgnoredWhileThePreviewLoads(t *testing.T) {
	f := newTFake()
	m := openCopy(t, f, "en", 100, 34)
	m, _ = update(t, m, key("f")) // starts a new preview, not yet answered
	before := f.count("inspect:")
	m, cmd := update(t, m, key("o"))
	if cmd != nil || f.count("inspect:") != before {
		t.Fatal("a second change while the preview is loading must wait")
	}
	_ = m
}

func TestExistingCopyAsksUpdateOrNewAndSendsTheAnswer(t *testing.T) {
	f := newTFake()
	f.existing = true
	m := openCopy(t, f, "en", 100, 34)
	m = sendKeys(t, m, "enter")
	if m.screen != screenCopy || m.cp == nil || !m.cp.choose {
		t.Fatalf("a 409 already_copied opens the question, screen=%v", m.screen)
	}
	v := m.View()
	for _, want := range []string{"already copied to this destination", "u update the old copy", "n make a new copy"} {
		if !strings.Contains(v, want) {
			t.Fatalf("the question lacks %q:\n%s", want, v)
		}
	}
	if foot := lines(v)[len(lines(v))-1]; !strings.Contains(foot, "u update") {
		t.Fatalf("the footer must show the answers: %q", foot)
	}
	// Other keys do nothing while the question is open.
	m = sendKeys(t, m, "f", "1")
	if !m.cp.choose || f.count("inspect:") != 1 {
		t.Fatal("the question keeps the dialog still")
	}
	// Esc closes the question but keeps the dialog.
	m, _ = update(t, m, key("esc"))
	if m.screen != screenCopy || m.cp.choose {
		t.Fatal("esc answers 'neither' and stays in the dialog")
	}
	m = sendKeys(t, m, "enter", "u")
	if got := f.recorded(); !f.has("submit:plan_1:update") {
		t.Fatalf("update was not sent: %v", got)
	}
	if m.screen != screenJobs {
		t.Fatalf("after the answer the copy starts and Jobs opens, got %v", m.screen)
	}
}

func TestExistingCopyNewCopyAnswer(t *testing.T) {
	f := newTFake()
	f.existing = true
	m := openCopy(t, f, "vi", 80, 34)
	m = sendKeys(t, m, "enter")
	if v := m.View(); !strings.Contains(v, "đã được sao chép tới đích này") || !strings.Contains(v, "u cập nhật bản cũ") {
		t.Fatalf("Vietnamese question missing:\n%s", v)
	}
	fitsEverywhere(t, m, 34)
	_ = sendKeys(t, m, "n")
	if !f.has("submit:plan_1:new") {
		t.Fatalf("new was not sent: %v", f.recorded())
	}
}

func TestCopyDialogWithoutTheNewActionsStillWorks(t *testing.T) {
	d := newDriveFake()
	m := driveModel(t, d, 100, 34)
	m, cmd := m.startCopy("SRC", screenHome)
	m = run(t, m, cmd)
	m = sendKeys(t, m, "f") // options re-run the preview with the older action
	if len(d.calls) < 2 {
		t.Fatalf("the preview was not re-run: %v", d.calls)
	}
	m = sendKeys(t, m, "enter")
	if m.screen != screenJobs {
		t.Fatalf("Submit still starts the copy, got %v", m.screen)
	}
}

// ---- Library: audit ----

func libCollection(f *tFake) Collection {
	return Collection{ID: "col_1", SourceName: "Project Alpha", State: "ready", Revision: 2, RootName: "Project Alpha",
		Sync: &SyncState{Mode: "scheduled", State: "active", Revision: 4}}
}

func openCollection(t *testing.T, f *tFake, lang string, w, h int) Model {
	t.Helper()
	m := tModel(t, f, lang, w, h)
	m, _ = update(t, m, key("3"))
	m, _ = update(t, m, key("enter"))
	if m.screen != screenCollection {
		t.Fatalf("enter must open the collection, got %v", m.screen)
	}
	return m
}

func TestAuditScanViewResultsAndRepair(t *testing.T) {
	f := newTFake()
	f.snap.Collections = []Collection{libCollection(f)}
	m := openCollection(t, f, "en", 100, 34)
	if v := m.View(); !strings.Contains(v, "Integrity check") || !strings.Contains(v, "not checked yet") {
		t.Fatalf("the audit section is missing:\n%s", v)
	}
	// v / f / F before any report explain instead of sending anything.
	for _, k := range []string{"v", "f", "F"} {
		m = sendKeys(t, m, k)
		if !strings.Contains(m.View(), "Run a check first") {
			t.Fatalf("%s without a report must say so: %q", k, m.status)
		}
	}
	m = sendKeys(t, m, "a")
	if !f.has("audit:col_1") {
		t.Fatalf("a must start the audit: %v", f.recorded())
	}
	if !strings.Contains(m.View(), "scanning") {
		t.Fatalf("the screen must say it is scanning:\n%s", m.View())
	}
	// A second a while scanning does not start another.
	m = sendKeys(t, m, "a")
	if f.count("audit:") != 1 {
		t.Fatal("one scan at a time")
	}
	// The scan finishes; the next refresh reads the report.
	f.audit = AuditReport{ID: "aud_1", CollectionID: "col_1", JobID: "job_aud", State: "completed", Revision: 3, Scanned: 12,
		Counts:     map[string]int{"missing_at_destination": 2, "name_mismatch": 1},
		Repairable: 2,
		Findings: []Finding{{Kind: "missing_at_destination", SourceID: "s1", Detail: "report.pdf", Repairable: true},
			{Kind: "missing_at_destination", SourceID: "s2", Detail: "notes.txt", Repairable: true},
			{Kind: "name_mismatch", SourceID: "s3", DestID: "d3", Detail: "photo.jpg"}}}
	m = refresh(t, m)
	m = refresh(t, m)
	v := m.View()
	for _, want := range []string{"3 difference(s) (2 repairable)", "missing at destination", "name differs"} {
		if !strings.Contains(v, want) {
			t.Fatalf("the report lacks %q:\n%s", want, v)
		}
	}
	// v toggles the findings.
	m = sendKeys(t, m, "v")
	v = m.View()
	for _, want := range []string{"report.pdf [repairable]", "photo.jpg"} {
		if !strings.Contains(v, want) {
			t.Fatalf("the findings lack %q:\n%s", want, v)
		}
	}
	// Repair needs a plan first.
	m = sendKeys(t, m, "F")
	if f.has("repair:") || !strings.Contains(m.View(), "Plan the repair first") {
		t.Fatalf("F without a plan must refuse: %v / %q", f.recorded(), m.status)
	}
	m = sendKeys(t, m, "f")
	if !f.has("repair:aud_1:3:true") {
		t.Fatalf("f plans the repair as a dry run on the audit's revision: %v", f.recorded())
	}
	if !strings.Contains(m.View(), "Nothing changed") {
		t.Fatalf("the plan must say nothing changed:\n%s", m.View())
	}
	// The plan job ends; the next refresh marks the plan as made.
	f.snap.Jobs = append(f.snap.Jobs, Job{ID: "job_fix", State: "succeeded", Kind: "repair"})
	m = refresh(t, m)
	m = refresh(t, m)
	if m.audits["col_1"].repairJob != "" || !m.audits["col_1"].planned {
		t.Fatalf("the plan is made: %+v", m.audits["col_1"])
	}
	// Applying asks for a yes.
	m, cmd := update(t, m, key("F"))
	if cmd != nil || m.confirm == nil || !strings.Contains(m.confirm.prompt, "Repair 2 item(s)?") {
		t.Fatalf("F must ask before repairing: %+v", m.confirm)
	}
	m, _ = update(t, m, key("n"))
	if f.count("repair:") != 1 {
		t.Fatal("answering no sends nothing")
	}
	m, _ = update(t, m, key("F"))
	m, cmd = update(t, m, key("y"))
	m = run(t, m, cmd)
	if !f.has("repair:aud_1:3:false") {
		t.Fatalf("y applies the repair: %v", f.recorded())
	}
}

func TestAuditCleanFailedAndErrors(t *testing.T) {
	f := newTFake()
	f.snap.Collections = []Collection{libCollection(f)}
	m := openCollection(t, f, "en", 100, 34)
	m = sendKeys(t, m, "a")
	f.audit = AuditReport{ID: "aud_1", CollectionID: "col_1", State: "completed", Revision: 1, Counts: map[string]int{}}
	m = refresh(t, m)
	m = refresh(t, m)
	if !strings.Contains(m.View(), "no differences") {
		t.Fatalf("a clean audit says so:\n%s", m.View())
	}
	m = sendKeys(t, m, "f")
	if !strings.Contains(m.View(), "Nothing can be repaired") {
		t.Fatal("nothing to repair is said, not sent")
	}
	f.audit.State = "failed"
	m, _ = update(t, m, key("v"))
	m = run(t, m, func() tea.Msg {
		r, _ := f.acts().GetAudit(context.Background(), "aud_1")
		return auditMsg{collectionID: "col_1", rep: r}
	})
	if !strings.Contains(m.View(), "the scan failed") {
		t.Fatalf("a failed scan says so:\n%s", m.View())
	}
	// A daemon error is shown, not hidden.
	f.auditErr = errors.New("collection is busy")
	m.audits["col_1"].report = nil
	m = sendKeys(t, m, "a")
	if !strings.Contains(m.View(), "collection is busy") {
		t.Fatalf("the error must be shown:\n%s", m.View())
	}
}

func TestAuditScreenFitsEveryWidthInBothLanguages(t *testing.T) {
	for _, lang := range []string{"en", "vi"} {
		f := newTFake()
		f.snap.Collections = []Collection{libCollection(f)}
		m := openCollection(t, f, lang, 100, 36)
		m = sendKeys(t, m, "a")
		f.audit = AuditReport{ID: "aud_1", CollectionID: "col_1", State: "completed", Revision: 1, Repairable: 5,
			Counts: map[string]int{"missing_at_destination": 4, "extra_at_destination": 1, "wrong_parent": 2, "duplicate_at_destination": 1,
				"size_or_md5_mismatch": 3, "source_removed": 1, "unreadable_source": 1, "name_mismatch": 2},
			Findings: []Finding{{Kind: "missing_at_destination", Detail: "một tệp có tên rất dài để thử cắt dòng.pdf", Repairable: true}}}
		m = refresh(t, m)
		m = refresh(t, m)
		m = sendKeys(t, m, "v")
		for w, v := range fitsEverywhere(t, m, 36) {
			if w >= 60 && !strings.Contains(v, map[string]string{"en": "Integrity check", "vi": "Kiểm tra toàn vẹn"}[lang]) {
				t.Errorf("%s width %d lacks the section:\n%s", lang, w, v)
			}
		}
	}
}

// ---- Library: lifecycle (L1) ----

func TestRetireAsksFirstAndSendsTheRevision(t *testing.T) {
	f := newTFake()
	f.snap.Collections = []Collection{libCollection(f)}
	m := openCollection(t, f, "en", 100, 34)
	m, cmd := update(t, m, key("o"))
	if cmd != nil || m.confirm == nil || !strings.Contains(m.View(), "Stop following “Project Alpha”?") {
		t.Fatalf("o must ask, with what stays: %+v\n%s", m.confirm, m.View())
	}
	if !strings.Contains(m.View(), "The copy on Drive and its history stay") {
		t.Fatal("the question says what is kept")
	}
	m, _ = update(t, m, key("n"))
	if f.has("life:") {
		t.Fatal("no means nothing is sent")
	}
	m, _ = update(t, m, key("o"))
	m, cmd = update(t, m, key("y"))
	m = run(t, m, cmd)
	if !f.has("life:retire:col_1:2") {
		t.Fatalf("retire missing: %v", f.recorded())
	}
}

func TestForgetNeedsRetiredFirstThenConfirm(t *testing.T) {
	f := newTFake()
	f.snap.Collections = []Collection{libCollection(f)}
	m := openCollection(t, f, "vi", 100, 34)
	m = sendKeys(t, m, "w")
	if f.has("life:") || !strings.Contains(m.View(), "Cần ngừng theo dõi trước") {
		t.Fatalf("forget on a live collection is refused: %v", f.recorded())
	}
	f.snap.Collections[0].State = "retired"
	m = refresh(t, m)
	m, cmd := update(t, m, key("w"))
	if cmd != nil || m.confirm == nil || !strings.Contains(m.confirm.prompt, "Quên “Project Alpha”?") {
		t.Fatalf("forget asks first: %+v", m.confirm)
	}
	m, cmd = update(t, m, key("y"))
	m = run(t, m, cmd)
	if !f.has("life:forget:col_1:2") {
		t.Fatalf("forget missing: %v", f.recorded())
	}
}

func TestResumeBringsARetiredCopyBackAndResumesAPausedSync(t *testing.T) {
	f := newTFake()
	c := libCollection(f)
	c.State = "retired"
	f.snap.Collections = []Collection{c}
	m := openCollection(t, f, "en", 100, 34)
	if !strings.Contains(m.View(), "RETIRED") || !strings.Contains(m.View(), "retired") {
		t.Fatalf("a retired source says so:\n%s", m.View())
	}
	_ = sendKeys(t, m, "u")
	if !f.has("life:resume:col_1:2") {
		t.Fatalf("resume missing: %v", f.recorded())
	}
	// A paused sync on a live collection is resumed with the same key.
	f2 := newTFake()
	c2 := libCollection(f2)
	c2.Sync.State = "paused"
	f2.snap.Collections = []Collection{c2}
	m2 := openCollection(t, f2, "en", 100, 34)
	_ = sendKeys(t, m2, "u")
	if !f2.has("sync:resume:col_1") {
		t.Fatalf("u resumes a paused sync: %v", f2.recorded())
	}
	// Nothing to resume: say so.
	f3 := newTFake()
	f3.snap.Collections = []Collection{libCollection(f3)}
	m3 := openCollection(t, f3, "en", 100, 34)
	m3 = sendKeys(t, m3, "u")
	if !strings.Contains(m3.View(), "not retired") || f3.has("life:") {
		t.Fatalf("u on a live source explains: %q", m3.status)
	}
}

func TestCleanupPlansFirstThenNeedsTheTypedName(t *testing.T) {
	f := newTFake()
	c := libCollection(f)
	c.State = "retired"
	f.snap.Collections = []Collection{c}
	m := openCollection(t, f, "en", 100, 36)
	// A live collection cannot be cleaned.
	f.snap.Collections[0].State = "ready"
	m = refresh(t, m)
	m = sendKeys(t, m, "c")
	if f.has("cleanup:") {
		t.Fatal("cleanup of a live collection is refused locally")
	}
	f.snap.Collections[0].State = "retired"
	m = refresh(t, m)
	m = sendKeys(t, m, "c")
	if !f.has("cleanup:col_1:2:true:") {
		t.Fatalf("c asks for the dry-run plan: %v", f.recorded())
	}
	if !strings.Contains(m.View(), "Planning the cleanup") {
		t.Fatalf("the plan is being made:\n%s", m.View())
	}
	// C before the plan is in does nothing.
	m = sendKeys(t, m, "C")
	if m.clean.input != nil {
		t.Fatal("the name is asked only after the plan")
	}
	total := 5
	f.snap.Jobs = []Job{{ID: "job_clean", State: "succeeded", Kind: "cleanup", Total: &total, Bytes: 1536}}
	m = refresh(t, m)
	if v := m.View(); !strings.Contains(v, "Cleanup: 5 item(s) · 1.5 KB will go to the Drive trash") {
		t.Fatalf("the plan must show the count and the size:\n%s", v)
	}
	m, _ = update(t, m, key("C"))
	if m.clean.input == nil || !strings.Contains(m.View(), "Type the folder name “Project Alpha”") {
		t.Fatalf("C asks the name again:\n%s", m.View())
	}
	// Esc backs out without sending anything.
	m, _ = update(t, m, key("esc"))
	if m.clean.input != nil || f.has("cleanup:col_1:2:false") {
		t.Fatal("esc cancels the typing")
	}
	m, _ = update(t, m, key("C"))
	m = typeText(t, m, "Project Alpha")
	m, cmd := update(t, m, key("enter"))
	m = run(t, m, cmd)
	if !f.has("cleanup:col_1:2:false:Project Alpha") {
		t.Fatalf("the typed name goes with the real run: %v", f.recorded())
	}
	if !strings.Contains(m.status, "job_clean") {
		t.Fatalf("the user is told which job: %q", m.status)
	}
	if m.clean != nil {
		t.Fatal("the flow ends once sent")
	}
}

func TestCleanupWithNothingToMoveOrAFailedPlan(t *testing.T) {
	f := newTFake()
	c := libCollection(f)
	c.State = "retired"
	f.snap.Collections = []Collection{c}
	m := openCollection(t, f, "en", 100, 36)
	m = sendKeys(t, m, "c")
	zero := 0
	f.snap.Jobs = []Job{{ID: "job_clean", State: "succeeded", Kind: "cleanup", Total: &zero}}
	m = refresh(t, m)
	if !strings.Contains(m.View(), "nothing in the destination folder") {
		t.Fatalf("an empty plan says so:\n%s", m.View())
	}
	m = sendKeys(t, m, "C")
	if m.clean.input != nil {
		t.Fatal("nothing to confirm")
	}
	// A plan that failed shows the daemon's reason, and no confirmation.
	m = sendKeys(t, m, "c")
	f.snap.Jobs = []Job{{ID: "job_clean_2", State: "failed", Kind: "cleanup", LastError: "The destination root folder does not carry a 502Drive intent marker."}}
	m = refresh(t, m)
	if !strings.Contains(m.View(), "intent marker") {
		t.Fatalf("the refusal is shown:\n%s", m.View())
	}
	m = sendKeys(t, m, "C")
	if m.clean.input != nil {
		t.Fatal("a failed plan cannot be confirmed")
	}
}

func TestRetiredSourcesAreHiddenUntilAsked(t *testing.T) {
	f := newTFake()
	f.snap.Collections = []Collection{libCollection(f)}
	m := tModel(t, f, "en", 100, 30)
	m, _ = update(t, m, key("3"))
	if strings.Contains(m.View(), "showing retired") {
		t.Fatal("retired sources are not listed by default")
	}
	retired := Collection{ID: "col_old", SourceName: "Old Project", State: "retired", Revision: 1}
	m, cmd := update(t, m, key("H"))
	f.snap.Collections = append(f.snap.Collections, retired)
	m = run(t, m, cmd)
	if !f.has("retired:true") {
		t.Fatalf("H must tell the client to include them: %v", f.recorded())
	}
	v := m.View()
	for _, want := range []string{"showing retired sources too", "Old Project", "RETIRED"} {
		if !strings.Contains(v, want) {
			t.Fatalf("the list lacks %q:\n%s", want, v)
		}
	}
	m, cmd = update(t, m, key("H"))
	f.snap.Collections = f.snap.Collections[:1]
	m = run(t, m, cmd)
	if !f.has("retired:false") || strings.Contains(m.View(), "Old Project") {
		t.Fatal("H again hides them")
	}
	fitsEverywhere(t, m, 30)
}

func TestCollectionFilterIsShownAndCycledWithPresets(t *testing.T) {
	f := newTFake()
	c := libCollection(f)
	f.snap.Collections = []Collection{c}
	m := openCollection(t, f, "en", 100, 34)
	if !strings.Contains(m.View(), "Filter") || !strings.Contains(m.View(), "no filter") {
		t.Fatalf("the filter line is missing:\n%s", m.View())
	}
	m = sendKeys(t, m, "b")
	if !f.has("filter:col_1:no_video:2") {
		t.Fatalf("b picks the next preset: %v", f.recorded())
	}
	if !strings.Contains(m.status, "applies to later runs") {
		t.Fatalf("the user is told when it applies: %q", m.status)
	}
	f.snap.Collections[0].Filter = FilterRules{Exclude: []string{"class:video"}}
	m = refresh(t, m)
	if !strings.Contains(m.View(), "no video") {
		t.Fatalf("the saved filter is shown:\n%s", m.View())
	}
	m = sendKeys(t, m, "b")
	if !f.has("filter:col_1:docs_only:2") {
		t.Fatalf("next preset after no video is documents only: %v", f.recorded())
	}
	f.snap.Collections[0].Filter = FilterRules{Include: []string{"*.pdf", "*.doc"}, MaxSize: 5}
	m = refresh(t, m)
	if !strings.Contains(m.View(), "custom (3 rules)") {
		t.Fatalf("a hand-made rule is called custom:\n%s", m.View())
	}
	m = sendKeys(t, m, "b")
	if !f.has("filter:col_1::2") {
		t.Fatalf("from a custom rule the cycle restarts at no filter: %v", f.recorded())
	}
}

func TestCollectionScreenFitsEveryWidthInBothLanguages(t *testing.T) {
	for _, lang := range []string{"en", "vi"} {
		f := newTFake()
		c := libCollection(f)
		c.State = "retired"
		c.Filter = FilterRules{Exclude: []string{"class:video"}}
		f.snap.Collections = []Collection{c}
		m := openCollection(t, f, lang, 100, 34)
		m = sendKeys(t, m, "c")
		total := 120
		f.snap.Jobs = []Job{{ID: "job_clean", State: "succeeded", Kind: "cleanup", Total: &total, Bytes: 5 << 30}}
		m = refresh(t, m)
		m, _ = update(t, m, key("C"))
		m = typeText(t, m, "Dự án tiếng Việt")
		for w, v := range fitsEverywhere(t, m, 34) {
			if w >= 60 && !strings.Contains(v, "5.0 GB") {
				t.Errorf("%s width %d lost the size:\n%s", lang, w, v)
			}
		}
	}
}

func TestLibraryKeysAreInertWithoutTheirActions(t *testing.T) {
	d := newDriveFake()
	d.snap.Collections = []Collection{libCollection(&tFake{})}
	m := driveModel(t, d, 100, 30)
	m, _ = update(t, m, key("3"))
	m, _ = update(t, m, key("enter"))
	for _, k := range []string{"a", "v", "f", "F", "b", "o", "u", "w", "c", "C"} {
		var cmd tea.Cmd
		m, cmd = update(t, m, key(k))
		if cmd != nil || m.confirm != nil {
			t.Fatalf("%s without its action must do nothing", k)
		}
	}
	if strings.Contains(m.View(), "Integrity check") {
		t.Fatal("the audit section only appears when the action exists")
	}
}

// ---- Settings ----

func settingsModel(t *testing.T, f *tFake, lang string, w, h int) Model {
	t.Helper()
	m := tModel(t, f, lang, w, h)
	m, _ = update(t, m, key("6"))
	return m
}

func TestSettingsListsTheNewRowsWithTheCursor(t *testing.T) {
	f := newTFake()
	f.snap.Prefs.QuietStart, f.snap.Prefs.QuietEnd = "22:00", "07:00"
	m := settingsModel(t, f, "en", 100, 30)
	v := m.View()
	for _, want := range []string{"Language", "English", "Start on login", "[ ] off", "Quiet hours", "22:00-07:00",
		"Keep history", "90 days", "Notify when done", "[x] on", "Notify on failure", "Notify when action is needed", "Timezone", "UTC"} {
		if !strings.Contains(v, want) {
			t.Fatalf("Settings lacks %q:\n%s", want, v)
		}
	}
	if !strings.Contains(v, "> Language") && !strings.Contains(v, ">") {
		t.Fatalf("the cursor is missing:\n%s", v)
	}
	// down moves through the rows, up at the first row goes to the tab bar.
	for i := 0; i < 6; i++ {
		m, _ = update(t, m, key("down"))
	}
	if m.cursor != 6 {
		t.Fatalf("the cursor should reach the last row, got %d", m.cursor)
	}
	m, _ = update(t, m, key("down"))
	if m.cursor != 6 {
		t.Fatal("the cursor stops at the last row")
	}
}

func TestSettingsToggleNotifyAndAutostartSaveAtOnce(t *testing.T) {
	f := newTFake()
	m := settingsModel(t, f, "en", 100, 30)
	m = sendKeys(t, m, "down") // autostart
	m = sendKeys(t, m, "enter")
	if !f.has("autostart:true") || !strings.Contains(m.View(), "[x] on") {
		t.Fatalf("autostart must switch on at once: %v", f.recorded())
	}
	m = sendKeys(t, m, "enter")
	if !f.has("autostart:false") {
		t.Fatalf("and off again: %v", f.recorded())
	}
	m = sendKeys(t, m, "down", "down", "down") // notify when done
	m = sendKeys(t, m, "enter")
	if !f.has("pref:notify_completed:false") {
		t.Fatalf("notify toggles: %v", f.recorded())
	}
	m = sendKeys(t, m, "down", "enter")
	if !f.has("pref:notify_failed:true") {
		t.Fatalf("second toggle: %v", f.recorded())
	}
	m = sendKeys(t, m, "down", "space")
	_ = m
}

func TestSettingsAutostartRowOnlyWhereItCanBeRead(t *testing.T) {
	f := newTFake()
	f.snap.Prefs.AutostartKnown = false
	m := settingsModel(t, f, "en", 100, 30)
	if strings.Contains(m.View(), "Start on login") {
		t.Fatal("no autostart row when the state is unknown")
	}
}

func TestSettingsRetentionStepsThroughPresets(t *testing.T) {
	f := newTFake()
	m := settingsModel(t, f, "en", 100, 30)
	m = sendKeys(t, m, "down", "down", "down") // retention (quiet is row 2)
	m = sendKeys(t, m, "down")
	_ = m
	f2 := newTFake()
	m2 := settingsModel(t, f2, "en", 100, 30)
	for i := 0; i < 3; i++ { // language, autostart, quiet -> retention
		m2, _ = update(t, m2, key("down"))
	}
	if row, _ := m2.currentSettingsRow(); row != rowRetention {
		t.Fatalf("row = %v", row)
	}
	m2 = sendKeys(t, m2, "right")
	if !f2.has("pref:retention_days:180") || !strings.Contains(m2.View(), "180 days") {
		t.Fatalf("→ steps 90 to 180: %v", f2.recorded())
	}
	m2 = sendKeys(t, m2, "left", "left")
	if !f2.has("pref:retention_days:30") {
		t.Fatalf("← steps back: %v", f2.recorded())
	}
	m2 = sendKeys(t, m2, "left")
	m2 = sendKeys(t, m2, "left")
	if !f2.has("pref:retention_days:0") || !strings.Contains(m2.View(), "forever") {
		t.Fatalf("0 means forever: %v", f2.recorded())
	}
}

func TestStepRetentionFromAnOddValue(t *testing.T) {
	cases := []struct{ cur, step, want int }{
		{90, 1, 180}, {90, -1, 30}, {365, 1, 0}, {0, 1, 7}, {7, -1, 0},
		{45, 1, 90}, {45, -1, 30}, {1000, 1, 0}, {1000, -1, 365},
	}
	for _, c := range cases {
		if got := stepRetention(c.cur, c.step); got != c.want {
			t.Errorf("stepRetention(%d, %d) = %d, want %d", c.cur, c.step, got, c.want)
		}
	}
}

func TestSettingsQuietHoursAreTypedAndValidated(t *testing.T) {
	f := newTFake()
	m := settingsModel(t, f, "en", 100, 30)
	m = sendKeys(t, m, "down", "down") // quiet hours
	m, _ = update(t, m, key("enter"))
	if m.setInput == nil || !m.typing() {
		t.Fatal("Enter opens the text field")
	}
	if !strings.Contains(m.View(), "Quiet hours as 22:00-07:00") {
		t.Fatalf("the prompt explains the form:\n%s", m.View())
	}
	m = typeText(t, m, "25:99")
	m, _ = update(t, m, key("enter"))
	if m.setInput == nil || !strings.Contains(m.status, "22:00-07:00") || f.has("pref:quiet_hours") {
		t.Fatalf("a bad time is refused and the field stays: %q", m.status)
	}
	m, _ = update(t, m, key("esc"))
	if m.setInput != nil || f.has("pref:quiet_hours") {
		t.Fatal("esc cancels")
	}
	m, _ = update(t, m, key("enter"))
	m = typeText(t, m, "22:30 - 7:00")
	m, cmd := update(t, m, key("enter"))
	m = run(t, m, cmd)
	if !f.has("pref:quiet_hours:22:30-07:00") || !strings.Contains(m.View(), "22:30-07:00") {
		t.Fatalf("a good range is saved normalised: %v", f.recorded())
	}
	// Emptying the field turns quiet hours off.
	m, _ = update(t, m, key("enter"))
	for i := 0; i < 11; i++ {
		m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyBackspace})
	}
	m, cmd = update(t, m, key("enter"))
	m = run(t, m, cmd)
	if !f.has("pref:quiet_hours:") || !strings.Contains(m.View(), "off") {
		t.Fatalf("empty turns it off: %v", f.recorded())
	}
}

func TestParseQuiet(t *testing.T) {
	good := map[string][2]string{
		"22:00-07:00": {"22:00", "07:00"}, " 22:00 – 07:00 ": {"22:00", "07:00"}, "22:00 07:00": {"22:00", "07:00"},
		"9:05-17:30": {"09:05", "17:30"}, "": {"", ""},
	}
	for in, want := range good {
		a, b, ok := parseQuiet(in)
		if !ok || a != want[0] || b != want[1] {
			t.Errorf("parseQuiet(%q) = %q %q %v", in, a, b, ok)
		}
	}
	for _, in := range []string{"22:00", "22:00-", "aa-bb", "24:00-07:00", "22:00-07:00-08:00", "22-7"} {
		if _, _, ok := parseQuiet(in); ok {
			t.Errorf("parseQuiet(%q) should fail", in)
		}
	}
}

func TestSettingsLanguageCyclesEnglishVietnameseAuto(t *testing.T) {
	f := newTFake()
	f.snap.Prefs.Language = "en"
	m := tModel(t, f, "", 100, 30, func(o *Options) { o.Language = "" })
	m, _ = update(t, m, key("6"))
	m = sendKeys(t, m, "enter")
	m = sendKeys(t, m, "enter")
	m = sendKeys(t, m, "enter")
	var langs []string
	for _, c := range f.recorded() {
		if strings.HasPrefix(c, "lang:") {
			langs = append(langs, c)
		}
	}
	if strings.Join(langs, ",") != "lang:vi,lang:auto,lang:en" {
		t.Fatalf("order = %v", langs)
	}
	m = sendKeys(t, m, "left")
	if !f.has("lang:auto") || m.snap.Prefs.Language != "auto" {
		t.Fatalf("← goes back: %v", f.recorded())
	}
	if !strings.Contains(m.View(), "● Auto") {
		t.Fatalf("Auto is marked:\n%s", m.View())
	}
}

func TestAutoLanguageFollowsTheMachineLocale(t *testing.T) {
	for locale, want := range map[string]string{"vi_VN.UTF-8": "Cài đặt", "en_US.UTF-8": "Settings", "C": "Settings"} {
		t.Run(locale, func(t *testing.T) {
			t.Setenv("LC_ALL", locale)
			f := newTFake()
			f.snap.Prefs.Language = "auto"
			m := tModel(t, f, "", 100, 30, func(o *Options) { o.Language = "" })
			m, _ = update(t, m, key("6"))
			if v := m.View(); !strings.Contains(v, want) {
				t.Fatalf("auto under %s should show %q:\n%s", locale, want, v)
			}
		})
	}
}

func TestSettingsSaveErrorIsShown(t *testing.T) {
	f := newTFake()
	f.prefErr = errors.New("daemon refused")
	m := settingsModel(t, f, "en", 100, 30)
	m = sendKeys(t, m, "down", "down", "down", "down", "enter")
	if !strings.Contains(m.View(), "daemon refused") {
		t.Fatalf("a failed save must be visible:\n%s", m.View())
	}
}

func TestSettingsFitsEveryWidthInBothLanguages(t *testing.T) {
	for _, lang := range []string{"en", "vi"} {
		f := newTFake()
		f.snap.Prefs.QuietStart, f.snap.Prefs.QuietEnd = "22:00", "07:00"
		m := settingsModel(t, f, lang, 100, 24)
		want := map[string]string{"en": "Quiet hours", "vi": "Giờ yên tĩnh"}[lang]
		for w, v := range fitsEverywhere(t, m, 24) {
			if !strings.Contains(v, want) {
				t.Errorf("%s width %d lost %q:\n%s", lang, w, want, v)
			}
		}
		m = sendKeys(t, m, "down", "down", "enter")
		fitsEverywhere(t, m, 24)
	}
}

func TestSettingsWithoutTheNewActionsKeepsOnlyLanguage(t *testing.T) {
	d := newDriveFake()
	m := driveModel(t, d, 100, 30)
	m, _ = update(t, m, key("6"))
	v := m.View()
	if strings.Contains(v, "Quiet hours") || strings.Contains(v, "Start on login") {
		t.Fatalf("rows without an action must not appear:\n%s", v)
	}
	if !strings.Contains(v, "● English") || !strings.Contains(v, "Auto") {
		t.Fatalf("the language row stays, now with Auto:\n%s", v)
	}
}

// ---- ASCII / no colour ----

func TestNewScreensReadWithoutColourOrUnicodeBars(t *testing.T) {
	f := newTFake()
	f.snap.Collections = []Collection{libCollection(f)}
	f.snap.Jobs = []Job{waitingJob()}
	f.snap.Status.Lifecycle = "sleeping"
	m := tModel(t, f, "en", 80, 30, func(o *Options) { o.ASCII = true; o.NoMotion = true })
	for _, tab := range []string{"1", "2", "3", "6"} {
		m, _ = update(t, m, key(tab))
		v := m.View()
		if strings.Contains(v, "\x1b") {
			t.Fatalf("tab %s prints colour codes without colour:\n%q", tab, v)
		}
		for _, bar := range []string{"█", "░", "▏"} {
			if strings.Contains(v, bar) {
				t.Fatalf("tab %s draws a unicode bar in ASCII mode", tab)
			}
		}
	}
	// The wait is readable as words, not only as a colour.
	m, _ = update(t, m, key("2"))
	if !strings.Contains(m.View(), "Waiting for quota until 14:05") {
		t.Fatalf("plain text must carry the meaning:\n%s", m.View())
	}
}
