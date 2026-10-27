//go:build linux

package tray

import "fyne.io/systray"

// trayIcon is the icon bytes the Linux tray takes (PNG with hard edges).
func trayIcon(st State, frame int) ([]byte, error) { return trayPNG(st, frame%busyFrames, iconSize) }

// setMenuGlyph shows a drawn glyph beside a menu row.
func setMenuGlyph(item *systray.MenuItem, key string) {
	if data := menuGlyphPNG(key); data != nil {
		item.SetIcon(data)
	}
}
