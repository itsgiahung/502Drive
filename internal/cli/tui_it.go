package cli

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/itsgiahung/502Drive/internal/tui"
)

// The adapters below serve the batch IT screens of the TUI: batch copy and the
// queue (S6), the deep preview (S7), the after-copy sync choice (S8), the
// problem report and CSV export (X1), the server-side job filter, Drive search
// and the newest audit on the Library. As in tui.go they only name fixed API
// calls; the daemon decides and nothing here writes the DB.

// tuiJobView is a job row with the queue fields the TUI shows.
type tuiJobView struct {
	jobView
	BatchID  string `json:"batch_id"`
	Priority int    `json:"priority"`
}

func (j tuiJobView) job() tui.Job {
	job := tui.Job{ID: j.ID, CollectionID: j.CollectionID, SourceName: j.SourceName, State: j.State,
		Desired: j.Desired, Revision: j.Revision, Completed: j.Counts.Completed, Failed: j.Counts.Failed,
		Blocked: j.Counts.Blocked, Skipped: j.Counts.Skipped, Total: j.Counts.Total,
		Kind: j.Kind, SkippedByCode: j.Counts.SkippedByCode, Bytes: j.Counts.Bytes,
		BatchID: j.BatchID, Priority: j.Priority}
	if j.ResumeAfter != nil {
		job.ResumeAfter = parseTime(j.ResumeAfter)
	}
	if j.CreatedAt != "" {
		job.Created = parseTime(&j.CreatedAt)
	}
	if j.Phase != nil {
		job.Phase = *j.Phase
	}
	if j.ResultURL != nil {
		job.ResultURL = *j.ResultURL
	}
	if j.LastError != nil {
		job.LastError = j.LastError.Message
	}
	return job
}

// ---- job filter ----

func (a *App) tuiSetJobQuery(q tui.JobQuery) {
	a.slow.mu.Lock()
	a.slow.jq = q
	a.slow.mu.Unlock()
}

func (a *App) tuiJobQuery() tui.JobQuery {
	a.slow.mu.Lock()
	defer a.slow.mu.Unlock()
	return a.slow.jq
}

// ---- copy with the sync choice (S8), batch (S6), deep preview (S7) ----

// tuiSubmitOpts starts one copy; AfterSync "" lets the daemon apply the saved
// preference.
func (a *App) tuiSubmitOpts(ctx context.Context, plan tui.Plan, o tui.SubmitOptions) (string, string, error) {
	if err := validateSyncMode(o.AfterSync); err != nil {
		return "", "", localErr(ExitUsage, "invalid_request", "unknown after-copy sync mode")
	}
	body := map[string]any{"plan_id": plan.ID, "expected_revision": plan.Revision}
	switch o.Mode {
	case "":
	case "update", "new":
		body["mode"] = o.Mode
	default:
		return "", "", localErr(ExitUsage, "invalid_request", "unknown copy mode")
	}
	if o.AfterSync != "" {
		body["after_copy_sync_mode"] = o.AfterSync
	}
	r, err := a.call(ctx, "POST", "/v1/jobs", nil, body, NewKey())
	if err != nil {
		var ae *APIError
		if o.Mode == "" && errors.As(err, &ae) && ae.Code == "already_copied" {
			return "", "", &tui.ExistingCopyError{Message: ae.Message}
		}
		return "", "", err
	}
	var job jobView
	if err = r.Decode(&job); err != nil {
		return "", "", err
	}
	return job.ID, job.CollectionID, nil
}

// tuiSubmitBatch admits one job per link under one batch id. A link the daemon
// refuses is reported on its own and does not stop the others. Every call is a
// new batch (a fresh key), like pressing Enter on a new dialog.
func (a *App) tuiSubmitBatch(ctx context.Context, req tui.BatchRequest) (tui.BatchResult, error) {
	var res tui.BatchResult
	if len(req.Sources) == 0 {
		return res, localErr(ExitUsage, "invalid_request", "no links to copy")
	}
	if err := validateSyncMode(req.Submit.AfterSync); err != nil {
		return res, localErr(ExitUsage, "invalid_request", "unknown after-copy sync mode")
	}
	spec, err := filterForPreset(req.Options.FilterPreset)
	if err != nil {
		return res, err
	}
	po := planOpts{Shortcuts: req.Options.Shortcuts, OnConflict: req.Options.OnConflict}
	if err = po.validate(); err != nil {
		return res, err
	}
	po.Filter = spec
	bkey := "tui-" + NewKey()
	bid := "batch-" + requestID(bkey)
	res.ID = bid
	for i, src := range req.Sources {
		base := fmt.Sprintf("%s:%d", bkey, i)
		item := tui.BatchJob{Source: src}
		name := ""
		if req.AutoName {
			plan, _, perr := a.makePlan(ctx, src, req.DestinationID, "", base+"-name", po)
			if perr != nil {
				item.Error = perr.Error()
				res.Jobs = append(res.Jobs, item)
				continue
			}
			name = autoCopyName(plan.SourceName)
		}
		job, jerr := a.submitBatchItem(ctx, src, req.DestinationID, name, base, bid, po, req.Submit.Mode, req.Submit.AfterSync)
		if jerr != nil {
			item.Error = jerr.Error()
		} else {
			item.JobID = job.ID
		}
		res.Jobs = append(res.Jobs, item)
	}
	return res, nil
}

