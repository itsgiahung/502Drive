package cli

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

// Job aliases accepted wherever a job ID is expected.
const (
	aliasLast    = "@last"
	aliasRunning = "@running"
)

var fullJobID = regexp.MustCompile(`^job_[0-9a-f]{32}$`)

// listJobs returns up to limit visible jobs, newest first.
func (a *App) listJobs(ctx context.Context, limit int) ([]jobView, error) {
	q := url.Values{"limit": {fmt.Sprint(limit)}}
	r, err := a.call(ctx, "GET", "/v1/jobs", q, nil, "")
	if err != nil {
		return nil, err
	}
	var page struct {
		Items []jobView `json:"items"`
	}
	if err = r.Decode(&page); err != nil {
		return nil, err
	}
	return page.Items, nil
}

func activeJob(s string) bool {
	switch s {
	case "queued", "running", "pausing", "paused", "cancelling", "reconciling", "blocked":
		return true
	}
	return false
}

// resolveJobID accepts a full ID, a unique prefix, @last (newest) and
// @running (newest job that has not finished).
func (a *App) resolveJobID(ctx context.Context, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", usage("a job ID is required")
	}
	switch ref {
	case aliasLast, aliasRunning:
		jobs, err := a.listJobs(ctx, 100)
		if err != nil {
			return "", err
		}
		for _, j := range jobs {
			if ref == aliasLast || activeJob(j.State) {
				return j.ID, nil
			}
		}
		if ref == aliasRunning {
			return "", localErr(ExitNotFound, "no_running_job", "No queued or running job.")
		}
		return "", localErr(ExitNotFound, "no_job", "No job yet.")
	}
	if fullJobID.MatchString(ref) {
		return ref, nil
	}
	if !strings.HasPrefix(ref, "job_") {
		// Not a possible prefix of a real job ID (synthetic or foreign IDs):
		// let the daemon answer instead of listing jobs first.
		return ref, nil
	}
	jobs, err := a.listJobs(ctx, 100)
	if err != nil {
		return "", err
	}
	var matches []string
	for _, j := range jobs {
		if j.ID == ref {
			return j.ID, nil
		}
		if strings.HasPrefix(j.ID, ref) {
			matches = append(matches, j.ID)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return ref, nil // let the daemon answer not_found
	}
	return "", usage("job ID %q is ambiguous (%s); give more characters", ref, strings.Join(matches, ", "))
}

// completeJobIDs is the shell completion for arguments that take a job ID.
func (a *App) completeJobIDs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	jobs, err := a.listJobs(cmd.Context(), 50)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	out := []string{aliasLast, aliasRunning}
	for _, j := range jobs {
		if strings.HasPrefix(j.ID, toComplete) {
			out = append(out, j.ID)
		}
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}

// completeCollectionIDs is the shell completion for collection arguments.
func (a *App) completeCollectionIDs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	q := url.Values{"limit": {"50"}, "include_retired": {"true"}}
	r, err := a.call(cmd.Context(), "GET", "/v1/collections", q, nil, "")
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var page struct {
		Items []collectionView `json:"items"`
	}
	if err = r.Decode(&page); err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	out := make([]string, 0, len(page.Items))
	for _, c := range page.Items {
		if strings.HasPrefix(c.ID, toComplete) {
			out = append(out, c.ID)
		}
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}
