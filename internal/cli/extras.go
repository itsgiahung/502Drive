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

	"github.com/itsgiahung/502Drive/internal/autostart"
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
		a.systemdAutostartCmd(),
	)
	return c
}

// systemdAutostartCmd manages the opt-in systemd --user unit. The XDG desktop
// entry starts the tray+daemon at login; the unit makes systemd restart the
// daemon after a crash and is the only thing doctor checks.
func (a *App) systemdAutostartCmd() *cobra.Command {
	c := &cobra.Command{Use: "systemd", Short: "Manage the systemd --user unit (Linux)", Args: cobra.NoArgs}
	c.AddCommand(
		&cobra.Command{Use: "status", Short: "Show the user unit path and whether it is enabled", Args: cobra.NoArgs, RunE: func(*cobra.Command, []string) error {
			st := autostart.SystemdStateNow()
			if a.g.json {
				b, _ := json.Marshal(map[string]any{"data": st, "request_id": ""})
				fmt.Fprintln(a.out(), string(b))
				return nil
			}
			word := "not installed"
			if st.Installed {
				word = "installed, disabled"
				if st.Enabled {
					word = "enabled"
				}
			}
			fmt.Fprintf(a.out(), "systemd user unit  %s\n", word)
			if st.Unit != "" {
				fmt.Fprintf(a.out(), "unit  %s\n", st.Unit)
			}
			if st.Detail != "" {
				fmt.Fprintf(a.out(), "detail  %s\n", st.Detail)
			}
			return nil
		}},
		&cobra.Command{Use: "install", Short: "Write, enable and reload the user unit (restarts the daemon after a crash)", Args: cobra.NoArgs, RunE: func(*cobra.Command, []string) error {
			cfg, err := a.loadConfig()
			if err != nil {
				return err
			}
			path, err := a.autostartManager().InstallSystemd(cfg.DataDir, filepath.Dir(cfg.TokenFile), true)
			if err != nil {
				return localErr(ExitInternal, "systemd_failed", err.Error())
			}
			if a.g.json {
				b, _ := json.Marshal(map[string]any{"data": map[string]any{"unit": path, "enabled": true}, "request_id": ""})
				fmt.Fprintln(a.out(), string(b))
				return nil
			}
			fmt.Fprintf(a.out(), "systemd user unit enabled: %s\n", path)
			fmt.Fprintln(a.out(), "Start it once with: systemctl --user start 502drive")
			return nil
		}},
		&cobra.Command{Use: "remove", Short: "Disable and remove the user unit", Args: cobra.NoArgs, RunE: func(*cobra.Command, []string) error {
			if err := a.autostartManager().DisableSystemd(); err != nil {
				return localErr(ExitInternal, "systemd_failed", err.Error())
			}
			if !a.g.quiet {
				fmt.Fprintln(a.out(), "systemd user unit removed.")
			}
			return nil
		}},
	)
	return c
}

func (a *App) autostartManager() *autostart.Manager {
	m := autostart.Default()
	if dir := a.env("XDG_CONFIG_HOME"); dir != "" {
		m.ConfigDir = dir
	}
	return m
}

// desktopExec quotes a path for the Exec key of a .desktop file.
func desktopExec(path string) string {
	return autostart.DesktopExec(path)
}

func (a *App) autostartSet(on bool) error {
	m := a.autostartManager()
	var err error
	if on {
		err = m.Enable()
	} else {
		err = m.Disable()
	}
	if err != nil {
		if runtime.GOOS == "darwin" {
			return localErr(ExitUsage, "not_supported", err.Error())
		}
		return localErr(ExitInternal, "autostart_failed", err.Error())
	}
	on2, where := m.Status()
	if !a.g.quiet && !a.g.json {
		fmt.Fprintf(a.out(), "autostart %v (%s)\n", map[bool]string{true: "enabled", false: "disabled"}[on2], where)
	}
	return nil
}

func (a *App) autostartState() (bool, string) {
	return a.autostartManager().Status()
}

// ---- update ----

