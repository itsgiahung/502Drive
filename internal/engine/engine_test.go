package engine

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/GiaHung07/502Drive/internal/drive"
	"github.com/GiaHung07/502Drive/internal/drive/drivetest"
	"github.com/GiaHung07/502Drive/internal/state"
)

type env struct {
	t     *testing.T
	store *state.Store
	fake  *drivetest.Fake
	dest  drive.File
	n     int
}

func newEnv(t *testing.T) *env {
	t.Helper()
	s, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	f := drivetest.New()
	return &env{t: t, store: s, fake: f, dest: f.Folder("Archive", "root")}
}

func (e *env) engine() *Engine {
	return &Engine{Store: e.store, Provider: func(context.Context) (drive.Provider, error) { return e.fake, nil },
		AfterRun: func(id string) { e.store.RunFinished(context.Background(), id) }, Workers: 3, BackoffBase: time.Millisecond, BackoffMax: 5 * time.Millisecond, Settle: time.Millisecond, Poll: 5 * time.Millisecond}
}

func (e *env) submit(src drive.File) state.Job {
	e.t.Helper()
	e.n++
	kind := state.KindFile
	if src.IsFolder() {
		kind = state.KindFolder
	}
	p := state.Plan{ID: fmt.Sprintf("plan%d", e.n), Principal: "owner", SourceID: src.ID, SourceName: src.Name, SourceKind: kind, SourceMime: src.MimeType, SourceSize: src.Size, SourceVersion: src.Fingerprint(),
		DestinationID: "dest", DestinationFolder: e.dest.ID, RootName: src.Name, ExpiresAt: time.Now().Add(time.Hour), Revision: 1}
	if err := e.store.PutPlan(context.Background(), p); err != nil {
		e.t.Fatal(err)
	}
	r, err := e.store.Admit(context.Background(), "owner", fmt.Sprintf("key%d", e.n), p.ID, 1)
	if err != nil {
		e.t.Fatal(err)
	}
	return r.Job
}

// runUntil starts an engine ("a process") and stops it once pred holds.
func (e *env) runUntil(id string, pred func(state.Job) bool) state.Job {
	e.t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	eng := e.engine()
	go func() { eng.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		j, err := e.store.GetJob(context.Background(), "owner", id)
		if err != nil {
			e.t.Fatal(err)
		}
		if pred(j) {
			return j
		}
		time.Sleep(5 * time.Millisecond)
	}
	j, _ := e.store.GetJob(context.Background(), "owner", id)
	e.t.Fatalf("timeout; job = %+v", j)
	return j
}

func terminal(j state.Job) bool { return state.IsTerminal(j.State) || j.State == state.JobBlocked }

// tree renders a folder as sorted "path|size" lines for comparison.
func tree(f *drivetest.Fake, id, prefix string) []string {
	var out []string
	for _, c := range f.Children(id) {
		p := prefix + "/" + c.Name
		switch {
		case c.IsFolder():
			out = append(out, p+"/")
			out = append(out, tree(f, c.ID, p)...)
		case c.IsShortcut():
			out = append(out, p+" ->shortcut")
		default:
			sz := int64(-1)
			if c.Size != nil {
				sz = *c.Size
			}
			out = append(out, fmt.Sprintf("%s|%d|%s", p, sz, c.MimeType))
		}
	}
	sort.Strings(out)
	return out
}

func sample(f *drivetest.Fake) drive.File {
	root := f.Folder("Project Alpha", "root")
	f.File("a.bin", root.ID, 10)
	f.File("dup", root.ID, 1)
	f.File("dup", root.ID, 2) // same name, different source IDs
	f.Put("Notes", "application/vnd.google-apps.document", root.ID, -1)
	sub := f.Folder("sub", root.ID)
	f.Folder("empty", sub.ID)
	deep := f.Folder("deep", sub.ID)
	f.File("we/ird\nname", deep.ID, 5)
	f.Put("link", drive.MimeShortcut, root.ID, -1)
	f.Mutate(f.Children(root.ID)[len(f.Children(root.ID))-1].ID, func(x *drive.File) { x.ShortcutTargetID = "elsewhere12345" })
	return root
}

