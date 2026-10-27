package cli

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/itsgiahung/502Drive/internal/tui"
)

type seen struct {
	mu    sync.Mutex
	calls []string
	body  map[string]map[string]any
}

func (s *seen) record(r *http.Request, body map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.body == nil {
		s.body = map[string]map[string]any{}
	}
	key := r.Method + " " + r.URL.Path
	s.calls = append(s.calls, key+"?"+r.URL.RawQuery)
	s.body[key] = body
}

func (s *seen) has(prefix string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.calls {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}

func TestTUIJobsSendTheServerSideFilterAndReadCursorPages(t *testing.T) {
	var s seen
	a := driveApp(t, func(w http.ResponseWriter, r *http.Request) {
		s.record(r, nil)
		q := r.URL.Query()
		switch q.Get("cursor") {
		case "":
			reply(w, 200, map[string]any{"items": []any{
				map[string]any{"id": "job_1", "state": "queued", "priority": 2, "batch_id": "batch-abc", "created_at": "2026-10-27T10:00:00Z", "counts": map[string]any{}},
			}, "next_cursor": "c2"})
		case "c2":
			reply(w, 200, map[string]any{"items": []any{
				map[string]any{"id": "job_2", "state": "queued", "counts": map[string]any{}},
			}, "next_cursor": "c3"})
		}
	})
	a.tuiSetJobQuery(tui.JobQuery{State: "queued", CollectionID: "col_9", Pages: 2})
	var snap tui.Snapshot
	a.tuiJobs(t.Context(), &snap)
	if len(snap.Jobs) != 2 || snap.Jobs[0].BatchID != "batch-abc" || snap.Jobs[0].Priority != 2 || snap.Jobs[0].Created.IsZero() {
		t.Fatalf("jobs = %+v", snap.Jobs)
	}
	if snap.JobsNext != "c3" {
		t.Fatalf("the cursor after the last page read must reach the TUI: %q", snap.JobsNext)
	}
	if !s.has("GET /v1/jobs?collection_id=col_9&limit=20&state=queued") || !s.has("GET /v1/jobs?collection_id=col_9&cursor=c2&limit=20&state=queued") {
		t.Fatalf("the filter and the cursor go to the server, not a client filter: %v", s.calls)
	}
	// The default query is one plain page.
	s.calls = nil
	a.tuiSetJobQuery(tui.JobQuery{})
	snap = tui.Snapshot{}
	a.tuiJobs(t.Context(), &snap)
	if len(s.calls) != 1 || strings.Contains(s.calls[0], "state=") || strings.Contains(s.calls[0], "collection_id") {
		t.Fatalf("calls = %v", s.calls)
	}
}

func TestTUISnapshotReadsQueuePausedAndAfterCopyPreference(t *testing.T) {
	a := driveApp(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/status":
			reply(w, 200, map[string]any{"version": "1", "ready": true, "account": map[string]any{"status": "connected"}, "queue": map[string]any{"paused": true}})
		case "/v1/preferences":
			reply(w, 200, map[string]any{"language": "vi", "timezone": "UTC", "after_copy_sync_mode": "six_hourly"})
		default:
			reply(w, 200, map[string]any{"items": []any{}})
		}
	})
	a.io.Getenv = func(k string) string {
		if k == "XDG_CONFIG_HOME" {
			return t.TempDir()
		}
		return ""
	}
	snap, err := a.tuiSnapshot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !snap.Status.QueuePaused || snap.Prefs.AfterCopySync != "six_hourly" {
		t.Fatalf("status %+v prefs %+v", snap.Status, snap.Prefs)
	}
}

