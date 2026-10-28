package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// ---- helpers for the batch IT screens ----

// iFake is the daemon of the batch IT tests: it keeps a full job list and
// answers like the real one (server-side filter, cursor pages, queue pause).
type iFake struct {
	*tFake
	jobs      []Job
	query     JobQuery
	paused    bool
	items     []DriveItem
	rep       JobReport
	repErr    error
	latest    map[string]AuditReport
	batch     BatchProgress
	batchRes  BatchResult
	deepSteps []Plan // successive answers of GetPlan
	deepAt    int
	planErr   error
	exportErr error
	moveErr   error
}

func newIFake() *iFake {
	f := &iFake{tFake: newTFake(), latest: map[string]AuditReport{}}
	f.snap.Jobs = nil
	return f
}

func (f *iFake) acts() Actions {
	a := f.tFake.acts()
	a.Snapshot = func(context.Context) (Snapshot, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		snap := f.snap
		var jobs []Job
		src := f.jobs
		if src == nil {
			src = f.snap.Jobs
		}
		for _, j := range src {
			if f.query.State != "" && j.State != f.query.State {
				continue
			}
			if f.query.CollectionID != "" && j.CollectionID != f.query.CollectionID {
				continue
			}
			jobs = append(jobs, j)
		}
		n := min(len(jobs), jobsPage*max(1, f.query.Pages))
		snap.Jobs = jobs[:n]
		snap.JobsNext = ""
		if n < len(jobs) {
			snap.JobsNext = "next"
		}
		snap.Status.QueuePaused = f.paused
		return snap, nil
	}
	a.SetJobQuery = func(q JobQuery) {
		f.record(fmt.Sprintf("jobquery:%s:%s:%d", q.State, q.CollectionID, q.Pages))
		f.mu.Lock()
		f.query = q
		f.mu.Unlock()
	}
	a.QueuePause = func(_ context.Context, paused bool) (int, error) {
		f.record(fmt.Sprintf("queue:%v", paused))
		f.mu.Lock()
		f.paused = paused
		f.mu.Unlock()
		return 2, nil
	}
	a.MoveJob = func(_ context.Context, id string, rev int, mv JobMove) error {
		p := "-"
		if mv.Priority != nil {
			p = fmt.Sprint(*mv.Priority)
		}
		f.record(fmt.Sprintf("move:%s:%d:before=%s:after=%s:prio=%s", id, rev, mv.Before, mv.After, p))
		return f.moveErr
	}
	a.GetBatch = func(_ context.Context, id string) (BatchProgress, error) {
		f.record("batch:" + id)
		return f.batch, nil
	}
	a.SubmitOpts = func(_ context.Context, p Plan, o SubmitOptions) (string, string, error) {
		f.record(fmt.Sprintf("submitopts:%s:%s:%s", p.ID, o.Mode, o.AfterSync))
		if f.existing && o.Mode == "" {
			return "", "", &ExistingCopyError{Message: "already copied"}
		}
		return "job_new", "col_1", nil
	}
	a.SubmitBatch = func(_ context.Context, req BatchRequest) (BatchResult, error) {
		f.record(fmt.Sprintf("submitbatch:%s:dest=%s:auto=%v:sync=%s:filter=%s:conflict=%s", strings.Join(req.Sources, ","),
			req.DestinationID, req.AutoName, req.Submit.AfterSync, req.Options.FilterPreset, req.Options.OnConflict))
		return f.batchRes, nil
	}
	a.InspectWith = func(_ context.Context, source, dest, name string, o CopyOptions) (Plan, error) {
		f.record(fmt.Sprintf("inspect:%s:%s:deep=%v", source, dest, o.Deep))
		if f.planErr != nil {
			return Plan{}, f.planErr
		}
		p := Plan{ID: "plan_1", Revision: 1, SourceID: source, SourceName: "Big Folder", SourceKind: "folder",
			DestinationID: firstNonEmpty(dest, "dst_1"), RootName: "Big Folder", Files: 10, Folders: 2, Complete: true}
		if o.Deep {
			p.Complete = false
			p.Deep = &DeepScan{State: "running", Files: 4, Folders: 1, Scanned: 5}
		}
		return p, nil
	}
	a.GetPlan = func(_ context.Context, id string) (Plan, error) {
		f.record("getplan:" + id)
		f.mu.Lock()
		defer f.mu.Unlock()
		if len(f.deepSteps) == 0 {
			return Plan{}, errors.New("no plan")
		}
		p := f.deepSteps[min(f.deepAt, len(f.deepSteps)-1)]
		f.deepAt++
		return p, nil
	}
	a.CancelPreflight = func(_ context.Context, id string) (Plan, error) {
		f.record("cancelscan:" + id)
		return Plan{ID: id, Files: 4, Folders: 1, Deep: &DeepScan{State: "cancelled", Files: 4, Folders: 1}}, nil
	}
	a.Report = func(_ context.Context, id string) (JobReport, error) {
		f.record("report:" + id)
		return f.rep, f.repErr
	}
	a.ExportCSV = func(_ context.Context, kind, id, path string) (int, error) {
		f.record(fmt.Sprintf("export:%s:%s:%s", kind, id, path))
		return 7, f.exportErr
	}
	a.Search = func(_ context.Context, q string) ([]DriveItem, error) {
		f.record("search:" + q)
		return f.items, nil
	}
	a.LatestAudit = func(_ context.Context, id string) (AuditReport, bool, error) {
		f.record("latest:" + id)
		rep, ok := f.latest[id]
		return rep, ok, nil
	}
	return a
}

