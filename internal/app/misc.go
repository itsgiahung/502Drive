package app

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/itsgiahung/502Drive/internal/drive"
	"github.com/itsgiahung/502Drive/internal/state"
)

type DestinationRequest struct {
	Source string `json:"source"`
	Name   string `json:"name"`
}

func (s *Service) AddDestination(ctx context.Context, p Principal, key string, req DestinationRequest) (state.Destination, bool, error) {
	if err := requireOwner(p); err != nil {
		return state.Destination{}, false, err
	}
	if err := state.ValidateKey(key); err != nil {
		return state.Destination{}, false, Fail(http.StatusBadRequest, "invalid_request", "Idempotency-Key is required (1-128 printable characters).")
	}
	if req.Name != "" && !validName(req.Name) {
		return state.Destination{}, false, Fail(http.StatusBadRequest, "invalid_request", "name must be 1-255 characters without control characters.")
	}
	ref, err := drive.Parse(req.Source)
	if err != nil {
		return state.Destination{}, false, Fail(http.StatusBadRequest, "ambiguous_source", err.Error())
	}
	prov, err := s.provider(ctx)
	if err != nil {
		return state.Destination{}, false, err
	}
	f, err := prov.GetFile(ctx, ref.ID, ref.Key)
	if err != nil {
		return state.Destination{}, false, err
	}
	if !f.IsFolder() || f.Trashed {
		return state.Destination{}, false, Fail(http.StatusBadRequest, "destination_not_folder", "The destination must be a folder.")
	}
	if !f.CanAddChildren {
		return state.Destination{}, false, Fail(http.StatusConflict, "destination_not_writable", "This account cannot add files to that folder.")
	}
	name := req.Name
	if name == "" {
		name = f.Name
	}
	return s.Store.AddDestination(ctx, p.ID, key, req, state.Destination{FolderID: f.ID, ResourceKey: ref.Key, DriveID: f.DriveID, Name: name})
}

func (s *Service) Destinations(ctx context.Context) ([]state.Destination, error) {
	return s.Store.ListDestinations(ctx)
}

func (s *Service) SetDefaultDestination(ctx context.Context, p Principal, key, id string, expected int) (state.Destination, bool, error) {
	if err := requireOwner(p); err != nil {
		return state.Destination{}, false, err
	}
	if err := state.ValidateKey(key); err != nil {
		return state.Destination{}, false, Fail(http.StatusBadRequest, "invalid_request", "Idempotency-Key is required (1-128 printable characters).")
	}
	return s.Store.SetDefaultDestination(ctx, p.ID, key, id, expected)
}

// Lifecycle is the daemon's power/network state, reported by /v1/status.
// It exists so the tray, the TUI and the CLI all show the same truth without
// guessing from failed requests.
type Lifecycle struct {
	State             string `json:"state"`
	Online            bool   `json:"online"`
	Since             string `json:"since,omitempty"`
	LastGapSeconds    int    `json:"last_gap_seconds,omitempty"`
	ReconnectRequired bool   `json:"reconnect_required"`
}

// ThrottleStatus is the account-wide AIMD/read-pacing view. It is reported by
// /v1/status so a user can tell "Google is throttling us" from "the daemon is
// broken".
type ThrottleStatus struct {
	WorkersNow      int        `json:"workers_now"`
	ThrottledLast5m int        `json:"throttled_last_5m"`
	NextRetryAt     *time.Time `json:"next_retry_at"`
}

type Status struct {
	Version       string
	SchemaVersion int
	Ready         bool
	Account       AccountView
	BotState      string
	Running       int
	Queued        int
	Blocked       int
	Lifecycle     Lifecycle
	Throttle      ThrottleStatus
}

