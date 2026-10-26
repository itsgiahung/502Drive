package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/itsgiahung/502Drive/internal/schedule"
)

// defaultSyncTimezone is used when a preset does not name a zone. Schedules
// never use the host timezone.
const defaultSyncTimezone = "Asia/Ho_Chi_Minh"

// syncView mirrors SyncSubscription for decode and printing; the JSON
// passthrough still prints the server envelope unchanged.
type syncView struct {
	ID             string        `json:"id"`
	CollectionID   string        `json:"collection_id"`
	Mode           string        `json:"mode"`
	State          string        `json:"state"`
	Revision       int           `json:"revision"`
	Schedule       *scheduleView `json:"schedule"`
	ContentPolicy  string        `json:"content_policy"`
	DeletionPolicy string        `json:"deletion_policy"`
	LastAttemptAt  *string       `json:"last_attempt_at"`
	LastSuccessAt  *string       `json:"last_success_at"`
	NextRunAt      *string       `json:"next_run_at"`
	ActiveJobID    *string       `json:"active_job_id"`
	LastError      *failure      `json:"last_error"`
}

type scheduleView struct {
	Kind            string  `json:"kind"`
	IntervalMinutes int     `json:"interval_minutes"`
	AnchorAt        *string `json:"anchor_at"`
	Timezone        string  `json:"timezone"`
	LocalTime       string  `json:"local_time"`
	Weekdays        []int   `json:"weekdays"`
}

// Describe is a short human summary; the API payload stays untouched.
func (s *scheduleView) Describe() string {
	if s == nil {
		return "none"
	}
	switch s.Kind {
	case schedule.KindInterval:
		sp := schedule.Spec{Kind: schedule.KindInterval, IntervalMinutes: s.IntervalMinutes}
		return sp.Describe()
	case schedule.KindDaily:
		return fmt.Sprintf("daily at %s (%s)", s.LocalTime, s.Timezone)
	case schedule.KindWeekly:
		return fmt.Sprintf("%s at %s (%s)", weekdayNames(s.Weekdays), s.LocalTime, s.Timezone)
	}
	return s.Kind
}