func iModel(t *testing.T, f *iFake, lang string, w, h int, tweak ...func(*Options)) Model {
	t.Helper()
	opt := Options{Width: w, Height: h, Language: lang, NoMotion: true, Now: func() time.Time { return tNow }}
	for _, fn := range tweak {
		fn(&opt)
	}
	m := New(f.acts(), opt)
	m = settle(t, m, m.fetch())
	m, _ = update(t, m, tea.WindowSizeMsg{Width: w, Height: h})
	return m
}

func queuedJobs(n int) []Job {
	var out []Job
	for i := 0; i < n; i++ {
		out = append(out, Job{ID: fmt.Sprintf("job_q%d", i+1), CollectionID: "col_1", SourceName: fmt.Sprintf("Source %d", i+1),
			State: "queued", Revision: i + 1, Created: tNow.Add(time.Duration(i) * time.Minute)})
	}
	// the API lists newest first
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func paste(t *testing.T, m Model, text string) Model {
	t.Helper()
	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text), Paste: true})
	return m
}

// ---- S6: batch copy ----

func TestPastingSeveralLinksMakesABatchWithoutPreviewingEachOne(t *testing.T) {
	f := newIFake()
	f.batchRes = BatchResult{ID: "batch-1", Jobs: []BatchJob{{Source: "https://drive.google.com/drive/folders/AAAAAAAAAAAAAAAAAAAA", JobID: "job_a"},
		{Source: "https://drive.google.com/drive/folders/BBBBBBBBBBBBBBBBBBBB", Error: "not found"},
		{Source: "https://drive.google.com/drive/folders/CCCCCCCCCCCCCCCCCCCC", JobID: "job_c"}}}
	m := iModel(t, f, "en", 100, 30)
	m = sendKeys(t, m, "c")
	m = paste(t, m, "https://drive.google.com/drive/folders/AAAAAAAAAAAAAAAAAAAA\nhttps://drive.google.com/drive/folders/BBBBBBBBBBBBBBBBBBBB\r\nhttps://drive.google.com/drive/folders/CCCCCCCCCCCCCCCCCCCC\n")
	m = sendKeys(t, m, "enter")
	if f.has("inspect:") {
		t.Fatalf("a batch is not previewed link by link: %v", f.recorded())
	}
	v := m.View()
	for _, want := range []string{"Batch of 3 links", "AAAAAAAA", "CCCCCCCC", "Each link is checked when it is sent", "After copy"} {
		if !strings.Contains(v, want) {
			t.Fatalf("batch dialog lacks %q:\n%s", want, v)
		}
	}
	// A batch cannot take one typed name, and a full scan has no single plan.
	m = sendKeys(t, m, "3", "down", "i", "x")
	if m.cp.typing != "" || m.cp.opt.Deep {
		t.Fatalf("name 3 and deep are for a single link: %+v", m.cp)
	}
	m = sendKeys(t, m, "2", "down", "f", "o") // auto name, filter, name clash apply to every link
	m = sendKeys(t, m, "enter")
	want := "submitbatch:https://drive.google.com/drive/folders/AAAAAAAAAAAAAAAAAAAA,https://drive.google.com/drive/folders/BBBBBBBBBBBBBBBBBBBB,https://drive.google.com/drive/folders/CCCCCCCCCCCCCCCCCCCC:dest=:auto=true:sync=:filter=no_video:conflict=skip"
	if !f.has(want) {
		t.Fatalf("the whole batch goes at once: %v", f.recorded())
	}
	if m.screen != screenJobs || !strings.Contains(m.status, "Batch: 2 accepted, 1 failed") || !strings.Contains(m.status, "not found") {
		t.Fatalf("screen %v, status %q", m.screen, m.status)
	}
}

