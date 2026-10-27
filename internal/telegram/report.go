package telegram

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/present"
	"github.com/itsgiahung/502Drive/internal/state"
)

// The daily report (S10): one short message per day, at a time chosen in the
// chat and read in the person's preferences time zone. Nothing happened, nothing
// is sent. The schedule is adapter state (settings table), the message goes
// through the durable outbox, so a restart neither loses nor repeats it, and
// quiet hours hold it like any other unsolicited notice.

const (
	reportNotePrefix = "report:"
	// reportMaxJobs bounds the jobs one report reads.
	reportMaxJobs = 5000
	defaultWindow = 24 * time.Hour
)

// reportConfig is one person's report schedule.
type reportConfig struct {
	Enabled bool   `json:"enabled"`
	At      string `json:"at"` // HH:MM in the preferences time zone
}

func reportSentKey(principal string) string { return "tg_report_sent:" + principal }

func validClock(s string) (hour, minute int, ok bool) {
	t, err := time.Parse("15:04", s)
	if err != nil || len(s) != 5 {
		return 0, 0, false
	}
	return t.Hour(), t.Minute(), true
}

func (t *Telegram) loadReportConfig(ctx context.Context, principal string) reportConfig {
	p, err := t.store.Preferences(ctx, principal)
	if err != nil {
		return reportConfig{}
	}
	if _, _, valid := validClock(p.DailyReportAt); !valid {
		return reportConfig{}
	}
	return reportConfig{Enabled: p.DailyReport, At: p.DailyReportAt}
}

// saveReportConfig stores the schedule through the preferences command (the
// same one the CLI and the HTTP API use), so the report time lives in
// daily_report / daily_report_at and nowhere else.
func (t *Telegram) saveReportConfig(ctx context.Context, principal string, cfg reportConfig) error {
	if _, _, ok := validClock(cfg.At); !ok {
		return errors.New("the report time must be HH:MM")
	}
	enabled, at := cfg.Enabled, cfg.At
	_, err := t.svc.PatchPreferences(ctx, principalByID(principal), "tg-report:"+state.ID("rep"),
		app.PreferencePatch{DailyReport: &enabled, DailyReportAt: &at})
	return err
}

// reportDue decides whether today's report should go out now. day is the local
// calendar date it belongs to; passing it back as lastSent after sending makes
// the report once per local day, also across DST jumps (a time that does not
// exist, or happens twice, still fires exactly once).
func reportDue(now time.Time, loc *time.Location, at, lastSent string) (bool, string) {
	hour, minute, ok := validClock(at)
	if !ok {
		return false, ""
	}
	local := now.In(loc)
	day := local.Format("2006-01-02")
	if lastSent == day {
		return false, day
	}
	target := time.Date(local.Year(), local.Month(), local.Day(), hour, minute, 0, 0, loc)
	return !local.Before(target), day
}

func (t *Telegram) window() time.Duration {
	if t.reportWindow > 0 {
		return t.reportWindow
	}
	return defaultWindow
}

// reportLoop checks once a minute whether somebody's report is due.
func (t *Telegram) reportLoop(ctx context.Context) {
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	t.reportTick(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			t.reportTick(ctx)
		}
	}
}

type reportTarget struct {
	p    app.Principal
	chat int64
}

// reportTargets are the people who can receive a report: the owner and the
// enabled Telegram operators, each in their own private chat.
func (t *Telegram) reportTargets(ctx context.Context) []reportTarget {
	var out []reportTarget
	if t.ownerID > 0 {
		out = append(out, reportTarget{app.Owner, t.ownerID})
	}
	ops, err := t.svc.Operators(ctx, app.Owner)
	if err != nil {
		return out
	}
	for _, op := range ops {
		if op.Enabled && op.TelegramUserID > 0 {
			out = append(out, reportTarget{app.Principal{ID: op.ID, Role: app.RoleOperator}, op.TelegramUserID})
		}
	}
	return out
}

func (t *Telegram) reportTick(ctx context.Context) {
	for _, target := range t.reportTargets(ctx) {
		if ctx.Err() != nil {
			return
		}
		t.reportFor(ctx, target)
	}
}

func (t *Telegram) reportFor(ctx context.Context, target reportTarget) {
	p := target.p
	cfg := t.loadReportConfig(ctx, p.ID)
	if !cfg.Enabled {
		return
	}
	prefs, err := t.store.Preferences(ctx, p.ID)
	if err != nil {
		return
	}
	loc := time.UTC
	if l, lerr := time.LoadLocation(prefs.Timezone); lerr == nil && prefs.Timezone != "" {
		loc = l
	}
	last, _, _ := t.store.Setting(ctx, reportSentKey(p.ID))
	now := t.now()
	due, day := reportDue(now, loc, cfg.At, last)
	if !due {
		return
	}
	start := now.Add(-t.window())
	stats, err := t.reportStats(ctx, p, start, now)
	if err != nil {
		t.log.Warn("telegram: cannot build the daily report", "err", err)
		return
	}
	if !stats.empty() {
		note := fmt.Sprintf("%s%d:%d", reportNotePrefix, start.Unix(), now.Unix())
		if _, err = t.store.EnqueueFinal(ctx, p.ID, "", target.chat, note, "report:"+p.ID+":"+day, now); err != nil {
			t.log.Warn("telegram: cannot queue the daily report", "err", err)
			return
		}
	}
	// One decision per local day, also when there was nothing to say.
	if err = t.store.SetSetting(ctx, reportSentKey(p.ID), day); err != nil {
		t.log.Warn("telegram: cannot record the daily report", "err", err)
	}
}

