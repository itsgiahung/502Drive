package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/itsgiahung/502Drive/internal/config"
	"github.com/itsgiahung/502Drive/internal/update"
)

// ---- logs ----

const logTailWindow = 256 << 10

func (a *App) logsCmd() *cobra.Command {
	var lines int
	var follow, trayLog bool
	c := &cobra.Command{Use: "logs", Short: "Show the daemon log written by `502drive up` or the bare command", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if lines < 1 || lines > 10000 {
				return usage("--lines must be between 1 and 10000")
			}
			name := "daemon.log"
			if trayLog {
				name = "tray.log"
			}
			path := filepath.Join(a.stateDir(), "logs", name)
			f, err := os.Open(path)
			if err != nil {
				return localErr(ExitNotFound, "log_not_found", "No log yet at "+path+". A daemon started by systemd logs to the journal: journalctl --user -u 502drive")
			}
			defer f.Close()
			tail, err := tailLines(f, lines)
			if err != nil {
				return localErr(ExitInternal, "log_unreadable", err.Error())
			}
			for _, l := range tail {
				fmt.Fprintln(a.out(), Sanitize(l))
			}
			if !follow {
				return nil
			}
			return a.followLog(cmd.Context(), f)
		}}
	c.Flags().IntVarP(&lines, "lines", "n", 50, "number of trailing lines")
	c.Flags().BoolVarP(&follow, "follow", "f", false, "keep printing new lines until interrupted")
	c.Flags().BoolVar(&trayLog, "tray", false, "show the tray log instead of the daemon log")
	return c
}

// tailLines returns the last n lines of r, reading at most logTailWindow bytes.
func tailLines(f *os.File, n int) ([]string, error) {
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	start := max(0, st.Size()-logTailWindow)
	if _, err = f.Seek(start, io.SeekStart); err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	if start > 0 { // drop the partial first line
		if i := bytes.IndexByte(raw, '\n'); i >= 0 {
			raw = raw[i+1:]
		}
	}
	all := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if len(all) == 1 && all[0] == "" {
		return nil, nil
	}
	if len(all) > n {
		all = all[len(all)-n:]
	}
	return all, nil
}

// followLog prints what is appended after the current end; f is already there.
func (a *App) followLog(ctx context.Context, f *os.File) error {
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		return localErr(ExitInternal, "log_unreadable", err.Error())
	}
	rd := bufio.NewReader(f)
	for {
		line, err := rd.ReadString('\n')
		if len(line) > 0 && strings.HasSuffix(line, "\n") {
			fmt.Fprintln(a.out(), Sanitize(strings.TrimRight(line, "\n")))
			continue
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return localErr(ExitInternal, "log_unreadable", err.Error())
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(400 * time.Millisecond):
		}
	}
}

// ---- autostart ----

func (a *App) autostartCmd() *cobra.Command {
	c := &cobra.Command{Use: "autostart", Short: "Start 502Drive (daemon and tray) when you sign in"}
	c.AddCommand(
		&cobra.Command{Use: "enable", Short: "Start 502Drive at sign-in", Args: cobra.NoArgs, RunE: func(*cobra.Command, []string) error {
			return a.autostartSet(true)
		}},
		&cobra.Command{Use: "disable", Short: "Stop starting 502Drive at sign-in", Args: cobra.NoArgs, RunE: func(*cobra.Command, []string) error {
			return a.autostartSet(false)
		}},
		&cobra.Command{Use: "status", Short: "Show whether autostart is on", Args: cobra.NoArgs, RunE: func(*cobra.Command, []string) error {
			on, where := a.autostartState()
			if a.g.json {
				b, _ := json.Marshal(map[string]any{"data": map[string]any{"enabled": on, "location": where}, "request_id": ""})
				fmt.Fprintln(a.out(), string(b))
				return nil
			}
			word := "off"
			if on {
				word = "on"
			}
			fmt.Fprintf(a.out(), "autostart  %s  (%s)\n", word, where)
			return nil
		}},
	)
	return c
}

const winRunKey = `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`

func (a *App) autostartFile() string {
	dir := a.env("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "autostart", "502drive.desktop")
}

// desktopExec quotes a path for the Exec key of a .desktop file.
func desktopExec(path string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "`", "\\`", "$", `\$`)
	return `"` + r.Replace(path) + `"`
}

func autostartEntry(exe string) string {
	return "[Desktop Entry]\nType=Application\nName=502Drive\nComment=Start the 502Drive daemon and tray\n" +
		"Exec=" + desktopExec(exe) + " up --quiet\nIcon=502drive\nTerminal=false\nX-GNOME-Autostart-enabled=true\n"
}

