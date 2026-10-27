package cli

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/itsgiahung/502Drive/internal/tui"
)

func replyError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": code, "message": msg, "retryable": false, "retry_after_seconds": nil}, "request_id": "req_1"})
}

func readBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	var body map[string]any
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	return body
}

func TestTUISnapshotReadsLifecycleQuotaSkipsAndPreferences(t *testing.T) {
	a := driveApp(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/status":
			reply(w, 200, map[string]any{"version": "1", "ready": true, "running_jobs": 0, "queued_jobs": 0, "blocked_jobs": 1,
				"account":   map[string]any{"status": "connected"},
				"lifecycle": map[string]any{"state": "resuming", "online": false, "last_gap_seconds": 3600, "reconnect_required": false},
				"throttle":  map[string]any{"workers_now": 2, "throttled_last_5m": 3, "next_retry_at": nil}})
		case "/v1/jobs":
			reply(w, 200, map[string]any{"items": []any{
				map[string]any{"id": "job_1", "collection_id": "col_1", "source_name": "Big", "state": "blocked", "revision": 4,
					"resume_after": "2026-10-27T14:05:00Z",
					"counts": map[string]any{"completed_items": 3, "failed_items": 0, "blocked_items": 0, "skipped_items": 5, "total_items": 9,
						"logical_bytes": 2048, "skipped_by_code": map[string]any{"filtered": 4, "conflict": 1}}},
				map[string]any{"id": "job_2", "source_name": "Archive", "state": "succeeded", "kind": "cleanup", "resume_after": nil,
					"counts": map[string]any{}},
			}})
		case "/v1/collections":
			reply(w, 200, map[string]any{"items": []any{map[string]any{"id": "col_1", "source_name": "Big", "state": "ready", "revision": 2,
				"root_name": "Big (copy)", "on_conflict": "skip", "filter": map[string]any{"exclude": []any{"class:video"}, "max_size": 1073741824}}}})
		case "/v1/preferences":
			reply(w, 200, map[string]any{"language": "auto", "timezone": "UTC", "notify_completed": true, "notify_failed": false,
				"notify_action_required": true, "quiet_hours_start": "22:00", "quiet_hours_end": "07:00", "retention_days": 30})
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
	st := snap.Status
	if st.Lifecycle != "resuming" || st.LifecycleOnline || st.GapSeconds != 3600 {
		t.Fatalf("lifecycle = %+v", st)
	}
	j := snap.Jobs[0]
	if j.ResumeAfter.IsZero() || j.ResumeAfter.UTC().Format("15:04") != "14:05" || j.SkippedByCode["filtered"] != 4 || j.Bytes != 2048 {
		t.Fatalf("job = %+v", j)
	}
	if snap.Jobs[1].Kind != "cleanup" || !snap.Jobs[1].ResumeAfter.IsZero() {
		t.Fatalf("a null resume_after must stay zero: %+v", snap.Jobs[1])
	}
	c := snap.Collections[0]
	if c.RootName != "Big (copy)" || c.OnConflict != "skip" || len(c.Filter.Exclude) != 1 || c.Filter.MaxSize != 1<<30 {
		t.Fatalf("collection = %+v", c)
	}
	p := snap.Prefs
	if p.Language != "auto" || !p.NotifyCompleted || p.NotifyFailed || !p.NotifyAction || p.QuietStart != "22:00" || p.QuietEnd != "07:00" || p.RetentionDays != 30 {
		t.Fatalf("prefs = %+v", p)
	}
	if runtime.GOOS == "linux" && (!p.AutostartKnown || p.Autostart) {
		t.Fatalf("autostart on a fresh config dir is known and off: %+v", p)
	}
}

func TestTUISnapshotOldDaemonWithoutNewFieldsStillWorks(t *testing.T) {
	a := driveApp(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/status":
			reply(w, 200, map[string]any{"version": "1", "ready": true, "account": map[string]any{"status": "connected"}})
		case "/v1/jobs":
			reply(w, 200, map[string]any{"items": []any{map[string]any{"id": "job_1", "source_name": "x", "state": "running", "counts": map[string]any{}}}})
		default:
			reply(w, 200, map[string]any{"items": []any{}})
		}
	})
	snap, err := a.tuiSnapshot(t.Context())
	if err != nil || snap.Status.Lifecycle != "" || !snap.Jobs[0].ResumeAfter.IsZero() {
		t.Fatalf("an older daemon reports none of the new fields: %+v %v", snap.Status, err)
	}
}

