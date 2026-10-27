package tui

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// ---- S7: the full scan in the copy preview ----

func openPlan(t *testing.T, f *iFake, lang string, w, h int) Model {
	t.Helper()
	m := iModel(t, f, lang, w, h)
	m = sendKeys(t, m, "c")
	m = typeText(t, m, "SRC1ABCDEFGHIJKLMNOP")
	return sendKeys(t, m, "enter")
}

func TestDeepScanFollowsTheDaemonAndNeverCallsPartialCountsATotal(t *testing.T) {
	f := newIFake()
	two := int64(125)
	f.deepSteps = []Plan{
		{ID: "plan_1", Files: 40, Folders: 5, Deep: &DeepScan{State: "running", Files: 40, Folders: 5, Bytes: 3 << 30, Scanned: 45}},
		{ID: "plan_1", Files: 90, Folders: 9, Complete: true, Deep: &DeepScan{State: "complete", Complete: true, Files: 90, Folders: 9, Bytes: 5 << 30,
			Native: 3, Shortcuts: 2, CannotCopy: 1, Unreadable: 4, EstimateSeconds: &two,
			Warnings: []string{"destination_storage_short", "may_hit_daily_limit"}}},
	}
	m := openPlan(t, f, "en", 100, 40)
	m = sendKeys(t, m, "i")
	if !f.has("inspect:SRC1ABCDEFGHIJKLMNOP::deep=true") {
		t.Fatalf("i asks for a preview with the full scan: %v", f.recorded())
	}
	v := m.View()
	for _, want := range []string{"Full scan", "Scanning… (5 items)", "4 files · 1 folders", "Not yet a total"} {
		if !strings.Contains(v, want) {
			t.Fatalf("while scanning the view lacks %q:\n%s", want, v)
		}
	}
	// the tick reads the plan again
	m = settle(t, m, func() tea.Msg { return deepTickMsg{planID: "plan_1"} })
	if !f.has("getplan:plan_1") || !strings.Contains(m.View(), "40 files · 5 folders · 3.0 GB") || !strings.Contains(m.View(), "Not yet a total") {
		t.Fatalf("the first answer is still partial:\n%s", m.View())
	}
	m = settle(t, m, func() tea.Msg { return deepTickMsg{planID: "plan_1"} })
	v = m.View()
	for _, want := range []string{"Scan finished", "90 files · 9 folders · 5.0 GB", "Google 3 · shortcuts 2 · cannot copy 1 · unreadable folders 4",
		"Estimated 2m", "free storage may not hold", "daily copy limit may be reached"} {
		if !strings.Contains(v, want) {
			t.Fatalf("the finished scan lacks %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "Not yet a total") {
		t.Fatalf("a complete scan is a total:\n%s", v)
	}
	// a late tick for a finished scan reads nothing more
	n := f.count("getplan:")
	m = settle(t, m, func() tea.Msg { return deepTickMsg{planID: "plan_1"} })
	if f.count("getplan:") != n {
		t.Fatalf("a finished scan is not polled: %v", f.recorded())
	}
	// the user can still start the copy
	_ = sendKeys(t, m, "enter")
	if !f.has("submitopts:plan_1::") {
		t.Fatalf("the copy goes with the plan: %v", f.recorded())
	}
}

func TestDeepScanCanBeCancelledAndAStaleAnswerIsIgnored(t *testing.T) {
	f := newIFake()
	m := openPlan(t, f, "en", 100, 40)
	m = sendKeys(t, m, "i")
	cmdOnTick := m.cp.plan.Deep.Running()
	if !cmdOnTick {
		t.Fatal("the scan is running")
	}
	m = sendKeys(t, m, "x")
	if !f.has("cancelscan:plan_1") {
		t.Fatalf("x cancels the scan: %v", f.recorded())
	}
	v := m.View()
	if !strings.Contains(v, "Scan cancelled") || !strings.Contains(v, "Not yet a total") {
		t.Fatalf("a cancelled scan is not a total:\n%s", v)
	}
	n := f.count("cancelscan:")
	m = sendKeys(t, m, "x")
	if f.count("cancelscan:") != n {
		t.Fatalf("nothing is running any more: %v", f.recorded())
	}
	// an answer for a plan that is no longer on screen changes nothing
	before := m.cp.plan.Deep.Files
	m, _ = update(t, m, deepMsg{planID: "plan_old", plan: Plan{Deep: &DeepScan{State: "complete", Complete: true, Files: 99}}})
	if m.cp.plan.Deep.Files != before || m.cp.plan.Deep.Complete {
		t.Fatalf("a stale answer must be ignored: %+v", m.cp.plan.Deep)
	}
	// turning the scan off asks for a shallow preview again
	m = sendKeys(t, m, "i")
	if !f.has("inspect:SRC1ABCDEFGHIJKLMNOP::deep=false") && f.count("inspect:SRC1ABCDEFGHIJKLMNOP::deep=true") < 2 {
		t.Fatalf("i toggles: %v", f.recorded())
	}
}

func TestChangingTheOptionsWhileScanningStopsTheOldScan(t *testing.T) {
	f := newIFake()
	m := openPlan(t, f, "en", 100, 40)
	m = sendKeys(t, m, "i")
	_ = sendKeys(t, m, "f") // a new preview replaces the one being scanned
	if !f.has("cancelscan:plan_1") {
		t.Fatalf("the scan nobody reads is stopped: %v", f.recorded())
	}
}

func TestDeepViewFitsEveryWidthInBothLanguages(t *testing.T) {
	for _, lang := range []string{"en", "vi"} {
		f := newIFake()
		est := int64(7300)
		f.deepSteps = []Plan{{ID: "plan_1", Files: 90, Folders: 9, Complete: true, Deep: &DeepScan{State: "complete", Complete: true, Files: 90, Folders: 9, Bytes: 5 << 30,
			Native: 3, Shortcuts: 2, CannotCopy: 1, Unreadable: 4, EstimateSeconds: &est,
			Warnings: []string{"estimate_unknown", "destination_storage_short", "may_hit_daily_limit", "brand_new_code"}}}}
		m := openPlan(t, f, lang, 100, 44)
		m = sendKeys(t, m, "i")
		m = settle(t, m, func() tea.Msg { return deepTickMsg{planID: "plan_1"} })
		views := fitsEverywhere(t, m, 44)
		if lang == "vi" {
			for w, v := range views {
				if !strings.Contains(v, "Quét xong") || !strings.Contains(v, "brand_new_code") {
					t.Errorf("width %d: Vietnamese deep view (and an unknown code shown as it is):\n%s", w, v)
				}
			}
			if !strings.Contains(views[120], "Ước tính 2 giờ") && !strings.Contains(views[120], "Ước tính 2") {
				t.Errorf("estimate in Vietnamese:\n%s", views[120])
			}
		}
	}
}

func TestADestinationTheAccountCannotWriteToIsExplainedAndBlocksStart(t *testing.T) {
	f := newIFake()
	m := openPlan(t, f, "vi", 100, 30)
	f.planErr = &CodedError{Code: "destination_not_writable", Message: "This account cannot add files to that folder."}
	m = sendKeys(t, m, "o") // any option re-runs the preview, which now fails
	v := m.View()
	if !strings.Contains(v, "không ghi được vào thư mục đích") || !strings.Contains(v, "Nhấn d") {
		t.Fatalf("the refusal is worded in Vietnamese with the way out:\n%s", v)
	}
	n := f.count("submitopts:")
	m = sendKeys(t, m, "enter")
	if f.count("submitopts:") != n || !m.cp.stale {
		t.Fatalf("what is shown is not what would run, so Enter does nothing: %v", f.recorded())
	}
	f.planErr = nil
	m = sendKeys(t, m, "o")
	if m.cp.stale {
		t.Fatal("a good preview clears it")
	}
	m = sendKeys(t, m, "enter")
	if f.count("submitopts:") != n+1 {
		t.Fatalf("now it starts: %v", f.recorded())
	}
}

// ---- S8: after-copy sync ----

func TestAfterCopySyncStartsAtThePreferenceAndOnlyDiffersAreSent(t *testing.T) {
	f := newIFake()
	f.snap.Prefs.AfterCopySync = afterHourly
	m := openPlan(t, f, "en", 100, 30)
	if !strings.Contains(m.View(), "● hourly") {
		t.Fatalf("the dialog starts at the saved mode:\n%s", m.View())
	}
	m = sendKeys(t, m, "enter")
	if !f.has("submitopts:plan_1::") || f.has("submitopts:plan_1:::hourly") {
		t.Fatalf("keeping the preference sends nothing, the daemon applies it: %v", f.recorded())
	}
	// another copy: choose off, which differs from the preference
	m = sendKeys(t, m, "1", "c")
	m = typeText(t, m, "SRC2ABCDEFGHIJKLMNOP")
	m = sendKeys(t, m, "enter")
	m = sendKeys(t, m, "s", "s", "s", "s", "s") // hourly → six → daily → off → realtime → hourly (full circle)
	if !strings.Contains(m.View(), "● hourly") {
		t.Fatalf("five presses make a full circle:\n%s", m.View())
	}
	m = sendKeys(t, m, "s", "s", "s") // six_hourly, daily, off
	_ = sendKeys(t, m, "enter")
	if !strings.Contains(strings.Join(f.recorded(), " "), "submitopts:plan_1::off") {
		t.Fatalf("off differs from hourly, so it is sent explicitly: %v", f.recorded())
	}
}

func TestAfterCopySyncRowInSettingsSavesAtOnce(t *testing.T) {
	f := newIFake()
	f.snap.Prefs.AfterCopySync = afterOff
	m := iModel(t, f, "vi", 100, 30)
	m = sendKeys(t, m, "6")
	for i := 0; i < 7; i++ {
		m = sendKeys(t, m, "down")
	}
	v := m.View()
	if !strings.Contains(v, "Đồng bộ sau copy") || !strings.Contains(v, "● tắt") {
		t.Fatalf("the row shows the saved mode:\n%s", v)
	}
	m = sendKeys(t, m, "enter")
	if !f.has("pref:after_copy_sync_mode:realtime") {
		t.Fatalf("Enter steps to realtime and saves: %v", f.recorded())
	}
	m = sendKeys(t, m, "h", "h")
	if !f.has("pref:after_copy_sync_mode:daily") {
		t.Fatalf("← wraps backwards to daily: %v", f.recorded())
	}
	for w, view := range fitsEverywhere(t, m, 30) {
		if !strings.Contains(view, "Đồng bộ sau copy") {
			t.Errorf("width %d lost the row:\n%s", w, view)
		}
	}
}

func TestStepAfterSyncWrapsBothWays(t *testing.T) {
	cases := []struct {
		cur  string
		step int
		want string
	}{{"off", 1, "realtime"}, {"daily", 1, "off"}, {"off", -1, "daily"}, {"", 1, "realtime"}, {"junk", -1, "daily"}, {"hourly", 1, "six_hourly"}}
	for _, c := range cases {
		if got := stepAfterSync(c.cur, c.step); got != c.want {
			t.Errorf("stepAfterSync(%q,%d) = %q, want %q", c.cur, c.step, got, c.want)
		}
	}
}

// ---- X1: report and CSV export ----

func reportFake() *iFake {
	f := newIFake()
	f.jobs = []Job{{ID: "job_9", CollectionID: "col_1", SourceName: "Project Alpha", State: "partial", Revision: 4, Completed: 7, Failed: 1, Skipped: 3,
		SkippedByCode: map[string]int{"filtered": 2, "cannot_copy": 1}}}
	f.rep = JobReport{Items: []ProblemItem{
		{State: "skipped", Code: "filtered", Path: "videos/holiday.mp4"},
		{State: "skipped", Code: "filtered", Path: "videos/party.mp4"},
		{State: "skipped", Code: "cannot_copy", Path: "docs/locked.pdf"},
		{State: "failed", Code: "unreadable", Path: "private/\u202edoc.txt", Attempts: 3},
	}}
	return f
}

func TestJobReportListsProblemsByCodeAndGoesBackToTheJob(t *testing.T) {
	f := reportFake()
	m := iModel(t, f, "en", 100, 30)
	m = sendKeys(t, m, "2", "enter")
	m = sendKeys(t, m, "R")
	if !f.has("report:job_9") || m.screen != screenReport {
		t.Fatalf("R opens the report: %v screen %v", f.recorded(), m.screen)
	}
	v := m.View()
	for _, want := range []string{"Failed and skipped items", "job_9", "2  excluded by the filter", "1  copying not allowed", "1  cannot be read",
		"videos/holiday.mp4", "docs/locked.pdf", "unreadable", "3 tries", "↑↓ scroll"} {
		if !strings.Contains(v, want) {
			t.Fatalf("report lacks %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "\u202e") || !strings.Contains(v, `\u202e`) {
		t.Fatalf("a bidi control in a Drive path must be neutralised:\n%q", v)
	}
	m = sendKeys(t, m, "down", "down")
	if m.cursor != 2 {
		t.Fatalf("↑↓ scroll the rows: %d", m.cursor)
	}
	m = sendKeys(t, m, "esc")
	if m.screen != screenJob || m.cursor != 0 || !strings.Contains(m.View(), "Project Alpha") {
		t.Fatalf("esc returns to the job with the cursor on it: screen %v cursor %d", m.screen, m.cursor)
	}
}

func TestJobReportStatesEmptyLoadingErrorAndCut(t *testing.T) {
	f := reportFake()
	f.rep = JobReport{}
	m := iModel(t, f, "vi", 80, 30)
	m = sendKeys(t, m, "2", "enter", "R")
	if !strings.Contains(m.View(), "Không có mục lỗi, bị chặn hay bị bỏ qua") {
		t.Fatalf("empty report:\n%s", m.View())
	}
	f.repErr = errors.New("job not found")
	m = sendKeys(t, m, "esc", "R")
	if !strings.Contains(m.View(), "job not found") {
		t.Fatalf("an error is shown:\n%s", m.View())
	}
	f.repErr = nil
	f.rep = JobReport{Items: []ProblemItem{{State: "failed", Code: "unreadable", Path: "a"}}, Truncated: true}
	m = sendKeys(t, m, "esc", "R")
	if !strings.Contains(m.View(), "Đã cắt ở 1 dòng") {
		t.Fatalf("a cut report says so:\n%s", m.View())
	}
	loading := m
	loading.rep = &reportView{jobID: "job_9", loading: true}
	if !strings.Contains(loading.View(), "Đang đọc báo cáo") {
		t.Fatalf("loading:\n%s", loading.View())
	}
}

func TestReportExportAsksForAPathNeverSendsWithoutOne(t *testing.T) {
	f := reportFake()
	m := iModel(t, f, "en", 100, 30)
	m = sendKeys(t, m, "2", "enter", "R", "E")
	if m.exp == nil || !strings.Contains(m.View(), "Save the CSV report to path") || !strings.Contains(m.View(), "502drive-report-9.csv") {
		t.Fatalf("E asks for a path with a suggestion:\n%s", m.View())
	}
	// esc sends nothing
	m, _ = update(t, m, key("esc"))
	if m.exp != nil || f.has("export:") {
		t.Fatalf("esc cancels: %v", f.recorded())
	}
	// an empty path is refused
	m = sendKeys(t, m, "E")
	for i := 0; i < 40; i++ {
		m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyBackspace})
	}
	m, _ = update(t, m, key("enter"))
	if f.has("export:") || !strings.Contains(m.status, "Type a .csv file path") {
		t.Fatalf("no path, no export: %v %q", f.recorded(), m.status)
	}
	m = typeText(t, m, "/tmp/out.csv")
	m, cmd := update(t, m, key("enter"))
	m = run(t, m, cmd)
	if !f.has("export:report:job_9:/tmp/out.csv") {
		t.Fatalf("the typed path goes to the daemon client: %v", f.recorded())
	}
	if !strings.Contains(m.status, "Saved 7 rows to /tmp/out.csv") {
		t.Fatalf("status = %q", m.status)
	}
	// a refusal (the file exists) is shown, not hidden
	f.exportErr = errors.New("That file already exists; choose another name.")
	m = sendKeys(t, m, "E")
	m, cmd = update(t, m, key("enter"))
	m = run(t, m, cmd)
	if !strings.Contains(m.status, "already exists") {
		t.Fatalf("status = %q", m.status)
	}
}

func TestExportKeysAreTypingNotNavigation(t *testing.T) {
	f := reportFake()
	m := iModel(t, f, "en", 100, 30)
	m = sendKeys(t, m, "2", "enter", "E")
	m = typeText(t, m, "q1R7")
	if m.quits || m.screen != screenJob || m.exp == nil {
		t.Fatalf("letters go to the path field: quit %v screen %v", m.quits, m.screen)
	}
	if !strings.Contains(m.exp.input.Value(), "q1R7") {
		t.Fatalf("value = %q", m.exp.input.Value())
	}
}

func TestHistoryExportFromTheJobsList(t *testing.T) {
	f := reportFake()
	m := iModel(t, f, "vi", 100, 30)
	m = sendKeys(t, m, "2", "E")
	if !strings.Contains(m.View(), "Lưu lịch sử 30 ngày") {
		t.Fatalf("E on the list exports the history:\n%s", m.View())
	}
	for i := 0; i < 40; i++ {
		m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyBackspace})
	}
	m = typeText(t, m, "lich-su.csv")
	m, cmd := update(t, m, key("enter"))
	m = run(t, m, cmd)
	if !f.has("export:history::lich-su.csv") || !strings.Contains(m.status, "Đã lưu 7 dòng vào lich-su.csv") {
		t.Fatalf("history export: %v %q", f.recorded(), m.status)
	}
}

