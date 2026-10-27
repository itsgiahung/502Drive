package i18n

import (
	"testing"
	"time"
)

func TestResolveAndLocale(t *testing.T) {
	origHook := osLocaleHook
	defer func() { osLocaleHook = origHook }()

	cases := []struct {
		envLocale string
		langArg   string
		want      string
	}{
		{"vi_VN.UTF-8", "auto", "vi"},
		{"vi-VN", "auto", "vi"},
		{"VI_VN", "auto", "vi"},
		{"en_US.UTF-8", "auto", "en"},
		{"ja_JP.UTF-8", "auto", "en"},
		{"", "auto", "en"},
		{"fr_FR", "", "en"},
		{"", "", "en"},
		{"en_US.UTF-8", "vi", "vi"},
		{"vi_VN.UTF-8", "en", "en"},
		{"vi_VN.UTF-8", "vi-VN", "vi"},
		{"", "en-US", "en"},
	}

	for _, tc := range cases {
		osLocaleHook = func() string { return tc.envLocale }
		got := Resolve(tc.langArg)
		if got != tc.want {
			t.Errorf("Resolve(%q) with osLocale=%q: got %q, want %q", tc.langArg, tc.envLocale, got, tc.want)
		}
	}
}

func TestFormatTimeAndDate(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Ho_Chi_Minh")
	if err != nil {
		t.Fatalf("cannot load timezone: %v", err)
	}

	// 2026-10-27 15:04:05 UTC -> 2026-10-27 22:04:05 +07
	target := time.Date(2026, 10, 27, 15, 4, 5, 0, time.UTC)

	// Vietnamese format
	if got := FormatTime(target, loc, "vi"); got != "27/10/2026 22:04" {
		t.Errorf("FormatTime(vi) = %q, want %q", got, "27/10/2026 22:04")
	}
	if got := FormatDate(target, loc, "vi"); got != "27/10/2026" {
		t.Errorf("FormatDate(vi) = %q, want %q", got, "27/10/2026")
	}

	// English format
	if got := FormatTime(target, loc, "en"); got != "2026-10-27 22:04" {
		t.Errorf("FormatTime(en) = %q, want %q", got, "2026-10-27 22:04")
	}
	if got := FormatDate(target, loc, "en"); got != "2026-10-27" {
		t.Errorf("FormatDate(en) = %q, want %q", got, "2026-10-27")
	}

	// Zero time
	if got := FormatTime(time.Time{}, loc, "vi"); got != "" {
		t.Errorf("FormatTime(zero) = %q, want empty", got)
	}
	if got := FormatDate(time.Time{}, loc, "vi"); got != "" {
		t.Errorf("FormatDate(zero) = %q, want empty", got)
	}
}