func TestTUIListsRetiredCollectionsOnlyWhenAskedWithIncludeRetired(t *testing.T) {
	var mu sync.Mutex
	var queries []string
	a := driveApp(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/collections" {
			reply(w, 200, map[string]any{"items": []any{}, "version": "1", "account": map[string]any{"status": "connected"}})
			return
		}
		mu.Lock()
		queries = append(queries, r.URL.RawQuery)
		mu.Unlock()
		own := map[string]any{"id": "col_own", "source_name": "Own", "state": "ready", "revision": 1}
		if r.URL.Query().Get("include_retired") == "true" {
			reply(w, 200, map[string]any{"items": []any{own,
				map[string]any{"id": "col_old", "source_name": "Old", "state": "retired", "revision": 3}}})
			return
		}
		reply(w, 200, map[string]any{"items": []any{own}})
	})
	var snap tui.Snapshot
	a.tuiCollections(t.Context(), &snap)
	if len(snap.Collections) != 1 || strings.Join(queries, "|") != "" {
		t.Fatalf("by default only the usual list is read: %+v %v", snap.Collections, queries)
	}
	a.showRetired.Store(true)
	snap = tui.Snapshot{}
	a.tuiCollections(t.Context(), &snap)
	var ids []string
	for _, c := range snap.Collections {
		ids = append(ids, c.ID)
	}
	if strings.Join(ids, ",") != "col_own,col_old" {
		t.Fatalf("retired ones are added: %v", ids)
	}
	if strings.Join(queries, "|") != "|include_retired=true" { // the default read, then one read with the flag
		t.Fatalf("one read with include_retired, never all=true: %v", queries)
	}
}

func TestTUIInspectWithSendsTheOptionsAndMapsThePreviewFilter(t *testing.T) {
	var got map[string]any
	a := driveApp(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/plans" || r.Method != "POST" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		got = readBody(t, r)
		reply(w, 200, map[string]any{"id": "plan_1", "revision": 1, "source_id": "S", "source_name": "Big", "source_kind": "folder",
			"destination_id": "dst_1", "root_name": "Big",
			"preview": map[string]any{"observed_files": 10, "observed_folders": 2, "scan_complete": true,
				"filter": map[string]any{"included": 7, "excluded": 3, "included_samples": []any{"a"}, "excluded_samples": []any{"x.mp4"}, "scope": "top_level"}}})
	})
	plan, err := a.tuiInspectWith(t.Context(), "SRC", "dst_1", "", tui.CopyOptions{Shortcuts: "skip", OnConflict: "update", FilterPreset: "no_video"})
	if err != nil {
		t.Fatal(err)
	}
	if got["shortcuts"] != "skip" || got["on_conflict"] != "update" || got["source"] != "SRC" {
		t.Fatalf("body = %v", got)
	}
	f, _ := got["filter"].(map[string]any)
	if ex, _ := f["exclude"].([]any); len(ex) != 1 || ex[0] != "class:video" {
		t.Fatalf("filter = %v", got["filter"])
	}
	if plan.Filter == nil || plan.Filter.Included != 7 || plan.Filter.Excluded != 3 || plan.Filter.Scope != "top_level" || plan.Filter.ExcludedSamples[0] != "x.mp4" {
		t.Fatalf("plan filter = %+v", plan.Filter)
	}
	// Defaults send nothing extra, and an unknown preset is refused locally.
	got = nil
	if _, err = a.tuiInspectWith(t.Context(), "SRC", "", "", tui.CopyOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, has := got["filter"]; has || got["shortcuts"] != nil || got["on_conflict"] != nil {
		t.Fatalf("the defaults must stay out of the request: %v", got)
	}
	if _, err = a.tuiInspectWith(t.Context(), "SRC", "", "", tui.CopyOptions{FilterPreset: "nope"}); err == nil {
		t.Fatal("an unknown preset must be refused")
	}
}

