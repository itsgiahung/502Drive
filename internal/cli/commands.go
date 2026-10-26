package cli

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/GiaHung07/502Drive/internal/config"
)

func newLogger(w io.Writer) *slog.Logger {
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelInfo}))
}

// ---- wire shapes (mirror docs/contracts/openapi.json) ----

type jobCounts struct {
	Observed  int   `json:"observed_items"`
	Total     *int  `json:"total_items"`
	Completed int   `json:"completed_items"`
	Failed    int   `json:"failed_items"`
	Blocked   int   `json:"blocked_items"`
	Skipped   int   `json:"skipped_items"`
	Bytes     int64 `json:"logical_bytes"`
}

type failure struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type jobView struct {
	ID           string    `json:"id"`
	CollectionID string    `json:"collection_id"`
	SourceName   string    `json:"source_name"`
	Destination  string    `json:"destination_id"`
	RootName     string    `json:"root_name"`
	State        string    `json:"state"`
	Phase        *string   `json:"phase"`
	Desired      string    `json:"desired_state"`
	Revision     int       `json:"revision"`
	Counts       jobCounts `json:"counts"`
	ResultURL    *string   `json:"result_url"`
	LastError    *failure  `json:"last_error"`
	UpdatedAt    string    `json:"updated_at"`
}

type planView struct {
	ID          string `json:"id"`
	Revision    int    `json:"revision"`
	ExpiresAt   string `json:"expires_at"`
	SourceID    string `json:"source_id"`
	SourceName  string `json:"source_name"`
	SourceKind  string `json:"source_kind"`
	Destination string `json:"destination_id"`
	RootName    string `json:"root_name"`
	Preview     struct {
		Files    int  `json:"observed_files"`
		Folders  int  `json:"observed_folders"`
		Complete bool `json:"scan_complete"`
	} `json:"preview"`
	Warnings []string `json:"warnings"`
}

func terminalState(s string) bool {
	switch s {
	case "succeeded", "partial", "failed", "cancelled", "blocked":
		return true
	}
	return false
}

func (a *App) printJob(j jobView) {
	s := a.style
	rows := []kv{{"job", j.ID}, {"state", s.stateColor(j.State)}, {"source", Sanitize(j.SourceName)}, {"target", Sanitize(j.RootName)}}
	if j.Phase != nil {
		rows = append(rows, kv{"phase", *j.Phase})
	}
	total := "unknown"
	if j.Counts.Total != nil {
		total = strconv.Itoa(*j.Counts.Total)
	}
	rows = append(rows, kv{"items", fmt.Sprintf("%d done / %s total, %d failed, %d blocked, %d skipped", j.Counts.Completed, total, j.Counts.Failed, j.Counts.Blocked, j.Counts.Skipped)})
	if j.ResultURL != nil {
		rows = append(rows, kv{"result", *j.ResultURL})
	}
	if j.LastError != nil {
		rows = append(rows, kv{"problem", s.warn(Sanitize(j.LastError.Message)) + " (" + j.LastError.Code + ")"})
	}
	printKV(a.out(), s, rows)
}

// ---- status ----

func (a *App) statusCmd() *cobra.Command {
	return &cobra.Command{Use: "status", Short: "Daemon, account and queue summary", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		r, err := a.call(cmd.Context(), "GET", "/v1/status", nil, nil, "")
		if err != nil {
			return err
		}
		var st struct {
			Version string `json:"version"`
			Schema  int    `json:"schema_version"`
			Ready   bool   `json:"ready"`
			Bot     string `json:"bot_state"`
			Running int    `json:"running_jobs"`
			Queued  int    `json:"queued_jobs"`
			Blocked int    `json:"blocked_jobs"`
			Account struct {
				Status string  `json:"status"`
				Label  *string `json:"label"`
			} `json:"account"`
		}
		if err = r.Decode(&st); err != nil {
			return err
		}
		a.emit(r, "", func() {
			s := a.style
			ready := s.bad("NOT READY")
			if st.Ready {
				ready = s.good("ready")
			}
			acct := s.stateColor(st.Account.Status)
			if st.Account.Label != nil {
				acct += "  " + Sanitize(*st.Account.Label)
			}
			printKV(a.out(), s, []kv{{"daemon", ready + "  v" + Sanitize(st.Version)}, {"google", acct}, {"telegram", st.Bot},
				{"jobs", fmt.Sprintf("%d running  %d queued  %d blocked", st.Running, st.Queued, st.Blocked)}})
		})
		return nil
	}}
}

