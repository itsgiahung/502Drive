package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// healthcheckCmd is what the container HEALTHCHECK runs: distroless has no
// shell, curl or wget, so the binary itself must answer. It exits 0 only when
// the daemon reports ready, and 1 for every other outcome (Docker treats any
// non-zero as unhealthy); the specific reason stays on stderr.
func (a *App) healthcheckCmd() *cobra.Command {
	return &cobra.Command{Use: "healthcheck", Short: "Exit 0 when the daemon is ready (0/1, for Docker)", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := a.api()
			if err != nil {
				return localErr(ExitInternal, "not_ready", "not ready: the daemon config is unreadable ("+err.Error()+")")
			}
			r, err := c.Do(cmd.Context(), "GET", "/health/ready", nil, nil, "")
			if err != nil {
				var ae *APIError
				if errors.As(err, &ae) && ae.Status > 0 {
					return localErr(ExitInternal, "not_ready", fmt.Sprintf("not ready: the daemon answered HTTP %d (%s)", ae.Status, ae.Message))
				}
				return localErr(ExitInternal, "not_ready", "not ready: "+err.Error())
			}
			if a.g.json {
				fmt.Fprintln(a.out(), strings.TrimSpace(string(r.Raw)))
				return nil
			}
			if !a.g.quiet {
				fmt.Fprintln(a.out(), "ready")
			}
			return nil
		}}
}
