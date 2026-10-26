package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/GiaHung07/502Drive/internal/i18n"
	"github.com/GiaHung07/502Drive/internal/schedule"
	"github.com/GiaHung07/502Drive/internal/tui"
)

var errBack = errors.New("back")

// prompter is the only thing the menu needs from a terminal. The line-based
// implementation also serves --plain, screen readers and tests.
type prompter interface {
	Select(title string, options []string) (int, error)
	Input(title string) (string, error)
}

type linePrompter struct {
	in   *bufio.Reader
	out  io.Writer
	lang string
}

func (p *linePrompter) readLine() (string, error) {
	s, err := p.in.ReadString('\n')
	if err != nil && s == "" {
		return "", errBack // EOF behaves like "back"
	}
	return strings.TrimSpace(s), nil
}

func (p *linePrompter) Select(title string, options []string) (int, error) {
	for {
		fmt.Fprintf(p.out, "\n%s\n", title)
		for i, o := range options {
			fmt.Fprintf(p.out, "  %d  %s\n", i+1, o)
		}
		fmt.Fprintf(p.out, "  0  %s\n%s: ", i18n.T(p.lang, i18n.MenuBack), i18n.T(p.lang, i18n.Choice))
		s, err := p.readLine()
		if err != nil {
			return 0, err
		}
		if s == "" || s == "0" {
			return 0, errBack
		}
		n, err := strconv.Atoi(s)
		if err == nil && n >= 1 && n <= len(options) {
			return n - 1, nil
		}
		fmt.Fprintf(p.out, i18n.T(p.lang, i18n.InvalidPick)+"\n", len(options))
	}
}

func (p *linePrompter) Input(title string) (string, error) {
	fmt.Fprintf(p.out, "\n%s (empty to go back): ", title)
	s, err := p.readLine()
	if err != nil || s == "" {
		return "", errBack
	}
	return s, nil
}

type huhPrompter struct {
	theme *huh.Theme
	out   io.Writer
	in    io.Reader
}

func (p *huhPrompter) Select(title string, options []string) (int, error) {
	opts := make([]huh.Option[int], len(options))
	for i, o := range options {
		opts[i] = huh.NewOption(o, i)
	}
	v := 0
	err := huh.NewForm(huh.NewGroup(huh.NewSelect[int]().Title(title).Options(opts...).Value(&v))).WithTheme(p.theme).WithOutput(p.out).WithInput(p.in).Run()
	if err != nil {
		return 0, errBack
	}
	return v, nil
}

func (p *huhPrompter) Input(title string) (string, error) {
	v := ""
	err := huh.NewForm(huh.NewGroup(huh.NewInput().Title(title).Value(&v))).WithTheme(p.theme).WithOutput(p.out).WithInput(p.in).Run()
	if err != nil || strings.TrimSpace(v) == "" {
		return "", errBack
	}
	return strings.TrimSpace(v), nil
}

func (a *App) menuCmd() *cobra.Command {
	return &cobra.Command{Use: "menu", Short: "Guided menu (needs a terminal)", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		return a.openUI(cmd.Context())
	}}
}

// openUI opens the full-screen TUI, or the line menu for plain terminals.
func (a *App) openUI(ctx context.Context) error {
	if !a.io.StdinTTY || !a.io.StdoutTTY || a.g.json || a.g.noInput {
		return usage("menu needs an interactive terminal; use subcommands in scripts (try `502drive --help`)")
	}
	if a.useLineMenu() {
		return a.runLineMenu(ctx)
	}
	return tui.Run(ctx, a.tuiActions(), tui.Options{
		NoMotion: a.g.noMotion,
		ASCII:    a.g.ascii,
		Color:    a.style.Color,
		Width:    a.io.Width,
		Version:  Version,
		Profile:  a.style.Profile,
		Dark:     a.style.Dark,
	})
}

// useLineMenu keeps the line menu for plain output, dumb terminals and NO_COLOR;
// --no-motion keeps the full-screen TUI but draws everything still, as the CLI contract promises.
func (a *App) useLineMenu() bool {
	return a.g.plain || a.env("TERM") == "dumb" || a.env("NO_COLOR") != ""
}

