package telegram

import "github.com/itsgiahung/502Drive/internal/present"

// Strings added by batch IG (multi-link batches and the queue, deep preflight,
// the after-copy sync choice and the problem report). Same rule as texts_g.go:
// every key exists in Vietnamese and English (a test compares both) and Drive
// names are escaped by the caller.

// igTexts is the batch-IG string table.
type igTexts struct {
	// S6: one message with several links becomes one batch.
	batchTitle, batchMore, batchBad, batchTooMany, batchGo, batchNone string
	batchNote, batchDone, batchRefresh, batchJobsMore                 string
	// S6: the queue (pause / resume / order).
	queuePaused, queuePauseBtn, queueResumeBtn, queuePos string
	moveTop, moveBottom, movePrio, moveNormal            string

	// S7: deep preflight on the preview card.
	deepBtn, deepTitle, deepRunning, deepTotals, deepPartial, deepEstimate string
	deepRefresh, deepCancel, deepAgain, deepFree, deepDaily                string
	deepNative, deepCannot, deepUnreadable, deepShortcuts                  string
	deepStates                                                             map[string]string
	deepWarn                                                               map[string]string

	// S8: sync after copy.
	syncAfterBtn, syncAfterDefault, syncAfterTitle, syncAfterHint string
	syncAfterNames                                                map[string]string

	// Jobs of one source (GET /v1/jobs?collection_id=).
	colJobsBtn, colJobsTitle, colJobsNone string

	// X1: the problem report sent as a CSV file.
	reportBtn, reportCaption, reportNone, reportTruncated, reportHow string
	reportHintLine, itemHintDefault                                  string
	itemHint                                                         map[string]string
}

func (tx texts) ig() igTexts {
	if tx.lang == langEN {
		return igEN
	}
	return igVI
}

