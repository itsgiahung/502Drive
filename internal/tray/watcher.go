package tray

import "context"

// watcherProbe reports whether the desktop can host a tray icon. Platforms
// without the SNI concept answer "present"; the Linux implementation asks the
// session bus who owns org.kde.StatusNotifierWatcher. It is a variable so
// tests can replace the probe without a real bus.
var watcherProbe = func(context.Context) (bool, string) { return true, "" }

// StatusNotifierWatcherPresent says whether a tray icon can be shown in this
// desktop session. reason is empty when present; otherwise it names what is
// missing (for example GNOME without the AppIndicator extension).
func StatusNotifierWatcherPresent(ctx context.Context) (present bool, reason string) {
	return watcherProbe(ctx)
}
