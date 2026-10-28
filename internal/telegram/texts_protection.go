package telegram

// Strings of the protection card (mirror, snapshots, restore). They follow the
// same rule as render.go: every field exists in Vietnamese and English, Drive
// names are escaped by the caller, and fixed retention values are buttons.

type protectTexts struct {
	btn, title string
	// Options card.
	mirrorLabel, snapLabel, keepFmt                       string
	mirrorAskBtn, mirrorOffBtn                            string
	mirrorAsk, mirrorConfirm, mirrorOnDone, mirrorOffDone string
	snapToggleOn, snapToggleOff, snapOnDone, snapOffDone  string
	keepBtn, keepTitle, keepHint                          string
	daysValue, countValue, invalidChoice                  string
	saved                                                 string
	captureBtn, captureDone                               string
	snapsBtn, snapsTitle, snapsEmpty, snapsBack           string
	snapRow                                               string
	// One snapshot.
	snapCardTitle, pinBtn, unpinBtn, pinDone, snapshotState string
	restoreBtn                                              string
	// Restore preview and apply.
	restoreTitle, restoreRunning, restoreCount, restoreCountMore string
	restoreFailed, restoreEmpty, restoreRead, restoreBack        string
	applyBtn, applyAsk, applyConfirm, applyDone                  string
}

func (tx texts) prot() protectTexts {
	if tx.lang == langEN {
		return protectEN
	}
	return protectVI
}

var protectVI = protectTexts{
	btn:   "🛡 Bảo vệ",
	title: "🛡 <b>Bảo vệ nguồn</b>",

	mirrorLabel: "Gương khi nguồn xóa", snapLabel: "Ảnh chụp tự động",
	keepFmt:      "%d ngày · %d bản không ghim",
	mirrorAskBtn: "Bật gương…", mirrorOffBtn: "Tắt gương",
	mirrorAsk:     "Bật gương? File bị xóa ở nguồn sẽ được chuyển vào thùng rác Drive (khôi phục được khoảng 30 ngày; không xóa vĩnh viễn).",
	mirrorConfirm: "✅ Bật gương",
	mirrorOnDone:  "Đã bật gương. Lần chạy sau sẽ xử lý các mục bị xóa ở nguồn.",
	mirrorOffDone: "Đã tắt gương.",
	snapToggleOn:  "Bật ảnh chụp tự động", snapToggleOff: "Tắt ảnh chụp tự động",
	snapOnDone: "Đã bật ảnh chụp tự động.", snapOffDone: "Đã tắt ảnh chụp tự động.",
	keepBtn:   "📅 Thời hạn giữ",
	keepTitle: "📅 <b>Giữ ảnh chụp</b>",
	keepHint:  "Chọn từ danh sách cố định. Ảnh chụp đã ghim luôn được giữ, không tính vào hai giới hạn này.",
	daysValue: "%d ngày", countValue: "%d bản", invalidChoice: "Giá trị không có trong danh sách; hãy chọn lại.",
	saved: "Đã lưu lựa chọn bảo vệ.",

	captureBtn:  "📸 Chụp ngay",
	captureDone: "Đã nhận yêu cầu chụp ảnh chụp (job <code>%s</code>).",
	snapsBtn:    "🗂 Ảnh chụp", snapsTitle: "🗂 <b>Ảnh chụp</b>",
	snapsEmpty: "Chưa có ảnh chụp nào. Bật ảnh chụp tự động hoặc chụp ngay.",
	snapsBack:  "↩️ Bảo vệ",
	snapRow:    "%s · %s · %d mục",

	snapCardTitle: "🗂 <b>Ảnh chụp</b>",
	pinBtn:        "📌 Ghim", unpinBtn: "📍 Bỏ ghim", pinDone: "Đã đổi trạng thái ghim.",
	snapshotState: "Trạng thái: %s",
	restoreBtn:    "🕘 Khôi phục…",

	restoreTitle:     "🕘 <b>Xem trước khôi phục</b>",
	restoreRunning:   "Đang chạy xem trước (dry-run); chưa thay đổi gì trên Drive. Mở lại thẻ để đọc kết quả.",
	restoreCount:     "Số thao tác sẽ thực hiện: <b>%d</b>",
	restoreCountMore: "Số thao tác sẽ thực hiện: ít nhất <b>%d</b> (còn nữa).",
	restoreFailed:    "Xem trước thất bại (%s); chưa thể áp dụng.",
	restoreEmpty:     "Không có thao tác nào cần làm.",
	restoreRead:      "🔄 Đọc lại",
	restoreBack:      "↩️ Ảnh chụp",
	applyBtn:         "▶️ Áp dụng khôi phục…",
	applyAsk:         "Khôi phục về ảnh chụp <code>%s</code>? Đồng bộ của nguồn này sẽ tạm dừng cho tới khi bạn bật lại. Bản đích không có trong ảnh chụp sẽ được dọn theo kế hoạch đã xem trước.",
	applyConfirm:     "⚠️ Áp dụng khôi phục",
	applyDone:        "Đã nhận yêu cầu khôi phục (job <code>%s</code>). Đồng bộ đang tạm dừng.",
}

