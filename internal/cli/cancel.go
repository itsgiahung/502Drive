package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// bulkActive are the states `jobs cancel --all` may act on. Terminal jobs are
// left alone so a finished job is never rewritten by a stale list.
var bulkActive = []string{"queued", "running", "pausing", "paused", "reconciling", "cancelling", "blocked"}

func activeState(s string) bool {
	for _, a := range bulkActive {
		if a == s {
			return true
		}
	}
	return false
}

// cancelCmd cancels one job, or every matching job with --all. The bulk form
// is the only place the CLI loops over user-visible jobs, so it asks first.
func (a *App) cancelCmd() *cobra.Command {
	var all, yes bool
	var stateFilter string
	c := &cobra.Command{Use: "cancel [JOB_ID]", Short: "Cancel one job, or every active job with --all", Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !all {
				if len(args) != 1 {
					return usage("give JOB_ID, or --all to cancel in bulk")
				}
				return a.jobActionByID(cmd.Context(), args[0], "cancel")
			}
			if len(args) != 0 {
				return usage("--all cancels every matching job; do not also give JOB_ID")
			}
			return a.cancelMany(cmd.Context(), stateFilter, yes)
		}}
	f := c.Flags()
	f.BoolVar(&all, "all", false, "cancel every job in an active state")
	f.StringVar(&stateFilter, "state", "", "only this state ("+strings.Join(bulkActive, ", ")+")")
	f.BoolVar(&yes, "yes", false, "skip the y/N question (required without a terminal)")
	return c
}

// jobActionByID performs one revision-checked action on one job and prints it.
func (a *App) jobActionByID(ctx context.Context, id, action string) error {
	nj, r, err := a.applyAction(ctx, id, action)
	if err != nil {
		return err
	}
	a.emit(r, nj.ID, func() {
		fmt.Fprintf(a.out(), "%s requested.\n", action)
		a.printJob(nj)
	})
	return nil
}

// applyAction reads the current revision and posts the action without
// printing, so a bulk loop stays quiet until its summary.
func (a *App) applyAction(ctx context.Context, id, action string) (jobView, *Response, error) {
	var nj jobView
	j, err := a.fetchJob(ctx, id)
	if err != nil {
		return nj, nil, err
	}
	r, err := a.call(ctx, "POST", "/v1/jobs/"+url.PathEscape(id)+"/"+action, nil, map[string]any{"expected_revision": j.Revision}, NewKey())
	if err != nil {
		return nj, nil, err
	}
	return nj, r, r.Decode(&nj)
}

// bulkResult is the local envelope of a bulk command (there is no single
// daemon response to pass through).
type bulkResult struct {
	Canceled []string    `json:"canceled"`
	Skipped  []string    `json:"skipped"`
	Failed   []bulkError `json:"failed"`
}

type bulkError struct {
	ID      string `json:"id"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (a *App) cancelMany(ctx context.Context, stateFilter string, yes bool) error {
	if stateFilter != "" && !activeState(stateFilter) {
		return usage("--state must be one of: %s", strings.Join(bulkActive, ", "))
	}
	jobs, err := a.activeJobs(ctx, stateFilter)
	if err != nil {
		return err
	}
	if len(jobs) == 0 {
		if a.g.json {
			return a.printBulk(bulkResult{})
		}
		fmt.Fprintln(a.out(), "nothing to cancel")
		return nil
	}
	if !yes {
		if !a.interactive() {
			return usage("pass --yes to cancel %d job(s) without a terminal", len(jobs))
		}
		if !a.confirm(fmt.Sprintf("Cancel %d job(s)? [y/N]", len(jobs))) {
			fmt.Fprintln(a.out(), "aborted; nothing was canceled")
			return nil
		}
	}
	res := bulkResult{Canceled: []string{}, Skipped: []string{}, Failed: []bulkError{}}
	for _, j := range jobs {
		_, _, err := a.applyAction(ctx, j.ID, "cancel")
		if err == nil {
			res.Canceled = append(res.Canceled, j.ID)
			continue
		}
		ae := classifyCLI(err)
		// A job that moved on between the list and the call is skipped, not a
		// failure: another device or the engine finished it first.
		if ae.Status == 409 || ae.Status == 404 {
			res.Skipped = append(res.Skipped, j.ID)
			continue
		}
		res.Failed = append(res.Failed, bulkError{ID: j.ID, Code: ae.Code, Message: ae.Message})
	}
	if a.g.json {
		if err := a.printBulk(res); err != nil {
			return err
		}
	} else {
		fmt.Fprintf(a.out(), "canceled %d, skipped %d, failed %d\n", len(res.Canceled), len(res.Skipped), len(res.Failed))
		for _, f := range res.Failed {
			fmt.Fprintf(a.errw(), "  %s: %s (%s)\n", f.ID, f.Message, f.Code)
		}
	}
	if len(res.Failed) > 0 {
		return localErr(ExitInternal, "bulk_failed", fmt.Sprintf("%d job(s) could not be canceled", len(res.Failed)))
	}
	return nil
}

// activeJobs lists the caller's jobs in an active state, newest first. The
// daemon scopes the list to the token's principal.
func (a *App) activeJobs(ctx context.Context, stateFilter string) ([]jobView, error) {
	var out []jobView
	cursor := int64(0)
	for pages := 0; pages < 20; pages++ {
		q := url.Values{"limit": {"100"}, "all": {"true"}}
		if stateFilter != "" {
			q.Set("state", stateFilter)
		}
		if cursor > 0 {
			q.Set("cursor", strconv.FormatInt(cursor, 10))
		}
		r, err := a.call(ctx, "GET", "/v1/jobs", q, nil, "")
		if err != nil {
			return nil, err
		}
		var page struct {
			Items []jobView `json:"items"`
			Next  *string   `json:"next_cursor"`
		}
		if err = r.Decode(&page); err != nil {
			return nil, err
		}
		for _, j := range page.Items {
			if !activeState(j.State) || (stateFilter != "" && j.State != stateFilter) {
				continue
			}
			out = append(out, j)
		}
		if page.Next == nil || *page.Next == "" {
			return out, nil
		}
		n, err := strconv.ParseInt(*page.Next, 10, 64)
		if err != nil || n <= cursor {
			return out, nil
		}
		cursor = n
	}
	return out, nil
}

func (a *App) printBulk(res bulkResult) error {
	b, err := json.Marshal(map[string]any{"data": res, "request_id": ""})
	if err != nil {
		return localErr(ExitInternal, "internal_error", err.Error())
	}
	fmt.Fprintln(a.out(), string(b))
	return nil
}

// confirm asks a yes/no question on the terminal; anything but y/yes is no.
func (a *App) confirm(question string) bool {
	fmt.Fprint(a.errw(), question+" ")
	if a.io.In == nil {
		return false
	}
	line, err := bufio.NewReader(a.io.In).ReadString('\n')
	if err != nil && line == "" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	}
	return false
}

// classifyCLI exposes the daemon error fields for bulk bookkeeping.
func classifyCLI(err error) *APIError {
	if ae, ok := err.(*APIError); ok {
		return ae
	}
	return &APIError{Code: "internal_error", Message: err.Error()}
}
