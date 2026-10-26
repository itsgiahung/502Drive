package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/itsgiahung/502Drive/internal/tray"
)

// trayCmd runs the desktop tray. It is a thin HTTP client: every action goes
// through the same API as the subcommands.
func (a *App) trayCmd() *cobra.Command {
	var iconDir string
	c := &cobra.Command{Use: "tray", Short: "Desktop tray launcher (Linux and Windows)", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if iconDir != "" {
				return a.writeIcons(cmd, iconDir)
			}
			return a.runTray(cmd.Context())
		}}
	c.Flags().StringVar(&iconDir, "write-icons", "", "write PNG/ICO icon assets to DIR and exit (build helper)")
	return c
}

func (a *App) writeIcons(cmd *cobra.Command, dir string) error {
	if err := tray.WriteIcons(dir); err != nil {
		return localErr(ExitInternal, "icons_failed", "Cannot write the icon assets: "+err.Error())
	}
	if a.g.json {
		fmt.Fprintf(a.out(), "{\"data\":{\"dir\":%q},\"request_id\":\"\"}\n", dir)
		return nil
	}
	fmt.Fprintf(a.out(), "Wrote %s and %s in %s\n", tray.IconPNG, tray.IconICO, dir)
	return nil
}

// probeTrayHost is the Linux StatusNotifierWatcher probe; tests replace it.
var probeTrayHost = tray.StatusNotifierWatcherPresent

func (a *App) runTray(ctx context.Context) error {
	if _, err := a.api(); err != nil {
		return err
	}
	release, err := tray.LockInstance(a.trayLock())
	if errors.Is(err, tray.ErrAlreadyRunning) {
		fmt.Fprintln(a.errw(), "The 502drive tray is already running.")
		return nil
	}
	if err != nil {
		return localErr(ExitInternal, "tray_unavailable", err.Error())
	}
	defer release()
	exe, err := os.Executable()
	if err != nil {
		return localErr(ExitInternal, "tray_unavailable", "Cannot locate the 502drive binary.")
	}
	host := tray.DefaultHost()
	// Use the CLI's env accessor so tests (and DRIVE502_* overrides) see the
	// same environment as the rest of the CLI.
	host.Getenv = a.env
	opts := tray.Options{
		Version:     Version,
		Motion:      !a.g.noMotion,
		NoNotify:    a.env("DRIVE502_NO_NOTIFY") == "1",
		Exe:         exe,
		Host:        host,
		Status:      a.trayStatus,
		Jobs:        a.trayJobs,
		Copy:        a.trayCopy,
		Language:    a.trayLanguage,
		StartDaemon: a.startDaemonAction,
		StopDaemon:  a.stopDaemon,
		Log:         slog.New(slog.NewTextHandler(a.errw(), &slog.HandlerOptions{Level: slog.LevelWarn})),
	}
	if cfg, cerr := a.loadConfig(); cerr == nil {
		opts.QuitStopsDaemon = cfg.TrayQuitStopsDaemon
		if cfg.CheckUpdates {
			opts.UpdateCheck = a.trayUpdateNote
		}
	}
	if err := tray.Run(ctx, opts); err != nil {
		return localErr(ExitInternal, "tray_unavailable", err.Error())
	}
	return nil
}

func (a *App) trayStatus(ctx context.Context) (tray.Status, error) {
	var out tray.Status
	r, err := a.call(ctx, "GET", "/v1/status", nil, nil, "")
	if err != nil {
		return out, err
	}
	var st struct {
		Version string `json:"version"`
		Ready   bool   `json:"ready"`
		Running int    `json:"running_jobs"`
		Queued  int    `json:"queued_jobs"`
		Blocked int    `json:"blocked_jobs"`
		Account struct {
			Status string  `json:"status"`
			Label  *string `json:"label"`
		} `json:"account"`
	}
	if err = r.Decode(&st); err != nil {
		return out, err
	}
	out = tray.Status{Version: st.Version, Ready: st.Ready, Running: st.Running, Queued: st.Queued,
		Blocked: st.Blocked, AccountStatus: st.Account.Status}
	if st.Account.Label != nil {
		out.AccountLabel = *st.Account.Label
	}
	return out, nil
}

func (a *App) trayJobs(ctx context.Context) ([]tray.Job, error) {
	r, err := a.call(ctx, "GET", "/v1/jobs", map[string][]string{"limit": {"5"}}, nil, "")
	if err != nil {
		return nil, err
	}
	var page struct {
		Items []jobView `json:"items"`
	}
	if err = r.Decode(&page); err != nil {
		return nil, err
	}
	out := make([]tray.Job, 0, len(page.Items))
	for _, j := range page.Items {
		job := tray.Job{ID: j.ID, SourceName: j.SourceName, State: j.State, Completed: j.Counts.Completed, Total: j.Counts.Total}
		if j.ResultURL != nil {
			job.ResultURL = *j.ResultURL
		}
		out = append(out, job)
	}
	return out, nil
}

func (a *App) trayCopy(ctx context.Context, link string) (string, error) {
	job, _, err := a.submitCopy(ctx, link, "", "", "")
	return job.ID, err
}

// trayLanguage reads the preference fresh, so a change in settings shows up in
// the tray without a restart.
func (a *App) trayLanguage(ctx context.Context) (string, error) {
	r, err := a.call(ctx, "GET", "/v1/preferences", nil, nil, "")
	if err != nil {
		return "", err
	}
	var prefs struct {
		Language string `json:"language"`
	}
	if err = r.Decode(&prefs); err != nil {
		return "", err
	}
	if prefs.Language == "" {
		return "vi", nil
	}
	return prefs.Language, nil
}
