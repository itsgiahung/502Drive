package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/itsgiahung/502Drive/internal/i18n"
	"github.com/itsgiahung/502Drive/internal/present"
)

// location is the user's timezone for clock times: the saved preference, else
// the machine's.
func (m Model) location() *time.Location {
	if tz := m.snap.Prefs.Timezone; tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			return l
		}
	}
	return time.Local
}

// when renders an instant as HH:MM, or with the date when it is not today.
func (m Model) when(t time.Time) string {
	loc := m.location()
	if t.In(loc).YearDay() == m.opt.now().In(loc).YearDay() && t.Sub(m.opt.now()) < 24*time.Hour {
		return t.In(loc).Format("15:04")
	}
	return i18n.FormatTime(t, loc, m.lang)
}

// ---- daemon lifecycle (T1) ----

// lifeShown says whether the daemon is in a state worth telling the user about.
func (m Model) lifeShown() bool {
	st := m.snap.Status
	if !m.online || st.Lifecycle == "" {
		return false
	}
	return st.Lifecycle != "running" || !st.LifecycleOnline
}

// lifeText is the banner for sleeping, reconnecting or "sign in again". It
// states the cause in words, so it reads the same without colour.
func (m Model) lifeText() string {
	if !m.lifeShown() {
		return ""
	}
	st := m.snap.Status
	switch st.Lifecycle {
	case "reconnect_required":
		return present.StateLabel("reconnect_required", m.lang) + " · " + m.t(i18n.LifeLoginHint)
	case "sleeping", "suspending":
		return present.StateLabel(st.Lifecycle, m.lang) + " · " + m.t(i18n.LifeSleepHint)
	}
	text := m.t(i18n.LifeReconnecting)
	if st.GapSeconds >= 60 {
		text += " (" + m.tf(i18n.LifeAfterSleep, present.HumanDuration(time.Duration(st.GapSeconds)*time.Second, m.lang)) + ")"
	}
	return text
}

// lifeBanner is lifeText wrapped to the width, one coloured line per row (none
// when all is well), so the list screens can count it in their layout.
func (m Model) lifeBanner() []string {
	t := m.lifeText()
	if t == "" {
		return nil
	}
	var out []string
	for _, l := range wrapLines(t, max(20, m.width-2)) {
		out = append(out, m.warn(l))
	}
	return out
}

// lifeChip is the short marker on the tab bar, ASCII only.
func (m Model) lifeChip() string {
	if !m.lifeShown() {
		return ""
	}
	switch m.snap.Status.Lifecycle {
	case "sleeping", "suspending":
		return m.warn("zZ")
	case "reconnect_required":
		return ""
	}
	return m.warn("~")
}

// ---- quota wait (Q1) ----

// waitingForQuota reports a blocked job that continues by itself at a known
// time: it needs no retry.
func (m Model) waitingForQuota(j Job) bool {
	return j.State == "blocked" && j.ResumeAfter.After(m.opt.now())
}

func (m Model) quotaText(j Job) string {
	return m.tf(i18n.QuotaWait, m.when(j.ResumeAfter))
}

// jobEligibleFor is jobEligible with the one rule that needs the whole job: a
// job waiting for quota resumes by itself, so "retry" is not offered.
func (m Model) jobEligibleFor(action string, j Job) bool {
	if action == "retry" && m.waitingForQuota(j) {
		return false
	}
	return jobEligible(action, j.State)
}

// ---- job kind, bytes and skip reasons (S1, L1, K1) ----

func (m Model) kindLabel(kind string) string {
	switch kind {
	case "audit":
		return m.t(i18n.JobKindAudit)
	case "repair":
		return m.t(i18n.JobKindRepair)
	case "cleanup":
		return m.t(i18n.JobKindClean)
	}
	return Sanitize(kind)
}

var skipKeys = map[string]i18n.Key{
	"filtered":                  i18n.SkipFiltered,
	"conflict":                  i18n.SkipConflict,
	"shortcut_unresolved":       i18n.SkipShortUnres,
	"shortcut_loop":             i18n.SkipShortLoop,
	"cannot_copy":               i18n.SkipCannotCopy,
	"google_native_export_only": i18n.SkipNativeOnly,
	"too_large_for_quota":       i18n.SkipTooLarge,
	"unreadable":                i18n.SkipUnreadable,
	"trashed_source":            i18n.SkipTrashed,
}

// skipLabel names a skip code; an unknown (newer) code is shown as it is.
func (m Model) skipLabel(code string) string {
	if k, ok := skipKeys[code]; ok {
		return m.t(k)
	}
	return Sanitize(code)
}

// skippedLines lists the skipped items by reason, biggest first, ties by code,
// so the order does not change between refreshes.
func (m Model) skippedLines(j Job) []string {
	if len(j.SkippedByCode) == 0 {
		return nil
	}
	codes := make([]string, 0, len(j.SkippedByCode))
	for c, n := range j.SkippedByCode {
		if n > 0 {
			codes = append(codes, c)
		}
	}
	sort.Slice(codes, func(a, b int) bool {
		if j.SkippedByCode[codes[a]] != j.SkippedByCode[codes[b]] {
			return j.SkippedByCode[codes[a]] > j.SkippedByCode[codes[b]]
		}
		return codes[a] < codes[b]
	})
	if len(codes) == 0 {
		return nil
	}
	lines := []string{m.bold(m.t(i18n.SkippedTitle))}
	for _, c := range codes {
		text := fmt.Sprintf("  %5d  %s", j.SkippedByCode[c], m.skipLabel(c))
		lines = append(lines, cut(text, max(10, m.width-2)))
	}
	return lines
}

// bytesText formats a byte count with the shared presenter.
func bytesText(n int64) string { return present.HumanBytes(n) }

// wrapLines breaks text to the width so a long hint is read in full on a
// narrow terminal instead of being cut at the edge.
func wrapLines(text string, width int) []string {
	width = max(12, width)
	var out []string
	for _, para := range strings.Split(text, "\n") {
		out = append(out, strings.Split(wrapWords(para, width), "\n")...)
	}
	return out
}

// wrapWords is a plain word wrap by display width (the strings here hold no
// colour codes when they reach it).
func wrapWords(s string, width int) string {
	words := strings.Fields(s)
	var lines []string
	cur := ""
	for _, w := range words {
		switch {
		case cur == "":
			cur = w
		case visible(cur+" "+w) <= width:
			cur += " " + w
		default:
			lines = append(lines, cur)
			cur = w
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return strings.Join(lines, "\n")
}

// orElse is a if it is not empty, else b.
func orElse(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
