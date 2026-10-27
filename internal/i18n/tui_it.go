package i18n

// Labels added for the TUI features of batch IT: batch copy and the queue
// (S6), the deep preview (S7), the after-copy sync choice (S8), the problem
// report and CSV export (X1), server-side job filters, Drive search and the
// last audit on the Library. They merge into the two tables at start-up so the
// completeness tests (both languages, same printf verbs) cover them too.

const (
	// queue and batch (S6)
	QueuePausedBanner Key = "queue_paused_banner"
	QueuePausedOK     Key = "queue_paused_ok"
	QueueResumedOK    Key = "queue_resumed_ok"
	QueueConfirmPause Key = "queue_confirm_pause"
	QueuePauseAll     Key = "queue_pause_all"
	QueueResumeAll    Key = "queue_resume_all"
	JobMovedOK        Key = "job_moved_ok"
	JobMoveNone       Key = "job_move_none"
	JobPriorityOK     Key = "job_priority_ok"
	JobPriorityOnly   Key = "job_priority_only_queued"
	PriorityLabel     Key = "priority_label"
	QueuePos          Key = "queue_pos"
	BatchTag          Key = "batch_tag"
	BatchProgressLine Key = "batch_progress_line"
	BatchPasteHint    Key = "batch_paste_hint"
	BatchResultText   Key = "batch_result"
	BatchTitle        Key = "batch_title"
	BatchNote         Key = "batch_note"
	BatchFailedRow    Key = "batch_failed_row"
	BatchKeys         Key = "batch_keys"

	// jobs list filter and paging
	JobsFilterLabel  Key = "jobs_filter_label"
	JobsFilterAll    Key = "jobs_filter_all"
	JobsFilterSource Key = "jobs_filter_source"
	JobsMoreRows     Key = "jobs_more_rows"
	JobsNoMatch      Key = "jobs_no_match"
	JobsKeysIt       Key = "jobs_keys_it"

	// deep preview (S7)
	DeepTitle         Key = "deep_title"
	DeepRunning       Key = "deep_running"
	DeepNotTotal      Key = "deep_not_total"
	DeepDone          Key = "deep_done"
	DeepCancelledText Key = "deep_cancelled"
	DeepExpiredText   Key = "deep_expired"
	DeepInterrupted   Key = "deep_interrupted"
	DeepFailedText    Key = "deep_failed"
	DeepTotals        Key = "deep_totals"
	DeepExtra         Key = "deep_extra"
	DeepEstimate      Key = "deep_estimate"
	DeepEstimateNone  Key = "deep_estimate_none"
	WarnEstimate      Key = "warn_estimate_unknown"
	WarnStorageShort  Key = "warn_destination_storage_short"
	WarnDailyLimit    Key = "warn_may_hit_daily_limit"
	DestNotWritable   Key = "dest_not_writable"
	DestUnverifiable  Key = "dest_unverifiable"
	CopyItKeys        Key = "copy_it_keys"

	// after-copy sync (S8)
	AfterSyncLabel Key = "after_sync_label"
	AfterOff       Key = "after_sync_off"
	AfterRealtime  Key = "after_sync_realtime"
	AfterHourly    Key = "after_sync_hourly"
	AfterSixHourly Key = "after_sync_six_hourly"
	AfterDaily     Key = "after_sync_daily"
	SetAfterSync   Key = "set_after_sync"

	// problem report and export (X1)
	ReportTitle     Key = "report_title"
	ReportLoading   Key = "report_loading"
	ReportEmpty     Key = "report_empty"
	ReportCut       Key = "report_cut"
	ReportKeys      Key = "report_keys"
	ReportRowTries  Key = "report_row_tries"
	ExportPrompt    Key = "export_prompt"
	ExportKeys      Key = "export_keys"
	ExportDone      Key = "export_done"
	ExportHistory   Key = "export_history_prompt"
	ExportNeedsPath Key = "export_needs_path"

	// Drive search
	SearchPrompt Key = "search_prompt"
	SearchTitle  Key = "search_title"
	SearchNone   Key = "search_none"
	SearchLimit  Key = "search_limit"
	SearchKeys   Key = "search_keys"
	SearchBusy   Key = "search_busy"

	// last audit on the Library
	LastAuditTitle   Key = "last_audit_title"
	LastAuditClean   Key = "last_audit_clean"
	LastAuditFinding Key = "last_audit_finding"
	LastAuditRunning Key = "last_audit_running"
	LastAuditFailed  Key = "last_audit_failed"
	LastAuditChip    Key = "last_audit_chip"
)

func init() {
	for k, v := range tuiItVI {
		VI[k] = v
	}
	for k, v := range tuiItEN {
		EN[k] = v
	}
}

