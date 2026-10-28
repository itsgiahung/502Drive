package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/itsgiahung/502Drive/internal/tui"
)

// historyClearResult mirrors POST /v1/history/clear.
type historyClearResult struct {
	DryRun  bool `json:"dry_run"`
	Hidden  int  `json:"hidden"`
	Skipped int  `json:"skipped"`
	Deleted struct {
		JobsPruned   int `json:"jobs_pruned"`
		EventsPruned int `json:"events_pruned"`
	} `json:"deleted"`
}

func (a *App) historyClear(ctx context.Context, body map[string]any) (historyClearResult, *Response, error) {
	var out historyClearResult
	r, err := a.call(ctx, "POST", "/v1/history/clear", nil, body, "")
	if err != nil {
		return out, nil, err
	}
	return out, r, r.Decode(&out)
}

func (a *App) historyClearCmd() *cobra.Command {
	var olderThan string
	var allFinished, dryRun, yes bool
	c := &cobra.Command{
		Use:   "clear (--older-than 30d | --all-finished) [--dry-run] [--yes]",
		Short: "Clear finished jobs from the history (collections and Drive are untouched)",
		Long: "Hides finished jobs from the job lists and deletes what nothing refers to.\n" +
			"Collections, copied-item mappings and Drive files are never touched. It shows\n" +
			"what it would do first; --yes applies it without asking (needed in scripts).",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if allFinished == (olderThan != "") {
				return usage("give exactly one of --older-than or --all-finished")
			}
			days := 0
			if !allFinished {
				dur, err := parseRetentionDuration(olderThan)
				if err != nil {
					return usage("%s", err.Error())
				}
				if dur%(24*time.Hour) != 0 {
					return usage("--older-than must be whole days (for example 30d); %s would be rounded", olderThan)
				}
				days = int(dur.Hours() / 24)
				if days < 1 {
					return usage("--older-than must be at least 1d; use --all-finished for everything")
				}
			}
			ctx := cmd.Context()
			prev, raw, err := a.historyClear(ctx, map[string]any{"older_than_days": days, "dry_run": true})
			if err != nil {
				return err
			}
			if dryRun {
				a.emit(raw, "", func() { a.printHistoryClear(prev) })
				return nil
			}
			if prev.Hidden == 0 && prev.Deleted.JobsPruned == 0 {
				a.emit(raw, "", func() { fmt.Fprintln(a.out(), "Nothing to clear.") })
				return nil
			}
			if !yes {
				if !a.io.StdinTTY || a.g.noInput || a.g.json {
					return usage("add --yes to clear %d finished jobs without a prompt (or --dry-run to preview)", prev.Hidden)
				}
				a.printHistoryClear(prev)
				if !a.confirm(fmt.Sprintf("Clear %d finished jobs from the history? [y/N]", prev.Hidden)) {
					return localErr(ExitUsage, "cancelled", "Nothing was cleared.")
				}
			}
			res, raw, err := a.historyClear(ctx, map[string]any{"older_than_days": days, "dry_run": false})
			if err != nil {
				return err
			}
			a.emit(raw, "", func() { a.printHistoryClear(res) })
			return nil
		},
	}
	c.Flags().StringVar(&olderThan, "older-than", "", "clear finished jobs older than this (e.g. 30d)")
	c.Flags().BoolVar(&allFinished, "all-finished", false, "clear every finished job")
	c.Flags().BoolVar(&dryRun, "dry-run", false, "only show what would be cleared")
	c.Flags().BoolVar(&yes, "yes", false, "clear without asking")
	return c
}

func (a *App) printHistoryClear(r historyClearResult) {
	verb := "Cleared"
	if r.DryRun {
		verb = "Would clear"
	}
	fmt.Fprintf(a.out(), "%s %d finished jobs from the history (%d of them deleted with %d events).\n",
		verb, r.Hidden, r.Deleted.JobsPruned, r.Deleted.EventsPruned)
	fmt.Fprintln(a.out(), "Your copies, folders being tracked and Drive files are not touched.")
}

// tuiClearHistory is the TUI's history action: olderThanDays >= 0 clears by
// age (0 = every finished job), a negative value clears the listed jobIDs.
func (a *App) tuiClearHistory(ctx context.Context, olderThanDays int, jobIDs []string, dryRun bool) (tui.HistoryClearResult, error) {
	body := map[string]any{"dry_run": dryRun}
	if olderThanDays >= 0 {
		body["older_than_days"] = olderThanDays
	} else {
		body["job_ids"] = jobIDs
	}
	res, _, err := a.historyClear(ctx, body)
	return tui.HistoryClearResult{DryRun: res.DryRun, Hidden: res.Hidden, Skipped: res.Skipped,
		DeletedJobs: res.Deleted.JobsPruned, DeletedEvents: res.Deleted.EventsPruned}, err
}
