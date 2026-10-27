package present

import (
	"strings"
	"testing"
	"time"
)

func TestRecentProgressRequiresThreeSamplesAndKnownStableTotal(t *testing.T) {
	now := time.Unix(100, 0)
	total := 10
	c := Counts{Completed: 2, Total: &total, Bytes: 2048}
	samples := []Sample{{At: now, Completed: 0}, {At: now.Add(time.Second), Completed: 1}, {At: now.Add(2 * time.Second), Completed: 2}}
	for _, lang := range []string{"en", "vi"} {
		if got := RecentProgress(c, samples[:2], lang); strings.Contains(got, "/s") || strings.Contains(got, "/giây") {
			t.Fatal(got)
		}
		got := RecentProgress(c, samples, lang)
		if !strings.Contains(got, "2.0 KB") || (!strings.Contains(got, "estimated") && !strings.Contains(got, "ước tính")) {
			t.Fatal(got)
		}
		c.Total = nil
		got = RecentProgress(c, samples, lang)
		if strings.Contains(got, "estimated") || strings.Contains(got, "ước tính") {
			t.Fatal(got)
		}
		c.Total = &total
	}
	samples[2].Completed = 1
	if got := RecentProgress(c, samples, "en"); strings.Contains(got, "estimated") {
		t.Fatal(got)
	}
}
