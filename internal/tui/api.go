// Package tui is the interactive terminal UI behind `502drive menu`. It is a
// thin client of the HTTP API: the CLI supplies the actions, the model only
// renders state and forwards fixed commands.
package tui

import (
	"context"
	"time"
)

// Status is the subset of /v1/status the TUI shows.
type Status struct {
	Version       string
	Ready         bool
	Running       int
	Queued        int
	Blocked       int
	AccountStatus string
	BotState      string
	// Throttle is Google rate-limiting; zero values mean none.
	Workers     int
	Throttled5m int
	RetryAt     time.Time
	// Lifecycle is the daemon's power/network state: running, suspending,
	// sleeping, resuming or reconnect_required. Empty means "not reported".
	Lifecycle       string
	LifecycleOnline bool
	GapSeconds      int
}

// Job is one job row; counts are real, never estimated.
type Job struct {
	ID           string
	CollectionID string
	SourceName   string
	State        string
	Phase        string
	Desired      string
	Revision     int
	Completed    int
	Failed       int
	Blocked      int
	Skipped      int
	Total        *int
	ResultURL    string
	LastError    string
	// Kind is empty for a copy or sync run, else audit, repair or cleanup.
	Kind string
	// ResumeAfter is when a quota-blocked job continues by itself (zero: not waiting).
	ResumeAfter time.Time
	// SkippedByCode counts skipped items by their stable reason code.
	SkippedByCode map[string]int
	Bytes         int64
}

// SyncState is the subscription shown on a library card.
type SyncState struct {
	Mode        string
	State       string
	Revision    int
	Schedule    string
	NextRun     string
	LastSuccess string
	ActiveJob   string
	LastError   string
}

// Collection is a copied source with its optional subscription.
type Collection struct {
	ID         string
	SourceName string
	State      string
	Revision   int
	ResultURL  string
	Sync       *SyncState
	RootName   string
	Filter     FilterRules
	OnConflict string
}

// FilterRules is the selective-sync rule of a collection (read-only here).
type FilterRules struct {
	Include, Exclude []string
	MinSize, MaxSize int64
}

// Active reports whether any rule would drop something.
func (f FilterRules) Active() bool {
	return len(f.Include) > 0 || len(f.Exclude) > 0 || f.MinSize > 0 || f.MaxSize > 0
}

// Destination is one saved target folder.
type Destination struct {
	ID       string
	Name     string
	Revision int
	Default  bool
}

// Account is the Google account summary.
type Account struct {
	Status  string
	Label   string
	Problem string
}

// Check is one doctor result.
type Check struct {
	Name    string
	Status  string
	Message string
}

// Prefs are the per-principal settings the TUI can change.
type Prefs struct {
	Language string // auto, vi or en
	Timezone string
	// Notifications and quiet hours (QuietStart/QuietEnd are HH:MM or empty).
	NotifyCompleted, NotifyFailed, NotifyAction bool
	QuietStart, QuietEnd                        string
	// RetentionDays is the history window; 0 keeps everything.
	RetentionDays int
	// Autostart is this machine's start-at-sign-in switch (not a daemon
	// preference); AutostartKnown is false where it cannot be read.
	Autostart, AutostartKnown bool
}

// Snapshot is everything one refresh reads.
type Snapshot struct {
	Status       Status
	Jobs         []Job
	Collections  []Collection
	Destinations []Destination
	Account      Account
	Prefs        Prefs
	Checks       []Check
	At           time.Time
}

// LoginSession is a Google sign-in the daemon started: the user opens URL.
type LoginSession struct {
	ID  string
	URL string
}

// DriveItem is one file or folder in the Drive browser. Kind is folder, binary,
// google_native or shortcut.
type DriveItem struct {
	ID, Name, Kind, Mime string
	Size                 *int64
	Modified, Created    time.Time
	Owner, WebURL        string
	CanCopy, CanWrite    bool
}

func (d DriveItem) IsFolder() bool { return d.Kind == "folder" }

// FolderPage is one page of a folder with its breadcrumb (first page only).
type FolderPage struct {
	Folder DriveItem
	Path   []DriveItem
	Items  []DriveItem
	Next   string
}

// Plan is the read-only preview of a copy. RootName is what the output folder
// will be called.
type Plan struct {
	ID, SourceID, SourceName, SourceKind string
	DestinationID, RootName              string
	Revision                             int
	Files, Folders                       int
	Complete                             bool
	Warnings                             []string
	// Filter is the preview of the selective-sync rule; nil when none applies.
	Filter *PlanFilter
}

// PlanFilter is what the rule would keep and drop in the part of the tree the
// preview looked at (the top level only, see Scope).
type PlanFilter struct {
	Included, Excluded               int
	IncludedSamples, ExcludedSamples []string
	Scope                            string
}

// CopyOptions are the choices that travel with a preview. Empty means the
// daemon default (follow shortcuts, rename on a name clash, no filter).
type CopyOptions struct {
	Shortcuts    string // follow or skip
	OnConflict   string // skip, rename or update
	FilterPreset string // "", no_video, docs_only or under_1gb
}

