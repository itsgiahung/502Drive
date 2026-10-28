package cli

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/itsgiahung/502Drive/internal/filter"
	"github.com/itsgiahung/502Drive/internal/tui"
)

// The adapters below serve the Library, copy dialog and Settings screens of
// the TUI. Like the rest of tui.go they only name fixed API calls: the daemon
// decides, the TUI never builds a request, and nothing here writes the DB.

// filterForPreset maps a copy-dialog preset to a selective-sync rule (S5).
func filterForPreset(preset string) (filter.Spec, error) {
	switch preset {
	case "":
		return filter.Spec{}, nil
	case "no_video":
		return filter.Spec{Exclude: []string{"class:video"}}, nil
	case "docs_only":
		return filter.Spec{Include: []string{"class:doc"}}, nil
	case "under_1gb":
		return filter.Spec{MaxSize: 1 << 30}, nil
	}
	return filter.Spec{}, localErr(ExitUsage, "invalid_request", "unknown filter preset")
}

func (a *App) tuiInspectWith(ctx context.Context, source, destinationID, rootName string, opt tui.CopyOptions) (tui.Plan, error) {
	spec, err := filterForPreset(opt.FilterPreset)
	if err != nil {
		return tui.Plan{}, err
	}
	po := planOpts{Shortcuts: opt.Shortcuts, OnConflict: opt.OnConflict, Filter: spec, Deep: opt.Deep}
	if err = po.validate(); err != nil {
		return tui.Plan{}, err
	}
	po.Filter = spec // validate re-reads the (unset) filter flags
	plan, _, err := a.makePlan(ctx, source, destinationID, rootName, NewKey(), po)
	if err != nil {
		return tui.Plan{}, codedPlanError(err)
	}
	return tuiPlan(plan), nil
}

// tuiPlan converts the daemon's plan, with its filter preview and deep scan.
func tuiPlan(plan planView) tui.Plan {
	out := tui.Plan{ID: plan.ID, Revision: plan.Revision, SourceID: plan.SourceID, SourceName: plan.SourceName, SourceKind: plan.SourceKind,
		DestinationID: plan.Destination, RootName: plan.RootName, Files: plan.Preview.Files, Folders: plan.Preview.Folders,
		Complete: plan.Preview.Complete, Warnings: plan.Warnings}
	if f := plan.Preview.Filter; f != nil {
		out.Filter = &tui.PlanFilter{Included: f.Included, Excluded: f.Excluded, IncludedSamples: f.IncludedSamples,
			ExcludedSamples: f.ExcludedSamples, Scope: f.Scope}
	}
	if d := plan.Preview.Deep; d != nil {
		out.Deep = &tui.DeepScan{State: d.State, Complete: d.Complete, Folders: d.Folders, Files: d.Files, Bytes: d.Bytes,
			Native: d.Native, Shortcuts: d.Shortcuts, CannotCopy: d.CannotCopy, Unreadable: d.Unreadable, Scanned: d.Scanned,
			EstimateSeconds: d.EstimateSecs, Warnings: d.Warnings}
	}
	return out
}

// codedPlanError keeps the stable code of the refusals the TUI words itself
// (a destination the account cannot write to).
func codedPlanError(err error) error {
	var ae *APIError
	if errors.As(err, &ae) && (ae.Code == "destination_not_writable" || ae.Code == "destination_unverifiable") {
		return &tui.CodedError{Code: ae.Code, Message: ae.Message}
	}
	return err
}