func TestReportAndExportFitEveryWidthInBothLanguages(t *testing.T) {
	for _, lang := range []string{"en", "vi"} {
		f := reportFake()
		for i := 0; i < 30; i++ {
			f.rep.Items = append(f.rep.Items, ProblemItem{State: "skipped", Code: "filtered", Path: fmt.Sprintf("thư mục dài/tệp số %d với một cái tên rất dài.mp4", i)})
		}
		m := iModel(t, f, lang, 100, 24, func(o *Options) { o.ASCII = true })
		m = sendKeys(t, m, "2", "enter", "R")
		m = sendKeys(t, m, "end")
		fitsEverywhere(t, m, 24)
		m = sendKeys(t, m, "E")
		fitsEverywhere(t, m, 24)
		m = sendKeys(t, m, "esc", "esc", "esc", "E")
		for w, v := range fitsEverywhere(t, m, 24) {
			if !strings.Contains(v, "›") && !strings.Contains(v, ".csv") {
				t.Errorf("%s width %d: the export prompt must stay visible:\n%s", lang, w, v)
			}
		}
	}
}

// ---- Drive search ----

func searchFake() *iFake {
	f := newIFake()
	size := int64(10)
	f.items = []DriveItem{
		{ID: "s1", Name: "Báo cáo 2026", Kind: "folder", CanCopy: true, CanWrite: true},
		{ID: "s2", Name: "bao-cao.pdf", Kind: "binary", Size: &size, CanCopy: true},
	}
	return f
}