func TestTUIFilterPresetsMatchTheRulesTheDialogShows(t *testing.T) {
	cases := map[string]filterWant{
		"no_video":  {exclude: "class:video"},
		"docs_only": {include: "class:doc"},
		"under_1gb": {max: 1 << 30},
	}
	for preset, want := range cases {
		spec, err := filterForPreset(preset)
		if err != nil {
			t.Fatal(err)
		}
		if (want.exclude != "" && (len(spec.Exclude) != 1 || spec.Exclude[0] != want.exclude)) ||
			(want.include != "" && (len(spec.Include) != 1 || spec.Include[0] != want.include)) || spec.MaxSize != want.max {
			t.Errorf("%s = %+v", preset, spec)
		}
		if err = spec.Validate(); err != nil {
			t.Errorf("%s is not a valid rule: %v", preset, err)
		}
	}
	if spec, _ := filterForPreset(""); spec.Active() {
		t.Fatal("no preset means no rule")
	}
}

type filterWant struct {
	include, exclude string
	max              int64
}

func TestTUISubmitModeAsksOnAnExistingCopyThenSendsTheAnswer(t *testing.T) {
	var bodies []map[string]any
	a := driveApp(t, func(w http.ResponseWriter, r *http.Request) {
		b := readBody(t, r)
		bodies = append(bodies, b)
		if b["mode"] == nil {
			replyError(w, 409, "already_copied", "This source is already copied to this destination (collection col_1, ready).")
			return
		}
		reply(w, 202, map[string]any{"id": "job_9", "collection_id": "col_1", "state": "queued", "counts": map[string]any{}})
	})
	plan := tui.Plan{ID: "plan_1", Revision: 3}
	_, _, err := a.tuiSubmitMode(t.Context(), plan, "")
	var existing *tui.ExistingCopyError
	if !errors.As(err, &existing) || !strings.Contains(existing.Message, "collection col_1") {
		t.Fatalf("a 409 already_copied must become the question: %v", err)
	}
	job, col, err := a.tuiSubmitMode(t.Context(), plan, "update")
	if err != nil || job != "job_9" || col != "col_1" || bodies[1]["mode"] != "update" || bodies[1]["plan_id"] != "plan_1" {
		t.Fatalf("update: %v %v %v %v", job, col, err, bodies)
	}
	if _, _, err = a.tuiSubmitMode(t.Context(), plan, "new"); err != nil || bodies[2]["mode"] != "new" {
		t.Fatalf("new: %v %v", err, bodies)
	}
	n := len(bodies)
	if _, _, err = a.tuiSubmitMode(t.Context(), plan, "overwrite"); err == nil || len(bodies) != n {
		t.Fatal("an unknown mode is refused without a request")
	}
	// Another error (stale plan) is passed through, not turned into the question.
	b := driveApp(t, func(w http.ResponseWriter, r *http.Request) {
		replyError(w, 409, "plan_expired", "Inspect the source again.")
	})
	_, _, err = b.tuiSubmitMode(t.Context(), plan, "")
	if errors.As(err, &existing) || err == nil {
		t.Fatalf("only already_copied is a question: %v", err)
	}
}

