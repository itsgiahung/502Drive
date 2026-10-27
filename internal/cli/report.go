package cli

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/itsgiahung/502Drive/internal/present"
)

// exportRowLimit caps both the per-job report and the history export. The
// command says on stderr when it cut rows; the exit code stays 0.
const exportRowLimit = 5000

// problemItem mirrors one failed/blocked/skipped item (GET /v1/jobs/{id}/items).
type problemItem struct {
	SourceID      string `json:"source_id"`
	Name          string `json:"name"`
	Path          string `json:"path"`
	Kind          string `json:"kind"`
	MimeType      string `json:"mime_type"`
	State         string `json:"state"`
	Code          string `json:"code"`
	Attempts      int    `json:"attempts"`
	SizeBytes     *int64 `json:"size_bytes"`
	DestinationID string `json:"destination_id"`
}

func (a *App) reportCmd() *cobra.Command {
	var format string
	var maxRows int
	c := &cobra.Command{Use: "report JOB_ID [--format table|json|csv]", Short: "List the failed, blocked and skipped items of a job", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resolved, err := a.resolveJobID(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			mode, err := a.outputFormat(format)
			if err != nil {
				return err
			}
			if maxRows < 1 || maxRows > exportRowLimit {
				return usage("--max-rows must be between 1 and %d", exportRowLimit)
			}
			return a.jobsReport(cmd.Context(), resolved, mode, maxRows)
		}}
	c.Flags().StringVar(&format, "format", "", "output format: table, json or csv (default table; --json means json)")
	c.Flags().IntVar(&maxRows, "max-rows", exportRowLimit, "stop after this many problem items")
	c.ValidArgsFunction = a.completeJobIDs
	return c
}

// outputFormat resolves --format against the global --json. Asking for CSV and
// JSON at once is a usage error rather than a guess.
func (a *App) outputFormat(format string) (string, error) {
	format = strings.ToLower(strings.TrimSpace(format))
	if format != "" && format != "table" && format != "json" && format != "csv" {
		return "", usage("--format must be table, json or csv")
	}
	if a.g.json {
		if format != "" && format != "json" {
			return "", usage("--json and --format %s conflict", format)
		}
		return "json", nil
	}
	if format == "" {
		return "table", nil
	}
	return format, nil
}

func (a *App) jobsReport(ctx context.Context, id, format string, maxRows int) error {
	j, err := a.fetchJob(ctx, id)
	if err != nil {
		return err
	}
	items, truncated, requestID, err := a.fetchProblems(ctx, j.ID, maxRows)
	if err != nil {
		return err
	}
	switch format {
	case "json":
		b, err := json.Marshal(map[string]any{
			"data":       map[string]any{"job": j, "items": items, "truncated": truncated},
			"request_id": requestID,
		})
		if err != nil {
			return err
		}
		fmt.Fprintln(a.out(), string(b))
	case "csv":
		if err := writeCSV(a.out(), reportHeader(), reportRows(items)); err != nil {
			return err
		}
	default:
		if a.g.quiet {
			return nil
		}
		a.printReport(j, items, truncated)
	}
	if truncated {
		fmt.Fprintf(a.errw(), "note: stopped at %d items; narrow the job or raise --max-rows (hard limit %d).\n", maxRows, exportRowLimit)
	}
	return nil
}

// fetchProblems pages the problem items, newest first, up to maxRows.
func (a *App) fetchProblems(ctx context.Context, jobID string, maxRows int) ([]problemItem, bool, string, error) {
	var all []problemItem
	cursor := ""
	requestID := ""
	for {
		q := url.Values{"problems": {"true"}, "limit": {"100"}}
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		r, err := a.call(ctx, "GET", "/v1/jobs/"+url.PathEscape(jobID)+"/items", q, nil, "")
		if err != nil {
			return nil, false, requestID, err
		}
		requestID = r.RequestID
		var page struct {
			Items []problemItem `json:"items"`
			Next  *string       `json:"next_cursor"`
		}
		if err = r.Decode(&page); err != nil {
			return nil, false, requestID, err
		}
		all = append(all, page.Items...)
		if len(all) > maxRows {
			return all[:maxRows], true, requestID, nil
		}
		if page.Next == nil || *page.Next == "" {
			return all, false, requestID, nil
		}
		if len(all) == maxRows {
			return all, true, requestID, nil
		}
		cursor = *page.Next
	}
}