func (a *App) tuiGetPlan(ctx context.Context, id string) (tui.Plan, error) {
	r, err := a.call(ctx, "GET", "/v1/plans/"+url.PathEscape(id), nil, nil, "")
	if err != nil {
		return tui.Plan{}, err
	}
	var plan planView
	if err = r.Decode(&plan); err != nil {
		return tui.Plan{}, err
	}
	return tuiPlan(plan), nil
}

func (a *App) tuiCancelPreflight(ctx context.Context, planID string) (tui.Plan, error) {
	r, err := a.call(ctx, "POST", "/v1/plans/"+url.PathEscape(planID)+"/preflight/cancel", nil, nil, "")
	if err != nil {
		return tui.Plan{}, err
	}
	var plan planView
	if err = r.Decode(&plan); err != nil {
		return tui.Plan{}, err
	}
	return tuiPlan(plan), nil
}

// ---- queue (S6) ----

func (a *App) tuiGetBatch(ctx context.Context, id string) (tui.BatchProgress, error) {
	r, err := a.call(ctx, "GET", "/v1/batches/"+url.PathEscape(id), nil, nil, "")
	if err != nil {
		return tui.BatchProgress{}, err
	}
	var b struct {
		ID        string         `json:"batch_id"`
		Total     int            `json:"total"`
		Completed int            `json:"completed_jobs"`
		ByState   map[string]int `json:"by_state"`
	}
	if err = r.Decode(&b); err != nil {
		return tui.BatchProgress{}, err
	}
	return tui.BatchProgress{ID: b.ID, Total: b.Total, CompletedJobs: b.Completed, ByState: b.ByState}, nil
}

func (a *App) tuiQueuePause(ctx context.Context, paused bool) (int, error) {
	verb := "resume"
	if paused {
		verb = "pause"
	}
	r, err := a.call(ctx, "POST", "/v1/queue/"+verb, nil, nil, "")
	if err != nil {
		return 0, err
	}
	var out struct {
		Affected int `json:"affected_jobs"`
	}
	if err = r.Decode(&out); err != nil {
		return 0, err
	}
	return out.Affected, nil
}

func (a *App) tuiMoveJob(ctx context.Context, id string, revision int, mv tui.JobMove) error {
	body := map[string]any{"expected_revision": revision}
	if mv.Before != "" {
		body["before"] = mv.Before
	}
	if mv.After != "" {
		body["after"] = mv.After
	}
	if mv.Priority != nil {
		body["priority"] = *mv.Priority
	}
	_, err := a.call(ctx, "POST", "/v1/jobs/"+url.PathEscape(id)+"/move", nil, body, "")
	return err
}

// ---- problem report and CSV export (X1) ----

// tuiReportRows is how many problem items the screen reads; the CSV export
// reads up to exportRowLimit.
const tuiReportRows = 500

func (a *App) tuiReport(ctx context.Context, jobID string) (tui.JobReport, error) {
	items, truncated, _, err := a.fetchProblems(ctx, jobID, tuiReportRows)
	if err != nil {
		return tui.JobReport{}, err
	}
	out := tui.JobReport{Truncated: truncated}
	for _, it := range items {
		out.Items = append(out.Items, tui.ProblemItem{State: it.State, Code: it.Code, Path: it.Path, Attempts: it.Attempts})
	}
	return out, nil
}

