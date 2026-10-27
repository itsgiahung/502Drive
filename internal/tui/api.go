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
	Language string
	Timezone string
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
}

// preset names shared with the CLI sync presets.
const (
	presetDaily    = "daily"
	presetHourly   = "hourly"
	presetSixHour  = "sixhourly"
	presetRealtime = "realtime"
)