// ExistingCopyError is returned by SubmitMode when the same source is already
// copied to the same destination and no mode was given (HTTP 409
// already_copied). The dialog then asks: update the old copy or make a new one.
type ExistingCopyError struct{ Message string }

func (e *ExistingCopyError) Error() string { return e.Message }

// Finding is one difference an integrity audit found.
type Finding struct {
	Kind, SourceID, DestID, Detail string
	Repairable, Repaired           bool
}

// AuditReport is one integrity audit with its (first) findings. Counts holds
// every finding by kind; Findings is only the first page.
type AuditReport struct {
	ID, CollectionID, JobID, State string
	Revision, Scanned              int
	Counts                         map[string]int
	Findings                       []Finding
	// Repairable counts the repairable findings that were read.
	Repairable int
}

// Total is the number of differences found.
func (r AuditReport) Total() int {
	n := 0
	for _, c := range r.Counts {
		n += c
	}
	return n
}

// WatchEvent is one signal from the daemon's realtime stream. Connected tells
// whether the stream is up; Changed asks the model to re-read what it shows.
type WatchEvent struct {
	Connected bool
	Changed   bool
}

// Actions is the API surface the model needs. The CLI implements it with
// internal/cli.Client; tests implement it with fakes.
type Actions struct {
	Snapshot              func(ctx context.Context) (Snapshot, error)
	JobAction             func(ctx context.Context, id, action string, revision int) error
	SyncPreset            func(ctx context.Context, collectionID, preset string) error
	SyncAction            func(ctx context.Context, collectionID, action string) error
	RunNow                func(ctx context.Context, collectionID string) error
	SetDefaultDestination func(ctx context.Context, id string, revision int) error
	Doctor                func(ctx context.Context) ([]Check, error)
	SetLanguage           func(ctx context.Context, lang string) error
	// Login starts a Google sign-in; LoginState reports pending, connected,
	// failed or expired; OpenURL tries to open the browser (nil or an error
	// just means the user opens the link by hand).
	Login      func(ctx context.Context) (LoginSession, error)
	LoginState func(ctx context.Context, id string) (string, error)
	OpenURL    func(url string) error
	// Drive browser and copy flow. Browse takes a folder id ("root" = My Drive)
	// and an optional page token; Inspect is read-only; Submit starts the job
	// and returns its id; AddDestination returns the existing destination of a
	// folder or creates one.
	Browse         func(ctx context.Context, folderID, pageToken string) (FolderPage, error)
	Mkdir          func(ctx context.Context, parentID, name string) (DriveItem, error)
	Inspect        func(ctx context.Context, source, destinationID, rootName string) (Plan, error)
	Submit         func(ctx context.Context, plan Plan) (jobID, collectionID string, err error)
	AddDestination func(ctx context.Context, folderID string) (Destination, error)
	// Watch opens the realtime stream; it reconnects by itself and closes its
	// channel when ctx ends. Nil means the model polls.
	Watch func(ctx context.Context) <-chan WatchEvent

	// Copy dialog with options. InspectWith previews with the shortcut, name
	// clash and filter choices; SubmitMode starts the job and answers an
	// existing copy ("" asks: ExistingCopyError; "update" refreshes the old
	// copy; "new" copies again). Nil falls back to Inspect and Submit.
	InspectWith func(ctx context.Context, source, destinationID, rootName string, opt CopyOptions) (Plan, error)
	SubmitMode  func(ctx context.Context, plan Plan, mode string) (jobID, collectionID string, err error)

	// Integrity audit (S1): StartAudit admits the read-only scan, GetAudit
	// reads the report, RepairAudit admits a repair job (dryRun plans only).
	StartAudit  func(ctx context.Context, collectionID string) (AuditReport, error)
	GetAudit    func(ctx context.Context, auditID string) (AuditReport, error)
	RepairAudit func(ctx context.Context, auditID string, revision int, dryRun bool) (jobID string, err error)

	// Collection lifecycle (L1). action is retire, resume or forget; Cleanup
	// admits a cleanup job (dryRun plans only; a real one needs the typed name).
	CollectionLifecycle func(ctx context.Context, collectionID, action string, revision int) error
	Cleanup             func(ctx context.Context, collectionID string, revision int, dryRun bool, confirmName string) (jobID string, err error)
	// SetFilterPreset replaces a collection's filter with a preset (S5).
	SetFilterPreset func(ctx context.Context, collectionID, preset string, revision int) error
	// ShowRetired makes the next snapshots include retired collections.
	ShowRetired func(on bool)

	// Settings. SetPreference changes one daemon preference by its API name
	// (notify_completed, notify_failed, notify_action_required, quiet_hours,
	// retention_days); SetAutostart is this machine's start-at-sign-in switch.
	SetPreference func(ctx context.Context, field string, value any) error
	SetAutostart  func(on bool) error
}

// preset names shared with the CLI sync presets.
const (
	presetDaily    = "daily"
	presetHourly   = "hourly"
	presetSixHour  = "sixhourly"
	presetRealtime = "realtime"
)