var tuiItVI = map[Key]string{
	QueuePausedBanner: "Hàng đợi đang tạm dừng · nhấn U để tiếp tục",
	QueuePausedOK:     "Đã tạm dừng hàng đợi (%d job đang dừng an toàn).",
	QueueResumedOK:    "Đã tiếp tục hàng đợi.",
	QueueConfirmPause: "Tạm dừng tất cả? Job đang chạy dừng an toàn, job chờ không bắt đầu. [y/N]",
	QueuePauseAll:     "Tạm dừng tất cả",
	QueueResumeAll:    "Tiếp tục tất cả",
	JobMovedOK:        "Đã đổi thứ tự job %s.",
	JobMoveNone:       "Không có job chờ nào để đổi chỗ.",
	JobPriorityOK:     "Ưu tiên job %s: %d.",
	JobPriorityOnly:   "Chỉ đổi được job đang chờ (queued).",
	PriorityLabel:     "Ưu tiên",
	QueuePos:          "chờ #%d",
	BatchTag:          "lô %s",
	BatchProgressLine: "Lô %s · %d/%d job xong · %s",
	BatchPasteHint:    "Dán nhiều link, mỗi dòng một link",
	BatchResultText:   "Lô: %d đã nhận, %d lỗi",
	BatchTitle:        "Lô %d link",
	BatchNote:         "Mỗi link được kiểm tra khi gửi; link lỗi không chặn các link khác.",
	BatchFailedRow:    "lỗi: %s — %s",
	BatchKeys:         "enter gửi cả lô · 1/2 tên · d nơi lưu · s đồng bộ · f/o/k tùy chọn · esc hủy",

	JobsFilterLabel:  "Lọc",
	JobsFilterAll:    "tất cả trạng thái",
	JobsFilterSource: "chỉ nguồn %s",
	JobsMoreRows:     "… còn job nữa · kéo xuống cuối để tải thêm",
	JobsNoMatch:      "Không có job nào khớp bộ lọc.",
	JobsKeysIt:       "f lọc trạng thái · s chỉ nguồn này · K/J lên/xuống hàng chờ · +/- ưu tiên · P/U tạm dừng/tiếp tục tất cả · E xuất lịch sử",

	DeepTitle:         "Quét đầy đủ",
	DeepRunning:       "Đang quét… (%d mục)",
	DeepNotTotal:      "Chưa đủ để là tổng",
	DeepDone:          "Quét xong",
	DeepCancelledText: "Đã hủy quét",
	DeepExpiredText:   "Quét hết hạn",
	DeepInterrupted:   "Quét bị gián đoạn",
	DeepFailedText:    "Quét bị lỗi",
	DeepTotals:        "%d tệp · %d thư mục · %s",
	DeepExtra:         "Google %d · shortcut %d · không copy được %d · thư mục không đọc được %d",
	DeepEstimate:      "Ước tính %s (theo các lần copy gần đây)",
	DeepEstimateNone:  "Chưa ước tính được thời gian (chưa có lần copy nào để đo)",
	WarnEstimate:      "Chưa có số đo tốc độ nên không ước tính được thời gian.",
	WarnStorageShort:  "Dung lượng trống của tài khoản có thể không đủ cho cả nguồn.",
	WarnDailyLimit:    "Có thể chạm giới hạn copy trong ngày; job sẽ chờ rồi tự tiếp tục.",
	DestNotWritable:   "Tài khoản không ghi được vào thư mục đích. Nhấn d để chọn thư mục khác.",
	DestUnverifiable:  "Không kiểm tra được thư mục đích. Nhấn d để chọn lại.",
	CopyItKeys:        "i quét đầy đủ · x hủy quét",

	AfterSyncLabel: "Sau khi copy",
	AfterOff:       "tắt",
	AfterRealtime:  "realtime",
	AfterHourly:    "mỗi giờ",
	AfterSixHourly: "6 giờ",
	AfterDaily:     "hằng ngày",
	SetAfterSync:   "Đồng bộ sau copy",

	ReportTitle:     "Báo cáo lỗi và mục bị bỏ qua",
	ReportLoading:   "Đang đọc báo cáo…",
	ReportEmpty:     "Không có mục lỗi, bị chặn hay bị bỏ qua.",
	ReportCut:       "Đã cắt ở %d dòng",
	ReportKeys:      "↑↓ cuộn · E xuất CSV · esc quay lại",
	ReportRowTries:  "%d lần",
	ExportPrompt:    "Lưu báo cáo CSV vào đường dẫn (Enter lưu, esc hủy):",
	ExportKeys:      "Enter lưu · esc hủy · không ghi đè tệp đã có",
	ExportDone:      "Đã lưu %d dòng vào %s",
	ExportHistory:   "Lưu lịch sử 30 ngày (CSV) vào đường dẫn (Enter lưu, esc hủy):",
	ExportNeedsPath: "Nhập đường dẫn tệp .csv.",

	SearchPrompt: "Tìm trong Drive theo tên (Enter tìm, esc hủy):",
	SearchTitle:  "Kết quả cho “%s”",
	SearchNone:   "Không thấy mục nào",
	SearchLimit:  "Tối đa %d kết quả; thu hẹp từ khóa để thấy đúng mục.",
	SearchKeys:   "↑↓ chọn · enter mở thư mục · c copy mục này · / tìm lại · esc quay lại",
	SearchBusy:   "Đang tìm…",

	LastAuditTitle:   "Lần kiểm tra gần nhất",
	LastAuditClean:   "sạch",
	LastAuditFinding: "%d lỗi (%d sửa được)",
	LastAuditRunning: "đang quét",
	LastAuditFailed:  "bị lỗi",
	LastAuditChip:    "kiểm tra: %s",
}