func TestCopyPreservesTreeNamesAndEmptyFolders(t *testing.T) {
	e := newEnv(t)
	e.fake.PageSize = 2 // force several list pages
	src := sample(e.fake)
	j := e.runUntil(e.submit(src).ID, terminal)
	if j.State != state.JobSucceeded {
		t.Fatalf("state %s err %+v counts %+v", j.State, j.LastError, j.Counts)
	}
	copies := e.fake.Children(e.dest.ID)
	if len(copies) != 1 || copies[0].Name != "Project Alpha" || copies[0].ID == src.ID {
		t.Fatalf("destination: %+v", copies)
	}
	want, got := tree(e.fake, src.ID, ""), tree(e.fake, copies[0].ID, "")
	if fmt.Sprint(want) != fmt.Sprint(got) {
		t.Fatalf("tree differs\nwant %q\ngot  %q", want, got)
	}
	if j.ResultURL == nil || *j.ResultURL != RootURL(copies[0].ID) {
		t.Fatalf("result url %v", j.ResultURL)
	}
	if j.Counts.Total == nil || j.Counts.Completed != *j.Counts.Total || j.Counts.Observed != 10 {
		t.Fatalf("counts %+v", j.Counts)
	}
	if e.fake.Calls["copy"] != 5 {
		t.Fatalf("copy calls = %d (folders must not use files.copy)", e.fake.Calls["copy"])
	}
	c, _ := e.store.GetCollection(context.Background(), "owner", j.CollectionID)
	if c.RootID == nil || *c.RootID != copies[0].ID {
		t.Fatalf("collection root %+v", c)
	}
}

func TestSingleFileIsWrapped(t *testing.T) {
	e := newEnv(t)
	f := e.fake.File("report.pdf", "root", 99)
	j := e.runUntil(e.submit(f).ID, terminal)
	if j.State != state.JobSucceeded {
		t.Fatalf("%+v", j)
	}
	w := e.fake.Children(e.dest.ID)
	if len(w) != 1 || !w[0].IsFolder() || w[0].Name != "report.pdf" {
		t.Fatalf("wrapper %+v", w)
	}
	in := e.fake.Children(w[0].ID)
	if len(in) != 1 || in[0].Name != "report.pdf" {
		t.Fatalf("inside %+v", in)
	}
}

func TestLostResponseIsReconciledNotDuplicated(t *testing.T) {
	e := newEnv(t)
	src := sample(e.fake)
	var mu sync.Mutex
	lost := 0
	e.fake.LoseResponse = func(op string, f drive.File) bool {
		mu.Lock()
		defer mu.Unlock()
		if lost < 4 { // a mix of folder and file writes
			lost++
			return true
		}
		return false
	}
	j := e.runUntil(e.submit(src).ID, terminal)
	if j.State != state.JobSucceeded {
		t.Fatalf("state %s %+v", j.State, j.LastError)
	}
	if lost != 4 {
		t.Fatalf("fault not exercised: %d", lost)
	}
	copies := e.fake.Children(e.dest.ID)
	if len(copies) != 1 || fmt.Sprint(tree(e.fake, src.ID, "")) != fmt.Sprint(tree(e.fake, copies[0].ID, "")) {
		t.Fatal("duplicates or missing output after lost responses")
	}
	if e.fake.Writes != 10 { // 10 objects, each written exactly once
		t.Fatalf("writes = %d", e.fake.Writes)
	}
}

func TestCrashBetweenWriteAndRecordAdoptsOutputOnRestart(t *testing.T) {
	e := newEnv(t)
	src := sample(e.fake)
	job := e.submit(src)
	ctx, cancel := context.WithCancel(context.Background())
	crashed := false
	e.fake.LoseResponse = func(op string, f drive.File) bool {
		if op == "copy" && !crashed {
			crashed = true
			cancel() // the process dies right after Google applied the copy
			return true
		}
		return false
	}
	eng := e.engine()
	done := make(chan struct{})
	go func() { eng.Run(ctx); close(done) }()
	<-done
	if !crashed {
		t.Fatal("crash not triggered")
	}
	j := e.runUntil(job.ID, terminal) // "restart"
	if j.State != state.JobSucceeded {
		t.Fatalf("state %s %+v", j.State, j.LastError)
	}
	copies := e.fake.Children(e.dest.ID)
	if fmt.Sprint(tree(e.fake, src.ID, "")) != fmt.Sprint(tree(e.fake, copies[0].ID, "")) {
		t.Fatal("restart duplicated or lost output")
	}
}

