//go:build windows

package tray

import "fyne.io/systray"

// trayIcon is the icon bytes the Windows tray takes (an ICO of several sizes).
func trayIcon(st State, frame int) ([]byte, error) {
	return icoFrame(st, frame%busyFrames, []int{16, 32, 48})
}

// setMenuGlyph is a no-op: the systray library cannot set menu row icons on Windows.
func setMenuGlyph(*systray.MenuItem, string) {}