// ---- inspect & copy ----

func (a *App) inspectCmd() *cobra.Command {
	var to, name string
	c := &cobra.Command{Use: "inspect SOURCE", Short: "Preview a copy; writes nothing to Drive", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		plan, r, err := a.makePlan(cmd.Context(), args[0], to, name, NewKey())
		if err != nil {
			return err
		}
		a.emit(r, plan.ID, func() { a.printPlan(plan) })
		return nil
	}}
	c.Flags().StringVar(&to, "to", "", "destination profile ID (default destination if empty)")
	c.Flags().StringVar(&name, "name", "", "root folder name")
	return c
}

func (a *App) makePlan(ctx context.Context, source, to, name, key string) (planView, *Response, error) {
	var plan planView
	body := map[string]any{"source": source}
	if to != "" {
		body["destination_id"] = to
	}
	if name != "" {
		body["root_name"] = name
	}
	r, err := a.call(ctx, "POST", "/v1/plans", nil, body, key)
	if err != nil {
		return plan, nil, err
	}
	return plan, r, r.Decode(&plan)
}

func (a *App) printPlan(p planView) {
	count := fmt.Sprintf("%d files, %d folders", p.Preview.Files, p.Preview.Folders)
	if !p.Preview.Complete {
		count += " seen so far (preview is not a full scan)"
	}
	printKV(a.out(), a.style, []kv{{"plan", p.ID}, {"source", Sanitize(p.SourceName) + "  (" + p.SourceKind + ")"}, {"target", Sanitize(p.RootName)}, {"preview", count}, {"expires", p.ExpiresAt}})
}

// pendingStore keeps idempotency keys for copy requests that may have been
// admitted although the client never saw the response.
type pendingStore struct {
	path string
	mu   sync.Mutex
}

type pendingEntry struct {
	PlanKey string    `json:"plan_key"`
	JobKey  string    `json:"job_key"`
	Created time.Time `json:"created"`
}

const pendingTTL = 24 * time.Hour

func (p *pendingStore) load() map[string]pendingEntry {
	m := map[string]pendingEntry{}
	if b, err := os.ReadFile(p.path); err == nil {
		json.Unmarshal(b, &m)
	}
	for k, v := range m {
		if time.Since(v.Created) > pendingTTL {
			delete(m, k)
		}
	}
	return m
}

func (p *pendingStore) save(m map[string]pendingEntry) error {
	if err := os.MkdirAll(filepath.Dir(p.path), 0o700); err != nil {
		return err
	}
	b, _ := json.Marshal(m)
	tmp, err := os.CreateTemp(filepath.Dir(p.path), "pending-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err = tmp.Chmod(0o600); err == nil {
		_, err = tmp.Write(b)
	}
	if e := tmp.Close(); err == nil {
		err = e
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp.Name(), p.path)
}

// acquire returns the stored keys for an identical request or creates and
// persists new ones before anything is sent.
func (p *pendingStore) acquire(id string) (pendingEntry, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	m := p.load()
	if e, ok := m[id]; ok {
		return e, nil
	}
	e := pendingEntry{PlanKey: NewKey(), JobKey: NewKey(), Created: time.Now()}
	m[id] = e
	return e, p.save(m)
}

func (p *pendingStore) release(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	m := p.load()
	delete(m, id)
	p.save(m)
}

