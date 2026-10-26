package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// stopCmd is the CLI's "Stop 502Drive…": the same graceful POST /v1/shutdown
// the tray uses, never a kill. With jobs still active it asks first, exactly
// like `jobs cancel --all` (—yes outside a terminal).
func (a *App) stopCmd() *cobra.Command {
	var yes bool
	c := &cobra.Command{Use: "stop", Short: "Stop the 502Drive daemon gracefully (running jobs resume on the next start)", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.stopDaemonConfirmed(cmd.Context(), yes)
		}}
	c.Flags().BoolVar(&yes, "yes", false, "skip the y/N question (required without a terminal)")
	return c
}

// stopDaemonConfirmed counts the daemon's active jobs, asks when there are
// any, and only then posts the shutdown and waits for it to take effect.
func (a *App) stopDaemonConfirmed(ctx context.Context, yes bool) error {
	if !a.daemonUp(ctx) {
		return a.emitStop("daemon not running", nil)
	}
	active, err := a.activeJobCount(ctx)
	if err != nil {
		return err
	}
	if active > 0 && !yes {
		if !a.interactive() {
			return usage("pass --yes to stop %d job(s) without a terminal", active)
		}
		if !a.confirm(fmt.Sprintf("Stop 502Drive? %d job(s) would pause and resume on the next start. [y/N]", active)) {
			fmt.Fprintln(a.out(), "aborted; the daemon keeps running")
			return nil
		}
	}
	out, err := a.stopDaemon(ctx)
	return a.emitStop(out, err)
}

// emitStop prints the result in the human or JSON shape. out is one of
// "daemon stopped" / "daemon not running" on success.
func (a *App) emitStop(out string, err error) error {
	if err != nil {
		return err
	}
	if a.g.json {
		b, _ := json.Marshal(map[string]any{"data": map[string]any{"daemon": strings.TrimPrefix(out, "daemon ")}, "request_id": ""})
		fmt.Fprintln(a.out(), string(b))
		return nil
	}
	fmt.Fprintf(a.out(), "502Drive %s.\n", out)
	return nil
}

// activeJobCount is running+queued from /v1/status; a status that cannot be
// read while the daemon is alive is an error, not zero.
func (a *App) activeJobCount(ctx context.Context) (int, error) {
	r, err := a.call(ctx, "GET", "/v1/status", nil, nil, "")
	if err != nil {
		return 0, err
	}
	var st struct {
		Running int `json:"running_jobs"`
		Queued  int `json:"queued_jobs"`
	}
	if err = r.Decode(&st); err != nil {
		return 0, err
	}
	return st.Running + st.Queued, nil
}