func TestUnknownOutcomeBlocksThenExplicitRetryResends(t *testing.T) {
	e := newEnv(t)
	root := e.fake.Folder("Src", "root")
	e.fake.File("only", root.ID, 3)
	failing := true
	e.fake.Before = func(op string, f drive.File) error {
		if op == "copy" && failing {
			return &drive.Error{Kind: drive.KindUnavailable} // no effect, outcome unknown to the caller
		}
		return nil
	}
	job := e.submit(root)
	j := e.runUntil(job.ID, terminal)
	if j.State != state.JobBlocked || j.LastError == nil || j.LastError.Code != "unknown_remote_outcome" {
		t.Fatalf("expected blocked, got %s %+v", j.State, j.LastError)
	}
	if e.fake.Calls["copy"] != 1 {
		t.Fatalf("blind resend: %d copy calls", e.fake.Calls["copy"])
	}
	if _, err := e.store.Action(context.Background(), "owner", "r1", job.ID, "resume", j.Revision); err == nil {
		t.Fatal("resume must not skip blocked items")
	}
	failing = false
	r, err := e.store.Action(context.Background(), "owner", "r2", job.ID, "retry", j.Revision)
	if err != nil {
		t.Fatal(err)
	}
	j = e.runUntil(r.Job.ID, func(j state.Job) bool { return j.State == state.JobSucceeded })
	if n := len(e.fake.Children(e.fake.Children(e.dest.ID)[0].ID)); n != 1 {
		t.Fatalf("children %d", n)
	}
}

func TestThrottlingIsRetriedWithoutLosingItems(t *testing.T) {
	e := newEnv(t)
	src := sample(e.fake)
	var mu sync.Mutex
	n := 0
	e.fake.Before = func(op string, f drive.File) error {
		mu.Lock()
		defer mu.Unlock()
		if (op == "copy" || op == "folder") && n < 5 {
			n++
			return &drive.Error{Kind: drive.KindThrottled, Status: 429, RetryAfter: time.Millisecond}
		}
		return nil
	}
	j := e.runUntil(e.submit(src).ID, terminal)
	if j.State != state.JobSucceeded {
		t.Fatalf("%s %+v", j.State, j.LastError)
	}
}

func TestPermissionDeniedGivesPartialAndSkipsSubtree(t *testing.T) {
	e := newEnv(t)
	src := sample(e.fake)
	e.fake.Before = func(op string, f drive.File) error {
		if op == "folder" && f.Name == "sub" {
			return &drive.Error{Kind: drive.KindPermission, Status: 403}
		}
		return nil
	}
	j := e.runUntil(e.submit(src).ID, terminal)
	if j.State != state.JobPartial || j.Counts.Failed != 1 || j.Counts.Skipped != 3 {
		t.Fatalf("%s counts %+v err %+v", j.State, j.Counts, j.LastError)
	}
}

func TestListingFailureIsNotAnEmptyFolder(t *testing.T) {
	e := newEnv(t)
	src := sample(e.fake)
	sub := ""
	for _, c := range e.fake.Children(src.ID) {
		if c.Name == "sub" {
			sub = c.ID
		}
	}
	e.fake.Before = func(op string, f drive.File) error {
		if op == "list" && f.ID == sub {
			return &drive.Error{Kind: drive.KindPermission, Status: 403}
		}
		return nil
	}
	j := e.runUntil(e.submit(src).ID, terminal)
	if j.State != state.JobPartial {
		t.Fatalf("state %s", j.State)
	}
}

