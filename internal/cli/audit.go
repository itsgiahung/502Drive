package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/spf13/cobra"
)

// auditView mirrors the audit DTO; --json passes the server envelope through
// untouched, so this is only for human output.
type auditView struct {
	Plan         json.RawMessage `json:"plan,omitempty"`
	Repairable   int             `json:"repairable_findings"`
	ID           string          `json:"id"`
	CollectionID string          `json:"collection_id"`
	JobID        string          `json:"job_id"`
	State        string          `json:"state"`
	Counts       map[string]int  `json:"counts"`
	Scanned      int             `json:"scanned_folders"`
	Revision     int             `json:"revision"`
	StartedAt    string          `json:"started_at"`
	FinishedAt   *string         `json:"finished_at"`
}

type findingView struct {
	Path       string `json:"path"`
	AuditID    string `json:"audit_id"`
	Kind       string `json:"kind"`
	SourceID   string `json:"source_id"`
	DestID     string `json:"dest_id"`
	Repairable bool   `json:"repairable"`
	Repaired   bool   `json:"repaired"`
	Detail     string `json:"detail"`
}

// findingKinds is the stable order the summary is printed in.
var findingKinds = []string{
	"missing_at_destination", "extra_at_destination", "name_mismatch", "size_or_md5_mismatch",
	"wrong_parent", "duplicate_at_destination", "source_removed", "unreadable_source",
}

// auditCmd compares the source and destination of a collection and, with
// --repair, fixes what is safe. The repair defaults to a dry run; --apply
// performs the (reversible, intent-checked) fixes.
func (a *App) auditCmd() *cobra.Command {
	var repair, dryRun, apply bool
	var wait time.Duration
	c := &cobra.Command{Use: "audit COLLECTION", Short: "Compare a copied source with its destination and report (or fix) the differences", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if apply && dryRun {
				return usage("choose --apply or --dry-run, not both")
			}
			return a.runAudit(cmd.Context(), args[0], repair, repair && !apply, wait)
		}}
	f := c.Flags()
	f.BoolVar(&repair, "repair", false, "after the audit, plan the safe fixes (dry run unless --apply)")
	f.BoolVar(&dryRun, "dry-run", false, "with --repair: plan only, change nothing (the default)")
	f.BoolVar(&apply, "apply", false, "with --repair: perform the planned fixes (only our own tagged objects are touched; nothing is deleted)")
	f.DurationVar(&wait, "wait", 10*time.Minute, "how long to watch each job")

	var kind string
	var limit int
	var all bool
	show := &cobra.Command{Use: "show AUDIT_ID", Short: "Show one audit report and its findings", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.showAudit(cmd.Context(), args[0], kind, limit, all)
		}}
	show.Flags().StringVar(&kind, "kind", "", "only this finding kind")
	show.Flags().IntVar(&limit, "limit", 50, "findings per page (raise, or add --all)")
	show.Flags().BoolVar(&all, "all", false, "page through every finding")
	c.AddCommand(show)
	return c
}

func (a *App) runAudit(ctx context.Context, collection string, repair, dry bool, wait time.Duration) error {
	// A new key per invocation: a key derived from the collection alone would
	// replay the first audit forever. Retries inside this call reuse it.
	key := NewKey()
	r, err := a.call(ctx, "POST", "/v1/collections/"+url.PathEscape(collection)+"/audits", nil, nil, key)
	if err != nil {
		return err
	}
	var av auditView
	if err = r.Decode(&av); err != nil {
		return err
	}
	if !a.g.quiet {
		fmt.Fprintf(a.errw(), "Audit %s started as job %s.\n", av.ID, av.JobID)
	}
	if err := a.waitJob(ctx, av.JobID, wait); err != nil {
		return err
	}
	out, _, err := a.fetchAudit(ctx, av.ID, "", true)
	if err != nil {
		return err
	}
	a.emitData(requestID("audit-report", av.ID), out, func() { a.printAudit(out) })

	if repair {
		body := map[string]any{"expected_revision": out.audit.Revision, "dry_run": dry}
		rr, err := a.call(ctx, "POST", "/v1/audits/"+url.PathEscape(av.ID)+"/repair", nil, body, requestID("repair", av.ID, fmt.Sprint(out.audit.Revision)))
		if err != nil {
			return err
		}
		var j jobView
		if err = rr.Decode(&j); err != nil {
			return err
		}
		if !a.g.quiet {
			word := "plan"
			if !dry {
				word = "fix"
			}
			fmt.Fprintf(a.errw(), "Repair %s started as job %s.\n", word, j.ID)
		}
		if err := a.waitJob(ctx, j.ID, wait); err != nil {
			return err
		}
		out2, _, err := a.fetchAudit(ctx, av.ID, "", true)
		if err != nil {
			return err
		}
		a.emitData(requestID("repair-report", av.ID), out2, func() { a.printAudit(out2) })
	}
	return nil
}

