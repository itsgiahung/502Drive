package autostart

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SystemdUnitName is the user unit name used by the package, the CLI and
// doctor. It is deliberately the same name as the distribution unit so both
// cannot run: the instance lock keeps a single daemon.
const SystemdUnitName = "502drive.service"

// SystemdUnitPath returns ~/.config/systemd/user/502drive.service (or under
// XDG_CONFIG_HOME when set).
func (m *Manager) SystemdUnitPath() (string, error) {
	dir, err := m.linuxConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "systemd", "user", SystemdUnitName), nil
}

// systemdPath renders a path for a user unit: under the home directory it uses
// the %h specifier, so a unit written once keeps working for the same user.
func systemdPath(p string) string {
	if p == "" {
		return ""
	}
	home, err := os.UserHomeDir()
	if err == nil && home != "" && p != home {
		if strings.HasPrefix(p, home+string(filepath.Separator)) {
			p = "%h" + p[len(home):]
		}
	}
	if strings.ContainsAny(p, " \t") {
		return `"` + p + `"`
	}
	return p
}

// SystemdUnitText builds the user unit. It is the packaging unit with user
// paths and an [Install] section, because only the user enables this one.
func SystemdUnitText(exe, dataDir, configDir string) string {
	if configDir == "" {
		configDir = "%h/.config/502drive-next"
	}
	if dataDir == "" {
		dataDir = "%h/.local/share/502drive"
	}
	return "[Unit]\n" +
		"Description=502Drive daemon (copy Google Drive links and keep them in sync)\n" +
		"Documentation=https://github.com/itsgiahung/502Drive\n" +
		"After=network-online.target\n" +
		"Wants=network-online.target\n" +
		"# Give up after 5 crashes in 5 minutes instead of restarting forever:\n" +
		"# the tray shows the daemon offline and the user decides what to do.\n" +
		"StartLimitIntervalSec=300\n" +
		"StartLimitBurst=5\n" +
		"\n" +
		"[Service]\n" +
		"Type=simple\n" +
		"ExecStart=" + systemdPath(exe) + " serve\n" +
		"Restart=on-failure\n" +
		"RestartSec=5\n" +
		"Nice=10\n" +
		"IOSchedulingClass=best-effort\n" +
		"IOSchedulingPriority=4\n" +
		"MemoryHigh=512M\n" +
		"TasksMax=64\n" +
		"NoNewPrivileges=yes\n" +
		"PrivateTmp=yes\n" +
		"ProtectSystem=strict\n" +
		"ProtectHome=read-only\n" +
		"ReadWritePaths=" + strings.Join(nonEmpty([]string{systemdPath(configDir), systemdPath(dataDir)}), " ") + "\n" +
		"RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6\n" +
		"RestrictNamespaces=yes\n" +
		"LockPersonality=yes\n" +
		"SystemCallArchitectures=native\n" +
		"UMask=0077\n" +
		"\n" +
		"[Install]\n" +
		"WantedBy=default.target\n"
}

func nonEmpty(in []string) []string {
	out := in[:0]
	for _, s := range in {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// InstallSystemd writes the user unit, reloads systemd and enables it. The write
// is idempotent; an existing unit is replaced with the current binary path.
// enable=false is useful for a dry install and for tests.
func (m *Manager) InstallSystemd(dataDir, configDir string, enable bool) (string, error) {
	exe, err := m.exe()
	if err != nil {
		return "", fmt.Errorf("cannot locate the 502drive binary: %w", err)
	}
	path, err := m.SystemdUnitPath()
	if err != nil {
		return "", err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", fmt.Errorf("cannot create the systemd user directory: %w", err)
	}
	if err = os.WriteFile(path, []byte(SystemdUnitText(exe, dataDir, configDir)), 0o600); err != nil {
		return "", fmt.Errorf("cannot write the systemd user unit: %w", err)
	}
	if _, err = m.runCmd("systemctl", "--user", "daemon-reload"); err != nil {
		return path, fmt.Errorf("systemctl --user daemon-reload failed: %w", err)
	}
	if !enable {
		return path, nil
	}
	if _, err = m.runCmd("systemctl", "--user", "enable", SystemdUnitName); err != nil {
		return path, fmt.Errorf("systemctl --user enable failed: %w", err)
	}
	return path, nil
}

// DisableSystemd disables and removes the user unit. Missing files are fine.
func (m *Manager) DisableSystemd() error {
	path, err := m.SystemdUnitPath()
	if err != nil {
		return err
	}
	_, _ = m.runCmd("systemctl", "--user", "disable", SystemdUnitName)
	if err = os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("cannot remove the systemd user unit: %w", err)
	}
	_, _ = m.runCmd("systemctl", "--user", "daemon-reload")
	return nil
}

// SystemdStatus reports the user unit and whether systemd would start it.
// When systemctl is unavailable the unit is reported as installed but not
// enabled, with the reason in Detail.
func (m *Manager) SystemdStatus() (installed, enabled bool, detail string) {
	if m.goos() != "linux" {
		return false, false, "systemd is Linux-only"
	}
	path, err := m.SystemdUnitPath()
	if err != nil {
		return false, false, err.Error()
	}
	if fi, err := os.Stat(path); err != nil || fi.IsDir() {
		return false, false, "no user unit at " + path
	}
	out, err := m.runCmd("systemctl", "--user", "is-enabled", SystemdUnitName)
	text := strings.TrimSpace(string(out))
	if err != nil {
		if text == "" {
			text = err.Error()
		}
		return true, false, text
	}
	return true, text == "enabled", text
}

// SystemdState is the flattened view used by doctor and the CLI.
type SystemdState struct {
	Installed bool   `json:"installed"`
	Enabled   bool   `json:"enabled"`
	Unit      string `json:"unit"`
	Detail    string `json:"detail"`
}

// SystemdState reports the real state of the current user's unit.
func SystemdStateNow() SystemdState {
	m := Default()
	st := SystemdState{}
	path, err := m.SystemdUnitPath()
	if err == nil {
		st.Unit = path
	}
	st.Installed, st.Enabled, st.Detail = m.SystemdStatus()
	return st
}
