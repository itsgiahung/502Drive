package schedule

import (
	"testing"
	"time"
)

func utc(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t.UTC()
}

func TestIntervalIsAnchoredAndMonotonic(t *testing.T) {
	sp := Spec{Kind: KindInterval, IntervalMinutes: 30, AnchorAt: utc("2026-01-01T00:10:00Z")}
	s, _ := sp.Next(utc("2026-01-01T00:00:00Z"))
	if !s.At.Equal(utc("2026-01-01T00:10:00Z")) {
		t.Fatalf("anchor ahead should be first: %v", s.At)
	}
	s, _ = sp.Next(utc("2026-01-01T00:10:00Z"))
	if !s.At.Equal(utc("2026-01-01T00:40:00Z")) {
		t.Fatalf("%v", s.At)
	}
	s2, _ := sp.Next(utc("2026-03-05T12:00:01Z"))
	if s2.At.Minute()%30 != 10%30 || !s2.At.After(utc("2026-03-05T12:00:01Z")) {
		t.Fatalf("not aligned to the anchor: %v", s2.At)
	}
	s3, _ := sp.Next(s2.At.Add(-time.Second))
	if s3.Key != s2.Key {
		t.Fatal("same slot must have the same key")
	}
}

func TestDailyUsesZoneNotHost(t *testing.T) {
	sp := Spec{Kind: KindDaily, Timezone: "Asia/Ho_Chi_Minh", LocalTime: "22:00"}
	s, err := sp.Next(utc("2026-10-26T10:00:00Z")) // 17:00 local
	if err != nil || !s.At.Equal(utc("2026-10-26T15:00:00Z")) {
		t.Fatalf("%v %v", s, err)
	}
	s, _ = sp.Next(utc("2026-10-26T15:00:00Z"))
	if !s.At.Equal(utc("2026-10-27T15:00:00Z")) {
		t.Fatalf("strictly after: %v", s.At)
	}
}

func TestWeeklyOnlyFiresOnChosenWeekdays(t *testing.T) {
	sp := Spec{Kind: KindWeekly, Timezone: "Asia/Ho_Chi_Minh", LocalTime: "07:00", Weekdays: []int{1, 5}}
	got, _ := sp.Preview(utc("2026-10-26T00:00:00Z"), 4) // Mon 26 Oct 2026 07:00 local = 00:00Z
	want := []string{"2026-10-30T00:00:00Z", "2026-11-02T00:00:00Z", "2026-11-06T00:00:00Z", "2026-11-09T00:00:00Z"}
	for i, w := range want {
		if !got[i].At.Equal(utc(w)) {
			t.Errorf("slot %d = %v want %s", i, got[i].At, w)
		}
	}
	sunday := Spec{Kind: KindWeekly, Timezone: "UTC", LocalTime: "12:00", Weekdays: []int{7}}
	s, _ := sunday.Next(utc("2026-10-26T00:00:00Z"))
	if s.At.Weekday() != time.Sunday {
		t.Fatalf("ISO 7 must be Sunday, got %v", s.At.Weekday())
	}
}

func TestDSTGapIsSkippedAndFoldRunsOnce(t *testing.T) {
	ny := Spec{Kind: KindDaily, Timezone: "America/New_York", LocalTime: "02:30"}
	// 2026-03-08 02:30 does not exist in New York
	got, _ := ny.Preview(utc("2026-03-07T12:00:00Z"), 3)
	for _, s := range got {
		if s.Key[:10] == "2026-03-08" {
			t.Fatalf("gap slot was scheduled: %v", s)
		}
	}
	if len(got) != 3 {
		t.Fatal("preview short")
	}
	// 2026-11-01 01:30 happens twice; both instants share one slot key
	fold := Spec{Kind: KindDaily, Timezone: "America/New_York", LocalTime: "01:30"}
	keys := map[string]int{}
	after := utc("2026-10-31T12:00:00Z")
	for range 4 {
		s, _ := fold.Next(after)
		keys[s.Key]++
		after = s.At
	}
	if keys["2026-11-01T01:30@America/New_York"] != 1 {
		t.Fatalf("fold slot key counted %v", keys)
	}
	// half-hour DST zone
	lh := Spec{Kind: KindDaily, Timezone: "Australia/Lord_Howe", LocalTime: "09:00"}
	if _, err := lh.Preview(utc("2026-10-01T00:00:00Z"), 10); err != nil {
		t.Fatal(err)
	}
}

func TestValidation(t *testing.T) {
	bad := []Spec{
		{Kind: "cron"},
		{Kind: KindInterval, IntervalMinutes: 1, AnchorAt: time.Now()},
		{Kind: KindInterval, IntervalMinutes: 30},
		{Kind: KindDaily, Timezone: "Mars/Olympus", LocalTime: "10:00"},
		{Kind: KindDaily, Timezone: "Local", LocalTime: "10:00"},
		{Kind: KindDaily, Timezone: "UTC", LocalTime: "25:00"},
		{Kind: KindDaily, Timezone: "UTC", LocalTime: "9:00"},
		{Kind: KindWeekly, Timezone: "UTC", LocalTime: "10:00"},
		{Kind: KindWeekly, Timezone: "UTC", LocalTime: "10:00", Weekdays: []int{0}},
		{Kind: KindWeekly, Timezone: "UTC", LocalTime: "10:00", Weekdays: []int{1, 1}},
	}
	for i, s := range bad {
		if s.Validate() == nil {
			t.Errorf("case %d accepted: %+v", i, s)
		}
	}
	ok := Spec{Kind: KindWeekly, Timezone: "Asia/Ho_Chi_Minh", LocalTime: "07:30", Weekdays: []int{1, 2, 3}}
	if err := ok.Validate(); err != nil || ok.Describe() == "" {
		t.Fatal(err)
	}
}