func TestSlashSearchesDriveByNameAndCopiesAResult(t *testing.T) {
	f := searchFake()
	m := iModel(t, f, "en", 100, 30)
	m = sendKeys(t, m, "7")
	m = sendKeys(t, m, "/")
	if m.drv.prompt == nil || !strings.Contains(m.View(), "Search Drive by name") {
		t.Fatalf("/ asks for the words:\n%s", m.View())
	}
	m = typeText(t, m, "bao cao")
	m, cmd := update(t, m, key("enter"))
	m = run(t, m, cmd)
	if !f.has("search:bao cao") {
		t.Fatalf("the words go to the search: %v", f.recorded())
	}
	v := m.View()
	for _, want := range []string{"Results for “bao cao”", "Báo cáo 2026/", "bao-cao.pdf", "enter open folder"} {
		if !strings.Contains(v, want) {
			t.Fatalf("results lack %q:\n%s", want, v)
		}
	}
	m = sendKeys(t, m, "down", "c")
	if m.screen != screenCopy || !f.has("inspect:s2:") {
		t.Fatalf("c copies the highlighted result: screen %v %v", m.screen, f.recorded())
	}
}

func TestSearchEscGoesBackToTheFolderItBeganIn(t *testing.T) {
	f := searchFake()
	m := iModel(t, f, "en", 100, 30)
	m = sendKeys(t, m, "7")
	m = sendKeys(t, m, "enter") // open Projects
	m = sendKeys(t, m, "/")
	m = typeText(t, m, "x")
	m, cmd := update(t, m, key("enter"))
	m = run(t, m, cmd)
	if m.drv.query != "x" {
		t.Fatalf("query = %q", m.drv.query)
	}
	m = sendKeys(t, m, "esc")
	if m.drv.query != "" || m.screen != screenDrive || !strings.Contains(m.View(), "My Drive › Projects") {
		t.Fatalf("esc returns to the folder, not Home: screen %v\n%s", m.screen, m.View())
	}
	// ← does the same, and a search with no result says so
	f.items = nil
	m = sendKeys(t, m, "/")
	m = typeText(t, m, "nothing")
	m, cmd = update(t, m, key("enter"))
	m = run(t, m, cmd)
	if !strings.Contains(m.View(), "Nothing found") {
		t.Fatalf("no result:\n%s", m.View())
	}
	m = sendKeys(t, m, "left")
	if m.drv.query != "" {
		t.Fatal("← leaves the search")
	}
}