func TestTUISetPreferenceAcceptsOnlyTheFiveAfterCopyModes(t *testing.T) {
	var s seen
	a := driveApp(t, func(w http.ResponseWriter, r *http.Request) {
		s.record(r, readBody(t, r))
		reply(w, 200, map[string]any{})
	})
	for _, mode := range []string{"off", "realtime", "hourly", "six_hourly", "daily"} {
		if err := a.tuiSetPreference(t.Context(), "after_copy_sync_mode", mode); err != nil {
			t.Fatalf("%s: %v", mode, err)
		}
		if got := s.body["PATCH /v1/preferences"]["after_copy_sync_mode"]; got != mode {
			t.Fatalf("%s sent as %v", mode, got)
		}
	}
	for _, bad := range []any{"", "weekly", 3, nil} {
		if err := a.tuiSetPreference(t.Context(), "after_copy_sync_mode", bad); err == nil {
			t.Fatalf("%v must be refused before it reaches the daemon", bad)
		}
	}
}

func TestTUISubmitOptsSendsTheSyncModeAndTheExistingCopyAnswer(t *testing.T) {
	var s seen
	a := driveApp(t, func(w http.ResponseWriter, r *http.Request) {
		body := readBody(t, r)
		s.record(r, body)
		if body["mode"] == nil && body["plan_id"] == "plan_dup" {
			replyError(w, 409, "already_copied", "This source is already copied.")
			return
		}
		reply(w, 202, map[string]any{"id": "job_1", "collection_id": "col_1", "counts": map[string]any{}})
	})
	id, col, err := a.tuiSubmitOpts(t.Context(), tui.Plan{ID: "plan_1", Revision: 3}, tui.SubmitOptions{AfterSync: "daily"})
	if err != nil || id != "job_1" || col != "col_1" {
		t.Fatalf("%q %q %v", id, col, err)
	}
	b := s.body["POST /v1/jobs"]
	if b["after_copy_sync_mode"] != "daily" || b["plan_id"] != "plan_1" || b["expected_revision"] != float64(3) {
		t.Fatalf("body = %v", b)
	}
	if _, _, err = a.tuiSubmitOpts(t.Context(), tui.Plan{ID: "plan_1"}, tui.SubmitOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, has := s.body["POST /v1/jobs"]["after_copy_sync_mode"]; has {
		t.Fatalf("an untouched choice sends nothing: %v", s.body["POST /v1/jobs"])
	}
	_, _, err = a.tuiSubmitOpts(t.Context(), tui.Plan{ID: "plan_dup"}, tui.SubmitOptions{})
	var ex *tui.ExistingCopyError
	if !asErr(err, &ex) {
		t.Fatalf("already_copied must become the dialog's question, got %v", err)
	}
	if _, _, err = a.tuiSubmitOpts(t.Context(), tui.Plan{ID: "plan_1"}, tui.SubmitOptions{AfterSync: "weekly"}); err == nil {
		t.Fatal("an unknown sync mode is refused locally")
	}
}

func asErr[T error](err error, target *T) bool {
	for err != nil {
		if e, ok := err.(T); ok {
			*target = e
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

func TestTUISubmitBatchAdmitsEachLinkUnderOneBatchAndReportsTheRefusedOne(t *testing.T) {
	var mu sync.Mutex
	var jobBodies []map[string]any
	var jobKeys []string
	a := driveApp(t, func(w http.ResponseWriter, r *http.Request) {
		body := readBody(t, r)
		switch r.URL.Path {
		case "/v1/plans":
			if body["source"] == "BAD" {
				replyError(w, 404, "not_found", "That item was not found.")
				return
			}
			reply(w, 200, map[string]any{"id": "plan_" + body["source"].(string), "revision": 1, "source_id": "S", "source_name": "Src " + body["source"].(string),
				"source_kind": "folder", "destination_id": "dst_1", "root_name": "x", "preview": map[string]any{}})
		case "/v1/jobs":
			mu.Lock()
			jobBodies = append(jobBodies, body)
			jobKeys = append(jobKeys, r.Header.Get("Idempotency-Key"))
			mu.Unlock()
			reply(w, 202, map[string]any{"id": "job_" + body["plan_id"].(string), "counts": map[string]any{}})
		default:
			if strings.HasSuffix(r.URL.Path, "/seal") {
				replyError(w, 404, "not_found", "older server")
				return
			}
			t.Errorf("unexpected %s", r.URL.Path)
		}
	})
	res, err := a.tuiSubmitBatch(t.Context(), tui.BatchRequest{Sources: []string{"A", "BAD", "C"}, AutoName: false,
		Options: tui.CopyOptions{FilterPreset: "no_video"}, Submit: tui.SubmitOptions{AfterSync: "hourly"}})
	if err != nil {
		t.Fatal(err)
	}
	accepted, failed := res.Counts()
	if accepted != 2 || failed != 1 || res.ID == "" {
		t.Fatalf("result = %+v", res)
	}
	if res.Jobs[1].Source != "BAD" || res.Jobs[1].Error == "" || res.Jobs[0].JobID != "job_plan_A" {
		t.Fatalf("jobs = %+v", res.Jobs)
	}
	if len(jobBodies) != 2 || jobBodies[0]["batch_id"] != res.ID || jobBodies[1]["batch_id"] != res.ID || jobBodies[0]["after_copy_sync_mode"] != "hourly" {
		t.Fatalf("every job carries the batch id and the sync mode: %v", jobBodies)
	}
	if jobKeys[0] == jobKeys[1] || jobKeys[0] == "" {
		t.Fatalf("each link has its own idempotency key: %v", jobKeys)
	}
	// A second submission is a new batch, not a replay of the first.
	res2, _ := a.tuiSubmitBatch(t.Context(), tui.BatchRequest{Sources: []string{"A"}})
	if res2.ID == res.ID {
		t.Fatal("pressing Enter on a new dialog must start a new batch")
	}
	if _, err = a.tuiSubmitBatch(t.Context(), tui.BatchRequest{}); err == nil {
		t.Fatal("an empty batch is refused")
	}
}

func TestTUIInspectWithDeepMapsTheScanAndTheDestinationRefusal(t *testing.T) {
	var got map[string]any
	est := int64(120)
	a := driveApp(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/v1/plans":
			got = readBody(t, r)
			if got["destination_id"] == "dst_ro" {
				replyError(w, 409, "destination_not_writable", "This account cannot add files to that folder.")
				return
			}
			reply(w, 200, map[string]any{"id": "plan_1", "revision": 1, "source_id": "S", "source_name": "Big", "source_kind": "folder",
				"destination_id": "dst_1", "root_name": "Big", "preview": map[string]any{"observed_files": 3, "observed_folders": 1, "scan_complete": false,
					"deep": map[string]any{"state": "running", "complete": false, "files": 3, "folders": 1, "bytes": 2048, "scanned": 4,
						"native_files": 1, "shortcuts": 2, "cannot_copy": 3, "unreadable_folders": 4, "estimate_seconds": est,
						"warnings": []any{"may_hit_daily_limit"}}}})
		case r.URL.Path == "/v1/plans/plan_1" || r.URL.Path == "/v1/plans/plan_1/preflight/cancel":
			reply(w, 200, map[string]any{"id": "plan_1", "revision": 1, "source_id": "S", "source_name": "Big", "source_kind": "folder",
				"destination_id": "dst_1", "root_name": "Big", "preview": map[string]any{"observed_files": 9, "observed_folders": 2, "scan_complete": true,
					"deep": map[string]any{"state": "complete", "complete": true, "files": 9, "folders": 2}}})
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	})
	plan, err := a.tuiInspectWith(t.Context(), "SRC", "", "", tui.CopyOptions{Deep: true})
	if err != nil {
		t.Fatal(err)
	}
	if got["deep"] != true {
		t.Fatalf("deep must be asked for: %v", got)
	}
	d := plan.Deep
	if d == nil || !d.Running() || d.Complete || d.Files != 3 || d.Bytes != 2048 || d.Native != 1 || d.Shortcuts != 2 || d.CannotCopy != 3 ||
		d.Unreadable != 4 || d.Scanned != 4 || *d.EstimateSeconds != 120 || len(d.Warnings) != 1 {
		t.Fatalf("deep = %+v", d)
	}
	p2, err := a.tuiGetPlan(t.Context(), "plan_1")
	if err != nil || p2.Deep == nil || !p2.Deep.Complete || p2.Files != 9 {
		t.Fatalf("get plan = %+v %v", p2, err)
	}
	if p3, err := a.tuiCancelPreflight(t.Context(), "plan_1"); err != nil || p3.Deep == nil {
		t.Fatalf("cancel = %+v %v", p3, err)
	}
	_, err = a.tuiInspectWith(t.Context(), "SRC", "dst_ro", "", tui.CopyOptions{})
	var ce *tui.CodedError
	if !asErr(err, &ce) || ce.Code != "destination_not_writable" {
		t.Fatalf("the TUI words a read-only destination itself: %v", err)
	}
	// Without --deep nothing extra is sent.
	got = nil
	if _, err = a.tuiInspectWith(t.Context(), "SRC", "", "", tui.CopyOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, has := got["deep"]; has {
		t.Fatalf("a shallow preview must not ask for a scan: %v", got)
	}
}

func TestTUIQueueMoveAndBatchCalls(t *testing.T) {
	var s seen
	a := driveApp(t, func(w http.ResponseWriter, r *http.Request) {
		s.record(r, readBody(t, r))
		switch r.URL.Path {
		case "/v1/queue/pause":
			reply(w, 200, map[string]any{"paused": true, "changed": true, "affected_jobs": 2})
		case "/v1/queue/resume":
			reply(w, 200, map[string]any{"paused": false, "changed": true, "affected_jobs": 0})
		case "/v1/batches/batch-1":
			reply(w, 200, map[string]any{"batch_id": "batch-1", "total": 4, "completed_jobs": 3, "by_state": map[string]any{"succeeded": 3, "running": 1}})
		default:
			reply(w, 200, map[string]any{"id": "job_1", "counts": map[string]any{}})
		}
	})
	if n, err := a.tuiQueuePause(t.Context(), true); err != nil || n != 2 {
		t.Fatalf("pause = %d %v", n, err)
	}
	if n, err := a.tuiQueuePause(t.Context(), false); err != nil || n != 0 || !s.has("POST /v1/queue/resume") {
		t.Fatalf("resume = %d %v", n, err)
	}
	prio := 3
	if err := a.tuiMoveJob(t.Context(), "job_1", 7, tui.JobMove{Before: "job_0", Priority: &prio}); err != nil {
		t.Fatal(err)
	}
	b := s.body["POST /v1/jobs/job_1/move"]
	if b["before"] != "job_0" || b["priority"] != float64(3) || b["expected_revision"] != float64(7) {
		t.Fatalf("move body = %v", b)
	}
	if _, has := b["after"]; has {
		t.Fatalf("only the given placement is sent: %v", b)
	}
	p, err := a.tuiGetBatch(t.Context(), "batch-1")
	if err != nil || p.Total != 4 || p.CompletedJobs != 3 || p.ByState["running"] != 1 {
		t.Fatalf("batch = %+v %v", p, err)
	}
}

func TestTUIReportAndExportWriteACSVAndNeverOverwrite(t *testing.T) {
	a := driveApp(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/items"):
			if r.URL.Query().Get("problems") != "true" {
				t.Errorf("the report reads problems only: %s", r.URL.RawQuery)
			}
			reply(w, 200, map[string]any{"items": []any{
				map[string]any{"source_id": "S1", "name": "=cmd()", "path": "=cmd()/a.mp4", "state": "skipped", "code": "filtered", "attempts": 0},
				map[string]any{"source_id": "S2", "name": "b", "path": "dir/b.pdf", "state": "failed", "code": "unreadable", "attempts": 3},
			}, "next_cursor": nil})
		case strings.HasPrefix(r.URL.Path, "/v1/history"):
			if r.URL.Query().Get("since") == "" {
				t.Errorf("history export needs a window")
			}
			reply(w, 200, map[string]any{"items": []any{
				map[string]any{"id": "job_1", "state": "succeeded", "source_name": "Src", "created_at": "2026-10-20T00:00:00Z", "updated_at": "2026-10-20T01:00:00Z", "counts": map[string]any{"completed_items": 5}},
			}, "next_cursor": nil})
		default:
			reply(w, 200, map[string]any{"id": "job_1", "counts": map[string]any{}})
		}
	})
	rep, err := a.tuiReport(t.Context(), "job_1")
	if err != nil || len(rep.Items) != 2 || rep.Items[1].Code != "unreadable" || rep.Items[1].Attempts != 3 {
		t.Fatalf("report = %+v %v", rep, err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "report.csv")
	rows, err := a.tuiExportCSV(t.Context(), "report", "job_1", path)
	if err != nil || rows != 2 {
		t.Fatalf("export = %d %v", rows, err)
	}
	b, _ := os.ReadFile(path)
	text := string(b)
	if !strings.HasPrefix(text, "state,code,reason,path,attempts,source_id,destination_id") || !strings.Contains(text, "'=cmd()/a.mp4") {
		t.Fatalf("the CSV needs the header and formula-guarded cells:\n%s", text)
	}
	if st, _ := os.Stat(path); st.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v", st.Mode())
	}
	if _, err = a.tuiExportCSV(t.Context(), "report", "job_1", path); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("an existing file is never overwritten: %v", err)
	}
	if b2, _ := os.ReadFile(path); string(b2) != text {
		t.Fatal("the existing file changed")
	}
	hist := filepath.Join(dir, "history.csv")
	if rows, err = a.tuiExportCSV(t.Context(), "history", "", hist); err != nil || rows != 1 {
		t.Fatalf("history = %d %v", rows, err)
	}
	for _, bad := range []string{"", dir, filepath.Join(dir, "no", "such", "x.csv")} {
		if _, err = a.tuiExportCSV(t.Context(), "report", "job_1", bad); err == nil {
			t.Errorf("%q must be refused", bad)
		}
	}
	if _, err = a.tuiExportCSV(t.Context(), "weird", "job_1", filepath.Join(dir, "w.csv")); err == nil {
		t.Fatal("unknown kind")
	}
}

func TestTUISearchAndLatestAudit(t *testing.T) {
	var s seen
	a := driveApp(t, func(w http.ResponseWriter, r *http.Request) {
		s.record(r, nil)
		switch {
		case r.URL.Path == "/v1/drive/search":
			reply(w, 200, map[string]any{"items": []any{
				map[string]any{"id": "f1", "name": "Báo cáo 2026", "kind": "folder", "can_add_children": true},
				map[string]any{"id": "f2", "name": "bao-cao.pdf", "kind": "binary", "size_bytes": 10, "can_copy": true},
			}, "limit": 20})
		case r.URL.Path == "/v1/collections/col_1/audits/latest":
			reply(w, 200, map[string]any{"id": "aud_1", "collection_id": "col_1", "job_id": "job_a", "state": "completed", "revision": 4,
				"counts": map[string]any{"missing_at_destination": 2, "name_mismatch": 1}, "repairable_findings": 2, "scanned_folders": 7})
		case r.URL.Path == "/v1/collections/col_2/audits/latest":
			replyError(w, 404, "not_found", "No audit yet.")
		default:
			replyError(w, 500, "internal", "boom")
		}
	})
	items, err := a.tuiSearch(t.Context(), "báo cáo")
	if err != nil || len(items) != 2 || !items[0].IsFolder() || items[0].Name != "Báo cáo 2026" || !items[1].CanCopy {
		t.Fatalf("search = %+v %v", items, err)
	}
	if !s.has("GET /v1/drive/search?q=b%C3%A1o+c%C3%A1o") {
		t.Fatalf("the words travel in the query: %v", s.calls)
	}
	rep, found, err := a.tuiLatestAudit(t.Context(), "col_1")
	if err != nil || !found || rep.Repairable != 2 || rep.Total() != 3 || rep.State != "completed" || rep.Revision != 4 {
		t.Fatalf("audit = %+v %v %v", rep, found, err)
	}
	if _, found, err = a.tuiLatestAudit(t.Context(), "col_2"); err != nil || found {
		t.Fatalf("no audit yet is not an error: %v %v", found, err)
	}
	if _, _, err = a.tuiLatestAudit(t.Context(), "col_3"); err == nil {
		t.Fatal("a real failure is reported")
	}
}