func (a *App) pending() *pendingStore {
	dir := filepath.Dir(a.configPath())
	return &pendingStore{path: filepath.Join(dir, "pending-requests.json")}
}

func requestID(parts ...string) string {
	h := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(h[:12])
}

func readStdinRef(r io.Reader) (string, error) {
	sc := bufio.NewScanner(io.LimitReader(r, 64<<10))
	var lines []string
	for sc.Scan() {
		if t := strings.TrimSpace(sc.Text()); t != "" {
			lines = append(lines, t)
		}
	}
	if len(lines) != 1 {
		return "", usage("--stdin expects exactly one Drive reference, got %d", len(lines))
	}
	return lines[0], nil
}

func (a *App) copyCmd() *cobra.Command {
	var to, name, key string
	var wait, useStdin, dry bool
	var waitTimeout time.Duration
	c := &cobra.Command{Use: "copy [SOURCE]", Short: "Plan and durably submit a server-side copy", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		var source string
		switch {
		case useStdin && len(args) == 1:
			return usage("give SOURCE or --stdin, not both")
		case useStdin:
			var err error
			if source, err = readStdinRef(a.io.In); err != nil {
				return err
			}
		case len(args) == 1:
			source = strings.TrimSpace(args[0])
		default:
			return usage("a Drive link or ID is required (or --stdin)")
		}
		if dry {
			plan, r, err := a.makePlan(cmd.Context(), source, to, name, NewKey())
			if err != nil {
				return err
			}
			a.emit(r, plan.ID, func() { a.printPlan(plan) })
			return nil
		}
		job, r, err := a.submitCopy(cmd.Context(), source, to, name, key)
		if err != nil {
			return err
		}
		if !wait {
			a.emit(r, job.ID, func() {
				fmt.Fprintf(a.out(), "%s  %s\n", a.style.stateColor("queued"), job.ID)
				printKV(a.out(), a.style, []kv{{"source", Sanitize(job.SourceName)}, {"target", Sanitize(job.RootName)}, {"next", "502drive jobs wait " + job.ID}})
			})
			return nil
		}
		if !a.g.json && !a.g.quiet {
			fmt.Fprintf(a.errw(), "Accepted job %s; waiting…\n", job.ID)
		}
		return a.waitJob(cmd.Context(), job.ID, waitTimeout)
	}}
	f := c.Flags()
	f.StringVar(&to, "to", "", "destination profile ID")
	f.StringVar(&name, "name", "", "root folder name")
	f.BoolVar(&wait, "wait", false, "follow the job; exit 0 only when it succeeds")
	f.BoolVar(&useStdin, "stdin", false, "read exactly one reference from stdin")
	f.BoolVar(&dry, "dry-run", false, "preview only (same as inspect)")
	f.StringVar(&key, "idempotency-key", "", "explicit key for the job submission")
	f.DurationVar(&waitTimeout, "wait-timeout", 0, "stop waiting after this long (job keeps running)")
	return c
}

// submitCopy plans and submits with keys that survive a lost response.
func (a *App) submitCopy(ctx context.Context, source, to, name, explicitKey string) (jobView, *Response, error) {
	var job jobView
	ps := a.pending()
	id := requestID(source, to, name, explicitKey)
	var planKey, jobKey string
	if explicitKey != "" {
		planKey, jobKey = explicitKey+"-plan", explicitKey
	} else {
		e, err := ps.acquire(id)
		if err != nil {
			return job, nil, localErr(ExitInternal, "pending_state", "Cannot write the private retry state: "+err.Error())
		}
		planKey, jobKey = e.PlanKey, e.JobKey
	}
	definitive := func(err error) bool {
		var ae *APIError
		return errors.As(err, &ae) && ae.Status > 0 && ae.Status < 500 && ae.Status != 429
	}
	plan, _, err := a.makePlan(ctx, source, to, name, planKey)
	if err != nil {
		if definitive(err) {
			ps.release(id)
		}
		return job, nil, err
	}
	r, err := a.call(ctx, "POST", "/v1/jobs", nil, map[string]any{"plan_id": plan.ID, "expected_revision": plan.Revision}, jobKey)
	if err != nil {
		if definitive(err) {
			ps.release(id)
		} else {
			fmt.Fprintln(a.errw(), a.style.warn("note:")+" the request may have been accepted; repeat the same command and it will not create a second job.")
		}
		return job, nil, err
	}
	ps.release(id)
	return job, r, r.Decode(&job)
}

