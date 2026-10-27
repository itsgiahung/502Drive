package tui

import (
	"context"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/itsgiahung/502Drive/internal/i18n"
)

// The Settings tab is a short list of rows the cursor moves through. Each row
// changes one thing at once and saves it through a fixed action; a failed save
// is shown and the next refresh brings back the real value.

type settingsRow int

const (
	rowLanguage settingsRow = iota
	rowAutostart
	rowQuiet
	rowRetention
	rowNotifyDone
	rowNotifyFailed
	rowNotifyAction
	rowAfterSync
)

// retentionChoices are the history windows ←/→ step through (0 keeps all).
var retentionChoices = []int{7, 30, 90, 180, 365, 0}

// langOrder is the order language changes in: English, Vietnamese, Auto.
var langOrder = []string{"en", "vi", "auto"}

// settingsRows lists the rows this build can change, in screen order. A row
// whose action is missing is not shown rather than shown dead.
func (m Model) settingsRows() []settingsRow {
	var rows []settingsRow
	if m.acts.SetLanguage != nil {
		rows = append(rows, rowLanguage)
	}
	if m.acts.SetAutostart != nil && m.snap.Prefs.AutostartKnown {
		rows = append(rows, rowAutostart)
	}
	if m.acts.SetPreference != nil {
		rows = append(rows, rowQuiet, rowRetention, rowNotifyDone, rowNotifyFailed, rowNotifyAction, rowAfterSync)
	}
	return rows
}

func (m Model) currentSettingsRow() (settingsRow, bool) {
	rows := m.settingsRows()
	if m.cursor < 0 || m.cursor >= len(rows) {
		return 0, false
	}
	return rows[m.cursor], true
}

// settingsStep acts on the row under the cursor; step is +1 (next value,
// toggle) or -1 (previous value).
func (m Model) settingsStep(step int) (tea.Model, tea.Cmd) {
	row, ok := m.currentSettingsRow()
	if !ok {
		return m, nil
	}
	p := &m.snap.Prefs
	switch row {
	case rowLanguage:
		return m.cycleLanguage(step)
	case rowAutostart:
		on := !p.Autostart
		p.Autostart = on
		acts := m.acts
		return m, m.act(func(context.Context) error { return acts.SetAutostart(on) })
	case rowQuiet:
		ti := newInput(m.width, "22:00-07:00")
		ti.SetValue(quietText(*p))
		ti.CursorEnd()
		m.setInput = ti
		return m, textinput.Blink
	case rowRetention:
		next := stepRetention(p.RetentionDays, step)
		p.RetentionDays = next
		return m, m.setPref("retention_days", next)
	case rowNotifyDone:
		p.NotifyCompleted = !p.NotifyCompleted
		return m, m.setPref("notify_completed", p.NotifyCompleted)
	case rowNotifyFailed:
		p.NotifyFailed = !p.NotifyFailed
		return m, m.setPref("notify_failed", p.NotifyFailed)
	case rowNotifyAction:
		p.NotifyAction = !p.NotifyAction
		return m, m.setPref("notify_action_required", p.NotifyAction)
	case rowAfterSync:
		next := stepAfterSync(p.AfterCopySync, step)
		p.AfterCopySync = next
		return m, m.setPref("after_copy_sync_mode", next)
	}
	return m, nil
}

func (m Model) setPref(field string, value any) tea.Cmd {
	acts := m.acts
	return m.act(func(ctx context.Context) error { return acts.SetPreference(ctx, field, value) })
}

// stepRetention moves to the next (or previous) window. From a value that is
// not on the list, "next" is the first preset above it and "previous" the last
// one below it; past either end the list wraps through "forever" (0).
func stepRetention(cur, step int) int {
	n := len(retentionChoices)
	for i, v := range retentionChoices {
		if v == cur {
			return retentionChoices[((i+step)%n+n)%n]
		}
	}
	if step > 0 {
		for _, v := range retentionChoices {
			if v != 0 && v > cur {
				return v
			}
		}
		return 0
	}
	best := 0
	for _, v := range retentionChoices {
		if v != 0 && v < cur {
			best = v
		}
	}
	return best
}

// stepAfterSync moves to the next (or previous) after-copy sync mode, wrapping
// around; an unknown or empty value counts as "off".
func stepAfterSync(cur string, step int) string {
	n := len(afterModes)
	idx := 0
	for i, v := range afterModes {
		if v == cur {
			idx = i
		}
	}
	return afterModes[((idx+step)%n+n)%n]
}

// afterSyncLabel names a mode for the user.
func (m Model) afterSyncLabel(mode string) string {
	switch mode {
	case afterRealtime:
		return m.t(i18n.AfterRealtime)
	case afterHourly:
		return m.t(i18n.AfterHourly)
	case afterSixHourly:
		return m.t(i18n.AfterSixHourly)
	case afterDaily:
		return m.t(i18n.AfterDaily)
	}
	return m.t(i18n.AfterOff)
}

// quietText is "22:00-07:00", or "" when quiet hours are off.
func quietText(p Prefs) string {
	if p.QuietStart == "" || p.QuietEnd == "" {
		return ""
	}
	return p.QuietStart + "-" + p.QuietEnd
}

// parseQuiet reads "22:00-07:00" (a dash, an en dash or a space between the
// two times); empty means off.
func parseQuiet(s string) (start, end string, ok bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", "", true
	}
	s = strings.NewReplacer("–", "-", "—", "-", " ", "-").Replace(s)
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == '-' })
	if len(parts) != 2 {
		return "", "", false
	}
	a, err1 := time.Parse("15:04", parts[0])
	b, err2 := time.Parse("15:04", parts[1])
	if err1 != nil || err2 != nil {
		return "", "", false
	}
	return a.Format("15:04"), b.Format("15:04"), true
}

