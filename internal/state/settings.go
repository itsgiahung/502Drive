package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"
)

const defaultDestinationKey = "default_destination"

func (s *Store) Setting(ctx context.Context, key string) (string, bool, error) {
	var v string
	err := s.db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key=?", key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return v, err == nil, err
}

func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, "INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", key, value)
	return err
}

// Preferences are per principal.
type Preferences struct {
	Language             string  `json:"language"`
	Timezone             string  `json:"timezone"`
	AutoCopy             bool    `json:"auto_copy"`
	DestinationID        string  `json:"destination_id,omitempty"`
	DestinationRevision  *int    `json:"destination_revision,omitempty"`
	NotifyCompleted      bool    `json:"notify_completed"`
	NotifyFailed         bool    `json:"notify_failed"`
	NotifyActionRequired bool    `json:"notify_action_required"`
	AfterCopySyncMode    string  `json:"after_copy_sync_mode"`
	QuietHoursStart      *string `json:"quiet_hours_start,omitempty"`
	QuietHoursEnd        *string `json:"quiet_hours_end,omitempty"`
	RetentionDays        int     `json:"retention_days"`
}

// InQuietHours reports whether the given wall clock time falls within quiet hours.
func (p Preferences) InQuietHours(t time.Time) bool {
	if p.QuietHoursStart == nil || p.QuietHoursEnd == nil {
		return false
	}
	start, end := strings.TrimSpace(*p.QuietHoursStart), strings.TrimSpace(*p.QuietHoursEnd)
	if len(start) != 5 || len(end) != 5 {
		return false
	}
	loc := time.UTC
	if p.Timezone != "" {
		if l, err := time.LoadLocation(p.Timezone); err == nil {
			loc = l
		}
	}
	hm := t.In(loc).Format("15:04")
	if start <= end {
		return hm >= start && hm < end
	}
	// Overnight span (e.g. 22:00 to 07:00)
	return hm >= start || hm < end
}

func DefaultPreferences() Preferences {
	return Preferences{Language: "auto", Timezone: DefaultTimezone(), NotifyCompleted: true, NotifyFailed: true, NotifyActionRequired: true, AfterCopySyncMode: "off", RetentionDays: 90}
}

// DefaultTimezone is the process TZ when it is a valid IANA zone, otherwise
// the project default. Schedules never depend on the host timezone implicitly.
func DefaultTimezone() string {
	if tz := strings.TrimSpace(os.Getenv("TZ")); tz != "" {
		if _, err := time.LoadLocation(tz); err == nil {
			return tz
		}
	}
	return "Asia/Ho_Chi_Minh"
}

func (s *Store) Preferences(ctx context.Context, principal string) (Preferences, error) {
	p := DefaultPreferences()
	v, ok, err := s.Setting(ctx, "prefs:"+principal)
	if err != nil || !ok {
		return p, err
	}
	return p, json.Unmarshal([]byte(v), &p)
}

func (s *Store) SavePreferences(ctx context.Context, principal string, p Preferences) error {
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return s.SetSetting(ctx, "prefs:"+principal, string(b))
}

func scanDestination(r interface{ Scan(...any) error }, def string) (Destination, error) {
	var d Destination
	var created int64
	err := r.Scan(&d.ID, &d.FolderID, &d.ResourceKey, &d.DriveID, &d.Name, &d.Revision, &created)
	d.CreatedAt = time.UnixMilli(created).UTC()
	d.IsDefault = d.ID == def
	return d, err
}

const destCols = "id,folder_id,resource_key,drive_id,name,revision,created_at"

// AddDestination persists a validated destination once per idempotency key.
func (s *Store) AddDestination(ctx context.Context, principal, key string, req any, d Destination) (Destination, bool, error) {
	body, replayed, err := s.idempotent(ctx, principal, "POST /v1/destinations", key, req, func(tx *sql.Tx) (any, error) {
		d.ID, d.Revision, d.CreatedAt = ID("dst"), 1, time.Now().UTC()
		if _, e := tx.ExecContext(ctx, "INSERT INTO destinations("+destCols+") VALUES(?,?,?,?,?,?,?)", d.ID, d.FolderID, d.ResourceKey, d.DriveID, d.Name, d.Revision, d.CreatedAt.UnixMilli()); e != nil {
			return nil, e
		}
		return d, nil
	})
	if err != nil {
		return Destination{}, false, err
	}
	var out Destination
	return out, replayed, json.Unmarshal(body, &out)
}

func (s *Store) ListDestinations(ctx context.Context) ([]Destination, error) {
	def, _, err := s.Setting(ctx, defaultDestinationKey)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, "SELECT "+destCols+" FROM destinations ORDER BY created_at, id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Destination{}
	for rows.Next() {
		d, e := scanDestination(rows, def)
		if e != nil {
			return nil, e
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) GetDestination(ctx context.Context, id string) (Destination, error) {
	def, _, err := s.Setting(ctx, defaultDestinationKey)
	if err != nil {
		return Destination{}, err
	}
	d, err := scanDestination(s.db.QueryRowContext(ctx, "SELECT "+destCols+" FROM destinations WHERE id=?", id), def)
	if errors.Is(err, sql.ErrNoRows) {
		return d, ErrNotFound
	}
	return d, err
}

func (s *Store) DefaultDestination(ctx context.Context) (Destination, error) {
	def, ok, err := s.Setting(ctx, defaultDestinationKey)
	if err != nil {
		return Destination{}, err
	}
	if !ok {
		return Destination{}, ErrNotFound
	}
	return s.GetDestination(ctx, def)
}

// SetDefaultDestination makes id the default. The destination's revision moves
// so anything bound to the previous revision (auto-copy, plans) is invalidated.
func (s *Store) SetDefaultDestination(ctx context.Context, principal, key, id string, expected int) (Destination, bool, error) {
	body, replayed, err := s.idempotent(ctx, principal, "PUT /v1/destinations/default", key, []any{id, expected}, func(tx *sql.Tx) (any, error) {
		d, e := scanDestination(tx.QueryRowContext(ctx, "SELECT "+destCols+" FROM destinations WHERE id=?", id), "")
		if errors.Is(e, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		if e != nil {
			return nil, e
		}
		if d.Revision != expected {
			return nil, ErrStale
		}
		d.Revision++
		if _, e = tx.ExecContext(ctx, "UPDATE destinations SET revision=? WHERE id=?", d.Revision, id); e != nil {
			return nil, e
		}
		if _, e = tx.ExecContext(ctx, "INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", defaultDestinationKey, id); e != nil {
			return nil, e
		}
		d.IsDefault = true
		return d, nil
	})
	if err != nil {
		return Destination{}, false, err
	}
	var out Destination
	return out, replayed, json.Unmarshal(body, &out)
}

// GenericIdempotent exposes the idempotency helper for adapters whose write
// is a single settings change.
func (s *Store) Idempotent(ctx context.Context, principal, path, key string, req any, fn func(tx *sql.Tx) (any, error)) ([]byte, bool, error) {
	return s.idempotent(ctx, principal, path, key, req, fn)
}

// CountJobs returns the number of jobs per state, for status.
func (s *Store) CountJobs(ctx context.Context) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT state,count(*) FROM jobs GROUP BY state")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var st string
		var n int
		if err = rows.Scan(&st, &n); err != nil {
			return nil, err
		}
		out[st] = n
	}
	return out, rows.Err()
}
