package present

import (
	"strings"
	"testing"
	"time"
)

func TestQuotaLineUsesReaderTimezoneAndNoUnknownLimit(t *testing.T) {
	at := time.Date(2026, 10, 28, 3, 30, 0, 0, time.UTC)
	loc := time.FixedZone("synthetic", 7*3600)
	got := QuotaLine(QuotaStatus{Used24hBytes: 1024, ResumeAfter: &at}, loc, "vi")
	if !strings.Contains(got, "10:30") || !strings.Contains(got, "1.0 KB") || strings.Contains(got, "750") {
		t.Fatal(got)
	}
}