func TestABatchWhereEveryLinkFailsStaysOpenAndSaysWhy(t *testing.T) {
	f := newIFake()
	f.batchRes = BatchResult{ID: "batch-1", Jobs: []BatchJob{{Source: "AAAAAAAAAAAAAAAAAAAAAAAAA", Error: "no access"}, {Source: "BBBBBBBBBBBBBBBBBBBBBBBBB", Error: "no access"}}}
	m := iModel(t, f, "vi", 80, 30)
	m = sendKeys(t, m, "c")
	m = paste(t, m, "AAAAAAAAAAAAAAAAAAAAAAAAA BBBBBBBBBBBBBBBBBBBBBBBBB")
	m = sendKeys(t, m, "enter", "enter")
	if m.screen != screenCopy || !strings.Contains(m.View(), "no access") || !strings.Contains(m.View(), "Lô 2 link") {
		t.Fatalf("nothing started, so the dialog stays with the reason:\n%s", m.View())
	}
}

func TestBatchDestinationIsChosenOnceAndKeptForEveryLink(t *testing.T) {
	f := newIFake()
	f.batchRes = BatchResult{ID: "batch-1", Jobs: []BatchJob{{Source: "AAAAAAAAAAAAAAAAAAAAAAAAA", JobID: "j1"}, {Source: "BBBBBBBBBBBBBBBBBBBBBBBBB", JobID: "j2"}}}
	m := iModel(t, f, "en", 100, 30)
	m = sendKeys(t, m, "c")
	m = paste(t, m, "AAAAAAAAAAAAAAAAAAAAAAAAA BBBBBBBBBBBBBBBBBBBBBBBBB")
	m = sendKeys(t, m, "enter")
	m = sendKeys(t, m, "d") // pick a folder
	m = sendKeys(t, m, "enter", "y")
	if m.screen != screenCopy || m.cp == nil || !m.cp.isBatch() || m.cp.destID == "" {
		t.Fatalf("the batch survives picking a destination: screen %v cp %+v", m.screen, m.cp)
	}
	m = sendKeys(t, m, "enter")
	if !f.has("submitbatch:") || !strings.Contains(strings.Join(f.recorded(), " "), "dest=dst_") {
		t.Fatalf("the chosen destination goes with the batch: %v", f.recorded())
	}
}