func TestTUIAuditAdaptersStartReadAndRepair(t *testing.T) {
	var repairBody map[string]any
	var findingCalls int
	a := driveApp(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/v1/collections/col_1/audits":
			reply(w, 202, map[string]any{"id": "aud_1", "collection_id": "col_1", "job_id": "job_a", "state": "running", "revision": 1, "counts": map[string]any{}})
		case r.URL.Path == "/v1/audits/aud_1":
			reply(w, 200, map[string]any{"id": "aud_1", "collection_id": "col_1", "job_id": "job_a", "state": "completed", "revision": 4, "scanned_folders": 12,
				"counts": map[string]any{"missing_at_destination": 2, "extra_at_destination": 1}})
		case r.URL.Path == "/v1/audits/aud_1/findings":
			findingCalls++
			if r.URL.Query().Get("cursor") == "" {
				reply(w, 200, map[string]any{"items": []any{
					map[string]any{"audit_id": "aud_1", "kind": "missing_at_destination", "source_id": "s1", "repairable": true, "repaired": false, "detail": "a.pdf"},
					map[string]any{"audit_id": "aud_1", "kind": "missing_at_destination", "source_id": "s2", "repairable": true, "repaired": true, "detail": "b.pdf"}},
					"next_cursor": "c2"})
				return
			}
			reply(w, 200, map[string]any{"items": []any{
				map[string]any{"audit_id": "aud_1", "kind": "extra_at_destination", "dest_id": "d3", "repairable": false, "repaired": false}},
				"next_cursor": nil})
		case r.Method == "POST" && r.URL.Path == "/v1/audits/aud_1/repair":
			repairBody = readBody(t, r)
			reply(w, 202, map[string]any{"id": "job_fix", "state": "queued", "counts": map[string]any{}})
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	})
	rep, err := a.tuiStartAudit(t.Context(), "col_1")
	if err != nil || rep.ID != "aud_1" || rep.JobID != "job_a" || rep.State != "running" {
		t.Fatalf("start: %+v %v", rep, err)
	}
	rep, err = a.tuiGetAudit(t.Context(), "aud_1")
	if err != nil || rep.State != "completed" || rep.Revision != 4 || rep.Scanned != 12 || rep.Total() != 3 {
		t.Fatalf("get: %+v %v", rep, err)
	}
	if rep.Repairable != 1 || len(rep.Findings) != 3 || findingCalls != 2 {
		t.Fatalf("an already repaired finding is not repairable again; both pages are read: %+v (%d calls)", rep, findingCalls)
	}
	job, err := a.tuiRepairAudit(t.Context(), "aud_1", 4, true)
	if err != nil || job != "job_fix" || repairBody["dry_run"] != true || repairBody["expected_revision"] != float64(4) {
		t.Fatalf("repair: %v %v %v", job, err, repairBody)
	}
}

func TestTUILifecycleCleanupAndFilterCalls(t *testing.T) {
	type call struct {
		method, path string
		body         map[string]any
	}
	var calls []call
	a := driveApp(t, func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, call{r.Method, r.URL.Path, readBody(t, r)})
		if strings.HasSuffix(r.URL.Path, "/cleanup") {
			reply(w, 202, map[string]any{"id": "job_c", "state": "queued", "counts": map[string]any{}})
			return
		}
		reply(w, 200, map[string]any{"id": "col_1"})
	})
	for _, act := range []string{"retire", "resume", "forget"} {
		if err := a.tuiCollectionLifecycle(t.Context(), "col_1", act, 7); err != nil {
			t.Fatal(err)
		}
	}
	for i, act := range []string{"retire", "resume", "forget"} {
		c := calls[i]
		if c.method != "POST" || c.path != "/v1/collections/col_1/"+act || c.body["expected_revision"] != float64(7) {
			t.Errorf("%s: %+v", act, c)
		}
	}
	n := len(calls)
	if err := a.tuiCollectionLifecycle(t.Context(), "col_1", "cleanup", 7); err == nil || len(calls) != n {
		t.Fatal("only the three lifecycle actions are allowed here, with no request for others")
	}
	job, err := a.tuiCleanup(t.Context(), "col_1", 7, false, "My Folder")
	last := calls[len(calls)-1]
	if err != nil || job != "job_c" || last.body["dry_run"] != false || last.body["confirm_name"] != "My Folder" || last.body["expected_revision"] != float64(7) {
		t.Fatalf("cleanup: %v %v %+v", job, err, last)
	}
	if _, err = a.tuiCleanup(t.Context(), "col_1", 7, true, ""); err != nil || calls[len(calls)-1].body["dry_run"] != true {
		t.Fatal("the plan is a dry run")
	}
	if err = a.tuiSetFilterPreset(t.Context(), "col_1", "docs_only", 7); err != nil {
		t.Fatal(err)
	}
	last = calls[len(calls)-1]
	f, _ := last.body["filter"].(map[string]any)
	if last.method != "PUT" || last.path != "/v1/collections/col_1/filter" || last.body["expected_revision"] != float64(7) {
		t.Fatalf("filter call = %+v", last)
	}
	if inc, _ := f["include"].([]any); len(inc) != 1 || inc[0] != "class:doc" {
		t.Fatalf("filter body = %v", last.body)
	}
	if err = a.tuiSetFilterPreset(t.Context(), "col_1", "", 7); err != nil {
		t.Fatal(err)
	}
	if f, _ := calls[len(calls)-1].body["filter"].(map[string]any); len(f) != 0 {
		t.Fatalf("the first preset clears the rule: %v", f)
	}
}