// ---- jobs ----

func (a *App) jobsCmd() *cobra.Command {
	jobs := &cobra.Command{Use: "jobs", Short: "Query and control jobs"}
	var state, cursor string
	var limit int
	var all bool
	list := &cobra.Command{Use: "list", Short: "List jobs (newest first)", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		q := url.Values{"limit": {strconv.Itoa(limit)}}
		if state != "" {
			q.Set("state", state)
		}
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		if all {
			q.Set("all", "true")
		}
		r, err := a.call(cmd.Context(), "GET", "/v1/jobs", q, nil, "")
		if err != nil {
			return err
		}
		var page struct {
			Items []jobView `json:"items"`
			Next  *string   `json:"next_cursor"`
		}
		if err = r.Decode(&page); err != nil {
			return err
		}
		a.emit(r, "", func() {
			if len(page.Items) == 0 {
				fmt.Fprintln(a.out(), "No jobs.")
				return
			}
			var rows [][]string
			for _, j := range page.Items {
				rows = append(rows, []string{j.ID, a.style.stateColor(j.State), Truncate(Sanitize(j.SourceName), 28, a.style.ASCII), fmt.Sprintf("%d/%s", j.Counts.Completed, totalText(j.Counts))})
			}
			printTable(a.out(), a.style, []string{"job", "state", "source", "items"}, rows)
			if page.Next != nil {
				fmt.Fprintf(a.out(), "\nMore: 502drive jobs list --cursor %s\n", *page.Next)
			}
		})
		return nil
	}}
	list.Flags().StringVar(&state, "state", "", "filter by state")
	list.Flags().StringVar(&cursor, "cursor", "", "cursor from a previous page")
	list.Flags().IntVar(&limit, "limit", 20, "page size (1-100)")
	list.Flags().BoolVar(&all, "all", false, "owner: include other principals' jobs")

	get := &cobra.Command{Use: "get JOB_ID", Short: "Show one job", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		r, err := a.call(cmd.Context(), "GET", "/v1/jobs/"+url.PathEscape(args[0]), nil, nil, "")
		if err != nil {
			return err
		}
		var j jobView
		if err = r.Decode(&j); err != nil {
			return err
		}
		a.emit(r, j.ID, func() { a.printJob(j) })
		return nil
	}}

	var waitTimeout time.Duration
	wait := &cobra.Command{Use: "wait JOB_ID", Short: "Follow a job until it finishes", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return a.waitJob(cmd.Context(), args[0], waitTimeout)
	}}
	wait.Flags().DurationVar(&waitTimeout, "wait-timeout", 0, "give up waiting after this long (job keeps running)")

	var after int64
	var follow bool
	events := &cobra.Command{Use: "events JOB_ID", Short: "Durable job events (NDJSON with --json)", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return a.streamEvents(cmd.Context(), args[0], after, follow)
	}}
	events.Flags().Int64Var(&after, "after-sequence", 0, "resume after this sequence")
	events.Flags().BoolVar(&follow, "follow", false, "keep polling until the job is finished")

	jobs.AddCommand(list, get, wait, events)
	for _, act := range []string{"pause", "resume", "cancel", "retry"} {
		act := act
		jobs.AddCommand(&cobra.Command{Use: act + " JOB_ID", Short: strings.ToUpper(act[:1]) + act[1:] + " a job", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			j, err := a.fetchJob(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			r, err := a.call(cmd.Context(), "POST", "/v1/jobs/"+url.PathEscape(args[0])+"/"+act, nil, map[string]any{"expected_revision": j.Revision}, NewKey())
			if err != nil {
				return err
			}
			var nj jobView
			if err = r.Decode(&nj); err != nil {
				return err
			}
			a.emit(r, nj.ID, func() { fmt.Fprintf(a.out(), "%s requested.\n", act); a.printJob(nj) })
			return nil
		}})
	}
	return jobs
}