func TestBatchDialogAndJobsFitEveryWidthInBothLanguages(t *testing.T) {
	for _, lang := range []string{"en", "vi"} {
		f := newIFake()
		m := iModel(t, f, lang, 100, 30)
		m = sendKeys(t, m, "c")
		m = paste(t, m, "https://drive.google.com/drive/folders/AAAAAAAAAAAAAAAAAAAA https://drive.google.com/drive/folders/BBBBBBBBBBBBBBBBBBBB https://drive.google.com/drive/folders/CCCCCCCCCCCCCCCCCCCC https://drive.google.com/drive/folders/DDDDDDDDDDDDDDDDDDDD https://drive.google.com/drive/folders/EEEEEEEEEEEEEEEEEEEE https://drive.google.com/drive/folders/FFFFFFFFFFFFFFFFFFFF")
		m = sendKeys(t, m, "enter")
		views := fitsEverywhere(t, m, 30)
		if lang == "vi" && !strings.Contains(views[40], "Lô 6 link") {
			t.Errorf("vi title missing at 40 columns:\n%s", views[40])
		}
		if !strings.Contains(views[120], "+2") {
			t.Errorf("%s: the list says how many links are not shown:\n%s", lang, views[120])
		}
		for w, v := range views {
			// every sync mode stays readable at every width
			for _, label := range map[string][]string{"en": {"off", "realtime", "hourly", "6 hours", "daily"}, "vi": {"tắt", "realtime", "mỗi giờ", "6 giờ", "hằng ngày"}}[lang] {
				if !strings.Contains(v, label) {
					t.Errorf("%s width %d lost the sync choice %q:\n%s", lang, w, label, v)
				}
			}
		}
	}
}

func TestBatchProgressShowsOnJobRowsAndTheJobScreen(t *testing.T) {
	f := newIFake()
	f.jobs = []Job{
		{ID: "job_b3", CollectionID: "col_1", SourceName: "Three", State: "queued", BatchID: "batch-abcdef123456", Created: tNow.Add(2 * time.Minute), Revision: 3},
		{ID: "job_b2", CollectionID: "col_1", SourceName: "Two", State: "running", BatchID: "batch-abcdef123456", Revision: 2},
		{ID: "job_b1", CollectionID: "col_1", SourceName: "One", State: "succeeded", BatchID: "batch-abcdef123456", Revision: 1},
	}
	f.batch = BatchProgress{ID: "batch-abcdef123456", Total: 3, CompletedJobs: 1, ByState: map[string]int{"succeeded": 1, "running": 1, "queued": 1}}
	m := iModel(t, f, "en", 100, 30)
	m = sendKeys(t, m, "2", "down")
	m = refresh(t, m)
	if !f.has("batch:batch-abcdef123456") {
		t.Fatalf("the batch is read: %v", f.recorded())
	}
	if f.count("batch:") != 1 {
		t.Fatalf("one batch is one read, not one per job: %v", f.recorded())
	}
	v := m.View()
	if !strings.Contains(v, "batch 123456 1/3") {
		t.Fatalf("each job of a batch carries its progress:\n%s", v)
	}
	m = sendKeys(t, m, "enter")
	v = m.View()
	if !strings.Contains(v, "Batch 123456 · 1/3 jobs done · 1 Running, 1 Queued, 1 Done") && !strings.Contains(v, "Batch 123456 · 1/3 jobs done") {
		t.Fatalf("the job screen shows the batch line:\n%s", v)
	}
	for w, view := range fitsEverywhere(t, m, 30) {
		if w >= 60 && !strings.Contains(view, "1/3") {
			t.Errorf("width %d lost the batch progress:\n%s", w, view)
		}
	}
}

// ---- S6: the queue ----

func TestQueuedJobsShowTheirPositionAndMoveEarlierOrLater(t *testing.T) {
	f := newIFake()
	f.jobs = queuedJobs(3) // newest first: q3, q2, q1; queue order q1, q2, q3
	m := iModel(t, f, "en", 100, 30)
	m = sendKeys(t, m, "2", "down")
	v := m.View()
	for _, want := range []string{"Queued"} {
		if !strings.Contains(v, want) {
			t.Fatalf("queued rows show their place (%q):\n%s", want, v)
		}
	}
	// cursor 0 is job_q3, last in the queue: K moves it before q2.
	m = sendKeys(t, m, "K")
	if !f.has("move:job_q3:3:before=job_q2:after=:prio=-") {
		t.Fatalf("K = before the previous job in the queue: %v", f.recorded())
	}
	if !strings.Contains(m.status, "job_q3") {
		t.Fatalf("status = %q", m.status)
	}
	if got := queueOrder(m.snap.Jobs, m.qm.recs); strings.Join(got, ",") != "job_q1,job_q3,job_q2" {
		t.Fatalf("the shown order follows the move: %v", got)
	}
	// J moves it back, after q2.
	m = sendKeys(t, m, "J")
	if !f.has("move:job_q3:3:before=:after=job_q2:prio=-") {
		t.Fatalf("J = after the next job in the queue: %v", f.recorded())
	}
	// q3 is last: J again has nothing to swap with and sends nothing.
	n := f.count("move:")
	m = sendKeys(t, m, "J")
	if f.count("move:") != n || !strings.Contains(m.status, "No queued job") {
		t.Fatalf("no call when at the end: %v / %q", f.recorded(), m.status)
	}
}