func (m Model) settingsTyping(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		m.quits = true
		return m, tea.Quit
	case tea.KeyEsc:
		m.setInput = nil
		return m, nil
	case tea.KeyEnter:
		start, end, ok := parseQuiet(m.setInput.Value())
		if !ok {
			m.status = m.t(i18n.SetQuietBad)
			return m, nil
		}
		m.setInput = nil
		m.snap.Prefs.QuietStart, m.snap.Prefs.QuietEnd = start, end
		value := ""
		if start != "" {
			value = start + "-" + end
		}
		return m, m.setPref("quiet_hours", value)
	}
	var cmd tea.Cmd
	*m.setInput, cmd = m.setInput.Update(msg)
	return m, cmd
}

// ---- view ----

func (m Model) settingsView() []string {
	cur := m.effectiveLanguage()
	pick := func(label string, on bool) string {
		if on {
			return m.good("● " + label)
		}
		return m.dim("○ " + label)
	}
	check := func(on bool) string {
		if on {
			return m.good("[x] " + m.t(i18n.SetOn))
		}
		return m.dim("[ ] " + m.t(i18n.SetOff))
	}
	// Labels line up in one column (capped, so a long label in a narrow
	// terminal does not push the value off the edge).
	labelW := 12
	for _, row := range m.settingsRows() {
		labelW = max(labelW, visible(m.rowLabel(row)))
	}
	labelW = min(labelW, 18)
	line := func(mark string, label, value string) string {
		pad := strings.Repeat(" ", max(0, labelW-visible(label)))
		return mark + m.dim(label) + pad + " " + value
	}
	lines := []string{"", m.bold(m.t(i18n.Settings)), ""}
	rows := m.settingsRows()
	for i, row := range rows {
		mark := "  "
		if i == m.cursor && !m.tabFocus {
			mark = m.accent("> ")
		}
		p := m.snap.Prefs
		label := m.rowLabel(row)
		switch row {
		case rowLanguage:
			picks := pick("English", cur == "en") + "  " + pick("Tiếng Việt", cur != "en" && cur != "auto") + "  " + pick(m.t(i18n.SetLangAuto), cur == "auto")
			if m.width < 64 { // three choices do not fit beside the label
				lines = append(lines, mark+m.dim(label), "    "+picks)
				continue
			}
			lines = append(lines, line(mark, label, picks))
		case rowAutostart:
			lines = append(lines, line(mark, label, check(p.Autostart)))
		case rowQuiet:
			v := m.dim(m.t(i18n.SetQuietOff))
			if q := quietText(p); q != "" {
				v = q
			}
			lines = append(lines, line(mark, label, v))
		case rowRetention:
			v := m.tf(i18n.SetDays, p.RetentionDays)
			if p.RetentionDays == 0 {
				v = m.t(i18n.SetForever)
			}
			lines = append(lines, line(mark, label, v))
		case rowNotifyDone:
			lines = append(lines, line(mark, label, check(p.NotifyCompleted)))
		case rowNotifyFailed:
			lines = append(lines, line(mark, label, check(p.NotifyFailed)))
		case rowNotifyAction:
			lines = append(lines, line(mark, label, check(p.NotifyAction)))
		case rowAfterSync:
			lines = append(lines, line(mark, label, m.afterSyncChoiceText(p.AfterCopySync)))
		}
	}
	lines = append(lines, line("  ", m.t(i18n.Timezone), Sanitize(orDash(m.snap.Prefs.Timezone))))
	if m.setInput != nil {
		lines = append(lines, "", m.accent(m.t(i18n.SetQuietPrompt)), m.setInput.View(), m.dim(m.t(i18n.SetKeysQuiet)))
		return lines
	}
	hint := m.t(i18n.SettingsKeys2)
	if len(rows) > 1 {
		hint = m.t(i18n.SetKeys)
	}
	lines = append(lines, "")
	for _, l := range wrapLines(hint, max(20, m.width-2)) {
		lines = append(lines, m.dim(l))
	}
	return lines
}

func (m Model) rowLabel(row settingsRow) string {
	switch row {
	case rowLanguage:
		return m.t(i18n.Language)
	case rowAutostart:
		return m.t(i18n.Autostart)
	case rowQuiet:
		return m.t(i18n.SetQuiet)
	case rowRetention:
		return m.t(i18n.SetRetention)
	case rowNotifyDone:
		return m.t(i18n.SetNotifyDone)
	case rowNotifyFailed:
		return m.t(i18n.SetNotifyFailed)
	case rowNotifyAction:
		return m.t(i18n.SetNotifyAction)
	case rowAfterSync:
		return m.t(i18n.SetAfterSync)
	}
	return ""
}

// afterSyncChoiceText shows the five modes with the saved one marked; on a
// narrow terminal only the saved one is named.
func (m Model) afterSyncChoiceText(cur string) string {
	if m.width < 72 {
		return m.good("● " + m.afterSyncLabel(cur))
	}
	parts := make([]string, len(afterModes))
	for i, mode := range afterModes {
		if mode == cur || (cur == "" && mode == afterOff) {
			parts[i] = m.good("● " + m.afterSyncLabel(mode))
		} else {
			parts[i] = m.dim("○ " + m.afterSyncLabel(mode))
		}
	}
	return strings.Join(parts, "  ")
}
