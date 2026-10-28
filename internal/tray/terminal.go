package tray

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// termSpec is one terminal emulator and the flag that starts a program.
type termSpec struct {
	name string
	exec []string
}

// terminalSpecs is the documented fallback order (name after $TERMINAL).
func terminalSpecs(goos string) []termSpec {
	if goos == "windows" {
		return []termSpec{
			{name: "wt.exe"},
			{name: "cmd", exec: []string{"/c", "start", ""}},
		}
	}
	// Native emulators come first: a wrapper that merely pretends to be
	// gnome-terminal (or an alternatives symlink) may not pass arguments on.
	return []termSpec{
		{name: "ghostty", exec: []string{"-e"}},
		{name: "kitty", exec: []string{}},
		{name: "alacritty", exec: []string{"-e"}},
		{name: "wezterm", exec: []string{"start", "--"}},
		{name: "foot", exec: []string{}},
		{name: "konsole", exec: []string{"-e"}},
		{name: "gnome-terminal", exec: []string{"--"}},
		{name: "xfce4-terminal", exec: []string{"-x"}},
		{name: "tilix", exec: []string{"-e"}},
		{name: "xterm", exec: []string{"-e"}},
		{name: "x-terminal-emulator", exec: []string{"-e"}},
	}
}

// TerminalCommand builds the fixed argv that opens `502drive menu` in a
// terminal. It never uses a shell or user text.
func TerminalCommand(h Host, exe string) ([]string, error) {
	return terminalCommand(h, exe, "menu")
}

// terminalCommand builds the fixed argv that opens exe with args in a
// terminal. Every argument is chosen by the tray, never by the user.
func terminalCommand(h Host, exe string, args ...string) ([]string, error) {
	specs := terminalSpecs(h.GOOS)
	if h.GOOS != "windows" {
		if term := h.Getenv("TERMINAL"); term != "" {
			spec := termSpec{name: term, exec: []string{"-e"}}
			for _, known := range specs {
				if baseName(term) == known.name {
					spec = known
					spec.name = term
					break
				}
			}
			if commandExists(h, spec.name) {
				return append(append(append([]string{spec.name}, spec.exec...), exe), args...), nil
			}
		}
	}
	for _, spec := range specs {
		if !commandExists(h, spec.name) {
			continue
		}
		argv := append([]string{spec.name}, spec.exec...)
		return append(append(argv, exe), args...), nil
	}
	return nil, errors.New("no terminal emulator found (set $TERMINAL or install ghostty, kitty, konsole or gnome-terminal)")
}

// UpdateCommand builds the fixed argv that installs the newer release. The
// arguments are constant; only the running binary's path is added.
func UpdateCommand(h Host, exe string) ([]string, error) {
	if exe == "" {
		return nil, errors.New("cannot locate the 502drive binary")
	}
	return terminalCommand(h, exe, "update", "--install", "--restart", "--yes")
}

// OpenUpdate opens a terminal running the verified install command, so the
// user sees the checksum, restart and package-managed messages. The tray stays
// alive; the command keeps its own lifetime.
func OpenUpdate(h Host, exe string) error {
	argv, err := UpdateCommand(h, exe)
	if err != nil {
		return err
	}
	return h.Start(argv)
}

// OpenTUI starts the menu in a terminal; the tray stays alive.
func OpenTUI(h Host, exe string) error {
	if exe == "" {
		return errors.New("cannot locate the 502drive binary")
	}
	argv, err := TerminalCommand(h, exe)
	if err != nil {
		return err
	}
	return h.Start(argv)
}

// DaemonAction is start or stop.
type DaemonAction string

const (
	DaemonStart DaemonAction = "start"
	DaemonStop  DaemonAction = "stop"
)

// ControlDaemon starts or stops the daemon: the systemd user unit on Linux,
// the background process on Windows.
func ControlDaemon(ctx context.Context, h Host, action DaemonAction, exe string) (string, error) {
	if action != DaemonStart && action != DaemonStop {
		return "", fmt.Errorf("unknown daemon action %q", action)
	}
	switch h.GOOS {
	case "windows":
		if action == DaemonStart {
			if exe == "" {
				return "", errors.New("cannot locate the 502drive binary")
			}
			if err := h.Start([]string{exe, "serve"}); err != nil {
				return "", err
			}
			return "daemon started", nil
		}
		// Never kill by name: the graceful stop goes through the HTTP API
		// (the tray's Stop 502Drive… item or `502drive stop`).
		return "", errors.New("stopping the daemon on Windows goes through POST /v1/shutdown (the tray's Stop item or `502drive stop`), not taskkill")
	case "darwin":
		return "", errors.New("the tray is not supported on macOS; run `502drive serve` under launchd")
	default:
		if !commandExists(h, "systemctl") {
			return "", errors.New("systemctl is not available")
		}
		verb := "start"
		if action == DaemonStop {
			verb = "stop"
		}
		out, ok := runHost(ctx, h, []string{"systemctl", "--user", verb, "502drive.service"})
		if !ok {
			return "", fmt.Errorf("systemctl --user %s 502drive.service failed", verb)
		}
		return firstLine(out), nil
	}
}

func firstLine(s string) string {
	for i, r := range s {
		if r == '\n' || r == '\r' {
			return s[:i]
		}
	}
	return s
}

// OpenTUIJob preserves explicit client flags and treats the ID only as data.
func OpenTUIJob(h Host, exe, id string, clientArgs ...string) error {
	if id != "" {
		if !strings.HasPrefix(id, "job_") || len(id) < 5 || len(id) > 128 {
			return errors.New("invalid job ID")
		}
		for _, c := range id {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
				return errors.New("invalid job ID")
			}
		}
	}
	if exe == "" {
		return errors.New("cannot locate the 502drive binary")
	}
	args := append(append([]string{}, clientArgs...), "menu")
	if id != "" {
		args = append(args, "--job", id)
	}
	if h.GOOS == "windows" && !commandExists(h, "wt.exe") {
		if h.StartConsole == nil {
			return errors.New("no terminal or native console available")
		}
		return h.StartConsole(append([]string{exe}, args...))
	}
	argv, err := TerminalCommand(h, exe)
	if err != nil {
		return err
	}
	argv = append(argv[:len(argv)-1], args...)
	return h.Start(argv)
}