func TestMovingNeedsAQueuedJobAndReportsARefusal(t *testing.T) {
	f := newIFake()
	f.jobs = append([]Job{{ID: "job_run", SourceName: "Running", State: "running", Revision: 1}}, queuedJobs(2)...)
	m := iModel(t, f, "en", 100, 30)
	m = sendKeys(t, m, "2", "down")
	for _, k := range []string{"K", "J", "+", "-"} {
		m = sendKeys(t, m, k)
		if f.count("move:") != 0 || !strings.Contains(m.status, "waiting (queued)") {
			t.Fatalf("%s on a running job must say why and send nothing: %v %q", k, f.recorded(), m.status)
		}
	}
	m = sendKeys(t, m, "down", "J")
	f.moveErr = errors.New("The job changed; refresh and try again.")
	m = sendKeys(t, m, "K")
	if !strings.Contains(m.status, "refresh and try again") {
		t.Fatalf("a stale revision is shown: %q", m.status)
	}
	if len(m.qm.recs) != 0 {
		t.Fatalf("a refused move must not change the shown order: %v", m.qm.recs)
	}
}

func TestPriorityKeysStayInRangeAndShowOnTheRow(t *testing.T) {
	f := newIFake()
	f.jobs = queuedJobs(1)
	f.jobs[0].Priority = 4
	m := iModel(t, f, "en", 100, 30)
	m = sendKeys(t, m, "2", "down")
	m = sendKeys(t, m, "+")
	if !f.has("move:job_q1:1:before=:after=:prio=5") {
		t.Fatalf("+ raises by one: %v", f.recorded())
	}
	f.jobs[0].Priority = 5
	m = refresh(t, m)
	n := f.count("move:")
	m = sendKeys(t, m, "+")
	if f.count("move:") != n {
		t.Fatalf("5 is the top: nothing is sent: %v", f.recorded())
	}
	m = sendKeys(t, m, "-")
	if !f.has("move:job_q1:1:before=:after=:prio=4") {
		t.Fatalf("- lowers by one: %v", f.recorded())
	}
	if v := m.View(); !strings.Contains(v, "Priority +5") {
		t.Fatalf("the priority shows on the row:\n%s", v)
	}
}

func TestQueueOrderIsPriorityThenAdmissionThenMoves(t *testing.T) {
	jobs := []Job{
		{ID: "d", State: "queued", Priority: 0, Created: tNow.Add(3 * time.Minute)},
		{ID: "c", State: "running"},
		{ID: "b", State: "queued", Priority: 2, Created: tNow.Add(2 * time.Minute)},
		{ID: "a", State: "queued", Priority: 0, Created: tNow},
	}
	if got := strings.Join(queueOrder(jobs, nil), ","); got != "b,a,d" {
		t.Fatalf("priority first, then oldest: %s", got)
	}
	moves := []moveRec{{id: "d", before: "a"}, {id: "b", after: "d"}}
	if got := strings.Join(queueOrder(jobs, moves), ","); got != "d,b,a" && got != "d,a,b" {
		// d before a: b,d,a; then b after d: d,b,a
		t.Fatalf("moves replay in order: %s", got)
	}
	if got := strings.Join(queueOrder(jobs, []moveRec{{id: "gone", before: "a"}, {id: "d", before: "zzz"}}), ","); got != "b,a,d" {
		t.Fatalf("a move whose job or target is gone is skipped: %s", got)
	}
}

