// Package cli is the 502drive command-line client. Every business command
// goes through the daemon's HTTP API; nothing here opens the database.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/itsgiahung/502Drive/internal/config"
	"github.com/itsgiahung/502Drive/internal/daemon"
)

// Version is set at build time.
var Version = "dev"

// IO bundles the process streams so commands are testable.
type IO struct {
	In        io.Reader
	Out, Err  io.Writer
	Getenv    func(string) string
	StdinTTY  bool
	StdoutTTY bool
	StderrTTY bool
	Width     int
}

type global struct {
	configPath, endpoint, tokenFile string
	json, quiet, noInput, ascii     bool
	plain, noMotion                 bool
	color                           string
	timeout                         time.Duration
}

type App struct {
	io     IO
	g      global
	style  Style
	client *Client
	cfg    config.Config
	cfgSet bool
	// uiLang is the resolved language for interactive menus (vi/en).
	uiLang string
	// slow caches the TUI's rarely changing sections.
	slow slowCache
}

func (a *App) out() io.Writer  { return a.io.Out }
func (a *App) errw() io.Writer { return a.io.Err }

func (a *App) env(key string) string {
	if a.io.Getenv == nil {
		return ""
	}
	return a.io.Getenv(key)
}

// Execute runs the CLI and returns the process exit code.
func Execute(ctx context.Context, args []string, io IO) int {
	a := &App{io: io}
	root := a.rootCommand()
	root.SetArgs(args)
	root.SetOut(io.Out)
	root.SetErr(io.Err)
	err := root.ExecuteContext(ctx)
	if err == nil {
		return ExitOK
	}
	return a.fail(err)
}

func (a *App) fail(err error) int {
	var ae *APIError
	if !errors.As(err, &ae) {
		ae = localErr(ExitUsage, "invalid_request", err.Error())
	}
	if a.g.json {
		var ra *int
		if ae.RetryAfter > 0 {
			ra = &ae.RetryAfter
		}
		b, _ := json.Marshal(map[string]any{"error": map[string]any{"code": ae.Code, "message": ae.Message, "retryable": ae.Retryable, "retry_after_seconds": ra}, "request_id": ae.RequestID})
		fmt.Fprintln(a.errw(), string(b))
		return ae.Exit
	}
	msg := Sanitize(ae.Message)
	fmt.Fprintf(a.errw(), "%s %s\n", a.style.bad("error:"), msg)
	if h := hintFor(ae); h != "" {
		fmt.Fprintf(a.errw(), "%s %s\n", a.style.dim("hint:"), h)
	}
	if ae.RetryAfter > 0 {
		fmt.Fprintf(a.errw(), "%s retry in %ds with the same idempotency key\n", a.style.dim("hint:"), ae.RetryAfter)
	}
	return ae.Exit
}

func hintFor(e *APIError) string {
	switch e.Code {
	case "account_reconnect_required":
		return "run `502drive account login`"
	case "destination_required":
		return "run `502drive destinations add FOLDER_LINK` then `502drive destinations default ID`"
	case "plan_expired", "plan_changed":
		return "run the command again to inspect the source afresh"
	case "daemon_unavailable":
		return "start the daemon with `502drive serve`"
	case "oauth_not_configured":
		return "run `502drive setup --oauth-client FILE` (see docs/oauth-setup.md)"
	}
	return ""
}

func usage(format string, args ...any) error {
	return localErr(ExitUsage, "invalid_request", fmt.Sprintf(format, args...))
}

// interactive reports a real terminal on both ends with prompting allowed.
func (a *App) interactive() bool {
	return a.io.StdinTTY && a.io.StdoutTTY && !a.g.json && !a.g.quiet && !a.g.noInput && !a.g.plain && a.env("TERM") != "dumb"
}

