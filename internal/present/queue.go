package present

import "github.com/itsgiahung/502Drive/internal/i18n"

func Queue(ahead *int, paused bool, lang string) string {
	if paused {
		return i18n.T(lang, i18n.QueuePausedBanner)
	}
	if ahead != nil && *ahead >= 0 {
		return i18n.Tf(lang, i18n.QueueAhead, *ahead)
	}
	return StateLabel("queued", lang)
}