func TestListingResumesFromCheckpointAfterRestart(t *testing.T) {
	e := newEnv(t)
	e.fake.PageSize = 2
	src := e.fake.Folder("Big", "root")
	for i := range 9 {
		e.fake.File(fmt.Sprintf("f%d", i), src.ID, 1)
	}
	job := e.submit(src)
	ctx, cancel := context.WithCancel(context.Background())
	lists := 0
	e.fake.Before = func(op string, f drive.File) error {
		if op == "list" {
			lists++
			if lists == 3 {
				cancel() // crash mid-listing
				return &drive.Error{Kind: drive.KindUnavailable}
			}
		}
		return nil
	}
	eng := e.engine()
	done := make(chan struct{})
	go func() { eng.Run(ctx); close(done) }()
	<-done
	j := e.runUntil(job.ID, terminal)
	if j.State != state.JobSucceeded || j.Counts.Observed != 10 {
		t.Fatalf("%s %+v", j.State, j.Counts)
	}
	if got := len(e.fake.Children(e.fake.Children(e.dest.ID)[0].ID)); got != 9 {
		t.Fatalf("copied %d", got)
	}
}

func TestPauseCancelAndRestartKeepDesiredState(t *testing.T) {
	e := newEnv(t)
	src := e.fake.Folder("Many", "root")
	for i := range 20 {
		e.fake.File(fmt.Sprintf("f%02d", i), src.ID, 1)
	}
	job := e.submit(src)
	var once sync.Once
	var paused state.Job
	e.fake.Before = func(op string, f drive.File) error {
		if op == "copy" {
			once.Do(func() {
				cur, _ := e.store.GetJob(context.Background(), "owner", job.ID)
				r, err := e.store.Action(context.Background(), "owner", "p", job.ID, "pause", cur.Revision)
				if err != nil {
					t.Error(err)
				}
				paused = r.Job
			})
		}
		return nil
	}
	j := e.runUntil(job.ID, func(j state.Job) bool { return j.State == state.JobPaused })
	if paused.State != state.JobPausing || j.DesiredState != "pause" {
		t.Fatalf("paused=%+v j=%+v", paused, j)
	}
	// restarting the process must not resume a paused job
	time.Sleep(50 * time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	eng := e.engine()
	done := make(chan struct{})
	go func() { eng.Run(ctx); close(done) }()
	time.Sleep(100 * time.Millisecond)
	cancel()
	<-done
	j, _ = e.store.GetJob(context.Background(), "owner", job.ID)
	if j.State != state.JobPaused {
		t.Fatalf("restart resumed paused job: %s", j.State)
	}
	partial := len(e.fake.Children(e.fake.Children(e.dest.ID)[0].ID))
	r, err := e.store.Action(context.Background(), "owner", "c", job.ID, "cancel", j.Revision)
	if err != nil || r.Job.State != state.JobCancelled {
		t.Fatalf("cancel %+v %v", r.Job, err)
	}
	if after := len(e.fake.Children(e.fake.Children(e.dest.ID)[0].ID)); after != partial {
		t.Fatal("cancel changed output")
	}
}

func TestAuthFailureBlocksJobAndResumeContinues(t *testing.T) {
	e := newEnv(t)
	src := sample(e.fake)
	connected := false
	eng := e.engine()
	eng.Provider = func(context.Context) (drive.Provider, error) {
		if !connected {
			return nil, drive.ErrReconnectRequired
		}
		return e.fake, nil
	}
	job := e.submit(src)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { eng.Run(ctx); close(done) }()
	var j state.Job
	for range 400 {
		j, _ = e.store.GetJob(context.Background(), "owner", job.ID)
		if j.State == state.JobBlocked {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	<-done
	if j.State != state.JobBlocked || j.LastError.Code != "account_reconnect_required" {
		t.Fatalf("%+v", j)
	}
	connected = true
	if _, err := e.store.Action(context.Background(), "owner", "res", job.ID, "resume", j.Revision); err != nil {
		t.Fatal(err)
	}
	j = e.runUntil(job.ID, terminal)
	if j.State != state.JobSucceeded {
		t.Fatalf("%s %+v", j.State, j.LastError)
	}
}
