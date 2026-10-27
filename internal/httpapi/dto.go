package httpapi

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/drive"
	"github.com/itsgiahung/502Drive/internal/schedule"
	"github.com/itsgiahung/502Drive/internal/state"
)

func ts(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func tsPtr(t *time.Time) any {
	if t == nil {
		return nil
	}
	return ts(*t)
}

func kindDTO(kind, mime string) string {
	switch {
	case kind == state.KindFolder:
		return "folder"
	case mime == drive.MimeShortcut:
		return "shortcut"
	case strings.HasPrefix(mime, "application/vnd.google-apps."):
		return "google_native"
	}
	return "binary"
}

func failureDTO(f *state.Failure) any {
	if f == nil {
		return nil
	}
	return map[string]string{"code": f.Code, "message": f.Message}
}

func jobDTO(j state.Job) map[string]any {
	out := map[string]any{
		"id": j.ID, "principal_id": j.Principal, "collection_id": j.CollectionID, "source_name": j.SourceName,
		"destination_id": j.DestinationID, "root_name": j.RootName, "state": j.State, "phase": j.Phase,
		"desired_state": j.DesiredState, "revision": j.Revision, "counts": j.Counts, "result_url": j.ResultURL,
		"last_error": failureDTO(j.LastError), "created_at": ts(j.CreatedAt), "updated_at": ts(j.UpdatedAt),
		"resume_after": tsPtr(j.ResumeAfter),
	}
	if j.Kind != "" {
		out["kind"] = j.Kind
		if j.AuditID != "" {
			out["audit_id"] = j.AuditID
		}
		if j.DryRun {
			out["dry_run"] = true
		}
	}
	return out
}

var eventText = map[string]string{
	"admitted": "Job accepted.", "started": "Job started.", "phase": "Phase changed.", "paused": "Job paused.", "pause": "Pause requested.",
	"resume": "Job resumed.", "cancel": "Cancel requested.", "cancelled": "Job cancelled.", "retry": "Retry requested.",
	"blocked": "Job needs attention.", "finished": "Job finished.",
}

func eventDTO(e state.Event) map[string]any {
	msg := eventText[e.Kind]
	if msg == "" {
		msg = fmt.Sprintf("Event %s.", e.Kind)
	}
	return map[string]any{"sequence": e.Sequence, "job_id": e.JobID, "kind": e.Kind, "revision": e.Revision, "message": msg, "occurred_at": ts(e.Timestamp)}
}

func planDTO(v app.PlanView) map[string]any {
	return map[string]any{
		"id": v.ID, "revision": v.Revision, "expires_at": ts(v.ExpiresAt), "source_id": v.SourceID, "source_name": v.SourceName,
		"source_kind": kindDTO(v.SourceKind, v.SourceMime), "destination_id": v.DestinationID, "destination_revision": v.DestinationRevision,
		"root_name": v.RootName, "warnings": v.Warnings,
		"preview": map[string]any{"observed_files": v.Preview.ObservedFiles, "observed_folders": v.Preview.ObservedFolders, "scan_complete": v.Preview.ScanComplete},
	}
}

func destinationDTO(d state.Destination) map[string]any {
	var drv any
	if d.DriveID != "" {
		drv = d.DriveID
	}
	return map[string]any{"id": d.ID, "name": d.Name, "folder_id": d.FolderID, "drive_id": drv, "revision": d.Revision, "is_default": d.IsDefault}
}

func accountDTO(a app.AccountView) map[string]any {
	return map[string]any{"id": "default", "status": a.Status, "label": a.Label, "last_error": failureDTO(a.LastError)}
}

func authSessionDTO(s drive.AuthSession) map[string]any {
	st := s.State
	if st == drive.SessionVerifying {
		st = drive.SessionPending
	}
	var u any
	if st == drive.SessionPending && s.AuthorizationURL != "" {
		u = s.AuthorizationURL
	}
	return map[string]any{"id": s.ID, "state": st, "authorization_url": u, "expires_at": ts(s.ExpiresAt)}
}

func prefsDTO(p state.Preferences) map[string]any {
	var dest, rev any
	if p.DestinationID != "" {
		dest = p.DestinationID
	}
	if p.DestinationRevision != nil {
		rev = *p.DestinationRevision
	}
	return map[string]any{"language": p.Language, "timezone": p.Timezone, "auto_copy": p.AutoCopy, "destination_id": dest, "destination_revision": rev,
		"notify_completed": p.NotifyCompleted, "notify_failed": p.NotifyFailed, "notify_action_required": p.NotifyActionRequired,
		"after_copy_sync_mode": p.AfterCopySyncMode, "after_copy_schedule": nil}
}

func operatorDTO(o state.Operator) map[string]any {
	return map[string]any{"id": o.ID, "telegram_user_id": o.TelegramUserID, "enabled": o.Enabled, "role": "operator"}
}

func collectionState(s string) string {
	switch s {
	case "copying", "ready", "partial", "needs_action", "legacy_read_only":
		return s
	case "blocked", "failed":
		return "needs_action"
	}
	return "partial" // cancelled or unknown: output exists but is incomplete
}

// scheduleDTO mirrors the ScheduleSpec oneOf: only the fields of the chosen
// kind are emitted, because the schema forbids the others.
func scheduleDTO(sp schedule.Spec) map[string]any {
	switch sp.Kind {
	case schedule.KindInterval:
		return map[string]any{"kind": sp.Kind, "interval_minutes": sp.IntervalMinutes, "anchor_at": ts(sp.AnchorAt)}
	case schedule.KindDaily:
		return map[string]any{"kind": sp.Kind, "timezone": sp.Timezone, "local_time": sp.LocalTime}
	case schedule.KindWeekly:
		return map[string]any{"kind": sp.Kind, "timezone": sp.Timezone, "local_time": sp.LocalTime, "weekdays": sp.Weekdays}
	}
	return map[string]any{"kind": sp.Kind}
}

func optionalTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return ts(*t)
}

func syncDTO(v app.SyncView) map[string]any {
	sub := v.Subscription
	var sched any
	if sub.Schedule != nil {
		sched = scheduleDTO(*sub.Schedule)
	}
	var active any
	if v.ActiveJobID != "" {
		active = v.ActiveJobID
	}
	return map[string]any{
		"id": sub.ID, "collection_id": sub.CollectionID, "mode": sub.Mode, "state": sub.State, "revision": sub.Revision,
		"schedule": sched, "content_policy": sub.ContentPolicy, "deletion_policy": "preserve_destination",
		"last_attempt_at": optionalTime(sub.LastAttemptAt), "last_success_at": optionalTime(sub.LastSuccessAt),
		"next_run_at": optionalTime(sub.NextDue), "active_job_id": active, "last_error": failureDTO(sub.LastError),
	}
}

// collectionDTO renders a collection with its real subscription. sync_mode is
// a filter-only field; callers strip it before writing the response.
func (s *Server) collectionDTO(ctx context.Context, c state.Collection) (map[string]any, error) {
	view, configured, err := s.Svc.SyncStatus(ctx, c.ID)
	if err != nil {
		return nil, err
	}
	var sync any
	if configured {
		sync = syncDTO(view)
	}
	return map[string]any{
		"id": c.ID, "source_id": c.SourceID, "source_name": c.SourceName, "source_kind": kindDTO(c.SourceKind, c.SourceMime),
		"destination_id": c.DestinationID, "destination_root_id": c.RootID, "result_url": c.ResultURL, "baseline_job_id": c.BaselineJobID,
		"state": collectionState(c.State), "revision": c.Revision, "sync": sync,
	}, nil
}