func weekdayNames(days []int) string {
	names := []string{"", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
	var out []string
	for _, d := range days {
		if d >= 1 && d <= 7 {
			out = append(out, names[d])
		}
	}
	return strings.Join(out, ",")
}

func (a *App) syncCmd() *cobra.Command {
	sc := &cobra.Command{Use: "sync", Short: "Scheduled or realtime sync of copied sources"}
	sc.AddCommand(a.syncEnableCmd(), a.syncStatusCmd(), a.syncRunCmd(),
		a.syncActionCmd("pause"), a.syncActionCmd("resume"), a.syncActionCmd("stop"))
	return sc
}

func (a *App) syncEnableCmd() *cobra.Command {
	var mode, daily, weekly, every, timezone, policy string
	var dryRun bool
	c := &cobra.Command{Use: "enable COLLECTION", Short: "Turn on a scheduled or realtime sync", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// The timezone preference applies unless the flag is explicit.
			tz := timezone
			if !cmd.Flags().Changed("timezone") {
				tz = a.preferredTimezone(cmd.Context())
			}
			spec, err := buildSchedule(mode, daily, weekly, every, tz)
			if err != nil {
				return err
			}
			if policy != "versioned_copy" && policy != "replace_copy" {
				return usage("--policy must be versioned_copy or replace_copy")
			}
			if mode == "" {
				mode = "scheduled"
			}
			if dryRun {
				return a.printSyncPreview(spec)
			}
			r, err := a.enableSync(cmd.Context(), args[0], mode, policy, spec)
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
	f := c.Flags()
	f.StringVar(&mode, "mode", "scheduled", "scheduled or realtime")
	f.StringVar(&daily, "daily", "", "daily preset, HH:MM")
	f.StringVar(&weekly, "weekly", "", "weekly preset, mon,wed@HH:MM")
	f.StringVar(&every, "every", "", "interval preset, e.g. 30m, 6h")
	f.StringVar(&timezone, "timezone", defaultSyncTimezone, "IANA timezone for daily/weekly presets (default: your preference)")
	f.StringVar(&policy, "policy", "versioned_copy", "versioned_copy keeps old copies; replace_copy retires them")
	f.BoolVar(&dryRun, "dry-run", false, "print the schedule and the next 3 runs without saving")
	return c
}

// preferredTimezone reads the principal preference; a failed read falls back
// to the project default so the CLI still works against a stale daemon.
func (a *App) preferredTimezone(ctx context.Context) string {
	r, err := a.call(ctx, "GET", "/v1/preferences", nil, nil, "")
	if err != nil {
		return defaultSyncTimezone
	}
	var prefs struct {
		Timezone string `json:"timezone"`
	}
	if r.Decode(&prefs) != nil || prefs.Timezone == "" {
		return defaultSyncTimezone
	}
	return prefs.Timezone
}

// printSyncPreview prints a schedule and its next three runs (dry run).
func (a *App) printSyncPreview(spec *schedule.Spec) error {
	if spec == nil {
		if a.g.json {
			fmt.Fprintln(a.out(), `{"data":{"mode":"realtime","next_runs":[]},"request_id":""}`)
			return nil
		}
		fmt.Fprintf(a.out(), "realtime (Drive change polling) — no scheduled runs\n")
		return nil
	}
	slots, err := spec.Preview(time.Now().UTC(), 3)
	if err != nil {
		return localErr(ExitUsage, "invalid_request", "The schedule has no future occurrence: "+err.Error())
	}
	runs := make([]string, 0, len(slots))
	loc := time.UTC
	if spec.Timezone != "" {
		if loaded, lerr := time.LoadLocation(spec.Timezone); lerr == nil {
			loc = loaded
		}
	}
	for _, slot := range slots {
		runs = append(runs, slot.At.Format(time.RFC3339))
	}
	human := make([]string, 0, len(slots))
	for _, slot := range slots {
		human = append(human, slot.At.In(loc).Format("2006-01-02 15:04 MST"))
	}
	if a.g.json {
		b, merr := json.Marshal(map[string]any{"data": map[string]any{"schedule": scheduleBody(*spec), "next_runs": runs}, "request_id": ""})
		if merr != nil {
			return localErr(ExitInternal, "internal_error", merr.Error())
		}
		fmt.Fprintln(a.out(), string(b))
		return nil
	}
	fmt.Fprintf(a.out(), "%s\n", a.style.bold("Schedule")+"  "+spec.Describe())
	printKV(a.out(), a.style, []kv{{"next", human[0]}, {"then", human[1]}, {"then", human[2]}})
	fmt.Fprintf(a.errw(), "%s dry run: nothing was saved; drop --dry-run to enable it\n", a.style.dim("hint:"))
	return nil
}

// buildSchedule parses exactly one preset. It validates locally so a typo is
// reported before a request is sent.
func buildSchedule(mode, daily, weekly, every, timezone string) (*schedule.Spec, error) {
	if mode != "" && mode != "scheduled" && mode != "realtime" {
		return nil, usage("--mode must be scheduled or realtime")
	}
	chosen := 0
	for _, v := range []string{daily, weekly, every} {
		if v != "" {
			chosen++
		}
	}
	if mode == "realtime" {
		if chosen != 0 {
			return nil, usage("--mode realtime takes no --daily, --weekly or --every preset")
		}
		return nil, nil
	}
	if chosen != 1 {
		return nil, usage("choose one preset: --daily HH:MM, --weekly DAYS@HH:MM or --every 30m")
	}
	spec := &schedule.Spec{Kind: schedule.KindDaily, Timezone: timezone, LocalTime: daily}
	switch {
	case weekly != "":
		days, clock, ok := strings.Cut(weekly, "@")
		if !ok || strings.Contains(clock, "@") {
			return nil, usage("--weekly must look like mon,wed@22:00")
		}
		weekdays, err := parseWeekdays(days)
		if err != nil {
			return nil, err
		}
		spec = &schedule.Spec{Kind: schedule.KindWeekly, Timezone: timezone, LocalTime: clock, Weekdays: weekdays}
	case every != "":
		d, err := time.ParseDuration(every)
		if err != nil || d%time.Minute != 0 {
			return nil, usage("--every needs a whole number of minutes, e.g. 30m or 6h")
		}
		spec = &schedule.Spec{Kind: schedule.KindInterval, IntervalMinutes: int(d.Minutes()), AnchorAt: time.Now().UTC()}
	}
	if err := spec.Validate(); err != nil {
		return nil, usage("%s", err.Error())
	}
	return spec, nil
}

func parseWeekdays(v string) ([]int, error) {
	names := map[string]int{"mon": 1, "tue": 2, "wed": 3, "thu": 4, "fri": 5, "sat": 6, "sun": 7}
	seen := map[int]bool{}
	var out []int
	for _, part := range strings.Split(strings.ToLower(v), ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		day, ok := names[part]
		if !ok {
			n, err := strconv.Atoi(part)
			if err != nil || n < 1 || n > 7 {
				return nil, usage("--weekly days must be mon..sun or 1..7")
			}
			day = n
		}
		if seen[day] {
			return nil, usage("--weekly lists a day twice")
		}
		seen[day] = true
		out = append(out, day)
	}
	if len(out) == 0 {
		return nil, usage("--weekly needs at least one day")
	}
	return out, nil
}

// scheduleBody mirrors the ScheduleSpec oneOf: only the chosen kind's fields.
func scheduleBody(sp schedule.Spec) map[string]any {
	switch sp.Kind {
	case schedule.KindInterval:
		return map[string]any{"kind": sp.Kind, "interval_minutes": sp.IntervalMinutes, "anchor_at": sp.AnchorAt.UTC().Format(time.RFC3339)}
	case schedule.KindDaily:
		return map[string]any{"kind": sp.Kind, "timezone": sp.Timezone, "local_time": sp.LocalTime}
	case schedule.KindWeekly:
		return map[string]any{"kind": sp.Kind, "timezone": sp.Timezone, "local_time": sp.LocalTime, "weekdays": sp.Weekdays}
	}
	return map[string]any{"kind": sp.Kind}
}

func (a *App) syncStatusCmd() *cobra.Command {
	return &cobra.Command{Use: "status COLLECTION", Short: "Show the sync state of a copied source", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, r, err := a.fetchCollection(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			a.emit(r, "", func() { a.printSync(c) })
			return nil
		}}
}

func (a *App) syncRunCmd() *cobra.Command {
	return &cobra.Command{Use: "run COLLECTION", Short: "Run a delta check now (coalesces with an active run)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := a.runSync(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			var job jobView
			if err = r.Decode(&job); err != nil {
				return err
			}
			a.emit(r, job.ID, func() {
				fmt.Fprintf(a.out(), "%s %s\n", a.style.stateColor(job.State), job.ID)
				printKV(a.out(), a.style, []kv{{"source", Sanitize(job.SourceName)}, {"target", Sanitize(job.RootName)}})
			})
			return nil
		}}
}

func (a *App) syncActionCmd(action string) *cobra.Command {
	return &cobra.Command{Use: action + " COLLECTION", Short: strings.ToUpper(action[:1]) + action[1:] + " the sync of a copied source", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := a.syncActionRequest(cmd.Context(), args[0], action)
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

// enableSync writes a subscription through the revision guard.
func (a *App) enableSync(ctx context.Context, id, mode, policy string, spec *schedule.Spec) (*Response, error) {
	return a.revisionGuard(ctx, id, func(c collectionView) (*Response, error) {
		body := map[string]any{
			"mode":                           mode,
			"content_policy":                 policy,
			"expected_collection_revision":   c.Revision,
			"expected_subscription_revision": subscriptionRevision(c.Sync),
		}
		if spec != nil {
			body["schedule"] = scheduleBody(*spec)
		}
		return a.call(ctx, "PUT", "/v1/collections/"+url.PathEscape(c.ID)+"/sync", nil, body, NewKey())
	})
}

// runSync admits a delta run now.
func (a *App) runSync(ctx context.Context, id string) (*Response, error) {
	return a.revisionGuard(ctx, id, func(c collectionView) (*Response, error) {
		return a.call(ctx, "POST", "/v1/collections/"+url.PathEscape(c.ID)+"/runs", nil,
			map[string]any{"expected_revision": c.Revision}, NewKey())
	})
}

// syncActionRequest pauses, resumes or stops the subscription.
func (a *App) syncActionRequest(ctx context.Context, id, action string) (*Response, error) {
	return a.revisionGuard(ctx, id, func(c collectionView) (*Response, error) {
		if c.Sync == nil {
			return nil, localErr(ExitConflict, "sync_off", "Sync is off for this collection; run `502drive sync enable "+c.ID+" ...` first.")
		}
		return a.call(ctx, "POST", "/v1/collections/"+url.PathEscape(c.ID)+"/sync/"+action, nil,
			map[string]any{"expected_revision": c.Sync.Revision}, NewKey())
	})
}

// revisionGuard fetches the collection, runs the revision-guarded mutation and
// retries once on a stale revision. A stale request was never applied, so a
// fresh idempotency key is correct.
func (a *App) revisionGuard(ctx context.Context, id string, mutate func(collectionView) (*Response, error)) (*Response, error) {
	var lastErr error
	for attempt := range 2 {
		c, _, err := a.fetchCollection(ctx, id)
		if err != nil {
			return nil, err
		}
		r, err := mutate(c)
		if err == nil {
			return r, nil
		}
		lastErr = err
		var ae *APIError
		if !errors.As(err, &ae) || ae.Code != "stale_revision" || attempt > 0 {
			return nil, err
		}
	}
	return nil, lastErr
}

func (a *App) fetchCollection(ctx context.Context, id string) (collectionView, *Response, error) {
	var c collectionView
	r, err := a.call(ctx, "GET", "/v1/collections/"+url.PathEscape(id), nil, nil, "")
	if err != nil {
		return c, nil, err
	}
	return c, r, r.Decode(&c)
}

func subscriptionRevision(v *syncView) any {
	if v == nil {
		return nil
	}
	return v.Revision
}

func (a *App) printSync(c collectionView) {
	rows := []kv{{"collection", c.ID}, {"source", Sanitize(c.SourceName)}, {"state", a.style.stateColor(c.State)}}
	if c.Sync == nil {
		rows = append(rows, kv{"sync", "off"})
		printKV(a.out(), a.style, rows)
		fmt.Fprintf(a.errw(), "%s 502drive sync enable %s --daily 22:00\n", a.style.dim("hint:"), c.ID)
		return
	}
	rows = append(rows, kv{"sync", a.style.stateColor(c.Sync.State)})
	rows = append(rows, kv{"mode", c.Sync.Mode}, kv{"schedule", c.Sync.Schedule.Describe()})
	rows = append(rows, kv{"policy", c.Sync.ContentPolicy + " (" + c.Sync.DeletionPolicy + ")"})
	if c.Sync.LastAttemptAt != nil {
		rows = append(rows, kv{"last attempt", *c.Sync.LastAttemptAt})
	}
	if c.Sync.LastSuccessAt != nil {
		rows = append(rows, kv{"last success", *c.Sync.LastSuccessAt})
	}
	if c.Sync.NextRunAt != nil {
		rows = append(rows, kv{"next run", *c.Sync.NextRunAt})
	}
	if c.Sync.ActiveJobID != nil {
		rows = append(rows, kv{"active job", *c.Sync.ActiveJobID})
	}
	if c.Sync.LastError != nil {
		rows = append(rows, kv{"problem", a.style.warn(Sanitize(c.Sync.LastError.Message)) + " (" + c.Sync.LastError.Code + ")"})
	}
	printKV(a.out(), a.style, rows)
}

func (a *App) printSyncView(v syncView) {
	rows := []kv{{"sync", a.style.stateColor(v.State)}, {"mode", v.Mode}, {"schedule", v.Schedule.Describe()},
		{"policy", v.ContentPolicy + " (" + v.DeletionPolicy + ")"}}
	if v.NextRunAt != nil {
		rows = append(rows, kv{"next run", *v.NextRunAt})
	}
	if v.ActiveJobID != nil {
		rows = append(rows, kv{"active job", *v.ActiveJobID})
	}
	printKV(a.out(), a.style, rows)
}