var igVI = igTexts{
	batchTitle:    "📦 <b>Lô %d link</b>",
	batchMore:     "+%d link nữa",
	batchBad:      "⚠️ %s — %s",
	batchTooMany:  "Chỉ nhận %d link đầu; %d link còn lại bị bỏ qua. Gửi lại phần còn lại sau.",
	batchGo:       "📥 Copy cả %d link",
	batchNone:     "Không có link nào dùng được trong tin này.",
	batchNote:     "Mỗi link là một công việc riêng, xếp hàng theo thứ tự gửi.",
	batchDone:     "📦 <b>Lô %d link</b> — đã nhận %d · bỏ qua %d · lỗi %d",
	batchRefresh:  "🔄 Làm mới",
	batchJobsMore: "+%d công việc nữa",

	queuePaused:    "⏸ Hàng đợi đang tạm dừng — các công việc chờ sẽ không chạy.",
	queuePauseBtn:  "⏸ Tạm dừng hàng đợi",
	queueResumeBtn: "▶️ Tiếp tục hàng đợi",
	queuePos:       "🔢 Vị trí trong hàng: %d/%d",
	moveTop:        "⬆️ Lên đầu hàng",
	moveBottom:     "⬇️ Xuống cuối hàng",
	movePrio:       "⭐ Ưu tiên",
	moveNormal:     "Bỏ ưu tiên",

	deepBtn:        "🔬 Quét kỹ",
	deepTitle:      "🔬 <b>Quét kỹ «%s»</b>",
	deepRunning:    "⏳ Đang quét… đã thấy %d mục (%d file, %d thư mục). Chưa đủ để là tổng.",
	deepTotals:     "📊 Tổng: %d file · %d thư mục · %s",
	deepPartial:    "📊 Mới đếm được (chưa phải tổng): %d file · %d thư mục · %s",
	deepEstimate:   "⏱ Ước tính khoảng %s (theo tốc độ các lần copy gần đây).",
	deepRefresh:    "🔄 Làm mới",
	deepCancel:     "⏹ Hủy quét",
	deepAgain:      "🔬 Quét lại",
	deepFree:       "💾 Dung lượng trống của Drive: %s",
	deepDaily:      "📆 Còn copy được hôm nay: khoảng %s",
	deepNative:     "📄 %d file Google Docs/Sheets/Slides (không tính vào dung lượng)",
	deepCannot:     "🚫 %d file chủ sở hữu không cho copy",
	deepUnreadable: "🔒 %d thư mục không đọc được",
	deepShortcuts:  "🔗 %d shortcut",
	deepStates: map[string]string{
		"cancelled":   "⏹ Đã hủy quét — số liệu dưới đây chưa đủ để là tổng.",
		"expired":     "⌛ Phiên quét đã hết hạn — số liệu chưa đủ để là tổng. Quét lại nếu cần.",
		"interrupted": "⚠️ Quét bị gián đoạn (daemon khởi động lại) — số liệu chưa đủ để là tổng.",
		"failed":      "❌ Quét lỗi giữa chừng — số liệu chưa đủ để là tổng.",
	},
	deepWarn: map[string]string{
		"estimate_unknown":          "⏱ Chưa ước tính được thời gian: chưa có lần copy nào để đo tốc độ.",
		"destination_storage_short": "⚠️ Drive có thể không đủ chỗ trống cho lượng cần copy.",
		"may_hit_daily_limit":       "⚠️ Có thể chạm giới hạn copy mỗi ngày của Google (khoảng 750 GB); công việc sẽ tự chờ rồi tiếp tục.",
	},

	syncAfterBtn:     "🔄 Sau khi copy: %s",
	syncAfterDefault: "theo cài đặt (%s)",
	syncAfterTitle:   "🔄 Đồng bộ sau khi copy: %s",
	syncAfterHint:    "Mặc định cho các bản copy mới; có thể đổi riêng ở thẻ xem trước.",
	syncAfterNames: map[string]string{
		"off": "Tắt", "realtime": "Realtime", "hourly": "Mỗi giờ", "six_hourly": "6 giờ", "daily": "Hằng ngày",
	},

	colJobsBtn:   "📋 Công việc của nguồn này",
	colJobsTitle: "<b>Công việc của «%s»</b> (%d)",
	colJobsNone:  "Chưa có công việc nào cho nguồn này.",

	reportBtn:       "📄 Báo cáo lỗi",
	reportCaption:   "📄 Báo cáo lỗi của «%s»: %d mục.",
	reportNone:      "Không có mục lỗi, bị chặn hay bỏ qua để báo cáo.",
	reportTruncated: "Đã cắt ở %d dòng — thu hẹp công việc để xem phần còn lại.",
	reportHow:       "Cách xử lý:",
	reportHintLine:  "• %s (%s) ×%d — %s",
	itemHintDefault: "Mở công việc, nhấn Thử lại; nếu còn lỗi, chạy 502drive doctor trên máy daemon.",
	itemHint: map[string]string{
		"filtered":                  present.Explain("filtered", "vi").Action,
		"conflict":                  present.Explain("conflict", "vi").Action,
		"shortcut_unresolved":       present.Explain("shortcut_unresolved", "vi").Action,
		"shortcut_loop":             present.Explain("shortcut_loop", "vi").Action,
		"cannot_copy":               present.Explain("cannot_copy", "vi").Action,
		"google_native_export_only": present.Explain("google_native_export_only", "vi").Action,
		"too_large_for_quota":       present.Explain("too_large_for_quota", "vi").Action,
		"unreadable":                present.Explain("unreadable", "vi").Action,
		"trashed_source":            present.Explain("trashed_source", "vi").Action,
		"retry_exhausted":           present.Explain("retry_exhausted", "vi").Action,
		"quota_exceeded":            present.Explain("quota_exceeded", "vi").Action,
		"unknown_remote_outcome":    present.Explain("unknown_remote_outcome", "vi").Action,
		"permission_denied":         present.Explain("permission_denied", "vi").Action,
	},
}

