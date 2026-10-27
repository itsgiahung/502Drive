// Package tray is the desktop tray launcher: a thin HTTP client that shows
// daemon state and shortcuts. Icon drawing, menu state and OS commands are
// kept free of GUI code so they can be tested without a desktop.
package tray

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"sync"
	"time"

	"github.com/itsgiahung/502Drive/internal/autostart"
	"github.com/itsgiahung/502Drive/internal/i18n"
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
	// StopDaemon is what the confirmed "Stop 502Drive…" item (and, with
	// QuitStopsDaemon, Quit) calls: a graceful POST /v1/shutdown.
	StartDaemon func(ctx context.Context) (string, error)
	StopDaemon  func(ctx context.Context) (string, error)
	// QuitStopsDaemon restores the old behaviour where quitting the tray also
	// stops the daemon. Off by default: Quit leaves 502Drive running.
	QuitStopsDaemon bool
	// UpdateCheck, when set, is called about once a day and may return a note
	// such as "update available: v1.4.0". It is opt-in (config check_updates).
	UpdateCheck func(ctx context.Context) (string, error)
	// NoNotify turns off desktop notifications for finished jobs.
	NoNotify bool
	// Language reads the UI language preference (vi/en); nil means English.
	Language func(ctx context.Context) (string, error)
	// Quiet reports whether desktop notifications are suppressed right now
	// (quiet hours). Nil means never quiet.
	Quiet func(ctx context.Context) (bool, error)
	// AutostartToggle flips autostart. If nil, autostart.Toggle is used.
	AutostartToggle func() (bool, error)
	// QueuePause pauses (true) or resumes (false) the whole job queue through
	// the daemon API. Nil disables the item.
	QueuePause func(ctx context.Context, paused bool) error
	Log        *slog.Logger
	Now        func() time.Time
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
	// stopArmed is the two-step confirmation of "Stop 502Drive…": the first
	// click only arms it; the second one inside stopConfirmWindow stops.
	stopArmed   bool
	stopArmedAt time.Time
	// sig is the signature of the last View shown. An unchanged view is not
	// pushed to the desktop again, so an idle tray does no menu work. It is
	// only read and written inside refresh, which refreshMu serializes.
	sig uint64
	// refreshMu serializes refresh: the poll loop and every menu action call
	// it, and two overlapping refreshes would race on sig and could push an
	// older snapshot over a newer one.
	refreshMu sync.Mutex
}

// stopConfirmWindow is how long the armed confirmation stays valid. It is
// short because the next poll (3–10s) disarms it again anyway.
const stopConfirmWindow = 10 * time.Second

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
	a.refreshMu.Lock()
	defer a.refreshMu.Unlock()
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
	now := a.o.now()
	// A stale confirmation disarms itself: the next poll (or going offline)
	// clears it, so a mis-click can never linger for minutes.
	if !online || a.stopArmed && now.Sub(a.stopArmedAt) > stopConfirmWindow {
		a.stopArmed = false
	}
	armed := a.stopArmed
	note := a.note
	last := a.snap
	a.mu.Unlock()

	view := BuildViewLang(last, online, note, a.language())
	if armed {
		active := last.Status.Running + last.Status.Queued
		for i := range view.Items {
			if view.Items[i].ID == ActionStopDaemon {
				view.Items[i].Title = i18n.Tf(a.language(), i18n.StopAppConfirm, active)
			}
		}
	}
	if s := viewSignature(view); s != a.sig {
		a.sig = s
		if showErr := a.p.show(view); showErr != nil {
			a.o.log().Warn("tray: cannot update the menu", "err", showErr)
		}
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
		// Quitting the tray keeps the daemon running unless the user opted in
		// (tray_quit_stops_daemon) — closing the tray is not a shutdown.
		if a.o.QuitStopsDaemon && a.o.StopDaemon != nil {
			stopCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			if _, derr := a.o.StopDaemon(stopCtx); derr != nil {
				a.o.log().Warn("tray: cannot stop the daemon on quit", "err", derr)
			}
			cancel()
		}
		a.p.quit()
		return
	case ActionStopDaemon:
		err = a.stopDaemonAction(ctx)
	case ActionQueueToggle:
		err = a.toggleQueue(ctx)
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
		// The toggle only starts the daemon; stopping is the confirmed item.
		a.mu.Lock()
		online := a.on
		a.mu.Unlock()
		if online {
			return
		}
		var out string
		if a.o.StartDaemon != nil {
			out, err = a.o.StartDaemon(ctx)
		} else {
			out, err = ControlDaemon(ctx, a.o.Host, DaemonStart, a.o.Exe)
		}
		if err == nil {
			a.setNote(out)
		}
	case ActionAutostart:
		toggleFn := autostart.Toggle
		if a.o.AutostartToggle != nil {
			toggleFn = a.o.AutostartToggle
		}
		on, aerr := toggleFn()
		if aerr != nil {
			err = aerr
		} else {
			if on {
				a.setNote(i18n.T(a.language(), i18n.Autostart) + " ✓")
			} else {
				a.setNote(i18n.T(a.language(), i18n.Autostart))
			}
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

// stopDaemonAction implements the two-step "Stop 502Drive…": with jobs in
// flight the first click only arms the confirmation; a second click inside
// stopConfirmWindow stops. Without jobs it stops at once.
func (a *agent) stopDaemonAction(ctx context.Context) error {
	a.mu.Lock()
	online, active := a.on, a.snap.Status.Running+a.snap.Status.Queued
	armed, armedAt := a.stopArmed, a.stopArmedAt
	a.mu.Unlock()
	if !online {
		return nil
	}
	now := a.o.now()
	if active > 0 && (!armed || now.Sub(armedAt) > stopConfirmWindow) {
		a.mu.Lock()
		a.stopArmed, a.stopArmedAt = true, now
		a.mu.Unlock()
		a.refresh(ctx)
		return nil
	}
	a.mu.Lock()
	a.stopArmed = false
	a.mu.Unlock()
	if a.o.StopDaemon == nil {
		return errors.New("this tray cannot stop the daemon")
	}
	out, derr := a.o.StopDaemon(ctx)
	if derr != nil {
		return derr
	}
	a.setNote(out)
	return nil
}

// toggleQueue pauses the whole queue, or resumes it when it is paused. It
// only calls the daemon; the next refresh shows the state the daemon reports.
func (a *agent) toggleQueue(ctx context.Context) error {
	if a.o.QueuePause == nil {
		return errors.New("this tray cannot pause the queue")
	}
	a.mu.Lock()
	paused := a.snap.Status.QueuePaused
	a.mu.Unlock()
	if err := a.o.QueuePause(ctx, !paused); err != nil {
		return err
	}
	if paused {
		a.setNote(i18n.T(a.language(), i18n.TrayResumeAll))
	} else {
		a.setNote(i18n.T(a.language(), i18n.TrayPauseAll))
	}
	return nil
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

// viewSignature hashes everything the desktop shows. Two equal signatures mean
// the menu, tooltip and icon state would not change, so nothing is pushed.
func viewSignature(v View) uint64 {
	h := fnv.New64a()
	write := func(parts ...any) { fmt.Fprintln(h, parts...) }
	write("state", v.State, "title", v.Title, "tip", v.Tooltip)
	for _, it := range v.Items {
		writeItem(write, it)
	}
	return h.Sum64()
}

func writeItem(write func(...any), it Item) {
	write("item", it.ID, it.Title, it.Enabled, it.Hidden, it.Separator)
	for _, c := range it.Children {
		writeItem(write, c)
	}
}