func (s *Service) Status(ctx context.Context) (Status, error) {
	v, err := s.Store.SchemaVersion(ctx)
	if err != nil {
		return Status{}, err
	}
	acc, err := s.Account(ctx)
	if err != nil {
		return Status{}, err
	}
	counts, err := s.Store.CountJobs(ctx)
	if err != nil {
		return Status{}, err
	}
	bot := "disabled"
	if s.BotState != nil {
		bot = s.BotState()
	}
	life := Lifecycle{State: "running", Online: true}
	if s.Lifecycle != nil {
		life = s.Lifecycle()
	}
	throttle := ThrottleStatus{WorkersNow: 4}
	if s.Throttle != nil {
		throttle = s.Throttle()
	}
	return Status{Version: s.Version, SchemaVersion: v, Ready: s.Store.Check(ctx) == nil, Account: acc, BotState: bot,
		Running: counts[state.JobRunning] + counts[state.JobPausing] + counts[state.JobCancelling] + counts[state.JobReconciling],
		Queued:  counts[state.JobQueued], Blocked: counts[state.JobBlocked], Lifecycle: life, Throttle: throttle}, nil
}

func (s *Service) Preferences(ctx context.Context, p Principal) (state.Preferences, error) {
	return s.Store.Preferences(ctx, p.ID)
}

type PreferencePatch struct {
	Language             *string `json:"language"`
	Timezone             *string `json:"timezone"`
	AutoCopy             *bool   `json:"auto_copy"`
	DestinationID        *string `json:"destination_id"`
	DestinationRevision  *int    `json:"destination_revision"`
	NotifyCompleted      *bool   `json:"notify_completed"`
	NotifyFailed         *bool   `json:"notify_failed"`
	NotifyActionRequired *bool   `json:"notify_action_required"`
	AfterCopySyncMode    *string `json:"after_copy_sync_mode"`
}

func (s *Service) PatchPreferences(ctx context.Context, p Principal, key string, patch PreferencePatch) (state.Preferences, error) {
	if err := state.ValidateKey(key); err != nil {
		return state.Preferences{}, Fail(http.StatusBadRequest, "invalid_request", "Idempotency-Key is required (1-128 printable characters).")
	}
	cur, err := s.Store.Preferences(ctx, p.ID)
	if err != nil {
		return cur, err
	}
	if patch.Language != nil {
		if *patch.Language != "vi" && *patch.Language != "en" {
			return cur, Fail(http.StatusBadRequest, "invalid_request", "language must be vi or en.")
		}
		cur.Language = *patch.Language
	}
	if patch.Timezone != nil {
		tz := strings.TrimSpace(*patch.Timezone)
		if _, err := time.LoadLocation(tz); err != nil || tz == "" {
			return cur, Fail(http.StatusBadRequest, "invalid_request", "timezone must be an IANA zone such as Asia/Ho_Chi_Minh.")
		}
		cur.Timezone = tz
	}
	if patch.NotifyCompleted != nil {
		cur.NotifyCompleted = *patch.NotifyCompleted
	}
	if patch.NotifyFailed != nil {
		cur.NotifyFailed = *patch.NotifyFailed
	}
	if patch.NotifyActionRequired != nil {
		cur.NotifyActionRequired = *patch.NotifyActionRequired
	}
	if patch.AfterCopySyncMode != nil {
		switch *patch.AfterCopySyncMode {
		case "off", "scheduled", "realtime":
			cur.AfterCopySyncMode = *patch.AfterCopySyncMode
		default:
			return cur, Fail(http.StatusBadRequest, "invalid_request", "after_copy_sync_mode must be off, scheduled or realtime.")
		}
	}
	if patch.AutoCopy != nil || patch.DestinationID != nil || patch.DestinationRevision != nil {
		// Auto-copy is bound to a validated destination revision; any change
		// to either must restate both.
		auto := cur.AutoCopy
		if patch.AutoCopy != nil {
			auto = *patch.AutoCopy
		}
		if auto {
			id := cur.DestinationID
			if patch.DestinationID != nil {
				id = *patch.DestinationID
			}
			d, derr := s.Store.GetDestination(ctx, id)
			if errors.Is(derr, state.ErrNotFound) {
				return cur, Fail(http.StatusBadRequest, "invalid_request", "auto_copy needs a valid destination_id.")
			}
			if derr != nil {
				return cur, derr
			}
			if patch.DestinationRevision == nil || *patch.DestinationRevision != d.Revision {
				return cur, state.ErrStale
			}
			cur.DestinationID, cur.DestinationRevision = d.ID, &d.Revision
		} else {
			cur.DestinationID, cur.DestinationRevision = "", nil
		}
		cur.AutoCopy = auto
	}
	return cur, s.Store.SavePreferences(ctx, p.ID, cur)
}

