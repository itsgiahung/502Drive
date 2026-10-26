//go:build linux

package tray

import (
	"context"
	"time"

	"github.com/godbus/dbus/v5"
)

func init() { watcherProbe = probeStatusNotifierWatcher }

// probeStatusNotifierWatcher asks the session bus who owns
// org.kde.StatusNotifierWatcher. Without it no tray icon can appear (a GNOME
// shell without the AppIndicator extension, for example), so the launcher
// falls back to the no-tray mode instead of spawning an invisible tray. A
// tray that is already running re-registers by itself once a watcher shows
// up (fyne.io/systray stayRegistered).
func probeStatusNotifierWatcher(ctx context.Context) (bool, string) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return false, "no session bus (" + err.Error() + ")"
	}
	defer conn.Close()
	obj := conn.Object("org.freedesktop.DBus", "/org/freedesktop/DBus")
	var owner string
	if err = obj.CallWithContext(ctx, "org.freedesktop.DBus.GetNameOwner", 0,
		"org.kde.StatusNotifierWatcher").Store(&owner); err != nil {
		return false, "no StatusNotifierWatcher on the session bus"
	}
	return true, ""
}
