package tui

import (
	"github.com/itsgiahung/502Drive/internal/i18n"
)

// describe returns the one-line explanation of the highlighted row, wrapped
// and dimmed, preceded by a blank line. Nothing when there is no text.
func (m Model) describe(key i18n.Key) []string {
	if key == "" {
		return nil
	}
	out := []string{""}
	for _, l := range wrapLines(m.t(key), max(20, m.width-4)) {
		out = append(out, "  "+m.dim(l))
	}
	return out
}

var syncChoiceDesc = map[string]i18n.Key{
	"d": i18n.DescPresetDaily, "h": i18n.DescPresetHourly, "e": i18n.DescPresetSixH, "t": i18n.DescPresetReal,
	"n": i18n.DescRunNow, "p": i18n.DescPauseSync, "u": i18n.DescResumeSync, "x": i18n.DescStopSync,
}

var detailRowDesc = map[int]i18n.Key{
	0: i18n.DescRowCopyState, 1: i18n.DescRowSync, 2: i18n.DescRowOutput,
	3: i18n.DescRowFilter, 4: i18n.DescRowIntegrity, 5: i18n.DescRowProtection,
}

var settingsRowDesc = map[settingsRow]i18n.Key{
	rowLanguage: i18n.DescSetLanguage, rowAutostart: i18n.DescSetAutostart, rowQuiet: i18n.DescSetQuiet,
	rowRetention: i18n.DescSetRetention, rowNotifyDone: i18n.DescSetNotifyDone, rowNotifyFailed: i18n.DescSetNotifyFail,
	rowNotifyAction: i18n.DescSetNotifyAct, rowAfterSync: i18n.DescSetAfterSync, rowClearHistory: i18n.DescSetClearHist,
}