func totalText(c jobCounts) string {
	if c.Total == nil {
		return "?"
	}
	return strconv.Itoa(*c.Total)
}

func (a *App) fetchJob(ctx context.Context, id string) (jobView, error) {
	var j jobView
	r, err := a.call(ctx, "GET", "/v1/jobs/"+url.PathEscape(id), nil, nil, "")
	if err != nil {
		return j, err
	}
	return j, r.Decode(&j)
}

// waitJob polls until a terminal state. Ctrl-C and timeouts only stop
// watching; the job keeps running.
func (a *App) waitJob(ctx context.Context, id string, maxWait time.Duration) error {
	deadline := time.Time{}
	if maxWait > 0 {
		deadline = time.Now().Add(maxWait)
	}
	last := ""
	for {
		r, err := a.call(ctx, "GET", "/v1/jobs/"+url.PathEscape(id), nil, nil, "")
		if err != nil {
			var ae *APIError
			if errors.As(err, &ae) && ae.Exit == ExitInterrupt {
				fmt.Fprintf(a.errw(), "Stopped watching; job %s keeps running.\n", id)
			}
			return err
		}
		var j jobView
		if err = r.Decode(&j); err != nil {
			return err
		}
		line := fmt.Sprintf("%s %d/%s", strings.ToUpper(j.State), j.Counts.Completed, totalText(j.Counts))
		if line != last && !a.g.json && !a.g.quiet {
			fmt.Fprintln(a.errw(), a.style.dim(line))
			last = line
		}
		if terminalState(j.State) {
			a.emit(r, j.ID, func() { a.printJob(j) })
			switch j.State {
			case "succeeded":
				return nil
			case "cancelled":
				return localErr(ExitCancelled, "job_cancelled", "The job was cancelled.")
			}
			msg := "The job finished as " + j.State + "."
			if j.LastError != nil {
				msg = j.LastError.Message
			}
			return localErr(ExitJobFailed, "job_"+j.State, msg)
		}
		if !deadline.IsZero() && time.Now().After(deadline) {
			return localErr(ExitUnavailable, "wait_timeout", fmt.Sprintf("Still %s after waiting; job %s keeps running.", j.State, id))
		}
		select {
		case <-ctx.Done():
			fmt.Fprintf(a.errw(), "Stopped watching; job %s keeps running.\n", id)
			return localErr(ExitInterrupt, "interrupted", "interrupted")
		case <-time.After(pollInterval):
		}
	}
}

func (a *App) streamEvents(ctx context.Context, id string, after int64, follow bool) error {
	for {
		q := url.Values{"after_sequence": {strconv.FormatInt(after, 10)}, "limit": {"100"}}
		r, err := a.call(ctx, "GET", "/v1/jobs/"+url.PathEscape(id)+"/events", q, nil, "")
		if err != nil {
			return err
		}
		var page struct {
			Items []json.RawMessage `json:"items"`
			Next  int64             `json:"next_sequence"`
		}
		if err = r.Decode(&page); err != nil {
			return err
		}
		for _, raw := range page.Items {
			var e struct {
				Seq     int64  `json:"sequence"`
				Kind    string `json:"kind"`
				Message string `json:"message"`
				At      string `json:"occurred_at"`
			}
			json.Unmarshal(raw, &e)
			if a.g.json {
				fmt.Fprintf(a.out(), "{\"data\":%s,\"request_id\":%q}\n", raw, r.RequestID)
			} else {
				fmt.Fprintf(a.out(), "%6d  %s  %-10s %s\n", e.Seq, e.At, e.Kind, Sanitize(e.Message))
			}
		}
		after = page.Next
		if !follow {
			return nil
		}
		j, err := a.fetchJob(ctx, id)
		if err != nil {
			return err
		}
		if terminalState(j.State) && len(page.Items) == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return localErr(ExitInterrupt, "interrupted", "interrupted")
		case <-time.After(pollInterval):
		}
	}
}

