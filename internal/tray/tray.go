// Package tray is the desktop tray launcher: a thin HTTP client that shows
// daemon state and shortcuts. Icon drawing, menu state and OS commands are
// kept free of GUI code so they can be tested without a desktop.
package tray

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

// ErrUnsupported is returned on platforms without a tray backend (macOS).
var ErrUnsupported = errors.New("the tray needs a Linux or Windows desktop session; use `502drive menu` instead")

// Options is what the tray needs from the CLI layer.
type Options struct {
	Version string
	// Motion enables the 4 fps busy icon; --no-motion turns it off.
	Motion bool
	// Exe is the 502drive binary used to open the TUI and start the daemon.
	Exe  string
	Host Host
	// Status and Jobs are the only daemon reads the tray performs.
	Status func(ctx context.Context) (Status, error)
	Jobs   func(ctx context.Context) ([]Job, error)
	// Copy submits one validated Drive link and returns the job id.
	Copy func(ctx context.Context, link string) (string, error)
	// StartDaemon and StopDaemon replace the OS-specific start/stop when set.
	// StopDaemon is what Quit calls: the tray is the one place that ends the
	// background daemon.
	StartDaemon func(ctx context.Context) (string, error)
	StopDaemon  func(ctx context.Context) (string, error)
	// UpdateCheck, when set, is called about once a day and may return a note
	// such as "update available: v1.4.0". It is opt-in (config check_updates).
	UpdateCheck func(ctx context.Context) (string, error)
	// NoNotify turns off desktop notifications for finished jobs.
	NoNotify bool
	// Language reads the UI language preference (vi/en); nil means English.
	Language func(ctx context.Context) (string, error)
	Log      *slog.Logger
	Now      func() time.Time
}

func (o Options) now() time.Time {
	if o.Now != nil {
		return o.Now()
	}
	return time.Now()
}

func (o Options) log() *slog.Logger {
	if o.Log != nil {
		return o.Log
	}
	return slog.New(slog.DiscardHandler)
}

// presenter renders a View on the desktop.
type presenter interface {
	show(View) error
	setIcon(State, int) error
	quit()
}

// Run shows the tray and blocks until the user quits or ctx is cancelled.
func Run(ctx context.Context, o Options) error {
	if o.Status == nil || o.Jobs == nil || o.Copy == nil {
		return errors.New("tray: status, jobs and copy are required")
	}
	if o.Host.GOOS == "" {
		o.Host = DefaultHost()
	}
	return runTray(ctx, o)
}

// agent owns the poll loop, the icon animation and the fixed actions.
type agent struct {
	o    Options
	mu   sync.Mutex
	snap Snapshot
	on   bool
	note string
	lang string
	p    presenter
	// primed is set after the first successful poll (the notification baseline).
	primed bool
}

func newAgent(o Options) *agent {
	lang := "en"
	if o.Language == nil {
		lang = "en"
	}
	return &agent{o: o, lang: lang}
}

// refreshLanguage picks up a changed language preference.
func (a *agent) refreshLanguage(ctx context.Context) {
	if a.o.Language == nil {
		return
	}
	lang, err := a.o.Language(ctx)
	if err != nil || lang == "" {
		return
	}
	a.mu.Lock()
	a.lang = lang
	a.mu.Unlock()
}

func (a *agent) language() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.lang
}