// runLineMenu is the fallback menu: same API calls, keyed by number.
func (a *App) runLineMenu(ctx context.Context) error {
	lang := a.language(ctx)
	var p prompter
	if a.g.plain || a.env("TERM") == "dumb" {
		p = &linePrompter{in: bufio.NewReader(a.io.In), out: a.out(), lang: lang}
	} else {
		th := huh.ThemeBase()
		if a.style.Color {
			th = huh.ThemeCharm()
		}
		p = &huhPrompter{theme: th, out: a.out(), in: a.io.In}
	}
	return a.runMenu(ctx, p)
}

// runMenu drives the same API calls as the subcommands; it owns no state.
func (a *App) runMenu(ctx context.Context, p prompter) error {
	first := true
	for {
		header, mood := a.homeHeader(ctx)
		a.drawHome(header, mood, first)
		first = false
		i, err := p.Select(a.label(i18n.MenuTitle), []string{
			a.label(i18n.MenuCopy), a.label(i18n.MenuJobs), a.label(i18n.MenuLibrary),
			a.label(i18n.MenuDest), a.label(i18n.MenuAccount), a.label(i18n.MenuSettings)})
		if err != nil {
			return nil
		}
		var e error
		switch i {
		case 0:
			e = a.menuCopy(ctx, p)
		case 1:
			e = a.menuJobs(ctx, p)
		case 2:
			e = a.menuLibrary(ctx, p)
		case 3:
			e = a.menuDestination(ctx, p)
		case 4:
			e = a.menuAccount(ctx, p)
		case 5:
			e = a.menuSettings(ctx, p)
		}
		if e != nil && !errors.Is(e, errBack) {
			if ctx.Err() != nil {
				return nil
			}
			var ae *APIError
			if errors.As(e, &ae) {
				fmt.Fprintf(a.out(), "\n%s %s\n", a.style.bad("!"), Sanitize(ae.Message))
				if h := hintFor(ae); h != "" {
					fmt.Fprintf(a.out(), "  %s\n", h)
				}
			} else {
				fmt.Fprintf(a.out(), "\n%s %v\n", a.style.bad("!"), e)
			}
		}
	}
}

func (a *App) homeHeader(ctx context.Context) ([]string, string) {
	r, err := a.call(ctx, "GET", "/v1/status", nil, nil, "")
	if err != nil {
		return []string{"daemon unreachable", "start it: 502drive serve"}, "error"
	}
	var st struct {
		Ready   bool `json:"ready"`
		Running int  `json:"running_jobs"`
		Queued  int  `json:"queued_jobs"`
		Blocked int  `json:"blocked_jobs"`
		Account struct {
			Status string `json:"status"`
		} `json:"account"`
	}
	if r.Decode(&st) != nil {
		return []string{"unreadable status"}, "error"
	}
	mood := "idle"
	switch {
	case st.Blocked > 0 || st.Account.Status != "connected":
		mood = "paused"
	case st.Running > 0:
		mood = "busy"
	}
	return []string{"Google: " + st.Account.Status, fmt.Sprintf("%d running, %d queued, %d blocked", st.Running, st.Queued, st.Blocked)}, mood
}

func (a *App) drawHome(header []string, mood string, first bool) {
	art := a.style.Mascot(mood)
	title := a.style.accent("502Drive") + "  Copy once. Keep in sync."
	if len(art) == 0 {
		fmt.Fprintf(a.out(), "\n502Drive\n")
		for _, h := range header {
			fmt.Fprintf(a.out(), "  %s\n", h)
		}
		return
	}
	lines := []string{art[0] + "   " + title, art[1] + "   " + header[0], art[2], art[3]}
	if len(header) > 1 {
		lines[2] = art[2] + "   " + header[1]
	}
	for _, l := range lines {
		fmt.Fprintln(a.out(), l)
		if first && a.style.Motion {
			time.Sleep(50 * time.Millisecond) // one 200 ms reveal; never delays data or actions
		}
	}
}

