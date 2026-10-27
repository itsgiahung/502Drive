package cli

import (
	"context"
	"fmt"
	"github.com/itsgiahung/502Drive/internal/present"
	"net/url"
	"time"

	"github.com/spf13/cobra"
)

// lifecycleCmds are the collection lifecycle (L1) and quick-action (L3)
// subcommands of `collections`. They are thin HTTP clients: the daemon owns
// every decision and every Drive call.
func (a *App) lifecycleCmds() []*cobra.Command {
	return []*cobra.Command{
		a.collectionRetireCmd(),
		a.collectionResumeCmd(),
		a.collectionForgetCmd(),
		a.collectionCleanupCmd(),
		a.collectionOpenCmd(),
	}
}

func (a *App) collectionRetireCmd() *cobra.Command {
	return &cobra.Command{Use: "retire COLLECTION", Short: "Stop following a copied source (keeps the copy and its history; no Drive change)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.collectionStateChange(cmd, args[0], "retire")
		}}
}

// collectionResumeCmd is context-sensitive: a retired copy comes back to the
// library; otherwise a paused sync is resumed. Nothing else is guessed.
func (a *App) collectionResumeCmd() *cobra.Command {
	return &cobra.Command{Use: "resume COLLECTION", Short: "Resume a retired copy (or a paused sync, when the copy is not retired)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := a.fetchCollection(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if c.State == "retired" {
				return a.collectionStateChange(cmd, args[0], "resume")
			}
			r, err := a.syncActionRequest(cmd.Context(), args[0], "resume")
			if err != nil {
				return err
			}
			var view syncView
			if err = r.Decode(&view); err != nil {
				return err
			}
			a.emit(r, "", func() { a.printSyncView(view) })
			return nil
		}}
}

// collectionStateChange runs retire/resume through the revision guard.
func (a *App) collectionStateChange(cmd *cobra.Command, id, action string) error {
	r, err := a.revisionGuard(cmd.Context(), id, func(c collectionView) (*Response, error) {
		return a.call(cmd.Context(), "POST", "/v1/collections/"+url.PathEscape(c.ID)+"/"+action, nil,
			map[string]any{"expected_revision": c.Revision}, NewKey())
	})
	if err != nil {
		return err
	}
	var c collectionView
	if err = r.Decode(&c); err != nil {
		return err
	}
	a.emit(r, c.ID, func() {
		fmt.Fprintf(a.out(), "%s %s\n", a.style.stateColor(c.State), c.ID)
	})
	return nil
}

// collectionForgetCmd deletes local records only. It always needs --yes, also
// in a terminal: forgetting removes history that cannot be rebuilt.
func (a *App) collectionForgetCmd() *cobra.Command {
	var yes bool
	c := &cobra.Command{Use: "forget COLLECTION --yes", Short: "Delete a retired copied source's local records (never touches Drive)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !yes {
				return usage("forget removes local history; pass --yes to confirm (the collection must be retired first)")
			}
			r, err := a.revisionGuard(cmd.Context(), args[0], func(c collectionView) (*Response, error) {
				return a.call(cmd.Context(), "POST", "/v1/collections/"+url.PathEscape(c.ID)+"/forget", nil,
					map[string]any{"expected_revision": c.Revision}, NewKey())
			})
			if err != nil {
				return err
			}
			var out struct {
				Deleted bool   `json:"deleted"`
				ID      string `json:"id"`
			}
			if err = r.Decode(&out); err != nil {
				return err
			}
			a.emit(r, out.ID, func() { fmt.Fprintf(a.out(), "forgotten %s (Drive untouched)\n", out.ID) })
			return nil
		}}
	c.Flags().BoolVar(&yes, "yes", false, "confirm forgetting the local records")
	return c
}