// ---- collections, sync ----

type collectionView struct {
	ID         string    `json:"id"`
	SourceName string    `json:"source_name"`
	State      string    `json:"state"`
	ResultURL  *string   `json:"result_url"`
	Revision   int       `json:"revision"`
	Baseline   string    `json:"baseline_job_id"`
	Sync       *syncView `json:"sync"`
}

func (a *App) collectionsCmd() *cobra.Command {
	cc := &cobra.Command{Use: "collections", Short: "Copied sources (library)"}
	var query string
	list := &cobra.Command{Use: "list", Short: "List copied sources", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		q := url.Values{}
		if query != "" {
			q.Set("query", query)
		}
		r, err := a.call(cmd.Context(), "GET", "/v1/collections", q, nil, "")
		if err != nil {
			return err
		}
		var page struct {
			Items []collectionView `json:"items"`
		}
		if err = r.Decode(&page); err != nil {
			return err
		}
		a.emit(r, "", func() {
			if len(page.Items) == 0 {
				fmt.Fprintln(a.out(), "No copied sources yet.")
				return
			}
			var rows [][]string
			for _, c := range page.Items {
				rows = append(rows, []string{c.ID, Truncate(Sanitize(c.SourceName), 30, a.style.ASCII), a.style.stateColor(c.State)})
			}
			printTable(a.out(), a.style, []string{"collection", "source", "state"}, rows)
		})
		return nil
	}}
	list.Flags().StringVar(&query, "query", "", "filter by source name")
	get := &cobra.Command{Use: "get COLLECTION_ID", Short: "Show a copied source", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		r, err := a.call(cmd.Context(), "GET", "/v1/collections/"+url.PathEscape(args[0]), nil, nil, "")
		if err != nil {
			return err
		}
		var c collectionView
		if err = r.Decode(&c); err != nil {
			return err
		}
		a.emit(r, c.ID, func() {
			rows := []kv{{"collection", c.ID}, {"source", Sanitize(c.SourceName)}, {"state", a.style.stateColor(c.State)}, {"baseline", c.Baseline}}
			if c.ResultURL != nil {
				rows = append(rows, kv{"output", *c.ResultURL})
			}
			printKV(a.out(), a.style, rows)
		})
		return nil
	}}
	cc.AddCommand(list, get)
	return cc
}

// ---- destinations ----

type destView struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	FolderID string  `json:"folder_id"`
	Revision int     `json:"revision"`
	Default  bool    `json:"is_default"`
	DriveID  *string `json:"drive_id"`
}

