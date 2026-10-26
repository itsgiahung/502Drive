// Package schedule turns structured presets into concrete run times. It never
// uses the host timezone and never keeps timers: callers persist the next due
// time and the slot key, and ask here only for "what comes after T".
package schedule

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

const (
	KindInterval = "interval"
	KindDaily    = "daily"
	KindWeekly   = "weekly"

	MinInterval = 5
	MaxInterval = 10080
)

// Spec mirrors the ScheduleSpec schema of the HTTP contract.
type Spec struct {
	Kind            string    `json:"kind"`
	IntervalMinutes int       `json:"interval_minutes,omitempty"`
	AnchorAt        time.Time `json:"anchor_at,omitempty"`
	Timezone        string    `json:"timezone,omitempty"`
	LocalTime       string    `json:"local_time,omitempty"`
	Weekdays        []int     `json:"weekdays,omitempty"` // ISO: 1=Monday..7=Sunday
}

func (s Spec) Validate() error {
	switch s.Kind {
	case KindInterval:
		if s.IntervalMinutes < MinInterval || s.IntervalMinutes > MaxInterval {
			return fmt.Errorf("interval_minutes must be between %d and %d", MinInterval, MaxInterval)
		}
		if s.AnchorAt.IsZero() {
			return errors.New("anchor_at is required")
		}
		return nil
	case KindDaily, KindWeekly:
		if _, err := time.LoadLocation(s.Timezone); err != nil || s.Timezone == "" || strings.EqualFold(s.Timezone, "local") {
			return errors.New("timezone must be an IANA zone such as Asia/Ho_Chi_Minh")
		}
		if _, _, err := parseClock(s.LocalTime); err != nil {
			return err
		}
		if s.Kind == KindWeekly {
			if len(s.Weekdays) == 0 || len(s.Weekdays) > 7 {
				return errors.New("weekdays must list 1-7 ISO weekdays")
			}
			seen := map[int]bool{}
			for _, d := range s.Weekdays {
				if d < 1 || d > 7 || seen[d] {
					return errors.New("weekdays must be unique values 1 (Monday) to 7 (Sunday)")
				}
				seen[d] = true
			}
		}
		return nil
	}
	return errors.New("kind must be interval, daily or weekly")
}

func parseClock(v string) (h, m int, err error) {
	t, err := time.Parse("15:04", v)
	if err != nil || len(v) != 5 {
		return 0, 0, errors.New("local_time must be HH:MM (24-hour)")
	}
	return t.Hour(), t.Minute(), nil
}

// Slot is one scheduled occurrence. Key identifies it independent of the
// instant, so a repeated wall-clock hour (DST fold) yields a single slot.
type Slot struct {
	At  time.Time
	Key string
}

// Next returns the first slot strictly after t.
func (s Spec) Next(after time.Time) (Slot, error) {
	if err := s.Validate(); err != nil {
		return Slot{}, err
	}
	if s.Kind == KindInterval {
		step := time.Duration(s.IntervalMinutes) * time.Minute
		n := int64(1)
		if after.After(s.AnchorAt) {
			n = int64(after.Sub(s.AnchorAt)/step) + 1
		} else {
			n = 0 // the anchor itself is the first slot when it is still ahead
			if !s.AnchorAt.After(after) {
				n = 1
			}
		}
		at := s.AnchorAt.Add(time.Duration(n) * step).UTC()
		return Slot{At: at, Key: fmt.Sprintf("i%d", n)}, nil
	}
	loc, _ := time.LoadLocation(s.Timezone)
	h, m, _ := parseClock(s.LocalTime)
	day := after.In(loc)
	y, mo, d := day.Date()
	for i := 0; i < 400; i++ {
		date := time.Date(y, mo, d+i, 0, 0, 0, 0, loc)
		if s.Kind == KindWeekly && !slices.Contains(s.Weekdays, isoWeekday(date)) {
			continue
		}
		cand := time.Date(date.Year(), date.Month(), date.Day(), h, m, 0, 0, loc)
		// A local time inside a spring-forward gap does not exist: Go shifts
		// it, so the clock no longer matches. That day is skipped.
		if cand.Hour() != h || cand.Minute() != m {
			continue
		}
		if cand.After(after) {
			return Slot{At: cand.UTC(), Key: fmt.Sprintf("%04d-%02d-%02dT%02d:%02d@%s", date.Year(), date.Month(), date.Day(), h, m, s.Timezone)}, nil
		}
	}
	return Slot{}, errors.New("no future occurrence")
}

func isoWeekday(t time.Time) int {
	if w := int(t.Weekday()); w != 0 {
		return w
	}
	return 7
}

// Preview lists the next n slots after t.
func (s Spec) Preview(after time.Time, n int) ([]Slot, error) {
	var out []Slot
	for len(out) < n {
		sl, err := s.Next(after)
		if err != nil {
			return out, err
		}
		out = append(out, sl)
		after = sl.At
	}
	return out, nil
}

// Describe is a short human summary used by the CLI and the bot.
func (s Spec) Describe() string {
	switch s.Kind {
	case KindInterval:
		switch {
		case s.IntervalMinutes%1440 == 0:
			return fmt.Sprintf("every %d day(s)", s.IntervalMinutes/1440)
		case s.IntervalMinutes%60 == 0:
			return fmt.Sprintf("every %d hour(s)", s.IntervalMinutes/60)
		}
		return fmt.Sprintf("every %d minutes", s.IntervalMinutes)
	case KindDaily:
		return fmt.Sprintf("daily at %s (%s)", s.LocalTime, s.Timezone)
	case KindWeekly:
		names := []string{"", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
		var ds []string
		for _, d := range slices.Sorted(slices.Values(s.Weekdays)) {
			ds = append(ds, names[d])
		}
		return fmt.Sprintf("%s at %s (%s)", strings.Join(ds, ", "), s.LocalTime, s.Timezone)
	}
	return ""
}