func TestOpeningAFolderFromResultsLeavesTheSearch(t *testing.T) {
	f := searchFake()
	f.folders["s1"] = FolderPage{Folder: DriveItem{ID: "s1", Name: "Báo cáo 2026", Kind: "folder"}, Path: []DriveItem{{ID: "root", Name: "My Drive"}, {ID: "s1", Name: "Báo cáo 2026"}}}
	m := iModel(t, f, "en", 100, 30)
	m = sendKeys(t, m, "7", "/")
	m = typeText(t, m, "bao")
	m, cmd := update(t, m, key("enter"))
	m = run(t, m, cmd)
	m = sendKeys(t, m, "enter")
	if m.drv.query != "" || !strings.Contains(m.View(), "My Drive › Báo cáo 2026") {
		t.Fatalf("enter on a folder opens it:\n%s", m.View())
	}
}

func TestWordsInTheCopyBoxSearchDriveButLinksAndIDsDoNot(t *testing.T) {
	f := searchFake()
	m := iModel(t, f, "vi", 100, 30)
	m = sendKeys(t, m, "c")
	m = typeText(t, m, "báo cáo tháng 10")
	m = sendKeys(t, m, "enter")
	if !f.has("search:báo cáo tháng 10") || m.screen != screenDrive {
		t.Fatalf("words are searched: %v screen %v", f.recorded(), m.screen)
	}
	if v := m.View(); !strings.Contains(v, "Kết quả cho “báo cáo tháng 10”") {
		t.Fatalf("Vietnamese title:\n%s", v)
	}
	for _, ref := range []string{"https://drive.google.com/drive/folders/1AbCdEfGhIjKlMnOpQrStUv", "1AbCdEfGhIjKlMnOpQrStUvWxYz_-"} {
		f2 := searchFake()
		m2 := iModel(t, f2, "vi", 100, 30)
		m2 = sendKeys(t, m2, "c")
		m2 = typeText(t, m2, ref)
		_ = sendKeys(t, m2, "enter")
		if f2.has("search:") || !f2.has("inspect:"+ref) {
			t.Fatalf("%q is a reference, not words: %v", ref, f2.recorded())
		}
	}
	// the Home prompt behaves the same way
	f3 := searchFake()
	m3 := iModel(t, f3, "en", 100, 30)
	m3 = typeText(t, m3, "annual report")
	_ = sendKeys(t, m3, "enter")
	if !f3.has("search:annual report") {
		t.Fatalf("the Home prompt searches words: %v", f3.recorded())
	}
}

