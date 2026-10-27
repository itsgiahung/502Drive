package state

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/itsgiahung/502Drive/internal/filter"
)

var (
	ErrMismatch             = errors.New("idempotency_mismatch")
	ErrNotFound             = errors.New("not_found")
	ErrStale                = errors.New("stale_revision")
	ErrExpired              = errors.New("plan_expired")
	ErrTransition           = errors.New("invalid_transition")
	ErrCapacity             = errors.New("capacity_exceeded")
	ErrPlanChanged          = errors.New("plan_changed")
	ErrCollectionNotRetired = errors.New("collection_not_retired")
	ErrInvalidMove          = errors.New("invalid_move")
)

type ErrCollectionBusy struct {
	ActiveJobID string
}

func (e ErrCollectionBusy) Error() string {
	return "collection_busy: " + e.ActiveJobID
}

// Job states, shared by every adapter.
const (
	JobQueued      = "queued"
	JobRunning     = "running"
	JobPausing     = "pausing"
	JobPaused      = "paused"
	JobCancelling  = "cancelling"
	JobReconciling = "reconciling"
	JobBlocked     = "blocked"
	JobSucceeded   = "succeeded"
	JobPartial     = "partial"
	JobFailed      = "failed"
	JobCancelled   = "cancelled"
)

// Item states.
const (
	ItemPending     = "pending"
	ItemExecuting   = "executing"
	ItemReconciling = "reconciling"
	ItemDone        = "done"
	ItemBlocked     = "blocked"
	ItemFailed      = "failed"
	ItemSkipped     = "skipped"
	ItemCancelled   = "cancelled"
)

// Skipped reasons (stable codes for skipped items).
const (
	SkipShortcutUnresolved = "shortcut_unresolved"
	SkipShortcutLoop       = "shortcut_loop"
	SkipCannotCopy         = "cannot_copy"
	SkipGoogleNativeExport = "google_native_export_only"
	SkipTooLargeForQuota   = "too_large_for_quota"
	SkipUnreadable         = "unreadable"
	SkipTrashedSource      = "trashed_source"
	SkipFiltered           = "filtered" // reserved for S5
	SkipConflict           = "conflict"
)

// Item operations. create writes a new object; align renames and/or moves an
// object we already own to match the source, and is idempotent.
const (
	OpCreate = "create"
	OpAlign  = "align"
)

const (
	KindWrapper  = "wrapper"
	KindFolder   = "folder"
	KindFile     = "file"
	KindShortcut = "shortcut"
)

func ID(prefix string) string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return prefix + "_" + hex.EncodeToString(b[:])
}

type Plan struct {
	ID                  string      `json:"id"`
	Principal           string      `json:"principal_id"`
	SourceID            string      `json:"source_id"`
	SourceName          string      `json:"source_name"`
	SourceKind          string      `json:"source_kind"` // folder or file
	SourceMime          string      `json:"source_mime,omitempty"`
	SourceSize          *int64      `json:"source_size,omitempty"`
	SourceVersion       string      `json:"source_version"`
	ResourceKey         string      `json:"resource_key,omitempty"`
	DestinationID       string      `json:"destination_id"`
	DestinationFolder   string      `json:"destination_folder"`
	DestinationRevision int         `json:"destination_revision"`
	RootName            string      `json:"root_name"`
	Shortcuts           string      `json:"shortcuts,omitempty"`
	OnConflict          string      `json:"on_conflict,omitempty"`
	Filter              filter.Spec `json:"filter,omitempty"`
	ExpiresAt           time.Time   `json:"expires_at"`
	Revision            int         `json:"revision"`
	// Deep is the read-only deep scan of the source (S7), when requested.
	Deep *DeepPreflight `json:"deep,omitempty"`
}