// reportStats summarises what happened for one person in [start, end]. The
// window comes from the server-side history query (jobs updated since start)
// and the blocked count from the state filter, so the cost follows the window,
// not the size of the history.
type reportStats struct {
	Done, Failed, Blocked, Sync int
	Bytes                       int64
	Seconds                     float64
}

func (s reportStats) empty() bool { return s.Done+s.Failed+s.Blocked+s.Sync == 0 }

func (t *Telegram) reportStats(ctx context.Context, p app.Principal, start, end time.Time) (reportStats, error) {
	var s reportStats
	blocked, _, err := t.jobsWhere(ctx, p, false, state.JobBlocked, "", reportMaxJobs)
	if err != nil {
		return s, err
	}
	for _, j := range blocked {
		if j.Kind == "" { // audits, repairs and cleanups are not copy work
			s.Blocked++
		}
	}
	var cur state.HistoryCursor
	for seen := 0; seen < reportMaxJobs; {
		jobs, next, err := t.svc.History(ctx, p, false, start, igPageSize, cur)
		if err != nil {
			return s, err
		}
		for _, j := range jobs {
			seen++
			if j.Kind != "" || j.State == state.JobBlocked {
				continue
			}
			if !state.IsTerminal(j.State) || j.UpdatedAt.Before(start) || j.UpdatedAt.After(end) {
				continue
			}
			initial := j.Trigger == "initial"
			switch j.State {
			case state.JobSucceeded:
				if initial {
					s.Done++
				} else if j.Counts.Observed > 0 {
					s.Sync++ // a sync run that changed nothing is not news
				} else {
					continue
				}
			case state.JobFailed, state.JobPartial:
				s.Failed++
				if !initial {
					s.Sync++
				}
			default:
				continue
			}
			if secs := j.UpdatedAt.Sub(j.CreatedAt).Seconds(); secs > 0 && j.Counts.Bytes > 0 {
				s.Bytes += j.Counts.Bytes
				s.Seconds += secs
			}
		}
		if next.RowID == 0 {
			break
		}
		cur = next
	}
	return s, nil
}

// principalByID rebuilds the principal of an outbox row.
func principalByID(id string) app.Principal {
	if id == app.Owner.ID {
		return app.Owner
	}
	return app.Principal{ID: id, Role: app.RoleOperator}
}

// reportText renders a report row ("report:<start>:<end>").
func (t *Telegram) reportText(ctx context.Context, m state.Outbox, tx texts) (string, bool) {
	var startUnix, endUnix int64
	if _, err := fmt.Sscanf(strings.TrimPrefix(m.Note, reportNotePrefix), "%d:%d", &startUnix, &endUnix); err != nil {
		return "", false
	}
	start, end := time.Unix(startUnix, 0).UTC(), time.Unix(endUnix, 0).UTC()
	stats, err := t.reportStats(ctx, principalByID(m.Principal), start, end)
	if err != nil {
		return "", false
	}
	v := t.jobViewFor(ctx, m.Principal)
	return renderReport(tx, stats, end.In(v.loc)), true
}

func renderReport(tx texts, s reportStats, at time.Time) string {
	r := tx.g().reportBody
	var b strings.Builder
	fmt.Fprintf(&b, r.title+"\n", at.Format("2006-01-02 15:04"))
	if s.Done > 0 {
		fmt.Fprintf(&b, r.done+"\n", s.Done)
	}
	if s.Failed > 0 {
		fmt.Fprintf(&b, r.failed+"\n", s.Failed)
	}
	if s.Blocked > 0 {
		fmt.Fprintf(&b, r.blocked+"\n", s.Blocked)
	}
	if s.Sync > 0 {
		fmt.Fprintf(&b, r.sync+"\n", s.Sync)
	}
	if s.Bytes > 0 {
		fmt.Fprintf(&b, r.bytes+"\n", present.HumanBytes(s.Bytes))
		if s.Seconds > 0 {
			fmt.Fprintf(&b, r.speed+"\n", present.HumanBytes(int64(float64(s.Bytes)/s.Seconds)))
		}
	}
	if s.Failed > 0 || s.Blocked > 0 {
		b.WriteString("\n" + r.footer)
	}
	return strings.TrimRight(b.String(), "\n")
}