func TestSearchFitsEveryWidthInBothLanguages(t *testing.T) {
	for _, lang := range []string{"en", "vi"} {
		f := searchFake()
		for i := 0; i < 20; i++ {
			f.items = append(f.items, DriveItem{ID: fmt.Sprintf("x%d", i), Name: fmt.Sprintf("Tệp báo cáo số %d dài dài dài dài dài", i), Kind: "binary"})
		}
		m := iModel(t, f, lang, 100, 24)
		m = sendKeys(t, m, "7", "/")
		m = typeText(t, m, "báo")
		m, cmd := update(t, m, key("enter"))
		m = run(t, m, cmd)
		fitsEverywhere(t, m, 24)
		if v := m.View(); !strings.Contains(v, "20") {
			t.Errorf("%s: the 20-result limit is stated:\n%s", lang, v)
		}
	}
}

// ---- Library: include retired, last audit, repairable ----

func auditFake() *iFake {
	f := newIFake()
	f.snap.Collections = []Collection{
		{ID: "col_1", SourceName: "Project Alpha", State: "ready", Revision: 2, RootName: "Project Alpha"},
		{ID: "col_2", SourceName: "Beta", State: "ready", Revision: 1},
		{ID: "col_3", SourceName: "Gamma", State: "ready", Revision: 1},
		{ID: "col_old", SourceName: "Old", State: "retired", Revision: 3},
	}
	f.latest["col_1"] = AuditReport{ID: "aud_1", CollectionID: "col_1", JobID: "job_a", State: "completed", Revision: 5, Scanned: 9,
		Counts: map[string]int{"missing_at_destination": 2, "name_mismatch": 1}, Repairable: 2}
	f.latest["col_2"] = AuditReport{ID: "aud_2", CollectionID: "col_2", State: "completed", Revision: 1}
	return f
}