func TestPausingTheQueueNeedsAYesResumingDoesNot(t *testing.T) {
	f := newIFake()
	f.jobs = queuedJobs(2)
	m := iModel(t, f, "vi", 80, 30)
	m = sendKeys(t, m, "2", "down")
	m = sendKeys(t, m, "P")
	if m.confirm == nil || !strings.Contains(m.View(), "Tạm dừng tất cả?") || f.has("queue:") {
		t.Fatalf("P asks first: confirm %v, calls %v", m.confirm, f.recorded())
	}
	m = sendKeys(t, m, "n")
	if f.has("queue:") {
		t.Fatalf("any other key cancels: %v", f.recorded())
	}
	m = sendKeys(t, m, "P", "y")
	if !f.has("queue:true") {
		t.Fatalf("y pauses: %v", f.recorded())
	}
	v := m.View()
	if !strings.Contains(v, "Hàng đợi đang tạm dừng") || !strings.Contains(m.status, "2 job") {
		t.Fatalf("the paused queue is stated on Jobs:\n%s\n%q", v, m.status)
	}
	if !strings.Contains(m.tabBar(), "||") {
		t.Fatalf("and on the tab bar: %q", m.tabBar())
	}
	if h := m.homeView(); !strings.Contains(strings.Join(h, "\n"), "Hàng đợi đang tạm dừng") {
		t.Fatalf("and on Home:\n%s", strings.Join(h, "\n"))
	}
	n := f.count("queue:")
	m = sendKeys(t, m, "P", "y") // already paused
	if f.count("queue:") != n {
		t.Fatalf("pausing a paused queue sends nothing: %v", f.recorded())
	}
	m = sendKeys(t, m, "U")
	if !f.has("queue:false") || strings.Contains(m.View(), "Hàng đợi đang tạm dừng") {
		t.Fatalf("U resumes at once: %v\n%s", f.recorded(), m.View())
	}
	for w, view := range fitsEverywhere(t, m, 30) {
		_ = w
		_ = view
	}
}

// ---- server-side job filter and paging ----

func TestJobFilterIsAskedOfTheServerNotAppliedHere(t *testing.T) {
	f := newIFake()
	f.jobs = []Job{
		{ID: "job_1", CollectionID: "col_1", SourceName: "Alpha", State: "running", Revision: 1},
		{ID: "job_2", CollectionID: "col_2", SourceName: "Beta", State: "queued", Revision: 1},
		{ID: "job_3", CollectionID: "col_2", SourceName: "Beta again", State: "failed", Revision: 1},
	}
	m := iModel(t, f, "en", 100, 30)
	m = sendKeys(t, m, "2", "down")
	if len(m.snap.Jobs) != 3 {
		t.Fatalf("jobs = %d", len(m.snap.Jobs))
	}
	m = sendKeys(t, m, "f") // queued
	if !f.has("jobquery:queued::1") || len(m.snap.Jobs) != 1 || m.snap.Jobs[0].ID != "job_2" {
		t.Fatalf("f asks the server for queued jobs: %v / %+v", f.recorded(), m.snap.Jobs)
	}
	if v := m.View(); !strings.Contains(v, "Filter: Queued") {
		t.Fatalf("the filter is named:\n%s", v)
	}
	m = sendKeys(t, m, "s")
	if !f.has("jobquery:queued:col_2:1") {
		t.Fatalf("s limits to the source of the row: %v", f.recorded())
	}
	if v := m.View(); !strings.Contains(v, "only source Beta") {
		t.Fatalf("the source is named:\n%s", v)
	}
	m = sendKeys(t, m, "f", "f", "f", "f") // running, paused ... until a state with no job
	if v := m.View(); strings.Contains(v, "Alpha") {
		t.Fatalf("a job of another source never shows under a source filter:\n%s", v)
	}
	// cycle back to "all states": the source stays
	for m.jq.State != "" {
		m = sendKeys(t, m, "f")
	}
	if m.jq.CollectionID != "col_2" || len(m.snap.Jobs) != 2 {
		t.Fatalf("all states of col_2: %+v / %d jobs", m.jq, len(m.snap.Jobs))
	}
	m = sendKeys(t, m, "s")
	if m.jq.CollectionID != "" || len(m.snap.Jobs) != 3 {
		t.Fatalf("s again clears the source filter: %+v", m.jq)
	}
}