func (a *App) updateCmd() *cobra.Command {
	var enable, disable, install, restart, yes bool
	c := &cobra.Command{Use: "update", Short: "Check for a newer release; --install verifies and installs it", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if enable && disable {
				return usage("choose --enable-check or --disable-check, not both")
			}
			if restart && !install {
				return usage("--restart goes with --install")
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
			if install {
				return a.installUpdate(cmd.Context(), rel, newer, restart, yes)
			}
			if a.g.json {
				b, _ := json.Marshal(map[string]any{"data": map[string]any{"current": Version, "latest": rel.Tag, "url": rel.URL, "update_available": newer}, "request_id": ""})
				fmt.Fprintln(a.out(), string(b))
				return nil
			}
			if !newer {
				fmt.Fprintf(a.out(), "502drive %s is up to date (latest release %s).\n", Version, rel.Tag)
				return nil
			}
			fmt.Fprintf(a.out(), "A newer release is available: %s (you have %s)\n%s\nInstall it with: 502drive update --install\n", rel.Tag, Version, rel.URL)
			return nil
		}}
	c.Flags().BoolVar(&install, "install", false, "download the newer release, verify its checksum and replace this binary")
	c.Flags().BoolVar(&restart, "restart", false, "with --install: stop the daemon, back up its data, and start the new version")
	c.Flags().BoolVarP(&yes, "yes", "y", false, "do not ask for confirmation")
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

// trayUpdateNote is the tray's opt-in check: the newer release tag when one
// exists, empty when this build is current.
func (a *App) trayUpdateNote(ctx context.Context) (string, error) {
	rel, err := update.Latest(ctx, nil, a.env("DRIVE502_UPDATE_API"))
	if err != nil || !update.Newer(Version, rel.Tag) {
		return "", err
	}
	return rel.Tag, nil
}

// installUpdate replaces this binary with the newer release. It never runs on
// its own: the user asks for it. Package-managed installs (rpm/deb, the
// Windows installer) are left to their package manager.
func (a *App) installUpdate(ctx context.Context, rel update.Release, newer, restart, yes bool) error {
	if !newer {
		if !a.g.quiet {
			fmt.Fprintf(a.out(), "502drive %s is up to date (latest release %s).\n", Version, rel.Tag)
		}
		return nil
	}
	target := a.env("DRIVE502_UPDATE_TARGET") // tests only
	if target == "" {
		exe, err := os.Executable()
		if err != nil {
			return localErr(ExitInternal, "update_failed", "Cannot locate the 502drive binary.")
		}
		if target, err = filepath.EvalSymlinks(exe); err != nil {
			target = exe
		}
	}
	dir := filepath.Dir(target)
	if strings.HasPrefix(target, "/usr/") || !update.Writable(dir) {
		return localErr(ExitUsage, "update_package_managed", fmt.Sprintf(
			"502drive in %s is managed by a package or an installer. Update it the same way: download %s from %s "+
				"(rpm: sudo dnf upgrade ./FILE.rpm · deb: sudo apt install ./FILE.deb · Windows: 502drive-setup.exe).",
			dir, rel.Tag, rel.URL))
	}
	if !yes {
		if !a.io.StdinTTY || a.g.noInput {
			return usage("add --yes to install %s without a prompt", rel.Tag)
		}
		if !a.confirm(fmt.Sprintf("Install %s over %s (you have %s)? [y/N]", rel.Tag, target, Version)) {
			return localErr(ExitUsage, "update_cancelled", "Nothing was installed.")
		}
	}
	base := update.ReleaseBase(a.env("DRIVE502_RELEASE_BASE"), rel.Tag)
	sums, err := update.Fetch(ctx, nil, base+"/checksums.txt", 1<<20)
	if err != nil {
		return localErr(ExitUnavailable, "update_failed", err.Error())
	}
	if err = a.verifyReleaseSignature(ctx, base, sums); err != nil {
		return localErr(ExitInternal, "update_signature", err.Error())
	}
	asset, err := update.PickAsset(sums, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return localErr(ExitNotFound, "update_failed", err.Error())
	}
	if !a.g.quiet {
		fmt.Fprintf(a.errw(), "Downloading %s…\n", asset.Name)
	}
	archive, err := update.Fetch(ctx, nil, base+"/"+asset.Name, 200<<20)
	if err != nil {
		return localErr(ExitUnavailable, "update_failed", err.Error())
	}
	if err = update.Verify(archive, asset.SHA256); err != nil {
		return localErr(ExitInternal, "update_checksum", err.Error())
	}
	names := []string{"502drive"}
	if runtime.GOOS == "windows" {
		names = []string{"502drive.exe", "502drive-tray.exe"}
	}
	files, err := update.Extract(archive, runtime.GOOS == "windows", names...)
	if err != nil {
		return localErr(ExitInternal, "update_failed", err.Error())
	}
	if err = update.Replace(target, files[names[0]]); err != nil {
		return localErr(ExitInternal, "update_failed", err.Error())
	}
	if len(names) > 1 {
		tray := filepath.Join(dir, names[1])
		if _, serr := os.Stat(tray); serr == nil {
			if err = update.Replace(tray, files[names[1]]); err != nil {
				return localErr(ExitInternal, "update_failed", err.Error())
			}
		}
	}
	if !a.g.quiet {
		fmt.Fprintf(a.out(), "Installed %s at %s (checksum verified). The previous binary is kept as %s.prev.\n", rel.Tag, target, target)
	}
	if !a.daemonUp(ctx) {
		if restart { // nothing to restart, but the new version may upgrade the database on its first start
			a.backupBeforeFirstStart(ctx, target, rel.Tag)
		}
		return nil
	}
	if !restart {
		if !a.g.quiet {
			fmt.Fprintln(a.out(), "The running daemon is still the old version. Restart it when convenient: 502drive update --install --restart, or 502drive stop then 502drive up.")
		}
		return nil
	}
	return a.restartWith(ctx, target, rel.Tag)
}

// releaseIdentity is the only workflow whose signature is accepted: this
// repository's release workflow, run for a version tag.
const releaseIdentity = `^https://github\.com/itsgiahung/502Drive/\.github/workflows/go-release\.yml@refs/tags/v.+$`

// verifyReleaseSignature checks the Sigstore signature of checksums.txt when
// `cosign` is installed. A bad signature always stops the update. A missing
// cosign or bundle only prints a note, unless DRIVE502_REQUIRE_SIGNATURE=1
// makes the signature mandatory (the same switch as REQUIRE_SIGNATURE in
// install.sh).
func (a *App) verifyReleaseSignature(ctx context.Context, base string, sums []byte) error {
	required := a.env("DRIVE502_REQUIRE_SIGNATURE") == "1"
	skip := func(why string) error {
		if required {
			return fmt.Errorf("the release signature is required (DRIVE502_REQUIRE_SIGNATURE=1) but %s", why)
		}
		if !a.g.quiet {
			fmt.Fprintf(a.errw(), "Signature not checked: %s. Only the SHA-256 checksum was verified.\n", why)
		}
		return nil
	}
	cosign, err := exec.LookPath("cosign")
	if err != nil {
		return skip("cosign is not installed")
	}
	bundle, err := update.Fetch(ctx, nil, base+"/checksums.txt.sigstore.json", 1<<20)
	if err != nil {
		return skip("this release has no signature bundle")
	}
	dir, err := os.MkdirTemp("", "502drive-sig-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	sumsPath, bundlePath := filepath.Join(dir, "checksums.txt"), filepath.Join(dir, "checksums.txt.sigstore.json")
	if err = errors.Join(os.WriteFile(sumsPath, sums, 0o600), os.WriteFile(bundlePath, bundle, 0o600)); err != nil {
		return err
	}
	c := exec.CommandContext(ctx, cosign, "verify-blob", "--bundle", bundlePath,
		"--certificate-identity-regexp", releaseIdentity,
		"--certificate-oidc-issuer", "https://token.actions.githubusercontent.com", sumsPath)
	if out, err := c.CombinedOutput(); err != nil {
		return fmt.Errorf("the release signature did not verify; nothing was installed (%s)", strings.TrimSpace(string(out)))
	}
	if !a.g.quiet {
		fmt.Fprintln(a.errw(), "Release signature verified (Sigstore).")
	}
	return nil
}

// backupBeforeFirstStart saves the data while no daemon runs, so the next
// `502drive up` on the new version cannot upgrade the only copy. A failure is
// reported, not fatal: the binary is already installed.
func (a *App) backupBeforeFirstStart(ctx context.Context, target, tag string) {
	backup, err := updateBackupPath(filepath.Dir(a.configPath()), tag, time.Now())
	if err == nil {
		argv := a.childArgv(target, "backup", backup)
		c := exec.CommandContext(ctx, argv[0], argv[1:]...)
		c.Stdout, c.Stderr = a.errw(), a.errw()
		err = c.Run()
	}
	if a.g.quiet {
		return
	}
	if err != nil {
		fmt.Fprintf(a.errw(), "Could not back up the data before the first start of %s (%v). Run `502drive backup DIR` before `502drive up`.\n", tag, err)
		return
	}
	fmt.Fprintf(a.out(), "No daemon was running. A backup of the data is in %s; start the new version with 502drive up.\n", backup)
}

// restartWith stops the daemon, backs up its data (the new version may upgrade
// the database, which the old one then cannot open), and starts the daemon from
// the newly installed binary. It runs that binary by path: after the swap, this
// process's own executable is the old file.
func (a *App) restartWith(ctx context.Context, target, tag string) error {
	run := func(args ...string) error {
		argv := a.childArgv(target, args...)
		c := exec.CommandContext(ctx, argv[0], argv[1:]...)
		c.Stdout, c.Stderr = a.errw(), a.errw()
		return c.Run()
	}
	if err := run("stop", "--yes"); err != nil {
		return localErr(ExitInternal, "update_restart_failed", "Could not stop the daemon: "+err.Error())
	}
	backup, err := updateBackupPath(filepath.Dir(a.configPath()), tag, time.Now())
	if err != nil {
		return localErr(ExitInternal, "update_restart_failed", "The daemon is stopped; cannot prepare a backup folder ("+err.Error()+"). Start it again with: 502drive up")
	}
	if err := run("backup", backup); err != nil {
		return localErr(ExitInternal, "update_restart_failed", "The daemon is stopped but the backup failed ("+err.Error()+"); start it again with: 502drive up")
	}
	if err := run("up"); err != nil {
		return localErr(ExitInternal, "update_restart_failed", "Could not start the new daemon: "+err.Error())
	}
	if !a.g.quiet {
		fmt.Fprintf(a.out(), "Restarted on %s. A backup of the previous data is in %s.\n", tag, backup)
	}
	return nil
}

// updateBackupPath is where `update --restart` backs the data up: a new folder
// under <config>/backups. The backup command creates only the last path
// element, so the backups folder (private, 0700) must exist first.
func updateBackupPath(configDir, tag string, now time.Time) (string, error) {
	dir := filepath.Join(configDir, "backups")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(dir, "before-"+tag+"-"+now.Format("20060102-150405")), nil
}
