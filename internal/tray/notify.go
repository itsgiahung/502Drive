package tray

import (
	"context"
	"strings"

	"github.com/GiaHung07/502Drive/internal/i18n"
)

// Notice is one desktop notification.
type Notice struct {
	Title string
	Body  string
}

// jobTransitions reports jobs that reached an end state since the previous
// poll. The first poll only primes the baseline, so opening the tray never
// replays old results.
func jobTransitions(prev, cur []Job, lang string) []Notice {
	before := make(map[string]string, len(prev))
	for _, j := range prev {
		before[j.ID] = j.State
	}
	var out []Notice
	for _, j := range cur {
		old, seen := before[j.ID]
		if !seen || old == j.State {
			continue
		}
		var title string
		switch j.State {
		case "succeeded":
			title = i18n.T(lang, i18n.NoticeDone)
		case "partial":
			title = i18n.T(lang, i18n.NoticePart)
		case "failed":
			title = i18n.T(lang, i18n.NoticeFail)
		default:
			continue
		}
		out = append(out, Notice{Title: title, Body: plain(j.SourceName)})
	}
	return out
}

// escapeMarkup stops a Drive name from being read as notification markup.
func escapeMarkup(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

// notifyArgv is the fixed command for a Linux desktop notification; the text
// is passed as separate arguments after "--", never through a shell.
func notifyArgv(n Notice) []string {
	return []string{"notify-send", "-a", "502Drive", "--", escapeMarkup(n.Title), escapeMarkup(n.Body)}
}

// notify shows each notice with notify-send when it exists. A missing program
// or a failure is not an error: notifications are a convenience.
func (a *agent) notify(ctx context.Context, notices []Notice) {
	if len(notices) == 0 || a.o.NoNotify || a.o.Host.GOOS != "linux" || !commandExists(a.o.Host, "notify-send") {
		return
	}
	for _, n := range notices {
		runHost(ctx, a.o.Host, notifyArgv(n))
	}
}