// tuiExportCSV writes the job's problem report (kind "report") or the last 30
// days of job history (kind "history") to a path the user typed. It never
// overwrites a file and writes nothing when reading fails.
func (a *App) tuiExportCSV(ctx context.Context, kind, jobID, path string) (int, error) {
	var header []string
	var rows [][]string
	switch kind {
	case "report":
		items, _, _, err := a.fetchProblems(ctx, jobID, exportRowLimit)
		if err != nil {
			return 0, err
		}
		header, rows = reportHeader(), reportRows(items)
	case "history":
		jobs, err := a.tuiHistory(ctx, time.Now().Add(-30*24*time.Hour), exportRowLimit)
		if err != nil {
			return 0, err
		}
		header, rows = historyHeader(), historyRows(jobs)
	default:
		return 0, localErr(ExitUsage, "invalid_request", "unknown export kind")
	}
	full, err := exportPath(path)
	if err != nil {
		return 0, err
	}
	f, err := os.OpenFile(full, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return 0, localErr(ExitUsage, "file_exists", "That file already exists; choose another name.")
		}
		return 0, localErr(ExitInternal, "export_failed", err.Error())
	}
	if err = writeCSV(f, header, rows); err == nil {
		err = f.Close()
	} else {
		f.Close()
	}
	if err != nil {
		os.Remove(full)
		return 0, localErr(ExitInternal, "export_failed", err.Error())
	}
	return len(rows), nil
}

// exportPath turns what the user typed into an absolute file path: ~/ means the
// home directory and a relative path is relative to the current directory. The
// parent folder must exist and the path must not be a folder.
func exportPath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" || strings.ContainsRune(p, 0) {
		return "", localErr(ExitUsage, "invalid_request", "Type a .csv file path.")
	}
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, "~"+string(filepath.Separator)) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", localErr(ExitInternal, "export_failed", "The home folder is unknown.")
		}
		p = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(p, "~"), string(filepath.Separator)))
	}
	full, err := filepath.Abs(p)
	if err != nil {
		return "", localErr(ExitUsage, "invalid_request", "That path cannot be used.")
	}
	if st, err := os.Stat(full); err == nil && st.IsDir() {
		return "", localErr(ExitUsage, "invalid_request", "That path is a folder; add a file name.")
	}
	if st, err := os.Stat(filepath.Dir(full)); err != nil || !st.IsDir() {
		return "", localErr(ExitUsage, "invalid_request", "The folder for that file does not exist.")
	}
	return full, nil
}

// tuiHistory pages GET /v1/history from since, up to maxRows jobs.
func (a *App) tuiHistory(ctx context.Context, since time.Time, maxRows int) ([]jobView, error) {
	var jobs []jobView
	cursor := ""
	for {
		q := url.Values{"limit": {"100"}, "since": {since.UTC().Format(time.RFC3339Nano)}}
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		r, err := a.call(ctx, "GET", "/v1/history", q, nil, "")
		if err != nil {
			return nil, err
		}
		var page struct {
			Items []jobView `json:"items"`
			Next  *string   `json:"next_cursor"`
		}
		if err = r.Decode(&page); err != nil {
			return nil, err
		}
		jobs = append(jobs, page.Items...)
		if len(jobs) >= maxRows {
			return jobs[:maxRows], nil
		}
		if page.Next == nil || *page.Next == "" {
			return jobs, nil
		}
		cursor = *page.Next
	}
}

// ---- Drive search and the newest audit ----

func (a *App) tuiSearch(ctx context.Context, query string) ([]tui.DriveItem, error) {
	r, err := a.call(ctx, "GET", "/v1/drive/search", url.Values{"q": {query}}, nil, "")
	if err != nil {
		return nil, err
	}
	var body struct {
		Items []driveFileView `json:"items"`
	}
	if err = r.Decode(&body); err != nil {
		return nil, err
	}
	out := make([]tui.DriveItem, 0, len(body.Items))
	for _, it := range body.Items {
		out = append(out, it.item())
	}
	return out, nil
}

// tuiLatestAudit reads the newest audit of a collection; a collection that was
// never audited answers 404, which is "none", not an error.
func (a *App) tuiLatestAudit(ctx context.Context, collectionID string) (tui.AuditReport, bool, error) {
	r, err := a.call(ctx, "GET", "/v1/collections/"+url.PathEscape(collectionID)+"/audits/latest", nil, nil, "")
	if err != nil {
		var ae *APIError
		if errors.As(err, &ae) && ae.Status == 404 {
			return tui.AuditReport{}, false, nil
		}
		return tui.AuditReport{}, false, err
	}
	var av struct {
		auditView
		Repairable int `json:"repairable_findings"`
	}
	if err = r.Decode(&av); err != nil {
		return tui.AuditReport{}, false, err
	}
	rep := auditReport(av.auditView, nil)
	rep.Repairable = av.Repairable
	return rep, true, nil
}
