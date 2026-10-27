// Package autostart manages starting 502Drive (daemon and tray) when the user
// logs into their OS desktop session.
// It configures native OS startup:
//   - Linux: XDG autostart entry (~/.config/autostart/502drive.desktop)
//   - Windows: HKCU\Software\Microsoft\Windows\CurrentVersion\Run
//   - macOS: unsupported (user can use launchd for the daemon)
package autostart

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// WinRunKey is the standard user-level startup registry key on Windows.
const WinRunKey = `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`

// WinValueName is the value name under WinRunKey.
const WinValueName = "502Drive"

// Manager encapsulates OS autostart operations.
type Manager struct {
	GOOS      string
	ConfigDir string
	ExePath   string
	ExecCmd   func(name string, args ...string) ([]byte, error)
}

// Default returns a Manager initialized with host system defaults.
func Default() *Manager {
	return &Manager{
		GOOS: runtime.GOOS,
	}
}

func (m *Manager) goos() string {
	if m.GOOS != "" {
		return m.GOOS
	}
	return runtime.GOOS
}

func (m *Manager) exe() (string, error) {
	if m.ExePath != "" {
		return m.ExePath, nil
	}
	return os.Executable()
}

func (m *Manager) linuxConfigDir() (string, error) {
	if m.ConfigDir != "" {
		return m.ConfigDir, nil
	}
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config"), nil
}

func (m *Manager) linuxDesktopFile() (string, error) {
	dir, err := m.linuxConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "autostart", "502drive.desktop"), nil
}

func (m *Manager) runCmd(name string, args ...string) ([]byte, error) {
	if m.ExecCmd != nil {
		return m.ExecCmd(name, args...)
	}
	return exec.Command(name, args...).CombinedOutput()
}

// DesktopExec quotes a path for the Exec key of a .desktop file.
func DesktopExec(path string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "`", "\\`", "$", `\$`)
	return `"` + r.Replace(path) + `"`
}

func autostartDesktopEntry(exe string) string {
	return "[Desktop Entry]\n" +
		"Type=Application\n" +
		"Name=502Drive\n" +
		"Comment=Start the 502Drive daemon and tray\n" +
		"Exec=" + DesktopExec(exe) + " up --quiet\n" +
		"Icon=502drive\n" +
		"Terminal=false\n" +
		"X-GNOME-Autostart-enabled=true\n"
}

// Status returns whether autostart is currently enabled and its configuration location.
func (m *Manager) Status() (bool, string) {
	switch m.goos() {
	case "windows":
		out, err := m.runCmd("reg", "query", WinRunKey, "/v", WinValueName)
		return err == nil && len(out) > 0, WinRunKey
	case "darwin":
		return false, "unsupported on macOS"
	default:
		path, err := m.linuxDesktopFile()
		if err != nil {
			return false, "unknown"
		}
		fi, err := os.Stat(path)
		if err != nil || fi.IsDir() {
			return false, path
		}
		// If file exists, verify it doesn't explicitly disable autostart
		data, err := os.ReadFile(path)
		if err == nil && strings.Contains(string(data), "X-GNOME-Autostart-enabled=false") {
			return false, path
		}
		return true, path
	}
}

// Enable registers 502Drive for autostart on user login.
func (m *Manager) Enable() error {
	exe, err := m.exe()
	if err != nil {
		return fmt.Errorf("cannot locate the 502drive binary: %w", err)
	}

	switch m.goos() {
	case "windows":
		argv := []string{"add", WinRunKey, "/v", WinValueName, "/t", "REG_SZ", "/d", `"` + exe + `" up --quiet`, "/f"}
		out, err := m.runCmd("reg", argv...)
		if err != nil {
			return fmt.Errorf("reg add failed: %s: %w", strings.TrimSpace(string(out)), err)
		}
		return nil
	case "darwin":
		return errors.New("autostart is not supported on macOS yet; run `502drive serve` under launchd")
	default:
		path, err := m.linuxDesktopFile()
		if err != nil {
			return fmt.Errorf("cannot find user config directory: %w", err)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return fmt.Errorf("cannot create autostart directory: %w", err)
		}
		if err := os.WriteFile(path, []byte(autostartDesktopEntry(exe)), 0o600); err != nil {
			return fmt.Errorf("cannot write desktop file: %w", err)
		}
		return nil
	}
}

// Disable removes 502Drive from autostart on user login.
func (m *Manager) Disable() error {
	switch m.goos() {
	case "windows":
		argv := []string{"delete", WinRunKey, "/v", WinValueName, "/f"}
		_, _ = m.runCmd("reg", argv...)
		return nil
	case "darwin":
		return errors.New("autostart is not supported on macOS yet")
	default:
		path, err := m.linuxDesktopFile()
		if err != nil {
			return fmt.Errorf("cannot find user config directory: %w", err)
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("cannot remove desktop file: %w", err)
		}
		return nil
	}
}

// Toggle flips autostart state and returns the new state.
func (m *Manager) Toggle() (bool, error) {
	on, _ := m.Status()
	if on {
		if err := m.Disable(); err != nil {
			return true, err
		}
		return false, nil
	}
	if err := m.Enable(); err != nil {
		return false, err
	}
	return true, nil
}

// Package-level functions delegating to Default().

// Status returns whether autostart is on and where it is configured.
func Status() (bool, string) {
	return Default().Status()
}

// Enable turns autostart on.
func Enable() error {
	return Default().Enable()
}

// Disable turns autostart off.
func Disable() error {
	return Default().Disable()
}

// Toggle flips autostart on or off.
func Toggle() (bool, error) {
	return Default().Toggle()
}