var igEN = igTexts{
	batchTitle:    "📦 <b>Batch of %d links</b>",
	batchMore:     "+%d more links",
	batchBad:      "⚠️ %s — %s",
	batchTooMany:  "Only the first %d links are taken; %d more were skipped. Send the rest later.",
	batchGo:       "📥 Copy all %d links",
	batchNone:     "No usable link in this message.",
	batchNote:     "Each link is its own job, queued in the order you sent them.",
	batchDone:     "📦 <b>Batch of %d links</b> — accepted %d · skipped %d · failed %d",
	batchRefresh:  "🔄 Refresh",
	batchJobsMore: "+%d more jobs",

	queuePaused:    "⏸ The queue is paused — waiting jobs will not start.",
	queuePauseBtn:  "⏸ Pause the queue",
	queueResumeBtn: "▶️ Resume the queue",
	queuePos:       "🔢 Place in the queue: %d/%d",
	moveTop:        "⬆️ Move to the front",
	moveBottom:     "⬇️ Move to the back",
	movePrio:       "⭐ Prioritise",
	moveNormal:     "Normal priority",

	deepBtn:        "🔬 Deep scan",
	deepTitle:      "🔬 <b>Deep scan of «%s»</b>",
	deepRunning:    "⏳ Scanning… %d entries seen so far (%d files, %d folders). Not a total yet.",
	deepTotals:     "📊 Total: %d files · %d folders · %s",
	deepPartial:    "📊 Counted so far (not a total): %d files · %d folders · %s",
	deepEstimate:   "⏱ About %s (from the speed of your recent copies).",
	deepRefresh:    "🔄 Refresh",
	deepCancel:     "⏹ Cancel the scan",
	deepAgain:      "🔬 Scan again",
	deepFree:       "💾 Free space in Drive: %s",
	deepDaily:      "📆 Can still copy today: about %s",
	deepNative:     "📄 %d Google Docs/Sheets/Slides files (they use no quota)",
	deepCannot:     "🚫 %d files the owner does not allow copying",
	deepUnreadable: "🔒 %d folders cannot be read",
	deepShortcuts:  "🔗 %d shortcuts",
	deepStates: map[string]string{
		"cancelled":   "⏹ The scan was cancelled — the numbers below are not a total.",
		"expired":     "⌛ The scan expired — the numbers are not a total. Scan again if you need it.",
		"interrupted": "⚠️ The scan was interrupted (the daemon restarted) — the numbers are not a total.",
		"failed":      "❌ The scan failed part-way — the numbers are not a total.",
	},
	deepWarn: map[string]string{
		"estimate_unknown":          "⏱ No time estimate yet: no earlier copy to measure the speed from.",
		"destination_storage_short": "⚠️ Drive may not have enough free space for what has to be copied.",
		"may_hit_daily_limit":       "⚠️ This may hit Google's daily copy limit (about 750 GB); the job waits and resumes by itself.",
	},

	syncAfterBtn:     "🔄 After copying: %s",
	syncAfterDefault: "as in settings (%s)",
	syncAfterTitle:   "🔄 Sync after copy: %s",
	syncAfterHint:    "The default for new copies; you can change it per copy on the preview card.",
	syncAfterNames: map[string]string{
		"off": "Off", "realtime": "Realtime", "hourly": "Hourly", "six_hourly": "6 hours", "daily": "Daily",
	},

	colJobsBtn:   "📋 Jobs of this source",
	colJobsTitle: "<b>Jobs of «%s»</b> (%d)",
	colJobsNone:  "No jobs for this source yet.",

	reportBtn:       "📄 Problem report",
	reportCaption:   "📄 Problem report for «%s»: %d item(s).",
	reportNone:      "No failed, blocked or skipped items to report.",
	reportTruncated: "Cut at %d rows — narrow the job to see the rest.",
	reportHow:       "What to do:",
	reportHintLine:  "• %s (%s) ×%d — %s",
	itemHintDefault: "Open the job and tap Retry; if it still fails, run 502drive doctor on the daemon machine.",
	itemHint: map[string]string{
		"filtered":                  present.Explain("filtered", "en").Action,
		"conflict":                  present.Explain("conflict", "en").Action,
		"shortcut_unresolved":       present.Explain("shortcut_unresolved", "en").Action,
		"shortcut_loop":             present.Explain("shortcut_loop", "en").Action,
		"cannot_copy":               present.Explain("cannot_copy", "en").Action,
		"google_native_export_only": present.Explain("google_native_export_only", "en").Action,
		"too_large_for_quota":       present.Explain("too_large_for_quota", "en").Action,
		"unreadable":                present.Explain("unreadable", "en").Action,
		"trashed_source":            present.Explain("trashed_source", "en").Action,
		"retry_exhausted":           present.Explain("retry_exhausted", "en").Action,
		"quota_exceeded":            present.Explain("quota_exceeded", "en").Action,
		"unknown_remote_outcome":    present.Explain("unknown_remote_outcome", "en").Action,
		"permission_denied":         present.Explain("permission_denied", "en").Action,
	},
}
