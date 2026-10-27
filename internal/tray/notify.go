package tray

import (
	"context"
	"strings"
	"time"

	"github.com/itsgiahung/502Drive/internal/i18n"
	"github.com/itsgiahung/502Drive/internal/present"
)

// Notice is one desktop notification.
type Notice struct {
	Kind, Reason, JobID, ActionLabel string
	Title                            string
	Body                             string
}

// jobTransitions reports jobs that reached an end state since the previous
// poll. The first poll only primes the baseline, so opening the tray never
// replays old results.
func jobTransitions(prev, cur []Job, lang string) []Notice {
	before := make(map[string]Job, len(prev))
	for _, j := range prev {
		before[j.ID] = j
	}
	var out []Notice
	for _, j := range cur {
		old, seen := before[j.ID]
		if (!seen && j.State != "blocked") || (old.State == j.State && old.LastErrorCode == j.LastErrorCode) {
			continue
		}
		var title, kind string
		switch j.State {
		case "succeeded":
			title = i18n.T(lang, i18n.NoticeDone)
			kind = "completed"
		case "partial":
			title = i18n.T(lang, i18n.NoticePart)
			kind = "failed"
		case "failed":
			title = i18n.T(lang, i18n.NoticeFail)
			kind = "failed"
		case "blocked":
			code := j.LastErrorCode
			if code == "" {
				code = "needs_action"
			}
			e := present.Explain(code, lang)
			out = append(out, Notice{Kind: "action_required", Reason: code, JobID: j.ID, Title: e.Title, Body: plain(j.SourceName) + " · " + plain(e.Message+" "+e.Action)})
			continue
		default:
			continue
		}
		out = append(out, Notice{Kind: kind, Title: title, Body: plain(j.SourceName)})
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
	if len(notices) == 0 || a.o.NoNotify || a.o.Host.GOOS != "linux" {
		return
	}
	if a.o.Quiet != nil {
		quiet, err := a.o.Quiet(ctx)
		if err != nil || quiet {
			return
		}
	}
	for _, n := range notices {
		if a.o.NotifyEnabled != nil {
			enabled, err := a.o.NotifyEnabled(ctx, n.Kind)
			if err != nil || !enabled {
				continue
			}
		}
		if n.Reason != "" {
			a.mu.Lock()
			last, seen := a.lastNotice[n.Reason]
			a.mu.Unlock()
			if seen && a.o.now().Sub(last) < time.Minute {
				continue
			}
		}
		sent := false
		if n.Kind == "action_required" && a.o.OpenJob != nil && a.o.Host.Notify != nil {
			id := n.JobID
			n.ActionLabel = i18n.T(a.language(), i18n.NotificationOpen)
			sent = a.o.Host.Notify(ctx, n, func() { _ = a.o.OpenJob(id) }) == nil
		}
		if !sent && commandExists(a.o.Host, "notify-send") {
			_, sent = runHost(ctx, a.o.Host, notifyArgv(n))
		}
		if sent && n.Reason != "" {
			a.mu.Lock()
			if a.lastNotice == nil {
				a.lastNotice = map[string]time.Time{}
			}
			a.lastNotice[n.Reason] = a.o.now()
			a.mu.Unlock()
		}
	}
}
