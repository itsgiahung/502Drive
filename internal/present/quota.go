package present

import (
	"github.com/itsgiahung/502Drive/internal/i18n"
	"time"
)

type QuotaStatus struct {
	Used24hBytes int64      `json:"used_24h_bytes"`
	LimitKnown   bool       `json:"limit_known"`
	ResumeAfter  *time.Time `json:"resume_after"`
}

func QuotaLine(q QuotaStatus, loc *time.Location, lang string) string {
	text := i18n.Tf(lang, i18n.QuotaCopied24h, HumanBytes(q.Used24hBytes))
	if q.ResumeAfter != nil {
		text += "; " + i18n.Tf(lang, i18n.QuotaResumeAt, i18n.FormatTime(*q.ResumeAfter, loc, lang))
	}
	return text
}