func reportHeader() []string {
	return []string{"state", "code", "reason", "path", "attempts", "source_id", "destination_id"}
}

func reportRows(items []problemItem) [][]string {
	rows := make([][]string, 0, len(items))
	for _, it := range items {
		rows = append(rows, []string{
			it.State, it.Code, present.Reason(it.Code), it.Path, strconv.Itoa(it.Attempts), it.SourceID, it.DestinationID,
		})
	}
	return rows
}

func (a *App) printReport(j jobView, items []problemItem, truncated bool) {
	fmt.Fprintf(a.out(), "job %s  %s  %d problem item(s)\n", j.ID, a.style.stateColor(j.State), len(items))
	if len(items) == 0 {
		fmt.Fprintln(a.out(), "No failed, blocked or skipped items.")
		return
	}
	rows := make([][]string, 0, len(items))
	for _, it := range items {
		rows = append(rows, []string{
			a.style.stateColor(it.State), it.Code, Truncate(present.Reason(it.Code), 48, a.style.ASCII),
			Truncate(Sanitize(it.Path), 48, a.style.ASCII), strconv.Itoa(it.Attempts),
		})
	}
	printTable(a.out(), a.style, []string{"state", "code", "reason", "path", "tries"}, rows)
	if truncated {
		fmt.Fprintf(a.out(), "… stopped at %d rows.\n", len(items))
	}
}

// ---- history export ----

var sincePattern = regexp.MustCompile(`^(\d+)([smhdw])$`)

// parseSince accepts a relative window (30d, 12h, 90m, 2w) or an RFC3339/date
// timestamp. Empty means "no lower bound".
func parseSince(raw string, now time.Time) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, nil
	}
	if m := sincePattern.FindStringSubmatch(raw); m != nil {
		n, err := strconv.Atoi(m[1])
		if err != nil || n <= 0 {
			return time.Time{}, usage("--since must be a positive duration")
		}
		unit := map[string]time.Duration{"s": time.Second, "m": time.Minute, "h": time.Hour, "d": 24 * time.Hour, "w": 7 * 24 * time.Hour}[m[2]]
		return now.UTC().Add(-time.Duration(n) * unit), nil
	}
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.Parse("2006-01-02", raw); err == nil {
		return t.UTC(), nil
	}
	return time.Time{}, usage("--since must look like 30d, 12h, 2w or an RFC3339 timestamp")
}

func (a *App) historyCmd() *cobra.Command {
	c := &cobra.Command{Use: "history", Short: "Job history exports", Args: cobra.NoArgs}
	c.AddCommand(a.historyExportCmd())
	return c
}

func (a *App) historyExportCmd() *cobra.Command {
	var since, format string
	var all bool
	var maxRows int
	c := &cobra.Command{Use: "export [--since 30d] [--format csv|json|table]", Short: "Export job history (CSV by default)", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cutoff, err := parseSince(since, time.Now())
			if err != nil {
				return err
			}
			if format == "" {
				if a.g.json {
					format = "json"
				} else {
					format = "csv"
				}
			}
			mode, err := a.outputFormat(format)
			if err != nil {
				return err
			}
			if maxRows < 1 || maxRows > exportRowLimit {
				return usage("--max-rows must be between 1 and %d", exportRowLimit)
			}
			return a.historyExport(cmd.Context(), cutoff, mode, all, maxRows)
		}}
	c.Flags().StringVar(&since, "since", "30d", "lower bound: 30d, 12h, 2w or RFC3339")
	c.Flags().StringVar(&format, "format", "", "output format: csv, json or table (default csv)")
	c.Flags().BoolVar(&all, "all", false, "owner: include every principal")
	c.Flags().IntVar(&maxRows, "max-rows", exportRowLimit, "stop after this many jobs")
	return c
}