func TestLibraryShowsTheNewestAuditOfEachVisibleRow(t *testing.T) {
	f := auditFake()
	m := iModel(t, f, "en", 100, 30)
	m = sendKeys(t, m, "3")
	m = refresh(t, m)
	for _, id := range []string{"col_1", "col_2", "col_3"} {
		if !f.has("latest:" + id) {
			t.Fatalf("the visible row %s is read: %v", id, f.recorded())
		}
	}
	if f.has("latest:col_old") {
		t.Fatalf("a retired source has no audit to show: %v", f.recorded())
	}
	v := m.View()
	for _, want := range []string{"check: 3 finding(s) (2 repairable)", "check: clean"} {
		if !strings.Contains(v, want) {
			t.Fatalf("library lacks %q:\n%s", want, v)
		}
	}
	for _, lang := range []string{"en", "vi"} {
		ml := iModel(t, f, lang, 100, 30)
		ml = sendKeys(t, ml, "3")
		ml = refresh(t, ml)
		fitsEverywhere(t, ml, 30)
	}
	// a source never audited shows no chip, and is not asked again within a minute
	n := f.count("latest:col_3")
	m = refresh(t, m)
	m = refresh(t, m)
	if f.count("latest:col_3") != n {
		t.Fatalf("one read per minute at most: %v", f.recorded())
	}
	// after the minute it is read again
	later := tNow.Add(2 * time.Minute)
	m.opt.Now = func() time.Time { return later }
	m = refresh(t, m)
	if f.count("latest:col_3") != n+1 {
		t.Fatalf("a stale answer is read again: %v", f.recorded())
	}
}