var protectEN = protectTexts{
	btn:   "🛡 Protection",
	title: "🛡 <b>Source protection</b>",

	mirrorLabel: "Mirror source deletions", snapLabel: "Automatic snapshots",
	keepFmt:      "%d days · %d unpinned",
	mirrorAskBtn: "Enable mirror…", mirrorOffBtn: "Turn mirror off",
	mirrorAsk:     "Enable mirror? Files deleted at the source move to the Drive trash (recoverable for about 30 days; never a permanent delete).",
	mirrorConfirm: "✅ Enable mirror",
	mirrorOnDone:  "Mirror is on. The next run handles source removals.",
	mirrorOffDone: "Mirror is off.",
	snapToggleOn:  "Enable automatic snapshots", snapToggleOff: "Turn automatic snapshots off",
	snapOnDone: "Automatic snapshots are on.", snapOffDone: "Automatic snapshots are off.",
	keepBtn:   "📅 Retention",
	keepTitle: "📅 <b>Snapshot retention</b>",
	keepHint:  "Pick from the fixed list. Pinned snapshots are always kept and do not count towards either limit.",
	daysValue: "%d days", countValue: "%d points", invalidChoice: "That value is not in the list; pick again.",
	saved: "Protection options saved.",

	captureBtn:  "📸 Capture now",
	captureDone: "Snapshot requested (job <code>%s</code>).",
	snapsBtn:    "🗂 Snapshots", snapsTitle: "🗂 <b>Snapshots</b>",
	snapsEmpty: "No snapshots yet. Enable automatic snapshots or capture one now.",
	snapsBack:  "↩️ Protection",
	snapRow:    "%s · %s · %d items",

	snapCardTitle: "🗂 <b>Snapshot</b>",
	pinBtn:        "📌 Pin", unpinBtn: "📍 Unpin", pinDone: "Pin changed.",
	snapshotState: "State: %s",
	restoreBtn:    "🕘 Restore…",

	restoreTitle:     "🕘 <b>Restore preview</b>",
	restoreRunning:   "The dry run is running; nothing on Drive changed yet. Open this card again to read the result.",
	restoreCount:     "Actions that would run: <b>%d</b>",
	restoreCountMore: "Actions that would run: at least <b>%d</b> (more follow).",
	restoreFailed:    "The preview failed (%s); apply is not available.",
	restoreEmpty:     "No actions are needed.",
	restoreRead:      "🔄 Read again",
	restoreBack:      "↩️ Snapshots",
	applyBtn:         "▶️ Apply the restore…",
	applyAsk:         "Restore to snapshot <code>%s</code>? Sync for this collection pauses until you resume it. Destination extras outside the previewed point are removed as planned.",
	applyConfirm:     "⚠️ Apply the restore",
	applyDone:        "Restore requested (job <code>%s</code>). Sync is paused.",
}
