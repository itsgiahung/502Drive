//go:build !linux && !windows

package tray

import "context"

// runTray reports that this platform has no tray backend. macOS needs CGO for
// systray, so the CLI and daemon keep working but the tray does not.
func runTray(context.Context, Options) error { return ErrUnsupported }