func TestCollectionScreenStartsFromTheLatestAuditAndCanPlanARepair(t *testing.T) {
	f := auditFake()
	m := iModel(t, f, "vi", 100, 40)
	m = sendKeys(t, m, "3", "enter")
	m = refresh(t, m)
	v := m.View()
	for _, want := range []string{"Lần kiểm tra gần nhất", "3 sai lệch (2 sửa được)"} {
		if !strings.Contains(v, want) {
			t.Fatalf("collection lacks %q:\n%s", want, v)
		}
	}
	f.audit = f.latest["col_1"] // what GetAudit answers
	m = sendKeys(t, m, "f")     // plan a repair from the audit that was only read
	if !f.has("repair:aud_1:5:true") {
		t.Fatalf("the repair plan uses the latest audit and its revision: %v", f.recorded())
	}
	// a new scan started here replaces the read one
	m = sendKeys(t, m, "a")
	if !f.has("audit:col_1") || m.audits["col_1"].fromLatest {
		t.Fatal("a scan started here is not 'the latest read'")
	}
	for w, view := range fitsEverywhere(t, m, 40) {
		if !strings.Contains(view, "Project Alpha") {
			t.Errorf("width %d lost the title:\n%s", w, view)
		}
	}
}

func TestLibraryWithoutTheActionDoesNotAskForAudits(t *testing.T) {
	f := auditFake()
	acts := f.acts()
	acts.LatestAudit = nil
	m := New(acts, Options{Width: 100, Height: 30, Language: "en", NoMotion: true, Now: func() time.Time { return tNow }})
	m = settle(t, m, m.fetch())
	m = sendKeys(t, m, "3")
	if f.has("latest:") || strings.Contains(m.View(), "check:") {
		t.Fatalf("no action, no audit line: %v", f.recorded())
	}
}

func TestShowRetiredStillWorksWithTheIncludeRetiredRead(t *testing.T) {
	f := auditFake()
	m := iModel(t, f, "en", 100, 30)
	m = sendKeys(t, m, "3", "H")
	if !f.has("retired:true") || !m.showRetired {
		t.Fatalf("H asks the client to include retired sources: %v", f.recorded())
	}
}

