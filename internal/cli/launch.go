package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/spf13/cobra"

	"github.com/itsgiahung/502Drive/internal/i18n"
	"github.com/itsgiahung/502Drive/internal/tray"
)

const (
	daemonStartWait = 8 * time.Second
	logRotateBytes  = 5 << 20
)

// launch is what a bare `502drive` does in a terminal: make sure the daemon and
// the tray are running (detached, so closing this tab changes nothing), then
// open the TUI. Quitting the TUI never stops the daemon; only the tray's Quit
// does.
func (a *App) launch(ctx context.Context) error {
	if err := a.ensureConfig(); err != nil {
		return err
	}
	res, err := a.bringUp(ctx)
	if err != nil {
		return err
	}
	if res.TrayNote != "" && res.Tray == "unavailable" {
		fmt.Fprintf(a.errw(), "tray: %s\n", res.TrayNote)
	}
	err = a.openUI(ctx)
	if a.io.StderrTTY && !a.g.quiet {
		fmt.Fprintln(a.errw(), i18n.T(a.language(ctx), i18n.StayRunning))
	}
	return err
}

type upResult struct {
	Daemon   string `json:"daemon"` // running | started
	Tray     string `json:"tray"`   // running | started | unavailable
	TrayNote string `json:"tray_note,omitempty"`
}

// upCmd is `502drive up`: the non-interactive half of launch, used by autostart.
func (a *App) upCmd() *cobra.Command {
	return &cobra.Command{Use: "up", Short: "Start the daemon and the tray in the background (no UI)", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := a.ensureConfig(); err != nil {
				return err
			}
			res, err := a.bringUp(cmd.Context())
			if err != nil {
				return err
			}
			if a.g.json {
				b, _ := json.Marshal(map[string]any{"data": res, "request_id": ""})
				fmt.Fprintln(a.out(), string(b))
				return nil
			}
			if !a.g.quiet {
				fmt.Fprintf(a.out(), "daemon  %s\ntray    %s", res.Daemon, res.Tray)
				if res.TrayNote != "" {
					fmt.Fprintf(a.out(), " (%s)", res.TrayNote)
				}
				fmt.Fprintln(a.out())
			}
			return nil
		}}
}

func (a *App) bringUp(ctx context.Context) (upResult, error) {
	var res upResult
	daemon, err := a.ensureDaemon(ctx)
	if err != nil {
		return res, err
	}
	res.Daemon = daemon
	res.Tray, res.TrayNote = a.ensureTray()
	return res, nil
}

// ensureConfig creates the private config and API token on the very first run.
// In a terminal it also asks for the Google client and the Telegram bot (all
// optional); keys found in the environment or a .env file are taken silently.
func (a *App) ensureConfig() error {
	if _, err := os.Lstat(a.configPath()); err == nil {
		return nil
	}
	return a.runSetup(&setupInput{}, a.interactive())
}

func (a *App) stateDir() string { return filepath.Dir(a.configPath()) }

// logFile returns the append-only log of a background process, rotating one
// generation when it grows past logRotateBytes.
func (a *App) logFile(name string) (string, error) {
	dir := filepath.Join(a.stateDir(), "logs")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	p := filepath.Join(dir, name)
	if st, err := os.Stat(p); err == nil && st.Size() > logRotateBytes {
		_ = os.Rename(p, p+".1")
	}
	return p, nil
}

// daemonUp pings the unauthenticated liveness endpoint.
func (a *App) daemonUp(ctx context.Context) bool {
	c, err := a.api()
	if err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, 600*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", c.Endpoint+"/health/live", nil)
	if err != nil {
		return false
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	res.Body.Close()
	return res.StatusCode == http.StatusOK
}

func (a *App) childArgv(exe string, args ...string) []string {
	argv := append([]string{exe}, args...)
	if a.g.configPath != "" {
		argv = append(argv, "--config", a.g.configPath)
	}
	return argv
}

// ensureDaemon starts `502drive serve` detached when nothing answers.
func (a *App) ensureDaemon(ctx context.Context) (string, error) {
	if a.daemonUp(ctx) {
		return "running", nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", localErr(ExitInternal, "daemon_start_failed", "Cannot locate the 502drive binary.")
	}
	logPath, err := a.logFile("daemon.log")
	if err != nil {
		return "", localErr(ExitInternal, "daemon_start_failed", err.Error())
	}
	if err = spawnDetached(a.childArgv(exe, "serve"), logPath); err != nil {
		return "", localErr(ExitInternal, "daemon_start_failed", "Cannot start the daemon: "+err.Error())
	}
	deadline := time.Now().Add(daemonStartWait)
	for time.Now().Before(deadline) {
		if a.daemonUp(ctx) {
			return "started", nil
		}
		select {
		case <-ctx.Done():
			return "", localErr(ExitInterrupt, "interrupted", "interrupted")
		case <-time.After(150 * time.Millisecond):
		}
	}
	return "", localErr(ExitUnavailable, "daemon_start_failed", "The daemon did not become ready in time. See "+logPath)
}

func (a *App) startDaemonAction(ctx context.Context) (string, error) {
	res, err := a.ensureDaemon(ctx)
	if err != nil {
		return "", err
	}
	return "daemon " + res, nil
}

// stopDaemon asks the daemon to shut down gracefully; running jobs resume on
// the next start.
func (a *App) stopDaemon(ctx context.Context) (string, error) {
	if !a.daemonUp(ctx) {
		return "daemon not running", nil
	}
	if _, err := a.call(ctx, "POST", "/v1/shutdown", nil, nil, ""); err != nil {
		return "", err
	}
	return "daemon stopping", nil
}

// trayPossible says whether a tray can be shown here, and why not.
func (a *App) trayPossible() (bool, string) {
	switch {
	case a.env("DRIVE502_NO_TRAY") == "1":
		return false, "disabled by DRIVE502_NO_TRAY"
	case runtime.GOOS == "darwin":
		return false, "the tray needs Linux or Windows; use `502drive menu`"
	case runtime.GOOS == "linux" && a.env("DISPLAY") == "" && a.env("WAYLAND_DISPLAY") == "":
		return false, "no desktop session"
	}
	return true, ""
}

func (a *App) trayLock() string { return filepath.Join(a.stateDir(), "tray.lock") }

// ensureTray starts the tray detached unless one runs or none can be shown.
func (a *App) ensureTray() (status, note string) {
	if ok, why := a.trayPossible(); !ok {
		return "unavailable", why
	}
	if tray.InstanceRunning(a.trayLock()) {
		return "running", ""
	}
	exe, err := os.Executable()
	if err != nil {
		return "unavailable", "cannot locate the 502drive binary"
	}
	logPath, err := a.logFile("tray.log")
	if err != nil {
		return "unavailable", err.Error()
	}
	if err = spawnDetached(a.childArgv(exe, "tray"), logPath); err != nil {
		return "unavailable", err.Error()
	}
	return "started", ""
}
