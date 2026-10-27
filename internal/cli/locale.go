package cli

import (
	"context"
	"time"

	"github.com/itsgiahung/502Drive/internal/i18n"
)

// locale resolves the human-output language and timezone from Preferences once
// per command. JSON always stays raw: only dates and labels are localized.
func (a *App) locale(ctx context.Context) (string, *time.Location) {
	if a.localeSet {
		return a.locLang, a.locZone
	}
	lang := i18n.Resolve("")
	loc := time.Local
	if r, err := a.call(ctx, "GET", "/v1/preferences", nil, nil, ""); err == nil {
		var prefs struct {
			Language string `json:"language"`
			Timezone string `json:"timezone"`
		}
		if r.Decode(&prefs) == nil {
			if prefs.Language != "" {
				lang = i18n.Resolve(prefs.Language)
			}
			if prefs.Timezone != "" {
				if z, err := time.LoadLocation(prefs.Timezone); err == nil {
					loc = z
				}
			}
		}
	}
	a.locLang, a.locZone, a.localeSet = lang, loc, true
	return lang, loc
}

// formatTimestamp renders an RFC3339 value for humans in the preference
// language and timezone. Unparsable values are returned unchanged so a daemon
// change never hides data.
func (a *App) formatTimestamp(ctx context.Context, raw string) string {
	if raw == "" {
		return "-"
	}
	t, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return raw
	}
	lang, loc := a.locale(ctx)
	return i18n.FormatTime(t, loc, lang)
}

// formatDate is formatTimestamp without the clock (Drive listings).
func (a *App) formatDate(ctx context.Context, raw *string) string {
	if raw == nil || *raw == "" {
		return "-"
	}
	t, err := time.Parse(time.RFC3339Nano, *raw)
	if err != nil {
		return "-"
	}
	lang, loc := a.locale(ctx)
	return i18n.FormatDate(t, loc, lang)
}