// collect polls the daemon. A status failure means offline; a jobs failure
// keeps the last data and reports a note.
func (a *agent) collect(ctx context.Context) (Snapshot, error) {
	status, err := a.o.Status(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	snap := Snapshot{Status: status, At: a.o.now()}
	jobs, err := a.o.Jobs(ctx)
	if err != nil {
		a.o.log().Warn("tray: cannot read jobs", "err", err)
	} else {
		snap.Jobs = jobs
	}
	return snap, nil
}

// refresh polls once and renders the result. It returns the view and whether
// the daemon answered.
func (a *agent) refresh(ctx context.Context) (View, bool) {
	a.refreshLanguage(ctx)
	snap, err := a.collect(ctx)
	online := err == nil
	if err == nil {
		a.mu.Lock()
		prev, primed := a.snap.Jobs, a.primed
		a.snap, a.primed = snap, true
		lang := a.lang
		a.mu.Unlock()
		if primed {
			a.notify(ctx, jobTransitions(prev, snap.Jobs, lang))
		}
	}
	a.mu.Lock()
	a.on = online
	note := a.note
	last := a.snap
	a.mu.Unlock()

	view := BuildViewLang(last, online, note, a.language())
	if showErr := a.p.show(view); showErr != nil {
		a.o.log().Warn("tray: cannot update the menu", "err", showErr)
	}
	return view, online
}

// pollLoop refreshes the tray at the documented cadence.
func (a *agent) pollLoop(ctx context.Context) {
	failures := 0
	nextUpdateCheck := a.o.now().Add(time.Minute)
	for {
		if a.o.UpdateCheck != nil && !a.o.now().Before(nextUpdateCheck) {
			nextUpdateCheck = a.o.now().Add(24 * time.Hour)
			if note, err := a.o.UpdateCheck(ctx); err == nil && note != "" {
				a.setNote(note)
			}
		}
		view, online := a.refresh(ctx)
		if online {
			failures = 0
		} else {
			failures++
			a.o.log().Warn("tray: daemon unreachable", "failures", failures)
		}
		wait := PollDelay(online, view.State, failures)
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

// iconLoop animates the busy icon at 4 fps at most, and only when motion is on.
func (a *agent) iconLoop(ctx context.Context) {
	tick := time.NewTicker(busyFrameMS * time.Millisecond)
	defer tick.Stop()
	last := -1
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			a.mu.Lock()
			online := a.on
			state := Classify(a.snap, online)
			a.mu.Unlock()
			if !a.o.Motion || state != StateBusy {
				last = -1
				continue
			}
			frame := FrameAt(a.o.now())
			if frame == last {
				continue
			}
			last = frame
			if err := a.p.setIcon(StateBusy, frame); err != nil {
				a.o.log().Warn("tray: cannot animate the icon", "err", err)
			}
		}
	}
}

// setNote records feedback for the next menu refresh.
func (a *agent) setNote(note string) {
	a.mu.Lock()
	a.note = note
	a.mu.Unlock()
}

// action runs one fixed menu action. The id is never user text.
func (a *agent) action(ctx context.Context, id string) {
	var err error
	switch id {
	case ActionQuit:
		if a.o.StopDaemon != nil {
			stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			if _, derr := a.o.StopDaemon(stopCtx); derr != nil {
				a.o.log().Warn("tray: cannot stop the daemon on quit", "err", derr)
			}
			cancel()
		}
		a.p.quit()
		return
	case ActionOpenTUI, ActionOpenTUI + ":jobs":
		err = OpenTUI(a.o.Host, a.o.Exe)
	case ActionPaste:
		err = a.paste(ctx)
	case ActionOpenResult:
		a.mu.Lock()
		url := latestResult(a.snap)
		a.mu.Unlock()
		if url == "" {
			err = errors.New("no completed copy yet")
		} else {
			err = OpenURL(ctx, a.o.Host, url)
		}
	case ActionDaemon:
		a.mu.Lock()
		online := a.on
		a.mu.Unlock()
		action := DaemonStart
		if online {
			action = DaemonStop
		}
		var out string
		var derr error
		switch {
		case online && a.o.StopDaemon != nil:
			out, derr = a.o.StopDaemon(ctx)
		case !online && a.o.StartDaemon != nil:
			out, derr = a.o.StartDaemon(ctx)
		default:
			out, derr = ControlDaemon(ctx, a.o.Host, action, a.o.Exe)
		}
		err = derr
		if err == nil {
			a.setNote(out)
		}
	default:
		return
	}
	if err != nil {
		a.o.log().Warn("tray: action failed", "action", id, "err", err)
		a.setNote(err.Error())
		a.refresh(ctx)
		return
	}
	// Show the result immediately instead of waiting for the poll tick.
	a.refresh(ctx)
}

// paste copies the Drive link found in the clipboard.
func (a *agent) paste(ctx context.Context) error {
	link, err := ReadDriveLink(ctx, a.o.Host)
	if err != nil {
		return err
	}
	id, err := a.o.Copy(ctx, link)
	if err != nil {
		return err
	}
	a.setNote("copied " + id)
	return nil
}
