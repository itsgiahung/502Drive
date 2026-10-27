package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// readStdinRefs reads one Drive reference per line. Blank lines and repeats
// are dropped; the order of first appearance is kept.
func readStdinRefs(r io.Reader) ([]string, error) {
	sc := bufio.NewScanner(io.LimitReader(r, 1<<20))
	var out []string
	seen := map[string]bool{}
	for sc.Scan() {
		t := strings.TrimSpace(sc.Text())
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	if err := sc.Err(); err != nil {
		return nil, usage("cannot read stdin: %s", err.Error())
	}
	return out, nil
}

// batchKey is the idempotency root of a batch. An explicit key wins; otherwise
// the key is derived from the request itself, so repeating the same command
// replays the same plans and jobs instead of copying everything twice.
func batchKey(explicit string, sources []string, to, name string, o planOpts, afterCopySync string) string {
	if explicit != "" {
		return explicit
	}
	parts := append([]string{"batch", to, name, o.Shortcuts, o.OnConflict, fmt.Sprint(o.Filter), afterCopySync}, sources...)
	return "batch-" + requestID(parts...)
}

// batchItemResult is one line of a batch: the job it produced or why it failed.
type batchItemResult struct {
	Source string `json:"source"`
	JobID  string `json:"job_id,omitempty"`
	Error  *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
	err error
}

// copyBatch admits one job per source under one batch id. Each source has its
// own idempotency keys (batch key + ":" + index), so one refused link does not
// stop the others, and a repeated command admits only what is still missing.
func (a *App) copyBatch(ctx context.Context, sources []string, to, name, explicitKey string, o planOpts, mode, afterCopySync string, autoName, dry, wait bool, waitTimeout time.Duration) error {
	bkey := batchKey(explicitKey, sources, to, name, o, afterCopySync)
	bid := "batch-" + requestID(bkey)
	results := make([]batchItemResult, 0, len(sources))
	var firstErr error
	for i, src := range sources {
		base := fmt.Sprintf("%s:%d", bkey, i)
		res := batchItemResult{Source: src}
		itemName := name
		if dry {
			plan, _, err := a.makePlan(ctx, src, to, name, NewKey(), o)
			res.err = err
			if err == nil {
				a.printPlan(plan)
			}
			results = append(results, res)
			if err != nil && firstErr == nil {
				firstErr = err
			}
			continue
		}
		if autoName {
			plan, _, err := a.makePlan(ctx, src, to, "", base+"-name", o)
			if err != nil {
				res.err = err
				results = append(results, res)
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			itemName = autoCopyName(plan.SourceName)
		}
		job, err := a.submitBatchItem(ctx, src, to, itemName, base, bid, o, mode, afterCopySync)
		if err != nil {
			res.err = err
			if firstErr == nil {
				firstErr = err
			}
		} else {
			res.JobID = job.ID
		}
		results = append(results, res)
	}
	if dry {
		return firstErr
	}
	for i := range results {
		if results[i].err != nil {
			var ae *APIError
			code, msg := "internal_error", results[i].err.Error()
			if errors.As(results[i].err, &ae) {
				code, msg = ae.Code, ae.Message
			}
			results[i].Error = &struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			}{code, msg}
		}
	}
	accepted, failed := 0, 0
	for _, r := range results {
		if r.JobID != "" {
			accepted++
		} else {
			failed++
		}
	}
	a.emitBatch(bid, results, accepted, failed)
	if wait {
		for _, r := range results {
			if r.JobID == "" {
				continue
			}
			if err := a.waitJob(ctx, r.JobID, waitTimeout); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func (a *App) submitBatchItem(ctx context.Context, source, to, name, base, bid string, o planOpts, mode, afterCopySync string) (jobView, error) {
	var job jobView
	plan, _, err := a.makePlan(ctx, source, to, name, base+"-plan", o)
	if err != nil {
		return job, err
	}
	body := map[string]any{"plan_id": plan.ID, "expected_revision": plan.Revision, "batch_id": bid}
	if mode != "" {
		body["mode"] = mode
	}
	if afterCopySync != "" {
		body["after_copy_sync_mode"] = afterCopySync
	}
	r, err := a.call(ctx, "POST", "/v1/jobs", nil, body, base)
	if err != nil {
		return job, err
	}
	return job, r.Decode(&job)
}

func (a *App) emitBatch(bid string, results []batchItemResult, accepted, failed int) {
	if a.g.json {
		b, _ := json.Marshal(map[string]any{"data": map[string]any{"batch_id": bid, "accepted": accepted, "failed": failed, "items": results}})
		fmt.Fprintln(a.out(), string(b))
		return
	}
	if a.g.quiet {
		for _, r := range results {
			if r.JobID != "" {
				fmt.Fprintln(a.out(), r.JobID)
			}
		}
		return
	}
	fmt.Fprintf(a.out(), "Batch %s: %d accepted, %d failed\n", bid, accepted, failed)
	for _, r := range results {
		if r.JobID != "" {
			fmt.Fprintf(a.out(), "%s  %s  %s\n", a.style.stateColor("queued"), r.JobID, Truncate(Sanitize(r.Source), 60, a.style.ASCII))
			continue
		}
		msg := "failed"
		if r.Error != nil {
			msg = r.Error.Code + ": " + r.Error.Message
		}
		fmt.Fprintf(a.errw(), "%s  %s  %s\n", a.style.stateColor("failed"), Truncate(Sanitize(r.Source), 60, a.style.ASCII), msg)
	}
}

// queueCmd is the global pause/resume of the job queue (S6).
func (a *App) queueCmd(paused bool) *cobra.Command {
	verb, short := "resume", "Resume the job queue: queued work may start again"
	if paused {
		verb, short = "pause", "Pause the job queue: running jobs settle safely, queued jobs wait"
	}
	return &cobra.Command{Use: verb, Short: short, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		r, err := a.call(cmd.Context(), "POST", "/v1/queue/"+verb, nil, nil, "")
		if err != nil {
			return err
		}
		var out struct {
			Paused       bool `json:"paused"`
			Changed      bool `json:"changed"`
			AffectedJobs int  `json:"affected_jobs"`
		}
		if err = r.Decode(&out); err != nil {
			return err
		}
		a.emit(r, verb, func() {
			switch {
			case !out.Changed && paused:
				fmt.Fprintln(a.out(), "The queue is already paused.")
			case !out.Changed:
				fmt.Fprintln(a.out(), "The queue is already running.")
			case paused:
				fmt.Fprintf(a.out(), "Queue paused (%d job(s) pausing). Queued jobs wait.\n", out.AffectedJobs)
			default:
				fmt.Fprintf(a.out(), "Queue resumed (%d job(s) back in line).\n", out.AffectedJobs)
			}
		})
		return nil
	}}
}

// jobsMoveCmd reorders or re-prioritises a queued job.
func (a *App) jobsMoveCmd() *cobra.Command {
	var before, after string
	var priority int
	c := &cobra.Command{Use: "move JOB_ID [--before ID | --after ID] [--priority N]", Short: "Move a queued job in the queue", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if before != "" && after != "" {
			return usage("give --before or --after, not both")
		}
		if before == "" && after == "" && !cmd.Flags().Changed("priority") {
			return usage("give --before, --after or --priority")
		}
		ctx := cmd.Context()
		cur, err := a.fetchJob(ctx, args[0])
		if err != nil {
			return err
		}
		id := cur.ID
		if before != "" {
			if before, err = a.resolveJobID(ctx, before); err != nil {
				return err
			}
		}
		if after != "" {
			if after, err = a.resolveJobID(ctx, after); err != nil {
				return err
			}
		}
		body := map[string]any{"expected_revision": cur.Revision}
		if before != "" {
			body["before"] = before
		}
		if after != "" {
			body["after"] = after
		}
		if cmd.Flags().Changed("priority") {
			body["priority"] = priority
		}
		r, err := a.call(ctx, "POST", "/v1/jobs/"+url.PathEscape(id)+"/move", nil, body, "")
		if err != nil {
			return err
		}
		var j jobView
		if err = r.Decode(&j); err != nil {
			return err
		}
		a.emit(r, j.ID, func() {
			fmt.Fprintf(a.out(), "%s  %s\n", a.style.stateColor(j.State), j.ID)
		})
		return nil
	}}
	f := c.Flags()
	f.StringVar(&before, "before", "", "place this job before the given queued job")
	f.StringVar(&after, "after", "", "place this job after the given queued job")
	f.IntVar(&priority, "priority", 0, "queue priority from -5 to 5 (higher runs first)")
	c.ValidArgsFunction = a.completeJobIDs
	return c
}