// tuiSubmitMode starts the job; mode answers an existing copy (L2). A 409
// already_copied becomes tui.ExistingCopyError so the dialog can ask.
func (a *App) tuiSubmitMode(ctx context.Context, plan tui.Plan, mode string) (string, string, error) {
	body := map[string]any{"plan_id": plan.ID, "expected_revision": plan.Revision}
	switch mode {
	case "":
	case "update", "new":
		body["mode"] = mode
	default:
		return "", "", localErr(ExitUsage, "invalid_request", "unknown copy mode")
	}
	r, err := a.call(ctx, "POST", "/v1/jobs", nil, body, NewKey())
	if err != nil {
		var ae *APIError
		if mode == "" && errors.As(err, &ae) && ae.Code == "already_copied" {
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

// ---- integrity audit (S1) ----

func (a *App) tuiStartAudit(ctx context.Context, collectionID string) (tui.AuditReport, error) {
	r, err := a.call(ctx, "POST", "/v1/collections/"+url.PathEscape(collectionID)+"/audits", nil, nil, NewKey())
	if err != nil {
		return tui.AuditReport{}, err
	}
	var av auditView
	if err = r.Decode(&av); err != nil {
		return tui.AuditReport{}, err
	}
	return auditReport(av, nil), nil
}

// maxAuditFindings bounds what the TUI reads: enough to count the repairable
// ones and list a few; the CLI pages through the rest.
const maxAuditFindings = 500

func (a *App) tuiGetAudit(ctx context.Context, auditID string) (tui.AuditReport, error) {
	r, err := a.call(ctx, "GET", "/v1/audits/"+url.PathEscape(auditID), nil, nil, "")
	if err != nil {
		return tui.AuditReport{}, err
	}
	var av auditView
	if err = r.Decode(&av); err != nil {
		return tui.AuditReport{}, err
	}
	var found []findingView
	q := url.Values{"limit": {"100"}}
	for len(found) < maxAuditFindings {
		rr, err := a.call(ctx, "GET", "/v1/audits/"+url.PathEscape(auditID)+"/findings", q, nil, "")
		if err != nil {
			return tui.AuditReport{}, err
		}
		var page struct {
			Items []findingView `json:"items"`
			Next  *string       `json:"next_cursor"`
		}
		if err = rr.Decode(&page); err != nil {
			return tui.AuditReport{}, err
		}
		found = append(found, page.Items...)
		if page.Next == nil || *page.Next == "" {
			break
		}
		q.Set("cursor", *page.Next)
	}
	return auditReport(av, found), nil
}

func auditReport(av auditView, found []findingView) tui.AuditReport {
	rep := tui.AuditReport{ID: av.ID, CollectionID: av.CollectionID, JobID: av.JobID, State: av.State,
		Revision: av.Revision, Scanned: av.Scanned, Counts: av.Counts, Repairable: av.Repairable}
	var preview struct {
		Actions []tui.RepairOp `json:"actions"`
	}
	_ = json.Unmarshal(av.Plan, &preview)
	rep.Plan = preview.Actions
	for _, f := range found {
		if f.Repairable && !f.Repaired {
			if av.Repairable == 0 {
				rep.Repairable++
			}
		}
		if len(rep.Findings) < 8 {
			rep.Findings = append(rep.Findings, tui.Finding{Path: f.Path, Kind: f.Kind, SourceID: f.SourceID, DestID: f.DestID,
				Detail: f.Detail, Repairable: f.Repairable, Repaired: f.Repaired})
		}
	}
	return rep
}

func (a *App) tuiRepairAudit(ctx context.Context, auditID string, revision int, dryRun bool) (string, error) {
	r, err := a.call(ctx, "POST", "/v1/audits/"+url.PathEscape(auditID)+"/repair", nil,
		map[string]any{"expected_revision": revision, "dry_run": dryRun}, NewKey())
	if err != nil {
		return "", err
	}
	var j jobView
	if err = r.Decode(&j); err != nil {
		return "", err
	}
	return j.ID, nil
}

// ---- collection lifecycle (L1) and filter (S5) ----

func (a *App) tuiCollectionLifecycle(ctx context.Context, collectionID, action string, revision int) error {
	switch action {
	case "retire", "resume", "forget":
	default:
		return localErr(ExitUsage, "invalid_request", "unknown lifecycle action")
	}
	_, err := a.call(ctx, "POST", "/v1/collections/"+url.PathEscape(collectionID)+"/"+action, nil,
		map[string]any{"expected_revision": revision}, NewKey())
	return err
}

func (a *App) tuiCleanup(ctx context.Context, collectionID string, revision int, dryRun bool, confirmName string) (string, error) {
	r, err := a.call(ctx, "POST", "/v1/collections/"+url.PathEscape(collectionID)+"/cleanup", nil, map[string]any{
		"expected_revision": revision, "dry_run": dryRun, "confirm_name": confirmName}, NewKey())
	if err != nil {
		return "", err
	}
	var j jobView
	if err = r.Decode(&j); err != nil {
		return "", err
	}
	return j.ID, nil
}

func (a *App) tuiSetFilterPreset(ctx context.Context, collectionID, preset string, revision int) error {
	spec, err := filterForPreset(preset)
	if err != nil {
		return err
	}
	_, err = a.call(ctx, "PUT", "/v1/collections/"+url.PathEscape(collectionID)+"/filter", nil,
		map[string]any{"filter": spec, "expected_revision": revision}, NewKey())
	return err
}

// ---- settings ----

// tuiSetPreference changes one preference the Settings screen offers. The
// field list is fixed here, so the TUI cannot patch anything else.
func (a *App) tuiSetPreference(ctx context.Context, field string, value any) error {
	body := map[string]any{}
	switch field {
	case "notify_completed", "notify_failed", "notify_action_required":
		b, ok := value.(bool)
		if !ok {
			return localErr(ExitUsage, "invalid_request", field+" must be true or false")
		}
		body[field] = b
	case "after_copy_sync_mode":
		mode, ok := value.(string)
		if !ok || mode == "" || validateSyncMode(mode) != nil {
			return localErr(ExitUsage, "invalid_request", "after_copy_sync_mode must be off, realtime, hourly, six_hourly or daily")
		}
		body[field] = mode
	case "retention_days":
		n, ok := value.(int)
		if !ok || n < 0 {
			return localErr(ExitUsage, "invalid_request", "retention_days must be 0 or more")
		}
		body[field] = n
	case "quiet_hours":
		s, ok := value.(string)
		if !ok {
			return localErr(ExitUsage, "invalid_request", "quiet_hours must be HH:MM-HH:MM or empty")
		}
		start, end, _ := strings.Cut(s, "-")
		if (start == "") != (end == "") {
			return localErr(ExitUsage, "invalid_request", "quiet_hours needs both times")
		}
		body["quiet_hours_start"], body["quiet_hours_end"] = start, end // empty clears
	default:
		return localErr(ExitUsage, "invalid_request", "unknown preference")
	}
	_, err := a.call(ctx, "PATCH", "/v1/preferences", nil, body, NewKey())
	a.forgetSlow()
	return err
}

// autostartForTUI reads this machine's start-at-sign-in switch; known is false
// where it cannot be read or changed (macOS has no support yet).
func (a *App) autostartForTUI() (on, known bool) {
	if runtime.GOOS == "darwin" {
		return false, false
	}
	on, where := a.autostartState()
	return on, where != "unknown"
}

func (a *App) tuiSetAutostart(on bool) error {
	defer a.forgetSlow()
	m := a.autostartManager()
	var err error
	if on {
		err = m.Enable()
	} else {
		err = m.Disable()
	}
	if err != nil {
		return localErr(ExitInternal, "autostart_failed", err.Error())
	}
	return nil
}

// ---- protection: mirror, snapshots and restore ----

// protectView is the JSON of GET/PUT .../protection.
type protectView struct {
	CollectionID string `json:"collection_id"`
	Revision     int    `json:"revision"`
	Options      struct {
		Mirror    bool `json:"mirror"`
		Snapshots bool `json:"snapshots"`
		KeepDays  int  `json:"keep_days"`
		KeepCount int  `json:"keep_count"`
	} `json:"options"`
}

func (v protectView) tui() tui.ProtectionView {
	return tui.ProtectionView{CollectionID: v.CollectionID, Revision: v.Revision,
		Options: tui.ProtectionOptions{Mirror: v.Options.Mirror, Snapshots: v.Options.Snapshots,
			KeepDays: v.Options.KeepDays, KeepCount: v.Options.KeepCount}}
}

// snapshotView is one restore point (GET .../snapshots).
type snapshotView struct {
	ID        string    `json:"id"`
	State     string    `json:"state"`
	Pinned    bool      `json:"pinned"`
	Revision  int       `json:"revision"`
	Items     int       `json:"items"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (a *App) tuiProtection(ctx context.Context, collectionID string) (tui.ProtectionView, error) {
	r, err := a.call(ctx, "GET", "/v1/collections/"+url.PathEscape(collectionID)+"/protection", nil, nil, "")
	if err != nil {
		return tui.ProtectionView{}, err
	}
	var v protectView
	if err = r.Decode(&v); err != nil {
		return tui.ProtectionView{}, err
	}
	return v.tui(), nil
}

func (a *App) tuiSetProtection(ctx context.Context, collectionID string, revision int, opts tui.ProtectionOptions) (tui.ProtectionView, error) {
	body := map[string]any{"expected_revision": revision, "options": map[string]any{
		"mirror": opts.Mirror, "snapshots": opts.Snapshots, "keep_days": opts.KeepDays, "keep_count": opts.KeepCount}}
	r, err := a.call(ctx, "PUT", "/v1/collections/"+url.PathEscape(collectionID)+"/protection", nil, body, NewKey())
	if err != nil {
		return tui.ProtectionView{}, err
	}
	var v protectView
	if err = r.Decode(&v); err != nil {
		return tui.ProtectionView{}, err
	}
	return v.tui(), nil
}

// maxTUISnapshots bounds the new snapshot page the screen reads.
const maxTUISnapshots = 100

func (a *App) tuiSnapshots(ctx context.Context, collectionID string) ([]tui.SnapshotInfo, error) {
	r, err := a.call(ctx, "GET", "/v1/collections/"+url.PathEscape(collectionID)+"/snapshots",
		url.Values{"limit": {strconv.Itoa(maxTUISnapshots)}}, nil, "")
	if err != nil {
		return nil, err
	}
	var page struct {
		Items []snapshotView `json:"items"`
	}
	if err = r.Decode(&page); err != nil {
		return nil, err
	}
	out := make([]tui.SnapshotInfo, 0, len(page.Items))
	for _, s := range page.Items {
		out = append(out, tui.SnapshotInfo{ID: s.ID, State: s.State, Pinned: s.Pinned, Revision: s.Revision,
			Items: s.Items, CreatedAt: s.CreatedAt, ExpiresAt: s.ExpiresAt})
	}
	return out, nil
}

func (a *App) tuiCreateSnapshot(ctx context.Context, collectionID string, revision int) (string, error) {
	r, err := a.call(ctx, "POST", "/v1/collections/"+url.PathEscape(collectionID)+"/snapshots", nil,
		map[string]any{"expected_revision": revision}, NewKey())
	if err != nil {
		return "", err
	}
	var j jobView
	if err = r.Decode(&j); err != nil {
		return "", err
	}
	return j.ID, nil
}

func (a *App) tuiPinSnapshot(ctx context.Context, snapshotID string, revision int, pin bool) error {
	method := "DELETE"
	if pin {
		method = "POST"
	}
	_, err := a.call(ctx, method, "/v1/snapshots/"+url.PathEscape(snapshotID)+"/pin", nil,
		map[string]any{"expected_revision": revision}, NewKey())
	return err
}

// tuiRestoreSnapshot admits the dry run (dryRun true) or applies it.
func (a *App) tuiRestoreSnapshot(ctx context.Context, snapshotID string, revision int, dryRun bool) (string, error) {
	r, err := a.call(ctx, "POST", "/v1/snapshots/"+url.PathEscape(snapshotID)+"/restore", nil,
		map[string]any{"expected_revision": revision, "dry_run": dryRun}, NewKey())
	if err != nil {
		return "", err
	}
	var j jobView
	if err = r.Decode(&j); err != nil {
		return "", err
	}
	return j.ID, nil
}

// tuiRestorePlan reads the first page of a dry-run plan; Next stays set when
// there are more actions, so the screen never claims a total it did not read.
func (a *App) tuiRestorePlan(ctx context.Context, jobID string) (tui.RestorePlan, error) {
	r, err := a.call(ctx, "GET", "/v1/jobs/"+url.PathEscape(jobID)+"/restore-plan", url.Values{"limit": {"100"}}, nil, "")
	if err != nil {
		return tui.RestorePlan{}, err
	}
	var page struct {
		Next  *string `json:"next_cursor"`
		Items []struct {
			Ordinal int    `json:"ordinal"`
			Action  string `json:"action"`
			State   string `json:"state"`
			Target  struct {
				Path string `json:"path"`
				Name string `json:"name"`
			} `json:"target"`
		} `json:"items"`
	}
	if err = r.Decode(&page); err != nil {
		return tui.RestorePlan{}, err
	}
	out := tui.RestorePlan{}
	if page.Next != nil {
		out.Next = *page.Next
	}
	for _, it := range page.Items {
		out.Actions = append(out.Actions, tui.RestoreAction{Ordinal: it.Ordinal, Action: it.Action, State: it.State,
			Path: it.Target.Path, Name: it.Target.Name})
	}
	return out, nil
}
