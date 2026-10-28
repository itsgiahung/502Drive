package i18n

// Labels for the tray's opt-in update check and its install row. They live in
// their own file and merge into the two tables at start-up so the completeness
// tests (both languages, same printf verbs) cover them too.

const (
	TrayUpdateTo        Key = "tray_update_to"
	TrayUpdateConfirm   Key = "tray_update_confirm"
	TrayUpdateAvailable Key = "tray_update_available"
	TrayCheckUpdates    Key = "tray_check_updates"
	TrayUpdateStarted   Key = "tray_update_started"
	TrayUpdateCheckOn   Key = "tray_update_check_on"
	TrayUpdateCheckOff  Key = "tray_update_check_off"
)

func init() {
	for k, v := range trayUpdateVI {
		VI[k] = v
	}
	for k, v := range trayUpdateEN {
		EN[k] = v
	}
}

var trayUpdateVI = map[Key]string{
	TrayUpdateTo:        "Cập nhật lên %s…",
	TrayUpdateConfirm:   "Xác nhận cập nhật %s (mở terminal)…",
	TrayUpdateAvailable: "Có bản mới: %s",
	TrayCheckUpdates:    "Kiểm tra bản mới hàng ngày",
	TrayUpdateStarted:   "Đang cập nhật %s trong terminal…",
	TrayUpdateCheckOn:   "Kiểm tra bản mới hàng ngày: bật",
	TrayUpdateCheckOff:  "Kiểm tra bản mới hàng ngày: tắt",
}

var trayUpdateEN = map[Key]string{
	TrayUpdateTo:        "Update to %s…",
	TrayUpdateConfirm:   "Confirm update to %s (opens a terminal)…",
	TrayUpdateAvailable: "New release available: %s",
	TrayCheckUpdates:    "Check for updates daily",
	TrayUpdateStarted:   "Updating to %s in a terminal…",
	TrayUpdateCheckOn:   "Check for updates daily: on",
	TrayUpdateCheckOff:  "Check for updates daily: off",
}
