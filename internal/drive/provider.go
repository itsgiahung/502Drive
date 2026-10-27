package drive

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	MimeFolder   = "application/vnd.google-apps.folder"
	MimeShortcut = "application/vnd.google-apps.shortcut"
	// IntentProperty is the private appProperties key used to find our own
	// writes again after an unknown outcome.
	IntentProperty = "502drive_intent"
)

// File is the subset of Drive metadata the product relies on.
type File struct {
	ID                 string
	Name               string
	MimeType           string
	Parents            []string
	Size               *int64
	MD5                string
	Version            string
	ResourceKey        string
	DriveID            string
	Trashed            bool
	ShortcutTargetID   string
	ShortcutTargetMime string
	ShortcutTargetKey  string
	CanCopy            bool
	CanAddChildren     bool
	AppProperties      map[string]string
	// Display metadata (not used by the copy engine).
	Created    time.Time
	Modified   time.Time
	OwnerName  string
	OwnerEmail string
	WebLink    string
}

func (f File) IsFolder() bool   { return f.MimeType == MimeFolder }
func (f File) IsShortcut() bool { return f.MimeType == MimeShortcut }

// Fingerprint changes when the content, name or version of a source changes.
func (f File) Fingerprint() string {
	return fmt.Sprintf("v%s|%s|%s", f.Version, f.MD5, f.Name)
}

type Account struct {
	Email string
	Name  string
}

// Provider is the seam between the copy engine and Google Drive. Copy and
// folder creation are server-side; there is deliberately no download/upload.
type Provider interface {
	Account(ctx context.Context) (Account, error)
	GetFile(ctx context.Context, id, resourceKey string) (File, error)
	// ListChildren returns one page of non-trashed children.
	ListChildren(ctx context.Context, folderID, resourceKey, pageToken string) ([]File, string, error)
	CreateFolder(ctx context.Context, name, parentID, intent string) (File, error)
	CopyFile(ctx context.Context, src File, parentID, intent string) (File, error)
	CreateShortcut(ctx context.Context, name, parentID, targetID, targetKey, intent string) (File, error)
	// FindByIntent returns children of parentID tagged with the intent key.
	FindByIntent(ctx context.Context, parentID, intent string) ([]File, error)
	// RenameFile, MoveFile and TrashFile change objects we created; they are
	// idempotent, so an unknown outcome may simply be repeated.
	RenameFile(ctx context.Context, id, name string) error
	MoveFile(ctx context.Context, id, fromParent, toParent string) error
	TrashFile(ctx context.Context, id string) error
	// StartPageToken returns the cursor for "changes from now on".
	StartPageToken(ctx context.Context) (string, error)
	// ListChanges returns one page of the change feed. When the feed is
	// drained next is empty and newStart is the cursor to persist.
	ListChanges(ctx context.Context, pageToken string) (changes []Change, next, newStart string, err error)
}

// Parent names one folder in a batch listing. ResourceKey is optional (shared
// drives need it beside the ID).
type Parent struct {
	ID          string
	ResourceKey string
}

// MaxBatchParents bounds one combined files.list query. Forty IDs keep the q
// parameter far below Google's length limit and still cut calls by ~40x.
const MaxBatchParents = 40

// BatchLister is an optional provider extension: one files.list returns the
// children of several folders. Engine discovery uses it when available and
// falls back to ListChildren otherwise, so test doubles stay valid.
type BatchLister interface {
	ListChildrenBatch(ctx context.Context, parents []Parent, pageToken string) ([]File, string, error)
}

// Change is one entry of the Drive change feed.
type Change struct {
	FileID  string
	Removed bool
	File    *File
}

// IsInvalidToken reports whether a change cursor is no longer usable.
func IsInvalidToken(err error) bool {
	var de *Error
	return errors.As(err, &de) && de.Kind == KindInvalid && (de.Status == 400 || de.Status == 410)
}

// Kind classifies a provider failure by what it implies for retrying.
type Kind string

const (
	// KindThrottled: Google rejected the call for rate; nothing was written.
	KindThrottled Kind = "throttled"
	// KindUnavailable: transport error (timeout, broken connection); a write
	// may or may not have happened.
	KindUnavailable Kind = "unavailable"
	// KindServer: the provider answered 5xx. Kept apart from a transport
	// failure so logs and status can tell "Google is broken" from "the network
	// is down".
	KindServer Kind = "server"
	// KindUnreachable: the connection could not be established; nothing was sent.
	KindUnreachable Kind = "unreachable"
	KindAuth        Kind = "auth"
	KindPermission  Kind = "permission"
	KindNotFound    Kind = "not_found"
	KindQuota       Kind = "quota"
	KindInvalid     Kind = "invalid"
	KindCancelled   Kind = "cancelled"
)

type Error struct {
	Kind       Kind
	Status     int
	Reason     string
	RetryAfter time.Duration
	Err        error
}

func (e *Error) Error() string {
	if e.Reason != "" {
		return fmt.Sprintf("drive %s (%d %s)", e.Kind, e.Status, e.Reason)
	}
	return fmt.Sprintf("drive %s (%d)", e.Kind, e.Status)
}
func (e *Error) Unwrap() error { return e.Err }

// KindOf extracts the classification of err (KindUnavailable for unknown errors,
// the safest assumption for a write).
func KindOf(err error) Kind {
	var de *Error
	if errors.As(err, &de) {
		return de.Kind
	}
	if errors.Is(err, context.Canceled) {
		return KindCancelled
	}
	return KindUnavailable
}

// QuotaScope tells a daily quota ("daily", resets at midnight Pacific) from a
// storage quota ("storage", needs a human) and any other quota error ("quota").
func QuotaScope(err error) string {
	var de *Error
	if !errors.As(err, &de) || de.Kind != KindQuota {
		return ""
	}
	switch de.Reason {
	case "dailyLimitExceeded":
		return "daily"
	case "storageQuotaExceeded":
		return "storage"
	}
	return "quota"
}

func RetryAfterOf(err error) time.Duration {
	var de *Error
	if errors.As(err, &de) {
		return de.RetryAfter
	}
	return 0
}

// Executed reports whether a failed write is proven not to have happened.
func NotExecuted(err error) bool {
	switch KindOf(err) {
	case KindThrottled, KindUnreachable, KindAuth, KindPermission, KindNotFound, KindQuota, KindInvalid:
		return true
	}
	return false
}

// QueryEscape quotes a value for a Drive query string literal.
func QueryEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(s)
}