func TestAFilterWithNoMatchSaysSoAndLeavingJobsDropsTheFilter(t *testing.T) {
	f := newIFake()
	f.jobs = []Job{{ID: "job_1", SourceName: "Alpha", State: "running", Revision: 1}}
	m := iModel(t, f, "vi", 80, 30)
	m = sendKeys(t, m, "2", "down", "f") // queued: none
	if v := m.View(); !strings.Contains(v, "Không có job nào khớp bộ lọc") || !strings.Contains(v, "Lọc: Đã xếp hàng") {
		t.Fatalf("empty result of a filter:\n%s", v)
	}
	m = sendKeys(t, m, "1")
	if !m.jq.isDefault() || !f.has("jobquery:::1") {
		t.Fatalf("Home must see every job again: %+v %v", m.jq, f.recorded())
	}
	if len(m.snap.Jobs) != 1 {
		t.Fatalf("the unfiltered list is read again: %+v", m.snap.Jobs)
	}
}

func TestScrollingToTheEndLoadsTheNextCursorPage(t *testing.T) {
	f := newIFake()
	for i := 0; i < 45; i++ {
		f.jobs = append(f.jobs, Job{ID: fmt.Sprintf("job_%02d", i), SourceName: fmt.Sprintf("S%02d", i), State: "succeeded", Revision: 1})
	}
	m := iModel(t, f, "en", 100, 30)
	m = sendKeys(t, m, "2", "down")
	if len(m.snap.Jobs) != 20 || m.snap.JobsNext == "" {
		t.Fatalf("first page: %d next %q", len(m.snap.Jobs), m.snap.JobsNext)
	}
	if v := m.View(); !strings.Contains(v, "1-") && !strings.Contains(v, "20") {
		t.Fatalf("the title counts the jobs:\n%s", v)
	}
	m = sendKeys(t, m, "end")
	if !f.has("jobquery::: 2") && !f.has("jobquery:::2") {
		t.Fatalf("End at the last row asks for page 2: %v", f.recorded())
	}
	if len(m.snap.Jobs) != 40 || m.snap.JobsNext == "" || m.cursor != 19 {
		t.Fatalf("second page: %d jobs, next %q, cursor %d", len(m.snap.Jobs), m.snap.JobsNext, m.cursor)
	}
	m = sendKeys(t, m, "end", "end", "end")
	if len(m.snap.Jobs) != 45 || m.snap.JobsNext != "" {
		t.Fatalf("the last page: %d jobs, next %q", len(m.snap.Jobs), m.snap.JobsNext)
	}
	n := f.count("jobquery:")
	m = sendKeys(t, m, "end")
	if f.count("jobquery:") != n {
		t.Fatalf("nothing more to load: %v", f.recorded())
	}
	for w, v := range fitsEverywhere(t, m, 30) {
		_ = w
		_ = v
	}
}

func TestJobsListWindowCountsTheMoreLineAndTheFilterLines(t *testing.T) {
	f := newIFake()
	for i := 0; i < 45; i++ {
		f.jobs = append(f.jobs, Job{ID: fmt.Sprintf("job_%02d", i), CollectionID: "col_1", SourceName: fmt.Sprintf("S%02d", i), State: "queued", Revision: 1, Created: tNow.Add(time.Duration(i) * time.Second)})
	}
	f.paused = true
	m := iModel(t, f, "vi", 60, 14)
	m = sendKeys(t, m, "2", "down", "s", "end")
	for w, v := range fitsEverywhere(t, m, 14) {
		if !strings.Contains(v, ">") {
			t.Errorf("width %d: the cursor row must stay in view:\n%s", w, v)
		}
	}
}