func (a *App) destinationsCmd() *cobra.Command {
	dc := &cobra.Command{Use: "destinations", Short: "Choose where copies go"}
	dc.AddCommand(&cobra.Command{Use: "list", Short: "List destinations", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		r, err := a.call(cmd.Context(), "GET", "/v1/destinations", nil, nil, "")
		if err != nil {
			return err
		}
		var page struct {
			Items []destView `json:"items"`
		}
		if err = r.Decode(&page); err != nil {
			return err
		}
		a.emit(r, "", func() {
			if len(page.Items) == 0 {
				fmt.Fprintln(a.out(), "No destinations. Add one: 502drive destinations add FOLDER_LINK")
				return
			}
			var rows [][]string
			for _, d := range page.Items {
				mark := ""
				if d.Default {
					mark = a.style.good("default")
				}
				rows = append(rows, []string{d.ID, Truncate(Sanitize(d.Name), 30, a.style.ASCII), mark})
			}
			printTable(a.out(), a.style, []string{"id", "name", ""}, rows)
		})
		return nil
	}})
	var name string
	add := &cobra.Command{Use: "add FOLDER_LINK", Short: "Verify a writable folder and save it", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		body := map[string]any{"source": args[0]}
		if name != "" {
			body["name"] = name
		}
		r, err := a.call(cmd.Context(), "POST", "/v1/destinations", nil, body, NewKey())
		if err != nil {
			return err
		}
		var d destView
		if err = r.Decode(&d); err != nil {
			return err
		}
		a.emit(r, d.ID, func() {
			fmt.Fprintf(a.out(), "Saved destination %s (%s).\nMake it the default: 502drive destinations default %s\n", d.ID, Sanitize(d.Name), d.ID)
		})
		return nil
	}}
	add.Flags().StringVar(&name, "name", "", "label for the destination")
	def := &cobra.Command{Use: "default DESTINATION_ID", Short: "Set the default destination", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		r, err := a.call(cmd.Context(), "GET", "/v1/destinations", nil, nil, "")
		if err != nil {
			return err
		}
		var page struct {
			Items []destView `json:"items"`
		}
		if err = r.Decode(&page); err != nil {
			return err
		}
		rev := -1
		for _, d := range page.Items {
			if d.ID == args[0] {
				rev = d.Revision
			}
		}
		if rev < 0 {
			return localErr(ExitNotFound, "not_found", "No such destination.")
		}
		r, err = a.call(cmd.Context(), "PUT", "/v1/destinations/default", nil, map[string]any{"destination_id": args[0], "expected_revision": rev}, NewKey())
		if err != nil {
			return err
		}
		a.emit(r, args[0], func() { fmt.Fprintf(a.out(), "Default destination is now %s.\n", args[0]) })
		return nil
	}}
	dc.AddCommand(add, def)
	return dc
}

// sshForwardHint prints the exact ssh tunnel for the session's redirect_uri
// port. It never guesses a port: a URL without one gets a generic hint.
func sshForwardHint(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	redirect, err := url.Parse(u.Query().Get("redirect_uri"))
	if err != nil {
		return ""
	}
	port := redirect.Port()
	if port == "" {
		return ""
	}
	return "Daemon on another machine? Run this on your machine first: ssh -L " + port + ":127.0.0.1:" + port + " YOU@DAEMON_HOST (see docs/oauth-setup.md)."
}

// ---- account ----

