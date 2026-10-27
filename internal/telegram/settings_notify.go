package telegram

import (
	"context"
	"fmt"
	"strings"

	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/state"
)

// Notification settings in chat (S10): per-kind switches, quiet hours and the
// daily report. They change the same preferences the CLI and HTTP API change
// (PatchPreferences); the report schedule is adapter-owned (report.go).

const (
	defaultQuietStart = "22:00"
	defaultQuietEnd   = "07:00"
)

// notifyKinds are the switches, in display order.
var notifyKinds = []string{"completed", "failed", "action"}

func notifyOn(p state.Preferences, kind string) bool {
	switch kind {
	case "completed":
		return p.NotifyCompleted
	case "failed":
		return p.NotifyFailed
	}
	return p.NotifyActionRequired
}

func quietOn(p state.Preferences) bool {
	return p.QuietHoursStart != nil && p.QuietHoursEnd != nil && *p.QuietHoursStart != "" && *p.QuietHoursEnd != ""
}

func quietBounds(p state.Preferences) (string, string) {
	if quietOn(p) {
		return *p.QuietHoursStart, *p.QuietHoursEnd
	}
	return defaultQuietStart, defaultQuietEnd
}

// notifySettings returns the text block and the button rows that settingsPage
// appends below language and paste-to-copy.
func (t *Telegram) notifySettings(ctx context.Context, p app.Principal, tx texts, prefs state.Preferences) (string, [][]button) {
	g := tx.g()
	var b strings.Builder
	var rows [][]button
	var cells []button
	var summary []string
	for _, kind := range notifyKinds {
		on := notifyOn(prefs, kind)
		label := fmt.Sprintf(g.notifFmt, g.notifNames[kind], onOff(on, tx))
		summary = append(summary, label)
		target := "on"
		if on {
			target = "off"
		}
		cells = append(cells, act("🔔 "+label, "notify_toggle", kind, 0, target))
	}
	fmt.Fprintf(&b, "%s — %s\n", g.notifTitle, strings.Join(summary, " · "))
	rows = append(rows, cells[:2], cells[2:])

	cfg := t.loadReportConfig(ctx, p.ID)
	tz := prefs.Timezone
	if cfg.Enabled {
		fmt.Fprintf(&b, g.reportFmt+"\n", cfg.At+" ("+tz+")")
	} else {
		fmt.Fprintf(&b, g.reportFmt+"\n", g.reportOff)
	}
	var times []button
	for _, at := range reportChoices {
		label := "📰 " + at
		if cfg.Enabled && cfg.At == at {
			label = "✓ " + label
		}
		times = append(times, act(label, "report_set", at, 0, ""))
	}
	rows = append(rows, times)
	if cfg.Enabled {
		rows = append(rows, []button{act(g.reportBtnOff, "report_set", "off", 0, "")})
	}

	start, end := quietBounds(prefs)
	quiet := quietOn(prefs)
	target := "on"
	if quiet {
		target = "off"
		fmt.Fprintf(&b, g.quietFmt+"\n", start, end, onOff(true, tx))
	} else {
		b.WriteString(g.quietNone + "\n")
	}
	rows = append(rows, []button{act(fmt.Sprintf(g.quietFmt, start, end, onOff(quiet, tx)), "quiet_set", target, 0, "")})
	return b.String(), rows
}

// actNotifyToggle sets one switch to the value shown on the button.
func (t *Telegram) actNotifyToggle(ctx context.Context, p app.Principal, st state.Callback) error {
	on := st.Payload == "on"
	var patch app.PreferencePatch
	switch st.Object {
	case "completed":
		patch.NotifyCompleted = &on
	case "failed":
		patch.NotifyFailed = &on
	case "action":
		patch.NotifyActionRequired = &on
	default:
		return t.page(ctx, p, st.ChatID, st.MessageID, "settings")
	}
	if _, err := t.svc.PatchPreferences(ctx, p, "tg-cb:"+st.ID, patch); err != nil {
		return t.showError(ctx, p, st.ChatID, st.MessageID, err)
	}
	return t.page(ctx, p, st.ChatID, st.MessageID, "settings")
}

// actQuietSet turns quiet hours on (22:00-07:00) or off.
func (t *Telegram) actQuietSet(ctx context.Context, p app.Principal, st state.Callback) error {
	start, end := "", ""
	if st.Object == "on" {
		start, end = defaultQuietStart, defaultQuietEnd
	}
	if _, err := t.svc.PatchPreferences(ctx, p, "tg-cb:"+st.ID,
		app.PreferencePatch{QuietHoursStart: &start, QuietHoursEnd: &end}); err != nil {
		return t.showError(ctx, p, st.ChatID, st.MessageID, err)
	}
	return t.page(ctx, p, st.ChatID, st.MessageID, "settings")
}

// actReportSet picks the report time, or turns the report off.
func (t *Telegram) actReportSet(ctx context.Context, p app.Principal, st state.Callback) error {
	cfg := t.loadReportConfig(ctx, p.ID)
	if st.Object == "off" {
		cfg.Enabled = false
	} else {
		cfg = reportConfig{Enabled: true, At: st.Object}
	}
	if err := t.saveReportConfig(ctx, p.ID, cfg); err != nil {
		return t.showError(ctx, p, st.ChatID, st.MessageID, app.Fail(400, "invalid_request", err.Error()))
	}
	return t.page(ctx, p, st.ChatID, st.MessageID, "settings")
}