func (a *App) autostartSet(on bool) error {
	exe, err := os.Executable()
	if err != nil {
		return localErr(ExitInternal, "autostart_failed", "Cannot locate the 502drive binary.")
	}
	switch runtime.GOOS {
	case "windows":
		argv := []string{"reg", "add", winRunKey, "/v", "502Drive", "/t", "REG_SZ", "/d", `"` + exe + `" up --quiet`, "/f"}
		if !on {
			argv = []string{"reg", "delete", winRunKey, "/v", "502Drive", "/f"}
		}
		if out, err := exec.Command(argv[0], argv[1:]...).CombinedOutput(); err != nil && on {
			return localErr(ExitInternal, "autostart_failed", "reg failed: "+strings.TrimSpace(string(out)))
		}
	case "darwin":
		return localErr(ExitUsage, "not_supported", "Autostart is not supported on macOS yet; run `502drive serve` under launchd.")
	default:
		path := a.autostartFile()
		if path == "" {
			return localErr(ExitInternal, "autostart_failed", "Cannot find your config directory.")
		}
		if !on {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return localErr(ExitInternal, "autostart_failed", err.Error())
			}
			break
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return localErr(ExitInternal, "autostart_failed", err.Error())
		}
		if err := os.WriteFile(path, []byte(autostartEntry(exe)), 0o600); err != nil {
			return localErr(ExitInternal, "autostart_failed", err.Error())
		}
	}
	on2, where := a.autostartState()
	if !a.g.quiet && !a.g.json {
		fmt.Fprintf(a.out(), "autostart %v (%s)\n", map[bool]string{true: "enabled", false: "disabled"}[on2], where)
	}
	return nil
}

func (a *App) autostartState() (bool, string) {
	switch runtime.GOOS {
	case "windows":
		err := exec.Command("reg", "query", winRunKey, "/v", "502Drive").Run()
		return err == nil, winRunKey
	case "darwin":
		return false, "unsupported on macOS"
	}
	path := a.autostartFile()
	_, err := os.Stat(path)
	return err == nil, path
}

// ---- update ----

func (a *App) updateCmd() *cobra.Command {
	var enable, disable bool
	c := &cobra.Command{Use: "update", Short: "Check for a newer release (never installs anything)", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if enable && disable {
				return usage("choose --enable-check or --disable-check, not both")
			}
			if enable || disable {
				return a.setUpdateCheck(enable)
			}
			rel, err := update.Latest(cmd.Context(), nil, a.env("DRIVE502_UPDATE_API"))
			if errors.Is(err, update.ErrNoRelease) {
				return localErr(ExitNotFound, "no_release", "No published release was found. The repository may be private or have no release yet.")
			}
			if err != nil {
				return localErr(ExitUnavailable, "update_check_failed", err.Error())
			}
			newer := update.Newer(Version, rel.Tag)
			if a.g.json {
				b, _ := json.Marshal(map[string]any{"data": map[string]any{"current": Version, "latest": rel.Tag, "url": rel.URL, "update_available": newer}, "request_id": ""})
				fmt.Fprintln(a.out(), string(b))
				return nil
			}
			if !newer {
				fmt.Fprintf(a.out(), "502drive %s is up to date (latest release %s).\n", Version, rel.Tag)
				return nil
			}
			fmt.Fprintf(a.out(), "A newer release is available: %s (you have %s)\n%s\nDownload it from the release page and verify the checksums; 502drive never updates itself.\n", rel.Tag, Version, rel.URL)
			return nil
		}}
	c.Flags().BoolVar(&enable, "enable-check", false, "let the tray check for a new release once a day")
	c.Flags().BoolVar(&disable, "disable-check", false, "stop the daily tray check")
	return c
}

func (a *App) setUpdateCheck(on bool) error {
	path := a.configPath()
	cfg, err := config.Load(path)
	if err != nil {
		return localErr(ExitUsage, "config_invalid", err.Error())
	}
	cfg.CheckUpdates = on
	if err = config.Save(path, cfg); err != nil {
		return localErr(ExitInternal, "setup_failed", err.Error())
	}
	if !a.g.quiet && !a.g.json {
		fmt.Fprintf(a.out(), "Daily update check in the tray: %v\n", map[bool]string{true: "on", false: "off"}[on])
	}
	return nil
}

// trayUpdateNote is the tray's opt-in check: a one-line note when a newer
// release exists, empty otherwise.
func (a *App) trayUpdateNote(ctx context.Context) (string, error) {
	rel, err := update.Latest(ctx, nil, a.env("DRIVE502_UPDATE_API"))
	if err != nil || !update.Newer(Version, rel.Tag) {
		return "", err
	}
	return "update available: " + rel.Tag, nil
}
