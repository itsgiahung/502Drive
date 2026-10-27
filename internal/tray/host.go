package tray

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Host is every OS side effect the tray needs. Tests provide fakes; the tray
// never shells out with user text, only with fixed argv it builds itself.
var consoleStarter func([]string) error

type Host struct {
	StartConsole func(argv []string) error
	Notify       func(ctx context.Context, n Notice, open func()) error
	GOOS         string
	Getenv       func(string) string
	LookPath     func(string) (string, error)
	// Run executes argv and returns its stdout.
	Run func(ctx context.Context, argv []string) (string, error)
	// Start launches a detached process and returns immediately.
	Start func(argv []string) error
}

// DefaultHost returns the real host: exec with a bounded timeout.
func DefaultHost() Host {
	return Host{StartConsole: consoleStarter,
		Notify: notifyDesktop, GOOS: goos(),
		Getenv:   os.Getenv,
		LookPath: exec.LookPath,
		Run: func(ctx context.Context, argv []string) (string, error) {
			if len(argv) == 0 {
				return "", errNoCommand
			}
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			out, err := exec.CommandContext(ctx, argv[0], argv[1:]...).Output()
			return string(out), err
		},
		Start: startDetached,
	}
}

type trayError string

func (e trayError) Error() string { return string(e) }

const errNoCommand = trayError("no command to run")

// runHost executes argv, treating a missing program as "try the next
// candidate" instead of an error.
func runHost(ctx context.Context, h Host, argv []string) (string, bool) {
	if len(argv) == 0 {
		return "", false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if h.LookPath != nil {
		if _, err := h.LookPath(argv[0]); err != nil {
			return "", false
		}
	}
	out, err := h.Run(ctx, argv)
	if err != nil {
		return "", false
	}
	return out, true
}

// commandExists reports whether a program is on PATH.
func commandExists(h Host, name string) bool {
	if h.LookPath == nil {
		return true
	}
	_, err := h.LookPath(name)
	return err == nil
}

// baseName strips a directory from a program path.
func baseName(p string) string {
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		return p[i+1:]
	}
	return p
}