func (a *App) accountCmd() *cobra.Command {
	ac := &cobra.Command{Use: "account", Short: "Google authorization (through the daemon)"}
	ac.AddCommand(&cobra.Command{Use: "status", Short: "Show the Google account state", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		r, err := a.call(cmd.Context(), "GET", "/v1/account", nil, nil, "")
		if err != nil {
			return err
		}
		a.emit(r, "", func() { a.printAccount(r) })
		return nil
	}})
	var noBrowser bool
	var loginTimeout time.Duration
	login := &cobra.Command{Use: "login", Short: "Connect a Google account", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		r, err := a.call(cmd.Context(), "POST", "/v1/account/auth-sessions", nil, map[string]any{"no_browser": noBrowser}, NewKey())
		if err != nil {
			return err
		}
		var sess struct {
			ID  string  `json:"id"`
			URL *string `json:"authorization_url"`
		}
		if err = r.Decode(&sess); err != nil || sess.URL == nil {
			return localErr(ExitInternal, "bad_response", "No authorization URL was returned.")
		}
		interactive := a.io.StdoutTTY && !noBrowser && !a.g.noInput && !a.g.json
		if a.g.json {
			fmt.Fprintln(a.out(), strings.TrimSpace(string(r.Raw)))
		} else {
			fmt.Fprintln(a.errw(), "Open this link in a browser on this machine and approve access:")
			fmt.Fprintln(a.out(), *sess.URL)
			if interactive {
				if openBrowser(*sess.URL) == nil {
					fmt.Fprintln(a.errw(), a.style.dim("(opened your browser)"))
				}
			} else if hint := sshForwardHint(*sess.URL); hint != "" {
				fmt.Fprintln(a.errw(), a.style.dim(hint))
			} else {
				fmt.Fprintln(a.errw(), a.style.dim("Daemon on another machine? Forward its OAuth callback port with ssh -L (see docs/oauth-setup.md)."))
			}
			fmt.Fprintln(a.errw(), "Waiting for approval…")
		}
		deadline := time.Now().Add(loginTimeout)
		for {
			pr, err := a.call(cmd.Context(), "GET", "/v1/account/auth-sessions/"+url.PathEscape(sess.ID), nil, nil, "")
			if err != nil {
				return err
			}
			var st struct {
				State string `json:"state"`
			}
			pr.Decode(&st)
			switch st.State {
			case "connected":
				if !a.g.json {
					fmt.Fprintln(a.out(), a.style.good("Connected."))
				}
				return nil
			case "failed":
				return localErr(ExitAuth, "login_failed", "Login did not complete. Check the daemon log and try `502drive account login` again.")
			case "expired":
				return localErr(ExitAuth, "login_expired", "The login link expired. Run `502drive account login` again.")
			}
			if time.Now().After(deadline) {
				return localErr(ExitUnavailable, "wait_timeout", "Still waiting for approval; the link stays valid until it expires.")
			}
			select {
			case <-cmd.Context().Done():
				return localErr(ExitInterrupt, "interrupted", "interrupted")
			case <-time.After(pollInterval):
			}
		}
	}}
	login.Flags().BoolVar(&noBrowser, "no-browser", false, "only print the link")
	login.Flags().DurationVar(&loginTimeout, "login-timeout", 10*time.Minute, "stop waiting after this long")
	revoke := &cobra.Command{Use: "revoke", Short: "Remove local access and revoke the Google grant", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		r, err := a.call(cmd.Context(), "POST", "/v1/account/revoke", nil, map[string]any{}, NewKey())
		if err != nil {
			return err
		}
		a.emit(r, "", func() { a.printAccount(r) })
		return nil
	}}
	ac.AddCommand(login, revoke)
	return ac
}

func (a *App) printAccount(r *Response) {
	var acc struct {
		Status    string   `json:"status"`
		Label     *string  `json:"label"`
		LastError *failure `json:"last_error"`
	}
	r.Decode(&acc)
	rows := []kv{{"google", a.style.stateColor(acc.Status)}}
	if acc.Label != nil {
		rows = append(rows, kv{"account", Sanitize(*acc.Label)})
	}
	if acc.LastError != nil {
		rows = append(rows, kv{"problem", a.style.warn(Sanitize(acc.LastError.Message))})
	}
	printKV(a.out(), a.style, rows)
}

// ---- doctor ----

func (a *App) doctorCmd() *cobra.Command {
	var online bool
	c := &cobra.Command{Use: "doctor", Short: "Health checks (offline unless --online)", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		q := url.Values{}
		if online {
			q.Set("online", "true")
		}
		r, err := a.call(cmd.Context(), "GET", "/v1/doctor", q, nil, "")
		if err != nil {
			return err
		}
		var d struct {
			Checks []struct{ Name, Status, Message string } `json:"checks"`
		}
		if err = r.Decode(&d); err != nil {
			return err
		}
		failed := false
		a.emit(r, "", func() {
			for _, c := range d.Checks {
				fmt.Fprintf(a.out(), "%-8s %-14s %s\n", a.style.stateColor(c.Status), c.Name, Sanitize(c.Message))
			}
		})
		for _, c := range d.Checks {
			failed = failed || c.Status == "failed"
		}
		if failed {
			return localErr(ExitJobFailed, "doctor_failed", "One or more checks failed.")
		}
		return nil
	}}
	c.Flags().BoolVar(&online, "online", false, "also call Google Drive")
	return c
}

var _ = config.DefaultPath
