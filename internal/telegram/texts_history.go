package telegram

// Strings of the "clear history" cards (jobs page). Every field exists in
// Vietnamese and English.

type historyTexts struct {
	btn, title, pick, pickOwn      string
	days, all, own                 string
	preview, nothing, keep         string
	confirmBtn, done, failed, back string
}

func (tx texts) hist() historyTexts {
	if tx.lang == langEN {
		return historyEN
	}
	return historyVI
}

var historyVI = historyTexts{
	btn:        "🧹 Xóa lịch sử",
	title:      "🧹 <b>Xóa lịch sử</b>",
	pick:       "Xóa các job đã xong khỏi lịch sử. Chọn phạm vi:",
	pickOwn:    "Xóa các job đã xong của bạn khỏi lịch sử.",
	days:       "Cũ hơn %d ngày",
	all:        "Tất cả job đã xong",
	own:        "Job đã xong của tôi",
	preview:    "Sẽ xóa <b>%d</b> job khỏi lịch sử.",
	nothing:    "Không có gì để xóa.",
	keep:       "Các bản copy, thư mục đang theo dõi và file trên Drive không bị đụng tới.",
	confirmBtn: "✅ Xóa khỏi lịch sử",
	done:       "Đã xóa <b>%d</b> job khỏi lịch sử.",
	failed:     "Không xóa được lịch sử: %s",
	back:       "↩️ Công việc",
}

var historyEN = historyTexts{
	btn:        "🧹 Clear history",
	title:      "🧹 <b>Clear history</b>",
	pick:       "Remove finished jobs from the history. Pick a range:",
	pickOwn:    "Remove your finished jobs from the history.",
	days:       "Older than %d days",
	all:        "Every finished job",
	own:        "My finished jobs",
	preview:    "Will clear <b>%d</b> jobs from the history.",
	nothing:    "Nothing to clear.",
	keep:       "Your copies, tracked folders and Drive files are not touched.",
	confirmBtn: "✅ Clear from history",
	done:       "Cleared <b>%d</b> jobs from the history.",
	failed:     "Could not clear the history: %s",
	back:       "↩️ Jobs",
}
