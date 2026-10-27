package present

import "github.com/itsgiahung/502Drive/internal/i18n"

func BatchSummary(id string, done, total, needs int, lang string) string {
	return i18n.Tf(lang, i18n.BatchSummary, id, done, total, needs)
}