var tuiItEN = map[Key]string{
	QueuePausedBanner: "The queue is paused · press U to resume",
	QueuePausedOK:     "Queue paused (%d job(s) settling safely).",
	QueueResumedOK:    "Queue resumed.",
	QueueConfirmPause: "Pause everything? Running jobs settle safely, queued jobs do not start. [y/N]",
	QueuePauseAll:     "Pause all",
	QueueResumeAll:    "Resume all",
	JobMovedOK:        "Job %s moved in the queue.",
	JobMoveNone:       "No queued job to swap places with.",
	JobPriorityOK:     "Job %s priority: %d.",
	JobPriorityOnly:   "Only a waiting (queued) job can be changed.",
	PriorityLabel:     "Priority",
	QueuePos:          "queue #%d",
	BatchTag:          "batch %s",
	BatchProgressLine: "Batch %s · %d/%d jobs done · %s",
	BatchPasteHint:    "Paste several links, one per line",
	BatchResultText:   "Batch: %d accepted, %d failed",
	BatchTitle:        "Batch of %d links",
	BatchNote:         "Each link is checked when it is sent; a bad link does not stop the others.",
	BatchFailedRow:    "failed: %s — %s",
	BatchKeys:         "enter send the batch · 1/2 name · d where · s sync · f/o/k options · esc cancel",

	JobsFilterLabel:  "Filter",
	JobsFilterAll:    "all states",
	JobsFilterSource: "only source %s",
	JobsMoreRows:     "… more jobs · scroll to the end to load more",
	JobsNoMatch:      "No job matches the filter.",
	JobsKeysIt:       "f filter by state · s only this source · K/J move in queue · +/- priority · P/U pause/resume all · E export history",

	DeepTitle:         "Full scan",
	DeepRunning:       "Scanning… (%d items)",
	DeepNotTotal:      "Not yet a total",
	DeepDone:          "Scan finished",
	DeepCancelledText: "Scan cancelled",
	DeepExpiredText:   "Scan expired",
	DeepInterrupted:   "Scan interrupted",
	DeepFailedText:    "Scan failed",
	DeepTotals:        "%d files · %d folders · %s",
	DeepExtra:         "Google %d · shortcuts %d · cannot copy %d · unreadable folders %d",
	DeepEstimate:      "Estimated %s (from recent copies)",
	DeepEstimateNone:  "Time cannot be estimated yet (no measured copy)",
	WarnEstimate:      "No measured speed yet, so the time cannot be estimated.",
	WarnStorageShort:  "The account's free storage may not hold the whole source.",
	WarnDailyLimit:    "The daily copy limit may be reached; the job waits and resumes by itself.",
	DestNotWritable:   "This account cannot write to the destination folder. Press d to pick another.",
	DestUnverifiable:  "The destination folder cannot be verified. Press d to choose it again.",
	CopyItKeys:        "i full scan · x cancel scan",

	AfterSyncLabel: "After copy",
	AfterOff:       "off",
	AfterRealtime:  "realtime",
	AfterHourly:    "hourly",
	AfterSixHourly: "6 hours",
	AfterDaily:     "daily",
	SetAfterSync:   "Sync after copy",

	ReportTitle:     "Failed and skipped items",
	ReportLoading:   "Reading the report…",
	ReportEmpty:     "No failed, blocked or skipped items.",
	ReportCut:       "Cut at %d rows",
	ReportKeys:      "↑↓ scroll · E export CSV · esc back",
	ReportRowTries:  "%d tries",
	ExportPrompt:    "Save the CSV report to path (Enter saves, esc cancels):",
	ExportKeys:      "Enter save · esc cancel · an existing file is never overwritten",
	ExportDone:      "Saved %d rows to %s",
	ExportHistory:   "Save the 30-day history (CSV) to path (Enter saves, esc cancels):",
	ExportNeedsPath: "Type a .csv file path.",

	SearchPrompt: "Search Drive by name (Enter searches, esc cancels):",
	SearchTitle:  "Results for “%s”",
	SearchNone:   "Nothing found",
	SearchLimit:  "At most %d results; narrow the words to find the right item.",
	SearchKeys:   "↑↓ pick · enter open folder · c copy this item · / search again · esc back",
	SearchBusy:   "Searching…",

	LastAuditTitle:   "Last integrity check",
	LastAuditClean:   "clean",
	LastAuditFinding: "%d finding(s) (%d repairable)",
	LastAuditRunning: "scanning",
	LastAuditFailed:  "failed",
	LastAuditChip:    "check: %s",
}