func (a *App) historyExport(ctx context.Context, since time.Time, format string, all bool, maxRows int) error {
	var jobs []jobView
	cursor := ""
	requestID := ""
	truncated := false
	for {
		q := url.Values{"limit": {"100"}}
		if !since.IsZero() {
			q.Set("since", since.UTC().Format(time.RFC3339Nano))
		}
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		if all {
			q.Set("all", "true")
		}
		r, err := a.call(ctx, "GET", "/v1/history", q, nil, "")
		if err != nil {
			return err
		}
		requestID = r.RequestID
		var page struct {
			Items []jobView `json:"items"`
			Next  *string   `json:"next_cursor"`
		}
		if err = r.Decode(&page); err != nil {
			return err
		}
		jobs = append(jobs, page.Items...)
		if page.Next == nil || *page.Next == "" {
			break
		}
		if len(jobs) >= maxRows {
			jobs = jobs[:maxRows]
			truncated = true
			break
		}
		cursor = *page.Next
	}
	switch format {
	case "json":
		b, err := json.Marshal(map[string]any{
			"data": map[string]any{
				"since":     since.Format(time.RFC3339),
				"items":     jobs,
				"truncated": truncated,
			},
			"request_id": requestID,
		})
		if err != nil {
			return err
		}
		fmt.Fprintln(a.out(), string(b))
	case "csv":
		if err := writeCSV(a.out(), historyHeader(), historyRows(jobs)); err != nil {
			return err
		}
	default:
		if !a.g.quiet {
			a.printHistory(jobs)
		}
	}
	if truncated {
		fmt.Fprintf(a.errw(), "note: stopped at %d jobs; narrow --since or raise --max-rows (hard limit %d).\n", maxRows, exportRowLimit)
	}
	return nil
}

func historyHeader() []string {
	return []string{"job_id", "state", "kind", "collection_id", "source_name", "root_name",
		"created_at", "updated_at", "total_items", "completed_items", "failed_items", "blocked_items", "skipped_items"}
}

func historyRows(jobs []jobView) [][]string {
	rows := make([][]string, 0, len(jobs))
	for _, j := range jobs {
		total := ""
		if j.Counts.Total != nil {
			total = strconv.Itoa(*j.Counts.Total)
		}
		rows = append(rows, []string{
			j.ID, j.State, j.Kind, j.CollectionID, j.SourceName, j.RootName,
			j.CreatedAt, j.UpdatedAt, total,
			strconv.Itoa(j.Counts.Completed), strconv.Itoa(j.Counts.Failed), strconv.Itoa(j.Counts.Blocked), strconv.Itoa(j.Counts.Skipped),
		})
	}
	return rows
}

func (a *App) printHistory(jobs []jobView) {
	if len(jobs) == 0 {
		fmt.Fprintln(a.out(), "No job history in this window.")
		return
	}
	rows := make([][]string, 0, len(jobs))
	for _, j := range jobs {
		rows = append(rows, []string{j.ID, a.style.stateColor(j.State), Truncate(Sanitize(j.SourceName), 36, a.style.ASCII), j.UpdatedAt})
	}
	printTable(a.out(), a.style, []string{"job", "state", "source", "updated"}, rows)
}

// ---- CSV ----

// csvGuard prefixes cells that a spreadsheet would treat as a formula. The
// apostrophe is literal data, so the cell stays text in Excel/LibreOffice.
func csvGuard(cell string) string {
	if cell == "" {
		return cell
	}
	switch cell[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + cell
	}
	return cell
}

func writeCSV(w io.Writer, header []string, rows [][]string) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(guardRow(header)); err != nil {
		return err
	}
	for _, row := range rows {
		if err := cw.Write(guardRow(row)); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

func guardRow(row []string) []string {
	out := make([]string, len(row))
	for i, cell := range row {
		out[i] = csvGuard(cell)
	}
	return out
}