func (a *App) menuCopy(ctx context.Context, p prompter) error {
	link, err := p.Input("Paste a Google Drive link or ID")
	if err != nil {
		return err
	}
	job, _, err := a.submitCopy(ctx, link, "", "", "")
	if err != nil {
		return err
	}
	fmt.Fprintf(a.out(), "\nAccepted: %s → %s (job %s)\n", Sanitize(job.SourceName), Sanitize(job.RootName), job.ID)
	i, err := p.Select("Next", []string{a.label(i18n.Follow), a.label(i18n.BackToMenu)})
	if err != nil || i == 1 {
		return err
	}
	return a.waitJob(ctx, job.ID, 0)
}

func (a *App) menuJobs(ctx context.Context, p prompter) error {
	for {
		r, err := a.call(ctx, "GET", "/v1/jobs", url.Values{"limit": {"10"}}, nil, "")
		if err != nil {
			return err
		}
		var page struct {
			Items []jobView `json:"items"`
		}
		if err = r.Decode(&page); err != nil {
			return err
		}
		if len(page.Items) == 0 {
			fmt.Fprintln(a.out(), "\nNo jobs yet.")
			return nil
		}
		labels := make([]string, len(page.Items))
		for i, j := range page.Items {
			labels[i] = fmt.Sprintf("%-10s %s  %d/%s", strings.ToUpper(j.State), Truncate(Sanitize(j.SourceName), max(10, a.style.Width-26), a.style.ASCII), j.Counts.Completed, totalText(j.Counts))
		}
		i, err := p.Select(a.label(i18n.JobsLatest), labels)
		if err != nil {
			return err
		}
		if err = a.menuJob(ctx, p, page.Items[i].ID); err != nil && !errors.Is(err, errBack) {
			return err
		}
	}
}

func (a *App) menuJob(ctx context.Context, p prompter, id string) error {
	for {
		j, err := a.fetchJob(ctx, id)
		if err != nil {
			return err
		}
		fmt.Fprintln(a.out())
		a.printJob(j)
		var acts []string
		var names []string
		add := func(n, label string) { names = append(names, n); acts = append(acts, label) }
		switch j.State {
		case "queued", "running":
			add("pause", "Pause")
			add("cancel", "Cancel (keeps what was copied)")
		case "paused":
			add("resume", "Resume")
			add("cancel", "Cancel (keeps what was copied)")
		case "blocked":
			add("retry", "Retry (re-check blocked items)")
			add("resume", "Resume")
			add("cancel", "Cancel")
		case "partial", "failed":
			add("retry", "Retry failed items")
		}
		add("refresh", "Refresh")
		i, err := p.Select(a.label(i18n.JobActions), acts)
		if err != nil {
			return err
		}
		if names[i] == "refresh" {
			continue
		}
		r, err := a.call(ctx, "POST", "/v1/jobs/"+url.PathEscape(id)+"/"+names[i], nil, map[string]any{"expected_revision": j.Revision}, NewKey())
		if err != nil {
			return err
		}
		_ = r
		fmt.Fprintf(a.out(), "%s requested.\n", names[i])
	}
}

func (a *App) menuLibrary(ctx context.Context, p prompter) error {
	r, err := a.call(ctx, "GET", "/v1/collections", nil, nil, "")
	if err != nil {
		return err
	}
	var page struct {
		Items []collectionView `json:"items"`
	}
	if err = r.Decode(&page); err != nil {
		return err
	}
	if len(page.Items) == 0 {
		fmt.Fprintln(a.out(), "\nNothing copied yet.")
		return nil
	}
	labels := make([]string, len(page.Items))
	for i, c := range page.Items {
		labels[i] = fmt.Sprintf("%-12s %s", strings.ToUpper(c.State), Truncate(Sanitize(c.SourceName), max(10, a.style.Width-18), a.style.ASCII))
	}
	i, err := p.Select(a.label(i18n.CopiedSrcs), labels)
	if err != nil {
		return err
	}
	return a.menuCollection(ctx, p, page.Items[i].ID)
}

