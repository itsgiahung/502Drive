package tray

import (
	"context"
	"errors"
	"fmt"
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
				return append(append([]string{spec.name}, spec.exec...), exe, "menu"), nil
			}
		}
	}
	for _, spec := range specs {
		if !commandExists(h, spec.name) {
			continue
		}
		argv := append([]string{spec.name}, spec.exec...)
		return append(argv, exe, "menu"), nil
	}
	return nil, errors.New("no terminal emulator found (set $TERMINAL or install ghostty, kitty, konsole or gnome-terminal)")
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
		if _, ok := runHost(ctx, h, []string{"taskkill", "/IM", "502drive.exe", "/F"}); ok {
			return "daemon stopped", nil
		}
		return "", errors.New("cannot stop the daemon (taskkill failed)")
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
