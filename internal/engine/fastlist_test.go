package engine

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/itsgiahung/502Drive/internal/drive"
	"github.com/itsgiahung/502Drive/internal/state"
)

// noBatch hides the optional BatchLister interface, reproducing the discovery
// path before F1 (one files.list per folder).
type noBatch struct{ drive.Provider }

// engineWith returns a normal engine whose provider is fixed for the test.
func (e *env) engineWith(p drive.Provider) *Engine {
	eng := e.engine()
	eng.Provider = func(context.Context) (drive.Provider, error) { return p, nil }
	return eng
}

// manyFolders builds a flat tree: one source root and n folders with one file
// each, so discovery has to list n+1 folders.
func manyFolders(e *env, n int) drive.File {
	src := e.fake.Folder("Many", "root")
	for i := range n {
		dir := e.fake.Folder(fmt.Sprintf("d%03d", i), src.ID)
		e.fake.File("f.bin", dir.ID, int64(i+1))
	}
	return src
}

func listCalls(e *env) int {
	return e.fake.CallCount("list") + e.fake.CallCount("list_batch")
}

func TestBatchListCutsFileListCalls(t *testing.T) {
	const folders = 45
	before := newEnv(t)
	beforeSrc := manyFolders(before, folders)
	j := before.runEngine(before.engineWith(noBatch{before.fake}), before.submit(beforeSrc).ID)
	if j.State != state.JobSucceeded {
		t.Fatalf("before: state=%s err=%+v", j.State, j.LastError)
	}
	beforeCalls := listCalls(before)

	after := newEnv(t)
	afterSrc := manyFolders(after, folders)
	j = after.runEngine(after.engine(), after.submit(afterSrc).ID)
	if j.State != state.JobSucceeded {
		t.Fatalf("after: state=%s err=%+v", j.State, j.LastError)
	}
	afterCalls := listCalls(after)

	if beforeCalls < folders {
		t.Fatalf("the baseline path did not list every folder: %d calls", beforeCalls)
	}
	if afterCalls*5 > beforeCalls {
		t.Fatalf("fast-list did not cut calls 5x: before=%d after=%d", beforeCalls, afterCalls)
	}
	// The destination tree is identical: 45 folders inside the copied root.
	root := mustColl(after, j.CollectionID).RootID
	if root == nil || len(after.fake.Children(*root)) != folders {
		t.Fatalf("destination folders = %d, want %d", len(after.fake.Children(deref(root))), folders)
	}
	for i := range folders {
		name := fmt.Sprintf("d%03d", i)
		if len(find(after.fake, *root, name)) != 1 {
			t.Fatalf("folder %s missing or duplicated", name)
		}
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// Batch listing must not change creation order: a child folder is copied only
// after its parent exists at the destination.
func TestBatchListKeepsParentBeforeChild(t *testing.T) {
	e := newEnv(t)
	src := e.fake.Folder("Order", "root")
	var mu sync.Mutex
	var order []string
	e.fake.Before = func(op string, f drive.File) error {
		if op == "folder" {
			mu.Lock()
			order = append(order, f.Name)
			mu.Unlock()
		}
		return nil
	}
	// Enough siblings to force combined queries at the second level too.
	for i := range 10 {
		parent := e.fake.Folder(fmt.Sprintf("p%02d", i), src.ID)
		e.fake.Folder(fmt.Sprintf("p%02d-child", i), parent.ID)
		e.fake.File("leaf.bin", parent.ID, 1)
	}
	j := e.runEngine(e.engine(), e.submit(src).ID)
	if j.State != state.JobSucceeded {
		t.Fatalf("state=%s err=%+v", j.State, j.LastError)
	}
	index := map[string]int{}
	for i, name := range order {
		index[name] = i
	}
	for i := range 10 {
		parent, child := fmt.Sprintf("p%02d", i), fmt.Sprintf("p%02d-child", i)
		pi, pok := index[parent]
		ci, cok := index[child]
		if !pok || !cok || pi >= ci {
			t.Fatalf("%s must be created before %s (order %v)", parent, child, order)
		}
	}
}

// A combined query that the provider rejects falls back to per-folder listing,
// so one bad folder cannot hide the rest.
func TestBatchListFallsBackWhenTheQueryFails(t *testing.T) {
	e := newEnv(t)
	src := e.fake.Folder("Fallback", "root")
	for i := range 5 {
		dir := e.fake.Folder(fmt.Sprintf("d%d", i), src.ID)
		e.fake.File("f.bin", dir.ID, 1)
	}
	e.fake.Before = func(op string, _ drive.File) error {
		if op == "list_batch" {
			return &drive.Error{Kind: drive.KindInvalid, Status: 400, Reason: "invalid_query"}
		}
		return nil
	}
	j := e.runEngine(e.engine(), e.submit(src).ID)
	if j.State != state.JobSucceeded {
		t.Fatalf("state=%s err=%+v", j.State, j.LastError)
	}
	if e.fake.CallCount("list") < 6 { // root + five folders
		t.Fatalf("fallback listed only %d folders", e.fake.CallCount("list"))
	}
	if e.fake.CallCount("list_batch") == 0 {
		t.Fatal("the batched query was never tried")
	}
}

// A sync run over an unchanged tree re-lists it in batches, writes nothing and
// does not scan the same folder twice.
func TestUnchangedSyncDoesNotOverScan(t *testing.T) {
	e := newEnv(t)
	src := e.fake.Folder("Stable", "root")
	for i := range 30 {
		dir := e.fake.Folder(fmt.Sprintf("d%02d", i), src.ID)
		e.fake.File("f.bin", dir.ID, 1)
	}
	base, _ := e.baseline(src)
	before := listCalls(e)
	writes := e.fake.Writes

	j := e.runEngine(e.engine(), e.runNow(base.CollectionID).ID)
	if j.State != state.JobSucceeded {
		t.Fatalf("state=%s err=%+v", j.State, j.LastError)
	}
	if got := listCalls(e) - before; got > 3 {
		t.Fatalf("unchanged tree took %d list calls, want at most 3", got)
	}
	if e.fake.Writes != writes {
		t.Fatalf("unchanged sync wrote %d objects", e.fake.Writes-writes)
	}
}

// TestFastListTenThousand measures the 10k file / 1k folder tree. It runs in a
// dedicated process because peak RSS is process-wide and monotonic:
//
//	FASTLIST_MODE=before go test -run TestFastListTenThousand -timeout 15m ./internal/engine
//	FASTLIST_MODE=after  go test -run TestFastListTenThousand -timeout 15m ./internal/engine
func TestFastListTenThousand(t *testing.T) {
	mode := os.Getenv("FASTLIST_MODE")
	if mode != "before" && mode != "after" {
		t.Skip("measurement probe: set FASTLIST_MODE=before|after")
	}
	if testing.Short() {
		t.Skip("load test")
	}
	const folders, perFolder = 1000, 10
	e := newEnv(t)
	e.fake.PageSize = 1000
	src := e.fake.Folder("Big", "root")
	for f := range folders {
		dir := e.fake.Folder(fmt.Sprintf("d%03d", f), src.ID)
		for i := range perFolder {
			e.fake.File(fmt.Sprintf("f%02d.bin", i), dir.ID, int64(i+1))
		}
	}
	job := e.submit(src)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	eng := e.engine()
	if mode == "before" {
		eng = e.engineWith(noBatch{e.fake})
	}
	go func() { eng.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()

	start := time.Now()
	var discovery time.Duration
	deadline := time.Now().Add(10 * time.Minute)
	for time.Now().Before(deadline) {
		j, err := e.store.GetJob(context.Background(), "owner", job.ID)
		if err != nil {
			t.Fatal(err)
		}
		if j.Phase != nil && *j.Phase == "copying" {
			discovery = time.Since(start)
			break
		}
		if terminal(j) {
			discovery = time.Since(start)
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if discovery == 0 {
		t.Fatal("discovery did not finish within 10m")
	}
	calls := listCalls(e)
	var j state.Job
	for deadline := time.Now().Add(10 * time.Minute); time.Now().Before(deadline); {
		var err error
		j, err = e.store.GetJob(context.Background(), "owner", job.ID)
		if err != nil {
			t.Fatal(err)
		}
		if terminal(j) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if j.State != state.JobSucceeded {
		t.Fatalf("state=%s err=%+v", j.State, j.LastError)
	}
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	t.Logf("mode=%s folders=%d files=%d list_calls=%d discovery=%s heap=%d MiB peak_rss=%s",
		mode, folders, folders*perFolder, calls, discovery.Round(time.Millisecond),
		ms.HeapAlloc/(1<<20), peakRSS())
}

// peakRSS reads VmHWM on Linux; elsewhere it reports "n/a".
func peakRSS() string {
	b, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return "n/a"
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "VmHWM:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "VmHWM:"))
		}
	}
	return "n/a"
}