// collectionCleanupCmd admits a cleanup job that moves the 502Drive-created
// destination root to the Drive trash. Dry-run is the default. A real run needs
// --dry-run=false, --yes and --confirm-name equal to the destination root name.
func (a *App) collectionCleanupCmd() *cobra.Command {
	var dryRun, yes bool
	var confirmName string
	c := &cobra.Command{Use: "cleanup COLLECTION", Short: "Move the destination copy to the Drive trash (retired collections only; dry-run by default)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !dryRun && (!yes || confirmName == "") {
				return usage("a real cleanup needs --yes and --confirm-name with the destination folder name (or keep the default --dry-run)")
			}
			r, err := a.revisionGuard(cmd.Context(), args[0], func(c collectionView) (*Response, error) {
				return a.call(cmd.Context(), "POST", "/v1/collections/"+url.PathEscape(c.ID)+"/cleanup", nil, map[string]any{
					"expected_revision": c.Revision,
					"dry_run":           dryRun,
					"confirm_name":      confirmName,
				}, NewKey())
			})
			if err != nil {
				return err
			}
			var job jobView
			if err = r.Decode(&job); err != nil {
				return err
			}
			if job.DryRun && !a.g.json && !a.g.quiet {
				// A dry run only reads Drive and finishes in moments: show its
				// plan instead of sending the user to another command.
				if done, ok := a.pollTerminal(cmd.Context(), job.ID, 30*time.Second); ok {
					job = done
				}
			}
			a.emit(r, job.ID, func() {
				mode := "dry-run"
				if !job.DryRun {
					mode = "trash"
				}
				fmt.Fprintf(a.out(), "%s  %s (%s)\n", a.style.stateColor(job.State), job.ID, mode)
				if job.DryRun && job.State == "succeeded" && job.Counts.Total != nil {
					fmt.Fprintf(a.out(), "Would move %q to the Drive trash: %d copied items, %s. Nothing was changed.\n",
						Sanitize(job.RootName), *job.Counts.Total, present.HumanBytes(job.Counts.Bytes))
					printKV(a.out(), a.style, []kv{{"next", "502drive collections cleanup " + args[0] + " --dry-run=false --yes --confirm-name '" + Sanitize(job.RootName) + "'"}})
					return
				}
				if job.LastError != nil && job.LastError.Message != "" {
					// The preview stopped, for example because a restore point
					// still needs this copy: say why rather than only the job ID.
					fmt.Fprintln(a.out(), Sanitize(job.LastError.Message))
				}
				printKV(a.out(), a.style, []kv{{"next", "502drive jobs get " + job.ID}})
			})
			return nil
		}}
	f := c.Flags()
	f.BoolVar(&dryRun, "dry-run", true, "only plan; change nothing on Drive")
	f.BoolVar(&yes, "yes", false, "confirm the trash move (with --dry-run=false)")
	f.StringVar(&confirmName, "confirm-name", "", "the destination folder name, typed again to confirm")
	return c
}

// collectionOpenCmd prints the Drive link of the source or the destination.
// It never launches a browser: the link is printed for the user to open.
func (a *App) collectionOpenCmd() *cobra.Command {
	var dest bool
	c := &cobra.Command{Use: "open COLLECTION", Short: "Print the source (or --dest: destination) Drive link", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, r, err := a.fetchCollection(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			link := ""
			switch {
			case dest && c.ResultURL != nil:
				link = *c.ResultURL
			case dest:
				return localErr(ExitNotFound, "no_destination", "This copy has no destination link yet.")
			default:
				link = "https://drive.google.com/open?id=" + url.QueryEscape(c.SourceID)
			}
			a.emit(r, c.ID, func() { fmt.Fprintln(a.out(), link) })
			return nil
		}}
	c.Flags().BoolVar(&dest, "dest", false, "print the destination link instead of the source")
	return c
}

// pollTerminal reads a job until it finishes or maxWait passes, quietly (no
// progress lines). ok is false when it did not finish in time.
func (a *App) pollTerminal(ctx context.Context, id string, maxWait time.Duration) (jobView, bool) {
	deadline := time.Now().Add(maxWait)
	for {
		r, err := a.call(ctx, "GET", "/v1/jobs/"+url.PathEscape(id), nil, nil, "")
		if err != nil {
			return jobView{}, false
		}
		var j jobView
		if r.Decode(&j) == nil && terminalState(j.State) {
			return j, true
		}
		if time.Now().After(deadline) {
			return jobView{}, false
		}
		select {
		case <-ctx.Done():
			return jobView{}, false
		case <-time.After(300 * time.Millisecond):
		}
	}
}