// language resolves the UI language from preferences once per session.
func (a *App) language(ctx context.Context) string {
	if a.uiLang != "" {
		return a.uiLang
	}
	lang := "vi"
	if r, err := a.call(ctx, "GET", "/v1/preferences", nil, nil, ""); err == nil {
		var prefs struct {
			Language string `json:"language"`
		}
		if r.Decode(&prefs) == nil && prefs.Language != "" {
			lang = prefs.Language
		}
	}
	a.uiLang = lang
	return lang
}

// label translates one menu label in the session language.
func (a *App) label(key i18n.Key) string { return i18n.T(a.uiLang, key) }

func (a *App) menuDestination(ctx context.Context, p prompter) error {
	for {
		r, err := a.call(ctx, "GET", "/v1/destinations", nil, nil, "")
		if err != nil {
			return err
		}
		var page struct {
			Items []destView `json:"items"`
		}
		if err = r.Decode(&page); err != nil {
			return err
		}
		labels := []string{"Add a folder as destination"}
		for _, d := range page.Items {
			mark := ""
			if d.Default {
				mark = " (default)"
			}
			labels = append(labels, "Use: "+Sanitize(d.Name)+mark)
		}
		i, err := p.Select(a.label(i18n.Destination), labels)
		if err != nil {
			return err
		}
		if i == 0 {
			link, err := p.Input("Paste the destination folder link")
			if err != nil {
				continue
			}
			ar, err := a.call(ctx, "POST", "/v1/destinations", nil, map[string]any{"source": link}, NewKey())
			if err != nil {
				return err
			}
			var d destView
			if err = ar.Decode(&d); err != nil {
				return err
			}
			if _, err = a.call(ctx, "PUT", "/v1/destinations/default", nil, map[string]any{"destination_id": d.ID, "expected_revision": d.Revision}, NewKey()); err != nil {
				return err
			}
			fmt.Fprintf(a.out(), "Saved and set as default: %s\n", Sanitize(d.Name))
			continue
		}
		d := page.Items[i-1]
		if _, err = a.call(ctx, "PUT", "/v1/destinations/default", nil, map[string]any{"destination_id": d.ID, "expected_revision": d.Revision}, NewKey()); err != nil {
			return err
		}
		fmt.Fprintf(a.out(), "Default destination: %s\n", Sanitize(d.Name))
	}
}

func (a *App) menuAccount(ctx context.Context, p prompter) error {
	for {
		r, err := a.call(ctx, "GET", "/v1/account", nil, nil, "")
		if err != nil {
			return err
		}
		fmt.Fprintln(a.out())
		a.printAccount(r)
		i, err := p.Select(a.label(i18n.MenuAccount), []string{a.label(i18n.RunChecks), a.label(i18n.LoginHint), a.label(i18n.Revoke)})
		if err != nil {
			return err
		}
		switch i {
		case 0:
			dr, err := a.call(ctx, "GET", "/v1/doctor", url.Values{"online": {"true"}}, nil, "")
			if err != nil {
				return err
			}
			var d struct {
				Checks []struct{ Name, Status, Message string } `json:"checks"`
			}
			dr.Decode(&d)
			for _, c := range d.Checks {
				fmt.Fprintf(a.out(), "%-8s %-14s %s\n", a.style.stateColor(c.Status), c.Name, Sanitize(c.Message))
			}
		case 1:
			fmt.Fprintln(a.out(), "\nRun `502drive account login` in a terminal to finish connecting (it waits for the browser).")
		case 2:
			c, err := p.Select("Remove access to your Google Drive from 502drive?", []string{"Yes, disconnect"})
			if err != nil || c != 0 {
				continue
			}
			if _, err = a.call(ctx, "POST", "/v1/account/revoke", nil, map[string]any{}, NewKey()); err != nil {
				return err
			}
			fmt.Fprintln(a.out(), "Disconnected.")
		}
	}
}