func TestTUISetPreferenceNamesOnlyTheFieldsSettingsOffers(t *testing.T) {
	var bodies []map[string]any
	a := driveApp(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PATCH" || r.URL.Path != "/v1/preferences" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		bodies = append(bodies, readBody(t, r))
		reply(w, 200, map[string]any{})
	})
	for _, c := range []struct {
		field string
		value any
	}{{"notify_failed", false}, {"retention_days", 0}, {"quiet_hours", "22:00-07:00"}, {"quiet_hours", ""}} {
		if err := a.tuiSetPreference(t.Context(), c.field, c.value); err != nil {
			t.Fatalf("%s: %v", c.field, err)
		}
	}
	if bodies[0]["notify_failed"] != false || bodies[1]["retention_days"] != float64(0) {
		t.Fatalf("bodies = %v", bodies)
	}
	if bodies[2]["quiet_hours_start"] != "22:00" || bodies[2]["quiet_hours_end"] != "07:00" {
		t.Fatalf("quiet hours = %v", bodies[2])
	}
	if bodies[3]["quiet_hours_start"] != "" || bodies[3]["quiet_hours_end"] != "" {
		t.Fatalf("an empty value clears both: %v", bodies[3])
	}
	n := len(bodies)
	for _, c := range []struct {
		field string
		value any
	}{{"timezone", "UTC"}, {"auto_copy", true}, {"notify_failed", "yes"}, {"retention_days", -1}, {"retention_days", "7"}, {"quiet_hours", "22:00"}, {"quiet_hours", 5}} {
		if err := a.tuiSetPreference(t.Context(), c.field, c.value); err == nil {
			t.Errorf("%s=%v must be refused", c.field, c.value)
		}
	}
	if len(bodies) != n {
		t.Fatal("a refused change sends nothing")
	}
}

func TestTUIAutostartWritesAndRemovesTheUserEntry(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the XDG autostart entry is the Linux implementation")
	}
	dir := t.TempDir()
	a := &App{io: IO{Out: &strings.Builder{}, Err: &strings.Builder{}, Getenv: func(k string) string {
		if k == "XDG_CONFIG_HOME" {
			return dir
		}
		return ""
	}}}
	if on, known := a.autostartForTUI(); on || !known {
		t.Fatalf("fresh: on=%v known=%v", on, known)
	}
	if err := a.tuiSetAutostart(true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "autostart", "502drive.desktop")); err != nil {
		t.Fatalf("the entry must be written under the config dir: %v", err)
	}
	if on, _ := a.autostartForTUI(); !on {
		t.Fatal("autostart reads back as on")
	}
	if err := a.tuiSetAutostart(false); err != nil {
		t.Fatal(err)
	}
	if on, _ := a.autostartForTUI(); on {
		t.Fatal("and as off again")
	}
}

func TestTUIActionsAreAllWired(t *testing.T) {
	acts := (&App{}).tuiActions()
	missing := map[string]bool{
		"InspectWith": acts.InspectWith == nil, "SubmitMode": acts.SubmitMode == nil, "StartAudit": acts.StartAudit == nil,
		"GetAudit": acts.GetAudit == nil, "RepairAudit": acts.RepairAudit == nil, "CollectionLifecycle": acts.CollectionLifecycle == nil,
		"Cleanup": acts.Cleanup == nil, "SetFilterPreset": acts.SetFilterPreset == nil, "ShowRetired": acts.ShowRetired == nil,
		"SetPreference": acts.SetPreference == nil, "SetAutostart": acts.SetAutostart == nil,
	}
	for name, m := range missing {
		if m {
			t.Errorf("%s is not wired to the TUI", name)
		}
	}
	a := &App{}
	acts = a.tuiActions()
	acts.ShowRetired(true)
	if !a.showRetired.Load() {
		t.Fatal("ShowRetired must reach the snapshot reader")
	}
}