func (a *App) showAudit(ctx context.Context, id, kind string, limit int, all bool) error {
	r, err := a.call(ctx, "GET", "/v1/audits/"+url.PathEscape(id), nil, nil, "")
	if err != nil {
		return err
	}
	out, _, err := a.fetchAudit(ctx, id, kind, all)
	if err != nil {
		return err
	}
	a.emitData(r.RequestID, out, func() { a.printAudit(out) })
	return nil
}

// fetchAudit reads the audit and pages through its findings.
func (a *App) fetchAudit(ctx context.Context, id, kind string, all bool) (auditOut, *Response, error) {
	var out auditOut
	r, err := a.call(ctx, "GET", "/v1/audits/"+url.PathEscape(id), nil, nil, "")
	if err != nil {
		return out, r, err
	}
	if err = r.Decode(&out.audit); err != nil {
		return out, r, err
	}
	q := url.Values{"limit": {"100"}}
	if kind != "" {
		q.Set("kind", kind)
	}
	for {
		rr, err := a.call(ctx, "GET", "/v1/audits/"+url.PathEscape(id)+"/findings", q, nil, "")
		if err != nil {
			return out, r, err
		}
		var page struct {
			Items []findingView `json:"items"`
			Next  *string       `json:"next_cursor"`
		}
		if err = rr.Decode(&page); err != nil {
			return out, r, err
		}
		out.findings = append(out.findings, page.Items...)
		if page.Next == nil || *page.Next == "" || !all {
			break
		}
		q.Set("cursor", *page.Next)
	}
	return out, r, nil
}

type auditOut struct {
	audit    auditView
	findings []findingView
}

// emitData prints the CLI's own envelope for a composed report (the audit
// plus its findings); human output goes through the callback.
func (a *App) emitData(requestID string, out auditOut, human func()) {
	if a.g.json {
		ab, _ := json.Marshal(out.audit)
		var m map[string]any
		_ = json.Unmarshal(ab, &m)
		m["findings"] = out.findings
		b, _ := json.Marshal(map[string]any{"data": m, "request_id": requestID})
		fmt.Fprintln(a.out(), string(b))
		return
	}
	if !a.g.quiet {
		human()
	}
}

func (a *App) printAudit(out auditOut) {
	av := out.audit
	total := 0
	for _, n := range av.Counts {
		total += n
	}
	fmt.Fprintf(a.out(), "audit %s  collection %s  state %s\n", av.ID, av.CollectionID, av.State)
	fmt.Fprintf(a.out(), "scanned %d folders, %d finding(s)\n", av.Scanned, total)
	for _, kind := range findingKinds {
		if n := av.Counts[kind]; n > 0 {
			fmt.Fprintf(a.out(), "  %-24s %d\n", kind, n)
		}
	}
	for _, f := range out.findings {
		flags := ""
		if f.Repairable {
			flags += " repairable"
		}
		if f.Repaired {
			flags += " repaired"
		}
		fmt.Fprintf(a.out(), "  %-24s %s %s %s%s\n", f.Kind, f.SourceID, f.DestID, f.Detail, flags)
	}
	if av.State == "running" {
		fmt.Fprintln(a.out(), "the audit is still running; check again with `502drive audit show "+av.ID+"`.")
	}
}