// DeepPreflight is the result of a read-only scan of a plan's whole source
// tree (S7). State is running, complete, cancelled, expired, failed, or
// interrupted (the daemon stopped during the scan). Only a complete scan is
// Complete; partial counts are never presented as a total.
type DeepPreflight struct {
	State         string    `json:"state"`
	Complete      bool      `json:"complete"`
	Folders       int       `json:"folders"`
	Files         int       `json:"files"`
	Bytes         int64     `json:"bytes"`
	Native        int       `json:"native_files"`
	Shortcuts     int       `json:"shortcuts"`
	CannotCopy    int       `json:"cannot_copy"`
	Unreadable    int       `json:"unreadable_folders"`
	Scanned       int       `json:"scanned"`
	EstimateSecs  *int64    `json:"estimate_seconds,omitempty"`
	EstimateBasis string    `json:"estimate_basis"`
	StorageFree   *int64    `json:"storage_free_bytes,omitempty"`
	DailyLeft     *int64    `json:"daily_copy_left_bytes,omitempty"`
	Warnings      []string  `json:"warnings"`
	Error         string    `json:"error,omitempty"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type Counts struct {
	Observed      int            `json:"observed_items"`
	Total         *int           `json:"total_items"`
	Completed     int            `json:"completed_items"`
	Failed        int            `json:"failed_items"`
	Blocked       int            `json:"blocked_items"`
	Skipped       int            `json:"skipped_items"`
	Bytes         int64          `json:"logical_bytes"`
	SkippedByCode map[string]int `json:"skipped_by_code,omitempty"`
}

type Failure struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Job struct {
	QueuePosition *int   `json:"-"`
	QueueAhead    *int   `json:"-"`
	QueueIsPaused bool   `json:"-"`
	ID            string `json:"id"`
	Principal     string `json:"principal_id"`
	CollectionID  string `json:"collection_id"`
	Trigger       string `json:"trigger"`
	// Kind distinguishes the job families: "" is a copy, "audit" scans a
	// collection read-only, "repair" applies the fixes of an audit.
	Kind          string      `json:"kind,omitempty"`
	AuditID       string      `json:"audit_id,omitempty"`
	DryRun        bool        `json:"dry_run,omitempty"`
	SourceName    string      `json:"source_name"`
	DestinationID string      `json:"destination_id"`
	RootName      string      `json:"root_name"`
	State         string      `json:"state"`
	Phase         *string     `json:"phase"`
	DesiredState  string      `json:"desired_state"`
	Shortcuts     string      `json:"shortcuts,omitempty"`
	OnConflict    string      `json:"on_conflict,omitempty"`
	Filter        filter.Spec `json:"filter,omitempty"`
	Revision      int         `json:"revision"`
	Counts        Counts      `json:"counts"`
	ResultURL     *string     `json:"result_url"`
	LastError     *Failure    `json:"last_error"`
	NextAttemptAt *int64      `json:"next_attempt_at,omitempty"`
	CreatedAt     time.Time   `json:"created_at"`
	UpdatedAt     time.Time   `json:"updated_at"`
	// SourceRemoved counts mapping items that a delta scan no longer found at
	// the source. It is a body-only field (no column) used by the mass-removal
	// guard to ask for an audit.
	SourceRemoved int `json:"source_removed,omitempty"`
	// ResumeAfter is the instant a quota-blocked job may continue on its own.
	// It is body-only: the engine scans blocked jobs for it.
	ResumeAfter *time.Time `json:"resume_after,omitempty"`
	// BatchID groups the links of one "copy many" request (S6). Empty for a
	// lone job. Stored in the body; batch_id/fair_group columns are generated.
	BatchID string `json:"batch_id,omitempty"`
	// Priority orders queued jobs (-5..5, default 0; higher runs first).
	Priority int `json:"priority"`
	// QueueOrder is the FIFO position among queued jobs of the same priority.
	QueueOrder int64 `json:"queue_order,omitempty"`
	// QueuePaused marks a job that the global queue pause moved to pausing or
	// paused; only the queue resume (not a user action) may resume it.
	QueuePaused bool `json:"queue_paused,omitempty"`
	// AfterCopySync asks for a sync subscription on the collection this job
	// creates, once the initial copy succeeds (S8). Empty means "use the
	// principal's preference"; "off" means never.
	AfterCopySync string `json:"after_copy_sync_mode,omitempty"`
}

// Collection states.
const (
	CollectionCopying = "copying"
	CollectionReady   = "ready"
	CollectionRetired = "retired"
)

// Additional job kinds.
const (
	JobKindCleanup = "cleanup"
)

type Collection struct {
	ID            string      `json:"id"`
	SourceID      string      `json:"source_id"`
	SourceName    string      `json:"source_name"`
	SourceKind    string      `json:"source_kind"`
	SourceMime    string      `json:"source_mime,omitempty"`
	ResourceKey   string      `json:"resource_key,omitempty"`
	DestinationID string      `json:"destination_id"`
	RootName      string      `json:"root_name"`
	RootID        *string     `json:"destination_root_id"`
	ResultURL     *string     `json:"result_url"`
	BaselineJobID string      `json:"baseline_job_id"`
	State         string      `json:"state"`
	RetiredAt     *time.Time  `json:"retired_at,omitempty"`
	Revision      int         `json:"revision"`
	Sync          any         `json:"sync"`
	Shortcuts     string      `json:"shortcuts,omitempty"`
	OnConflict    string      `json:"on_conflict,omitempty"`
	Filter        filter.Spec `json:"filter,omitempty"`
}

type Event struct {
	Sequence  int64     `json:"sequence"`
	JobID     string    `json:"job_id"`
	Kind      string    `json:"kind"`
	Revision  int       `json:"revision"`
	Timestamp time.Time `json:"timestamp"`
	Data      any       `json:"data"`
}

type Result struct {
	Job      Job
	Replayed bool
}

type Destination struct {
	ID          string    `json:"id"`
	FolderID    string    `json:"folder_id"`
	ResourceKey string    `json:"-"`
	DriveID     string    `json:"drive_id,omitempty"`
	Name        string    `json:"name"`
	Revision    int       `json:"revision"`
	IsDefault   bool      `json:"is_default"`
	CreatedAt   time.Time `json:"created_at"`
}

// Item is one source object and, at the same time, its write intent.
type Item struct {
	CollectionID      string
	SourceID          string
	JobID             string
	ParentSourceID    string
	Kind              string
	Name              string
	MimeType          string
	ResourceKey       string
	Size              *int64
	ShortcutTarget    string
	ShortcutTargetMim string
	Fingerprint       string
	Generation        int
	IntentKey         string
	State             string
	DestParentID      string
	DestID            string
	Attempts          int
	NextAttemptAt     int64
	ErrorCode         string
	ListState         string
	PageToken         string
	Op                string
	SeenJob           string
	ScanJob           string
	OldDestID         string
	ConflictAction    string
	DestName          string
	// RelPath is the path below the copied root ("a/b/c.txt"), stored so a
	// filter's path rules keep working after a restart.
	RelPath string
	// Modified is the source modification time seen while listing. It is not
	// stored: the filter decides during discovery, before the item is written.
	Modified time.Time
}

// TargetName is the name the copy should carry at the destination: the name the
// conflict policy decided (for example "a (2).txt"), or the source name when no
// decision was needed.
func (it Item) TargetName() string {
	if it.DestName != "" {
		return it.DestName
	}
	return it.Name
}