func (a *App) rootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "502drive",
		Short:         "Copy Google Drive links server-side and keep them in sync",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       Version,
		RunE: func(cmd *cobra.Command, args []string) error {
			// In a real terminal a bare `502drive` brings everything up and opens
			// the UI; scripts and pipes keep getting the short help below.
			if a.interactive() {
				return a.launch(cmd.Context())
			}
			fmt.Fprintln(a.out(), "502drive copies Google Drive folders and files server-side and can keep them in sync.")
			fmt.Fprintln(a.out(), "\nExamples:")
			fmt.Fprintln(a.out(), "  502drive copy 'https://drive.google.com/drive/folders/ID' --wait")
			fmt.Fprintln(a.out(), "  502drive jobs list")
			fmt.Fprintln(a.out(), "  502drive status")
			fmt.Fprintln(a.out(), "\nUse `502drive menu` for guided mode, `502drive --help` for all commands.")
			return nil
		},
	}
	root.SetVersionTemplate("502drive {{.Version}}\n")
	f := root.PersistentFlags()
	f.StringVar(&a.g.configPath, "config", "", "config file (env DRIVE502_CONFIG)")
	f.StringVar(&a.g.endpoint, "endpoint", "", "daemon URL (env DRIVE502_ENDPOINT)")
	f.StringVar(&a.g.tokenFile, "token-file", "", "file holding the API token (env DRIVE502_TOKEN_FILE)")
	f.BoolVar(&a.g.json, "json", false, "machine-readable JSON output")
	f.BoolVar(&a.g.quiet, "quiet", false, "print only IDs/results")
	f.BoolVar(&a.g.noInput, "no-input", false, "never prompt")
	f.DurationVar(&a.g.timeout, "timeout", 30*time.Second, "per-request timeout")
	f.BoolVar(&a.g.ascii, "ascii", false, "7-bit ASCII chrome")
	f.BoolVar(&a.g.plain, "plain", false, "no colour, mascot or cursor control; line input")
	f.StringVar(&a.g.color, "color", "auto", "colour: auto|always|never")
	f.BoolVar(&a.g.noMotion, "no-motion", false, "disable animations")
	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error { return a.prepare(cmd) }
	root.SetFlagErrorFunc(func(c *cobra.Command, err error) error { return usage("%s", err.Error()) })

	root.AddCommand(a.versionCmd(), a.setupCmd(), a.serveCmd(), a.statusCmd(), a.inspectCmd(), a.copyCmd(), a.jobsCmd(),
		a.collectionsCmd(), a.syncCmd(), a.destinationsCmd(), a.accountCmd(), a.doctorCmd(), a.menuCmd(), a.auditCmd(),
		a.lsCmd(), a.infoCmd(), a.mkdirCmd(), a.healthcheckCmd(),
		a.trayCmd(), a.upCmd(), a.stopCmd(), a.logsCmd(), a.autostartCmd(), a.updateCmd(), a.assetsCmd())
	root.AddCommand(a.maintenanceCmd())
	root.AddCommand(a.maintenanceCmds()...)
	return root
}

// prepare resolves style, config and the API client. It never contacts the daemon.
func (a *App) prepare(cmd *cobra.Command) error {
	switch a.g.color {
	case "auto", "always", "never":
	default:
		return usage("--color must be auto, always or never")
	}
	width := a.io.Width
	if width == 0 {
		width = 80
	}
	a.style = ResolveStyle(StyleInput{ColorMode: a.g.color, ASCII: a.g.ascii, Plain: a.g.plain, JSON: a.g.json, Quiet: a.g.quiet, NoMotion: a.g.noMotion,
		StdoutTTY: a.io.StdoutTTY, StderrTTY: a.io.StderrTTY, Term: a.env("TERM"), NoColor: a.env("NO_COLOR"), Width: width,
		ColorTerm: a.env("COLORTERM"), ColorFGBG: a.env("COLORFGBG")})
	if a.g.timeout <= 0 {
		return usage("--timeout must be positive")
	}
	return nil
}

func (a *App) configPath() string {
	switch {
	case a.g.configPath != "":
		return a.g.configPath
	case a.env("DRIVE502_CONFIG") != "":
		return a.env("DRIVE502_CONFIG")
	}
	return config.DefaultPath()
}

