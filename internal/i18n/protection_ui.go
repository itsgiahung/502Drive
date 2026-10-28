package i18n

// Labels for the opt-in mirror, snapshots and restore of one collection (the
// collection detail screen of the TUI). Every key exists in Vietnamese and
// English, like the other feature tables, and merges into the same maps so the
// completeness tests cover them.

const (
	ProtRow            Key = "protection_row"
	ProtTitle          Key = "protection_title"
	ProtSummary        Key = "protection_summary"
	ProtHint           Key = "protection_hint"
	ProtMirror         Key = "protection_mirror"
	ProtAuto           Key = "protection_auto_snapshots"
	ProtDays           Key = "protection_keep_days"
	ProtCount          Key = "protection_keep_count"
	ProtCapture        Key = "protection_capture"
	ProtList           Key = "protection_list"
	ProtRestoreEntry   Key = "protection_restore_entry"
	ProtOn             Key = "protection_on"
	ProtOff            Key = "protection_off"
	ProtConfirmMirror  Key = "protection_confirm_mirror"
	ProtMirrorOff      Key = "protection_mirror_off"
	ProtSaved          Key = "protection_saved"
	ProtCaptureStarted Key = "protection_capture_started"
	ProtSnapshotsTitle Key = "protection_snapshots_title"
	ProtSnapshotsEmpty Key = "protection_snapshots_empty"
	ProtSnapshotsKeys  Key = "protection_snapshots_keys"
	ProtSnapshotsPick  Key = "protection_snapshots_pick"
	ProtPinned         Key = "protection_pinned"
	ProtUnpinned       Key = "protection_unpinned"
	ProtPinOK          Key = "protection_pin_ok"
	ProtRestoreTitle   Key = "protection_restore_title"
	ProtDryRunning     Key = "protection_dry_running"
	ProtDryFailed      Key = "protection_dry_failed"
	ProtActions        Key = "protection_actions"
	ProtActionsMore    Key = "protection_actions_more"
	ProtApplyHint      Key = "protection_apply_hint"
	ProtConfirmRestore Key = "protection_confirm_restore"
	ProtRestoreStarted Key = "protection_restore_started"
	ProtNoPreview      Key = "protection_no_preview"
	ProtUnavailable    Key = "protection_unavailable"
)

func init() {
	for k, v := range protectionVI {
		VI[k] = v
	}
	for k, v := range protectionEN {
		EN[k] = v
	}
}

