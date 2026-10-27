package state

import (
	"testing"
	"time"
)

func TestDefaultPreferencesLanguageIsAuto(t *testing.T) {
	p := DefaultPreferences()
	if p.Language != "auto" {
		t.Fatalf("DefaultPreferences language = %q, want %q", p.Language, "auto")
	}
}

func TestInQuietHours(t *testing.T) {
	start := "22:00"
	end := "07:00"
	p := Preferences{
		Timezone:        "Asia/Ho_Chi_Minh",
		QuietHoursStart: &start,
		QuietHoursEnd:   &end,
	}

	loc, err := time.LoadLocation("Asia/Ho_Chi_Minh")
	if err != nil {
		t.Fatalf("cannot load timezone: %v", err)
	}

	// 23:30 is in quiet hours
	t1 := time.Date(2026, 10, 27, 23, 30, 0, 0, loc)
	if !p.InQuietHours(t1) {
		t.Errorf("23:30 should be in quiet hours")
	}

	// 06:15 is in quiet hours
	t2 := time.Date(2026, 10, 27, 6, 15, 0, 0, loc)
	if !p.InQuietHours(t2) {
		t.Errorf("06:15 should be in quiet hours")
	}

	// 12:00 is outside quiet hours
	t3 := time.Date(2026, 10, 27, 12, 0, 0, 0, loc)
	if p.InQuietHours(t3) {
		t.Errorf("12:00 should not be in quiet hours")
	}

	// Daytime span: 13:00 to 14:00
	s2 := "13:00"
	e2 := "14:00"
	p2 := Preferences{
		Timezone:        "Asia/Ho_Chi_Minh",
		QuietHoursStart: &s2,
		QuietHoursEnd:   &e2,
	}
	if !p2.InQuietHours(time.Date(2026, 10, 27, 13, 30, 0, 0, loc)) {
		t.Errorf("13:30 should be in quiet hours for 13:00-14:00")
	}
	if p2.InQuietHours(time.Date(2026, 10, 27, 14, 30, 0, 0, loc)) {
		t.Errorf("14:30 should not be in quiet hours for 13:00-14:00")
	}

	// Nil quiet hours
	p3 := Preferences{}
	if p3.InQuietHours(t1) {
		t.Errorf("nil quiet hours should always return false")
	}
}