func (s *Service) Collections(ctx context.Context, p Principal, all bool) ([]state.Collection, error) {
	return s.Store.Collections(ctx, scope(p, all), all)
}

func (s *Service) Collection(ctx context.Context, p Principal, id string) (state.Collection, error) {
	return s.Store.GetCollection(ctx, scope(p, true), id)
}

// CollectionPage pages the library. The cursor is bound to the principal and
// the active filters by the HTTP layer.
func (s *Service) CollectionPage(ctx context.Context, p Principal, all bool, query, mode string, limit int, cursor int64) ([]state.Collection, int64, error) {
	return s.Store.CollectionPage(ctx, scope(p, all), query, mode, all, limit, cursor)
}

func (s *Service) Operators(ctx context.Context, p Principal) ([]state.Operator, error) {
	if err := requireOwner(p); err != nil {
		return nil, err
	}
	return s.Store.Operators(ctx)
}

func (s *Service) AddOperator(ctx context.Context, p Principal, key string, telegramID int64) (state.Operator, bool, error) {
	if err := requireOwner(p); err != nil {
		return state.Operator{}, false, err
	}
	if err := state.ValidateKey(key); err != nil || telegramID < 1 {
		return state.Operator{}, false, Fail(http.StatusBadRequest, "invalid_request", "A numeric telegram_user_id and an Idempotency-Key are required.")
	}
	return s.Store.AddOperator(ctx, p.ID, key, telegramID)
}

func (s *Service) RemoveOperator(ctx context.Context, p Principal, key, id string) error {
	if err := requireOwner(p); err != nil {
		return err
	}
	if err := state.ValidateKey(key); err != nil {
		return Fail(http.StatusBadRequest, "invalid_request", "Idempotency-Key is required (1-128 printable characters).")
	}
	return s.Store.DisableOperator(ctx, p.ID, key, id)
}

type Check struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

// Doctor is offline by default; online=true calls Google.
func (s *Service) Doctor(ctx context.Context, p Principal, online bool) ([]Check, error) {
	if err := requireOwner(p); err != nil {
		return nil, err
	}
	var out []Check
	add := func(name string, ok bool, pass, fail string) {
		c := Check{Name: name, Status: "passed", Message: pass}
		if !ok {
			c.Status, c.Message = "failed", fail
		}
		out = append(out, c)
	}
	add("database", s.Store.Check(ctx) == nil, "State database is open.", "State database is not responding.")
	add("oauth_client", s.Auth != nil, "Google OAuth client is configured.", "No Google OAuth client is configured (see docs/oauth-setup.md).")
	acc, _ := s.Account(ctx)
	add("account", acc.Status == "connected", "Google account is connected.", "Google account status: "+acc.Status+".")
	_, derr := s.Store.DefaultDestination(ctx)
	add("destination", derr == nil, "A default destination is set.", "No default destination is set.")
	if online {
		c := Check{Name: "google_drive", Status: "passed", Message: "Google Drive answered."}
		if prov, err := s.provider(ctx); err != nil {
			c.Status, c.Message = "failed", "Not connected."
		} else if _, err = prov.Account(ctx); err != nil {
			c.Status, c.Message = "failed", Classify(err).Message
		}
		out = append(out, c)
	} else {
		out = append(out, Check{Name: "google_drive", Status: "skipped", Message: "Run with online=true to contact Google."})
	}
	return out, nil
}