var protectionVI = map[Key]string{
	ProtRow:            "Bảo vệ",
	ProtTitle:          "Bảo vệ và khôi phục",
	ProtSummary:        "gương %s · ảnh chụp %s",
	ProtHint:           "↑↓ chọn · Enter/→ mở · ←/Esc quay lại · r tải lại",
	ProtMirror:         "Gương khi nguồn xóa (bản sao vào thùng rác)",
	ProtAuto:           "Ảnh chụp tự động sau mỗi lần chạy",
	ProtDays:           "Giữ ảnh chụp %d ngày",
	ProtCount:          "Giữ tối đa %d bản không ghim",
	ProtCapture:        "Chụp ảnh chụp ngay",
	ProtList:           "Danh sách ảnh chụp (ghim/bỏ ghim)",
	ProtRestoreEntry:   "Khôi phục…",
	ProtOn:             "bật",
	ProtOff:            "tắt",
	ProtConfirmMirror:  "Bật gương? File bị xóa ở nguồn sẽ được chuyển vào thùng rác Drive (khôi phục được khoảng 30 ngày; không xóa vĩnh viễn). [y/N]",
	ProtMirrorOff:      "Đã tắt gương.",
	ProtSaved:          "Đã lưu lựa chọn bảo vệ.",
	ProtCaptureStarted: "Đã nhận yêu cầu chụp ảnh chụp (job %s).",
	ProtSnapshotsTitle: "Ảnh chụp của nguồn",
	ProtSnapshotsEmpty: "Chưa có ảnh chụp nào.",
	ProtSnapshotsKeys:  "p ghim/bỏ ghim · r khôi phục · esc quay lại",
	ProtSnapshotsPick:  "Enter chọn ảnh chụp để xem trước khôi phục · esc quay lại",
	ProtPinned:         "đã ghim",
	ProtUnpinned:       "chưa ghim",
	ProtPinOK:          "Đã đổi trạng thái ghim.",
	ProtRestoreTitle:   "Xem trước khôi phục",
	ProtDryRunning:     "Đang chạy xem trước (dry-run)… nhấn r để đọc lại.",
	ProtDryFailed:      "Xem trước thất bại (%s); chưa thể áp dụng.",
	ProtActions:        "Số thao tác sẽ thực hiện: %d",
	ProtActionsMore:    "Số thao tác sẽ thực hiện: ít nhất %d (còn nữa)",
	ProtApplyHint:      "a Áp dụng khôi phục (đồng bộ sẽ tạm dừng) · r đọc lại · esc quay lại",
	ProtConfirmRestore: "Áp dụng khôi phục? Đồng bộ của nguồn này sẽ tạm dừng cho tới khi bạn bật lại. [y/N]",
	ProtRestoreStarted: "Đã nhận yêu cầu khôi phục (job %s). Đồng bộ đang tạm dừng.",
	ProtNoPreview:      "Hãy để xem trước (dry-run) chạy xong trước khi áp dụng.",
	ProtUnavailable:    "Daemon chưa hỗ trợ bảo vệ; hãy dùng CLI 502drive sync options.",
}

var protectionEN = map[Key]string{
	ProtRow:            "Protection",
	ProtTitle:          "Protection and restore",
	ProtSummary:        "mirror %s · snapshots %s",
	ProtHint:           "↑↓ choose · Enter/→ open · ←/Esc back · r reload",
	ProtMirror:         "Mirror source deletions (copies go to the trash)",
	ProtAuto:           "Automatic snapshots after each run",
	ProtDays:           "Keep snapshots for %d days",
	ProtCount:          "Keep up to %d unpinned snapshots",
	ProtCapture:        "Capture a snapshot now",
	ProtList:           "Snapshot list (pin/unpin)",
	ProtRestoreEntry:   "Restore…",
	ProtOn:             "on",
	ProtOff:            "off",
	ProtConfirmMirror:  "Enable mirror? Files deleted at the source move to the Drive trash (recoverable for about 30 days; never a permanent delete). [y/N]",
	ProtMirrorOff:      "Mirror is off.",
	ProtSaved:          "Protection options saved.",
	ProtCaptureStarted: "Snapshot requested (job %s).",
	ProtSnapshotsTitle: "Snapshots of this source",
	ProtSnapshotsEmpty: "No snapshots yet.",
	ProtSnapshotsKeys:  "p pin/unpin · r restore · esc back",
	ProtSnapshotsPick:  "Enter picks the snapshot to preview a restore · esc back",
	ProtPinned:         "pinned",
	ProtUnpinned:       "unpinned",
	ProtPinOK:          "Pin changed.",
	ProtRestoreTitle:   "Restore preview",
	ProtDryRunning:     "The dry run is in progress… press r to read it again.",
	ProtDryFailed:      "The preview failed (%s); apply is not available.",
	ProtActions:        "Actions that would run: %d",
	ProtActionsMore:    "Actions that would run: at least %d (more follow)",
	ProtApplyHint:      "a apply the restore (sync pauses) · r refresh · esc back",
	ProtConfirmRestore: "Apply the restore? Sync for this collection pauses until you resume it. [y/N]",
	ProtRestoreStarted: "Restore requested (job %s). Sync is paused.",
	ProtNoPreview:      "Let the dry-run preview finish before applying.",
	ProtUnavailable:    "The daemon does not support protection yet; use 502drive sync options.",
}