func TestHomePromptWithSeveralPastedLinksOpensTheBatchDialog(t *testing.T) {
	f := newIFake()
	m := iModel(t, f, "vi", 80, 30)
	m = paste(t, m, "https://drive.google.com/drive/folders/AAAAAAAAAAAAAAAAAAAA\nhttps://drive.google.com/drive/folders/BBBBBBBBBBBBBBBBBBBB\n")
	m = sendKeys(t, m, "enter")
	if m.screen != screenCopy || m.cp == nil || !m.cp.isBatch() || len(m.cp.sources) != 2 {
		t.Fatalf("two pasted links make a batch: screen %v cp %+v", m.screen, m.cp)
	}
	if f.has("inspect:") || !strings.Contains(m.View(), "Lô 2 link") {
		t.Fatalf("no preview per link:\n%s", m.View())
	}
	// the same link pasted twice is one link
	m2 := iModel(t, f, "vi", 80, 30)
	m2 = paste(t, m2, "https://drive.google.com/drive/folders/AAAAAAAAAAAAAAAAAAAA https://drive.google.com/drive/folders/AAAAAAAAAAAAAAAAAAAA")
	m2 = sendKeys(t, m2, "enter")
	if m2.cp == nil || m2.cp.isBatch() || !f.has("inspect:https://drive.google.com/drive/folders/AAAAAAAAAAAAAAAAAAAA") {
		t.Fatalf("duplicates collapse to a single copy: %+v", m2.cp)
	}
}

func TestBatchITScreensReadWithoutColourOrUnicodeBars(t *testing.T) {
	f := reportFake()
	f.paused = true
	f.jobs = append(f.jobs, queuedJobs(2)...)
	f.jobs[1].BatchID = "batch-abcdef123456"
	f.snap.Collections = auditFake().snap.Collections
	f.latest = auditFake().latest
	f.items = searchFake().items
	est := int64(90)
	f.deepSteps = []Plan{{ID: "plan_1", Complete: true, Deep: &DeepScan{State: "complete", Complete: true, Files: 3, EstimateSeconds: &est, Warnings: []string{"may_hit_daily_limit"}}}}
	check := func(label string, m Model, wants ...string) {
		t.Helper()
		v := m.View()
		if strings.Contains(v, "\x1b") {
			t.Fatalf("%s prints colour codes without colour:\n%q", label, v)
		}
		for _, bar := range []string{"█", "░", "▏"} {
			if strings.Contains(v, bar) {
				t.Fatalf("%s draws a unicode bar in ASCII mode", label)
			}
		}
		for _, want := range wants {
			if !strings.Contains(v, want) {
				t.Fatalf("%s: the meaning must be in words, lacks %q:\n%s", label, want, v)
			}
		}
	}
	m := iModel(t, f, "en", 80, 40, func(o *Options) { o.ASCII = true })
	m = sendKeys(t, m, "2")
	m = refresh(t, m)
	check("jobs", m, "The queue is paused", "queue #1")
	m = sendKeys(t, m, "f")
	check("jobs filtered", m, "Filter: Queued")
	m = sendKeys(t, m, "f", "f", "f", "f", "f", "f", "enter") // back to all states, open a job
	m = sendKeys(t, m, "R")
	check("report", m, "Failed and skipped items", "excluded by the filter", "FAILED")
	m = sendKeys(t, m, "E")
	check("export", m, "Save the CSV report to path")
	m = sendKeys(t, m, "esc", "esc", "esc", "3")
	m = refresh(t, m)
	check("library", m, "check: 3 finding(s) (2 repairable)", "check: clean")
	m = sendKeys(t, m, "7", "/")
	m = typeText(t, m, "bao")
	m, cmd := update(t, m, key("enter"))
	m = run(t, m, cmd)
	check("search", m, "Results for", "bao-cao.pdf")
	m = sendKeys(t, m, "1", "c")
	m = typeText(t, m, "SRC1ABCDEFGHIJKLMNOP")
	m = sendKeys(t, m, "enter", "i")
	m = settle(t, m, func() tea.Msg { return deepTickMsg{planID: "plan_1"} })
	check("deep", m, "Scan finished", "daily copy limit may be reached", "Estimated")
	m = sendKeys(t, m, "esc", "1", "c")
	m = paste(t, m, "AAAAAAAAAAAAAAAAAAAAAAAAA BBBBBBBBBBBBBBBBBBBBBBBBB")
	m = sendKeys(t, m, "enter")
	check("batch dialog", m, "Batch of 2 links", "● off")
}
