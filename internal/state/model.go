package state

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"
)

var (
	ErrMismatch    = errors.New("idempotency_mismatch")
	ErrNotFound    = errors.New("not_found")
	ErrStale       = errors.New("stale_revision")
	ErrExpired     = errors.New("plan_expired")
	ErrTransition  = errors.New("invalid_transition")
	ErrCapacity    = errors.New("capacity_exceeded")
	ErrPlanChanged = errors.New("plan_changed")
)

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
	ID                  string    `json:"id"`
	Principal           string    `json:"principal_id"`
	SourceID            string    `json:"source_id"`
	SourceName          string    `json:"source_name"`
	SourceKind          string    `json:"source_kind"` // folder or file
	SourceMime          string    `json:"source_mime,omitempty"`
	SourceSize          *int64    `json:"source_size,omitempty"`
	SourceVersion       string    `json:"source_version"`
	ResourceKey         string    `json:"resource_key,omitempty"`
	DestinationID       string    `json:"destination_id"`
	DestinationFolder   string    `json:"destination_folder"`
	DestinationRevision int       `json:"destination_revision"`
	RootName            string    `json:"root_name"`
	ExpiresAt           time.Time `json:"expires_at"`
	Revision            int       `json:"revision"`
}

type Counts struct {
	Observed  int   `json:"observed_items"`
	Total     *int  `json:"total_items"`
	Completed int   `json:"completed_items"`
	Failed    int   `json:"failed_items"`
	Blocked   int   `json:"blocked_items"`
	Skipped   int   `json:"skipped_items"`
	Bytes     int64 `json:"logical_bytes"`
}

type Failure struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Job struct {
	ID           string `json:"id"`
	Principal    string `json:"principal_id"`
	CollectionID string `json:"collection_id"`
	Trigger      string `json:"trigger"`
	// Kind distinguishes the job families: "" is a copy, "audit" scans a
	// collection read-only, "repair" applies the fixes of an audit.
	Kind          string    `json:"kind,omitempty"`
	AuditID       string    `json:"audit_id,omitempty"`
	DryRun        bool      `json:"dry_run,omitempty"`
	SourceName    string    `json:"source_name"`
	DestinationID string    `json:"destination_id"`
	RootName      string    `json:"root_name"`
	State         string    `json:"state"`
	Phase         *string   `json:"phase"`
	DesiredState  string    `json:"desired_state"`
	Revision      int       `json:"revision"`
	Counts        Counts    `json:"counts"`
	ResultURL     *string   `json:"result_url"`
	LastError     *Failure  `json:"last_error"`
	NextAttemptAt *int64    `json:"next_attempt_at,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	// SourceRemoved counts mapping items that a delta scan no longer found at
	// the source. It is a body-only field (no column) used by the mass-removal
	// guard to ask for an audit.
	SourceRemoved int `json:"source_removed,omitempty"`
	// ResumeAfter is the instant a quota-blocked job may continue on its own.
	// It is body-only: the engine scans blocked jobs for it.
	ResumeAfter *time.Time `json:"resume_after,omitempty"`
}

type Collection struct {
	ID            string  `json:"id"`
	SourceID      string  `json:"source_id"`
	SourceName    string  `json:"source_name"`
	SourceKind    string  `json:"source_kind"`
	SourceMime    string  `json:"source_mime,omitempty"`
	ResourceKey   string  `json:"resource_key,omitempty"`
	DestinationID string  `json:"destination_id"`
	RootName      string  `json:"root_name"`
	RootID        *string `json:"destination_root_id"`
	ResultURL     *string `json:"result_url"`
	BaselineJobID string  `json:"baseline_job_id"`
	State         string  `json:"state"`
	Revision      int     `json:"revision"`
	Sync          any     `json:"sync"`
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
}