func (a *App) loadConfig() (config.Config, error) {
	if a.cfgSet {
		return a.cfg, nil
	}
	c, err := config.Load(a.configPath())
	if err != nil {
		return c, localErr(ExitUsage, "config_missing", fmt.Sprintf("%s (config: %s). Run `502drive setup` first.", err.Error(), a.configPath()))
	}
	a.cfg, a.cfgSet = c, true
	return c, nil
}

// api builds the HTTP client: flags > env > config.
func (a *App) api() (*Client, error) {
	if a.client != nil {
		return a.client, nil
	}
	endpoint := firstNonEmpty(a.g.endpoint, a.env("DRIVE502_ENDPOINT"))
	tokenFile := firstNonEmpty(a.g.tokenFile, a.env("DRIVE502_TOKEN_FILE"))
	token := a.env("DRIVE502_TOKEN")
	if endpoint == "" || (tokenFile == "" && token == "") {
		c, err := a.loadConfig()
		if err != nil {
			if endpoint != "" && token != "" {
				goto build
			}
			return nil, err
		}
		if endpoint == "" {
			endpoint = "http://" + c.Listen
		}
		if tokenFile == "" && token == "" {
			tokenFile = c.TokenFile
		}
	}
build:
	if token == "" || a.g.tokenFile != "" {
		b, err := config.ReadPrivate(tokenFile)
		if err != nil {
			return nil, localErr(ExitUsage, "token_unavailable", "Cannot read the API token file: "+err.Error())
		}
		token = strings.TrimSpace(string(b))
	}
	u, err := url.Parse(endpoint)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, usage("--endpoint must be an http(s) URL")
	}
	a.client = &Client{Endpoint: endpoint, Token: token, HTTP: &http.Client{Timeout: a.g.timeout}}
	return a.client, nil
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

// call performs a request with the per-request timeout.
func (a *App) call(ctx context.Context, method, path string, q url.Values, body any, key string) (*Response, error) {
	c, err := a.api()
	if err != nil {
		return nil, err
	}
	return c.Do(ctx, method, path, q, body, key)
}

// emit prints the passthrough JSON envelope or runs the human renderer.
func (a *App) emit(r *Response, quiet string, human func()) {
	switch {
	case a.g.json:
		fmt.Fprintln(a.out(), strings.TrimSpace(string(r.Raw)))
	case a.g.quiet:
		if quiet != "" {
			fmt.Fprintln(a.out(), quiet)
		}
	default:
		human()
	}
}

func (a *App) versionCmd() *cobra.Command {
	return &cobra.Command{Use: "version", Short: "Print the version", Args: cobra.NoArgs, RunE: func(*cobra.Command, []string) error {
		if a.g.json {
			b, _ := json.Marshal(map[string]any{"data": map[string]string{"version": Version, "go": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH}, "request_id": ""})
			fmt.Fprintln(a.out(), string(b))
			return nil
		}
		fmt.Fprintf(a.out(), "502drive %s\n", Version)
		return nil
	}}
}

func (a *App) serveCmd() *cobra.Command {
	var maintenance bool
	c := &cobra.Command{Use: "serve", Short: "Run the daemon in the foreground", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := a.loadConfig()
		if err != nil {
			return err
		}
		log := newLogger(a.errw())
		err = daemon.Run(cmd.Context(), cfg, daemon.Options{Version: Version, Log: log, Maintenance: maintenance})
		if err != nil {
			return localErr(ExitInternal, "serve_failed", err.Error())
		}
		return nil
	}}
	c.Flags().BoolVar(&maintenance, "maintenance", false, "do not start workers or accept new work")
	return c
}

// openBrowser launches the system browser without a shell.
func openBrowser(u string) error {
	if !strings.HasPrefix(u, "https://accounts.google.com/") {
		return errors.New("refusing to open an unexpected URL")
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", u)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", u)
	default:
		cmd = exec.Command("xdg-open", u)
	}
	return cmd.Start()
}
