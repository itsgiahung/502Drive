package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/itsgiahung/502Drive/internal/present"
	"github.com/spf13/cobra"
)

// waitCmd is the short form of `jobs wait`.
func (a *App) waitCmd() *cobra.Command {
	var waitTimeout time.Duration
	c := &cobra.Command{Use: "wait JOB_ID", Short: "Follow a job until it finishes (same as `jobs wait`)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.waitJob(cmd.Context(), args[0], waitTimeout)
		}}
	c.Flags().DurationVar(&waitTimeout, "wait-timeout", 0, "give up waiting after this long (job keeps running)")
	c.ValidArgsFunction = a.completeJobIDs
	return c
}

// watchCmd prints one compact line per tick (one JSON envelope per second with
// --json) and stops when the job reaches a terminal state.
func (a *App) watchCmd() *cobra.Command {
	var interval time.Duration
	c := &cobra.Command{Use: "watch [JOB_ID]", Short: "Print a job's progress once a second", Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref := aliasLast
			if len(args) == 1 {
				ref = args[0]
			}
			return a.watchJob(cmd.Context(), ref, interval)
		}}
	c.Flags().DurationVar(&interval, "interval", time.Second, "print interval")
	c.ValidArgsFunction = a.completeJobIDs
	return c
}

func (a *App) watchJob(ctx context.Context, ref string, interval time.Duration) error {
	id, err := a.resolveJobID(ctx, ref)
	if err != nil {
		return err
	}
	if interval < 100*time.Millisecond {
		return usage("--interval must be at least 100ms")
	}
	tick := time.NewTicker(interval)
	defer tick.Stop()
	last := ""
	var samples []present.Sample
	for {
		r, err := a.call(ctx, "GET", "/v1/jobs/"+url.PathEscape(id), nil, nil, "")
		if err != nil {
			return err
		}
		var j jobView
		if err = r.Decode(&j); err != nil {
			return err
		}
		samples = present.AppendSample(samples, present.Sample{At: time.Now(), Completed: j.Counts.Completed})
		if a.g.json {
			b, _ := json.Marshal(map[string]any{"data": j, "request_id": r.RequestID})
			fmt.Fprintln(a.out(), string(b))
		} else {
			line := strings.ToUpper(j.State) + " " + j.progress(samples) + " " + Sanitize(j.SourceName)
			if line != last {
				fmt.Fprintln(a.out(), line)
				last = line
			}
		}
		if terminalState(j.State) {
			return jobOutcome(j)
		}
		select {
		case <-ctx.Done():
			return localErr(ExitInterrupt, "interrupted", "interrupted")
		case <-tick.C:
		}
	}
}
