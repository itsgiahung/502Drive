package i18n

// Labels for clearing the job history (Settings → Clear history, and `x` on
// the Jobs screen). Every key exists in Vietnamese and English and merges into
// the same maps so the completeness tests cover them.

const (
	HistRow       Key = "history_row"
	HistTitle     Key = "history_title"
	HistPick      Key = "history_pick"
	HistOlder     Key = "history_older"
	HistAll       Key = "history_all"
	HistPreview   Key = "history_preview"
	HistKeep      Key = "history_keep"
	HistNothing   Key = "history_nothing"
	HistLoading   Key = "history_loading"
	HistScopeKeys Key = "history_scope_keys"
	HistAskKeys   Key = "history_ask_keys"
	HistDoneKeys  Key = "history_done_keys"
	HistDone      Key = "history_done"
	HistJobsAsk   Key = "history_jobs_ask"
	HistJobsDone  Key = "history_jobs_done"
	HistJobsNone  Key = "history_jobs_none"
)

var historyVI = map[Key]string{
	HistRow:       "Xóa lịch sử…",
	HistTitle:     "Xóa lịch sử",
	HistPick:      "Xóa các job đã xong khỏi lịch sử. Chọn phạm vi:",
	HistOlder:     "Cũ hơn %d ngày",
	HistAll:       "Tất cả job đã xong",
	HistPreview:   "Sẽ xóa %d job khỏi lịch sử (%d job bị xóa hẳn cùng %d sự kiện).",
	HistKeep:      "Các bản copy, thư mục đang theo dõi và file trên Drive không bị đụng tới.",
	HistNothing:   "Không có gì để xóa.",
	HistLoading:   "Đang xem trước…",
	HistScopeKeys: "↑↓ chọn · Enter/→ xem trước · ←/Esc quay lại",
	HistAskKeys:   "y/Enter xóa · n/Esc/← quay lại",
	HistDoneKeys:  "Enter/Esc/← quay lại",
	HistDone:      "Đã xóa %d job khỏi lịch sử.",
	HistJobsAsk:   "Xóa %d job đã xong khỏi lịch sử? (y/N)",
	HistJobsDone:  "Đã xóa %d job khỏi lịch sử, bỏ qua %d (chưa xong).",
	HistJobsNone:  "Chỉ job đã xong mới xóa được khỏi lịch sử.",
}

var historyEN = map[Key]string{
	HistRow:       "Clear history…",
	HistTitle:     "Clear history",
	HistPick:      "Remove finished jobs from the history. Pick a range:",
	HistOlder:     "Older than %d days",
	HistAll:       "Every finished job",
	HistPreview:   "Will clear %d jobs from the history (%d deleted for good with %d events).",
	HistKeep:      "Your copies, tracked folders and Drive files are not touched.",
	HistNothing:   "Nothing to clear.",
	HistLoading:   "Previewing…",
	HistScopeKeys: "↑↓ choose · Enter/→ preview · ←/Esc back",
	HistAskKeys:   "y/Enter clear · n/Esc/← back",
	HistDoneKeys:  "Enter/Esc/← back",
	HistDone:      "Cleared %d jobs from the history.",
	HistJobsAsk:   "Clear %d finished jobs from the history? (y/N)",
	HistJobsDone:  "Cleared %d jobs from the history, skipped %d (not finished).",
	HistJobsNone:  "Only finished jobs can be cleared from the history.",
}

func init() {
	for k, v := range historyVI {
		VI[k] = v
	}
	for k, v := range historyEN {
		EN[k] = v
	}
}