func (a *App) menuSettings(ctx context.Context, p prompter) error {
	r, err := a.call(ctx, "GET", "/v1/preferences", nil, nil, "")
	if err != nil {
		return err
	}
	var pr struct {
		Language string `json:"language"`
	}
	r.Decode(&pr)
	i, err := p.Select(i18n.Tf(a.uiLang, i18n.SettingsHead, pr.Language), []string{a.label(i18n.LangVI), a.label(i18n.LangEN)})
	if err != nil {
		return err
	}
	lang := []string{"vi", "en"}[i]
	if _, err = a.call(ctx, "PATCH", "/v1/preferences", nil, map[string]any{"language": lang}, NewKey()); err != nil {
		return err
	}
	fmt.Fprintf(a.out(), "Language set to %s.\n", lang)
	return nil
}

// menuCollection shows one copied source and drives its sync through the same
// API the subcommands use.
func (a *App) menuCollection(ctx context.Context, p prompter, id string) error {
	for {
		c, _, err := a.fetchCollection(ctx, id)
		if err != nil {
			return err
		}
		fmt.Fprintln(a.out())
		a.printSync(c)
		actions, names := syncMenuActions(c.Sync, a.uiLang)
		i, err := p.Select(a.label(i18n.CopiedSrcs), actions)
		if err != nil {
			return err
		}
		if err = a.runSyncMenuAction(ctx, id, names[i]); err != nil {
			return err
		}
	}
}

// syncMenuActions lists only the actions that can succeed in the current
// state, so no button is offered just to fail after the click.
func syncMenuActions(sync *syncView, lang string) (labels, names []string) {
	add := func(name, label string) {
		names = append(names, name)
		labels = append(labels, label)
	}
	add("run", i18n.T(lang, i18n.RunNow))
	add("daily", i18n.T(lang, i18n.PresetDaily))
	add("6h", i18n.T(lang, i18n.PresetSixH))
	add("1h", i18n.T(lang, i18n.PresetHourly))
	add("realtime", i18n.T(lang, i18n.PresetReal))
	if sync != nil {
		switch sync.State {
		case "active", "bootstrapping", "needs_reconcile", "stopping":
			add("pause", i18n.T(lang, i18n.PauseSync))
		}
		if sync.State == "paused" || sync.State == "stopped" || sync.State == "error" {
			add("resume", i18n.T(lang, i18n.ResumeSync))
		}
		if sync.State != "stopped" {
			add("stop", i18n.T(lang, i18n.StopSync))
		}
	}
	add("back", i18n.T(lang, i18n.Back))
	return labels, names
}

func (a *App) runSyncMenuAction(ctx context.Context, id, name string) error {
	switch name {
	case "back":
		return errBack
	case "run":
		r, err := a.runSync(ctx, id)
		if err != nil {
			return err
		}
		var j jobView
		if err = r.Decode(&j); err != nil {
			return err
		}
		fmt.Fprintf(a.out(), "\nQueued sync check %s (%s)\n", j.ID, j.State)
	case "pause", "resume", "stop":
		r, err := a.syncActionRequest(ctx, id, name)
		if err != nil {
			return err
		}
		var v syncView
		if err = r.Decode(&v); err != nil {
			return err
		}
		fmt.Fprintf(a.out(), "\nSync %s: %s\n", name, v.State)
	case "daily", "6h", "1h", "realtime":
		mode, spec := "scheduled", presetSchedule(name, a.preferredTimezone(ctx))
		if name == "realtime" {
			mode = "realtime"
		}
		r, err := a.enableSync(ctx, id, mode, "versioned_copy", spec)
		if err != nil {
			return err
		}
		var v syncView
		if err = r.Decode(&v); err != nil {
			return err
		}
		fmt.Fprintf(a.out(), "\nSync %s: %s, %s\n", mode, v.State, v.Schedule.Describe())
	}
	return nil
}

func presetSchedule(name, timezone string) *schedule.Spec {
	if timezone == "" {
		timezone = defaultSyncTimezone
	}
	switch name {
	case "daily":
		return &schedule.Spec{Kind: schedule.KindDaily, Timezone: timezone, LocalTime: "22:00"}
	case "6h":
		return &schedule.Spec{Kind: schedule.KindInterval, IntervalMinutes: 360, AnchorAt: time.Now().UTC()}
	case "1h":
		return &schedule.Spec{Kind: schedule.KindInterval, IntervalMinutes: 60, AnchorAt: time.Now().UTC()}
	}
	return nil
}
