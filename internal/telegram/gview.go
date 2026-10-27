package telegram

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/itsgiahung/502Drive/internal/state"
)

// jobViewFor builds the clock and zone for a principal's texts. A missing or
// invalid time zone falls back to UTC (the label always says which zone).
func (t *Telegram) jobViewFor(ctx context.Context, principalID string) jobView {
	v := jobView{now: t.now(), loc: time.UTC}
	prefs, err := t.store.Preferences(ctx, principalID)
	if err != nil {
		return v
	}
	if loc, lerr := time.LoadLocation(prefs.Timezone); lerr == nil && prefs.Timezone != "" {
		v.loc = loc
	}
	return v
}

// quotaBlocked reports a job that waits for Google's quota and resumes by
// itself (Q1): there is nothing for the user to retry.
func quotaBlocked(j state.Job) bool {
	return j.State == state.JobBlocked && j.ResumeAfter != nil
}

// quotaWaitLine says when a quota-blocked job continues, in the reader's zone.
func quotaWaitLine(g gTexts, j state.Job, v jobView) string {
	if !quotaBlocked(j) {
		return ""
	}
	if !v.now.IsZero() && !j.ResumeAfter.After(v.now) {
		return g.quotaDue
	}
	loc := v.loc
	if loc == nil {
		loc = time.UTC
	}
	return fmt.Sprintf(g.quotaWait, j.ResumeAfter.In(loc).Format("15:04"), loc.String())
}

// skippedLine counts the skipped items by reason code (K1), most frequent first.
// Unknown codes stay visible as the raw code.
func skippedLine(g gTexts, c state.Counts) string {
	if len(c.SkippedByCode) == 0 {
		return ""
	}
	type kv struct {
		code string
		n    int
	}
	rows := make([]kv, 0, len(c.SkippedByCode))
	for code, n := range c.SkippedByCode {
		if n > 0 {
			rows = append(rows, kv{code, n})
		}
	}
	if len(rows) == 0 {
		return ""
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].n != rows[j].n {
			return rows[i].n > rows[j].n
		}
		return rows[i].code < rows[j].code
	})
	parts := make([]string, 0, len(rows))
	for _, r := range rows {
		parts = append(parts, fmt.Sprintf("%s ×%d", escape(g.skipLabel(r.code)), r.n))
	}
	return fmt.Sprintf("⏭ %s: %s", g.skipTitle, strings.Join(parts, " · "))
}
