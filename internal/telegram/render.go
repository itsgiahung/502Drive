package telegram

import (
	"fmt"
	"html"
	"strings"

	"github.com/itsgiahung/502Drive/internal/schedule"
	"github.com/itsgiahung/502Drive/internal/state"
)

type language string

const (
	langVI language = "vi"
	langEN language = "en"
)

// texts holds every user-visible string. Drive names are escaped separately,
// so these templates may carry HTML.
type texts struct {
	home, copy, jobs, library, destination, account, settings string
	details, back, pause, resume, cancel, retry               string
	pasteHint, copyHint, oneLink, invalidLink                 string
	google, destLabel, running, queued, needAction            string
	connected, disconnected, reconnect                        string
	expired, cancelled, noDestination, jobsEmpty              string
	libraryEmpty, accountHint                                 string
	syncLabel, syncOff, enableSync, resumeSync                string
	pauseSync, stopSync, runNow, changeSchedule               string
	pickPreset, every30m, everyHour, every6h                  string
	daily2200, weeklyMWF, realtimePreset                      string
	scheduleLabel, nextRun, lastSuccess, activeJob            string
	copyStateLabel, keepCopies                                string
	// Bulk cancellation of in-flight jobs.
	cancelAll, cancelAllAsk, cancelAllDone, cancelAllNone string
	// Integrity audit of a collection (S1).
	auditCheck, auditDone, auditClean                       string
	auditRepair, auditStarted, auditRepairStarted           string
	noticeCompleted, noticeFailed, noticeAction             string
	save, nextRuns                                          string
	syncErrorNotice                                         string
	syncStates                                              map[string]string
	settingsHint, autoCopy, on, off, language, totalUnknown string
	autoCopyNeedsDestination, notAvailable                  string
	destHint                                                string
	previewTitle, sourceLabel, targetLabel, seenLabel       string
	scanIncomplete, confirmDest, copyStarted, jobLabel      string
	updated, itemsLabel, resultLabel, problemLabel          string
	queueFull, linkOnly                                     string
	// Home screen and the persistent keyboard.
	kbCopy, kbJobs, kbSync, kbDest, kbAccount, kbSettings    string
	kbPlaceholder, kbReady                                   string
	welcomeTitle, welcomeHello, welcomeFeatures, welcomeTail string
	// Link-first copy flow.
	copyNow, renameBtn, pickBtn, saveHere, newFolderBtn    string
	upBtn, nextBtn, backBtn, askName, askFolder, pickTitle string
	saveToLabel, nameLabel, noFolders, renamed             string
	errMsg                                                 map[string]string
	states                                                 map[string]string
}

func langOf(s string) language {
	if s == string(langEN) {
		return langEN
	}
	return langVI
}

func textsFor(l language) texts {
	if l == langEN {
		return english
	}
	return vietnamese
}

var vietnamese = texts{
	home: "Trang chính", copy: "Sao chép", jobs: "Công việc", library: "Nguồn đã copy / Đồng bộ",
	destination: "Thư mục đích", account: "Tài khoản", settings: "Cài đặt",
	details: "Chi tiết", back: "Về menu", pause: "Tạm dừng", resume: "Tiếp tục",
	cancel: "Huỷ", retry: "Thử lại",
	pasteHint:   "Dán link Drive hoặc chọn bên dưới.",
	copyHint:    "Gửi một link Drive (thư mục hoặc file) vào chat này. Một link mỗi lần.",
	oneLink:     "Mỗi lần chỉ một link. Gửi lại link bạn muốn sao chép.",
	invalidLink: "Link không hợp lệ. Chỉ nhận link Drive/Docs HTTPS hoặc ID Drive.",
	google:      "Google", destLabel: "Thư mục đích", running: "Đang chạy", queued: "Chờ", needAction: "Cần xử lý",
	connected: "Đã kết nối", disconnected: "Chưa kết nối", reconnect: "Cần kết nối lại",
	expired: "Nút đã hết hạn", cancelled: "Đã huỷ", noDestination: "Chưa có thư mục đích",
	jobsEmpty: "Chưa có công việc nào.", libraryEmpty: "Chưa copy nguồn nào.",
	syncLabel: "Đồng bộ", syncOff: "Chưa bật", enableSync: "Bật đồng bộ", resumeSync: "Bật lại đồng bộ",
	pauseSync: "Tạm dừng", stopSync: "Dừng đồng bộ", runNow: "Chạy ngay", changeSchedule: "Đổi lịch",
	pickPreset: "Chọn lịch đồng bộ", every30m: "Mỗi 30 phút", everyHour: "Mỗi giờ", every6h: "Mỗi 6 giờ",
	daily2200: "Hằng ngày 22:00", weeklyMWF: "Thứ 2, 4, 6 lúc 22:00", realtimePreset: "Realtime (theo dõi thay đổi)",
	scheduleLabel: "Lịch", nextRun: "Lần tới", lastSuccess: "Thành công gần nhất", activeJob: "Công việc đang chạy",
	copyStateLabel: "Copy gốc", keepCopies: "Không xóa bản đích khi nguồn mất",
	noticeCompleted: "Công việc đã xong", noticeFailed: "Công việc gặp lỗi", noticeAction: "Công việc cần xử lý",
	save: "Lưu", nextRuns: "Lần chạy tới",
	syncErrorNotice: "<b>Đồng bộ cần xử lý</b>\n<code>%s</code>\nMở menu để thử lại hoặc kiểm tra tài khoản.",
	syncStates: map[string]string{
		"waiting_for_baseline": "Chờ copy gốc", "bootstrapping": "Đang đối chiếu", "active": "Đang bật",
		"paused": "Tạm dừng", "stopping": "Đang dừng", "stopped": "Đã dừng",
		"needs_reconcile": "Cần đối chiếu lại", "error": "Lỗi",
	},
	accountHint:  "Đăng nhập trên máy chạy daemon: 502drive account login",
	settingsHint: "Chỉ áp dụng cho tài khoản Telegram của bạn.",
	autoCopy:     "Dán là copy", on: "Bật", off: "Tắt", language: "Ngôn ngữ",
	totalUnknown:             "chưa rõ",
	autoCopyNeedsDestination: "Cần đặt thư mục đích mặc định trước (CLI: 502drive destinations).",
	notAvailable:             "Chưa hỗ trợ",
	destHint:                 "Thêm hoặc đổi thư mục đích mặc định bằng CLI: 502drive destinations list|add|default.",
	previewTitle:             "Sao chép %s", sourceLabel: "Nguồn", targetLabel: "Thư mục đích", seenLabel: "Đã thấy",
	scanIncomplete: "(chưa quét hết)", confirmDest: "Cần xác nhận lại thư mục đích",
	copyStarted: "Đã xếp hàng", jobLabel: "Công việc", updated: "Cập nhật", itemsLabel: "Mục",
	resultLabel: "Kết quả", problemLabel: "Lỗi", queueFull: "Chưa xếp hàng",
	linkOnly: "Chỉ nhận link Drive.",
	kbCopy:   "📥 Copy link", kbJobs: "📋 Công việc", kbSync: "🔄 Đồng bộ", kbDest: "📁 Thư mục đích",
	kbAccount: "👤 Tài khoản", kbSettings: "⚙️ Cài đặt",
	kbPlaceholder: "📎 Dán link Google Drive vào đây…",
	kbReady:       "⌨️ Menu nhanh đã bật ở phía dưới. Dán link Drive bất kỳ lúc nào để copy.",
	welcomeTitle:  "🌟 <b>502Drive</b> — copy và đồng bộ Google Drive",
	welcomeHello:  "👋 Xin chào <b>%s</b>!",
	welcomeFeatures: "📥 <b>Copy</b> — dán link, Google copy ngay trong Drive, giữ nguyên cây thư mục\n" +
		"🔄 <b>Đồng bộ</b> — theo lịch hoặc realtime, không mất thư mục đích\n" +
		"📋 <b>Công việc</b> — theo dõi tiến trình, tạm dừng hoặc tiếp tục\n" +
		"📁 <b>Thư mục đích</b> — chọn nơi lưu bản copy\n" +
		"⚙️ <b>Cài đặt</b> — ngôn ngữ, thông báo, dán là copy",
	welcomeTail: "Dán link Drive hoặc chọn một mục bên dưới 👇",
	copyNow:     "✅ Copy ngay", renameBtn: "✏️ Đổi tên", pickBtn: "📁 Chọn nơi lưu", saveHere: "✅ Lưu vào đây",
	newFolderBtn: "➕ Thư mục mới", upBtn: "⬆️ Lên", nextBtn: "⏭ Trang sau", backBtn: "↩️ Quay lại",
	askName:   "✏️ Gửi tên mới cho thư mục kết quả (hoặc gửi /cancel để thôi).",
	askFolder: "➕ Gửi tên cho thư mục mới (hoặc gửi /cancel để thôi).",
	pickTitle: "📁 <b>Chọn nơi lưu</b>", saveToLabel: "📂 Lưu vào", nameLabel: "🏷 Tên kết quả",
	noFolders: "Chưa có thư mục con nào ở đây.", renamed: "Đã đổi tên.",
	cancelAll:          "🧹 Huỷ tất cả",
	cancelAllAsk:       "<b>Huỷ tất cả công việc đang mở?</b>\nCó %d công việc sẽ dừng ngay. Bản đã copy xong vẫn giữ nguyên.",
	cancelAllDone:      "Đã huỷ %d, bỏ qua %d, lỗi %d.",
	cancelAllNone:      "Không có công việc nào đang mở.",
	auditCheck:         "🔍 Kiểm tra toàn vẹn",
	auditDone:          "Kiểm tra toàn vẹn: %d sai lệch (%d sửa được)",
	auditClean:         "Kiểm tra toàn vẹn: sạch, không có sai lệch",
	auditRepair:        "🛠 Sửa theo báo cáo",
	auditStarted:       "🔍 Đã bắt đầu kiểm tra toàn vẹn (công việc <code>%s</code>). Mở lại thẻ này để xem kết quả.",
	auditRepairStarted: "🛠 Đã bắt đầu sửa theo báo cáo (công việc <code>%s</code>).",
	errMsg: map[string]string{
		"destination_required":       "Chọn thư mục đích trước (nhấn 📁 Chọn nơi lưu).",
		"ambiguous_source":           "Link không rõ ràng. Hãy gửi đúng một link Drive.",
		"source_not_found":           "Không tìm thấy mục này, hoặc bạn chưa được chia sẻ.",
		"not_found":                  "Không tìm thấy mục này, hoặc bạn chưa được chia sẻ.",
		"copy_not_permitted":         "Chủ sở hữu không cho phép copy file này.",
		"unsupported_source":         "Không copy trực tiếp shortcut: hãy mở mục đích rồi gửi link của nó.",
		"destination_not_writable":   "Tài khoản này không thêm được file vào thư mục đó.",
		"capacity_exceeded":          "Hàng đợi đang đầy, thử lại sau ít phút.",
		"rate_limited":               "Google đang giới hạn tốc độ, thử lại sau.",
		"provider_unavailable":       "Google tạm thời không phản hồi, thử lại sau.",
		"account_reconnect_required": "Cần kết nối lại Google: nhấn l trong TUI hoặc chạy 502drive account login.",
		"forbidden":                  "Bạn không có quyền làm việc này.",
		"not_a_folder":               "Mục này không phải thư mục.",
		"invalid_request":            "Yêu cầu không hợp lệ.",
	},
	states: map[string]string{
		state.JobQueued: "Đã xếp hàng", state.JobRunning: "Đang sao chép", state.JobPausing: "Đang dừng",
		state.JobPaused: "Đã tạm dừng", state.JobCancelling: "Đang huỷ", state.JobReconciling: "Đang xác minh",
		state.JobBlocked: "Cần xử lý", state.JobSucceeded: "Xong", state.JobPartial: "Xong một phần",
		state.JobFailed: "Thất bại", state.JobCancelled: "Đã huỷ",
	},
}

var english = texts{
	home: "Home", copy: "Copy", jobs: "Jobs", library: "Library & sync",
	destination: "Destination", account: "Account", settings: "Settings",
	details: "Details", back: "Home", pause: "Pause", resume: "Resume",
	cancel: "Cancel", retry: "Retry",
	pasteHint:   "Paste a Drive link or pick below.",
	copyHint:    "Send one Drive link (folder or file) to this chat. One link at a time.",
	oneLink:     "One link at a time. Send the link you want to copy again.",
	invalidLink: "That is not a Drive link. Only HTTPS Drive/Docs links or Drive IDs are accepted.",
	google:      "Google", destLabel: "Destination", running: "Running", queued: "Queued", needAction: "Needs action",
	connected: "Connected", disconnected: "Not connected", reconnect: "Reconnect required",
	expired: "This button has expired", cancelled: "Cancelled", noDestination: "No destination yet",
	jobsEmpty: "No jobs yet.", libraryEmpty: "Nothing copied yet.",
	syncLabel: "Sync", syncOff: "Off", enableSync: "Enable sync", resumeSync: "Resume sync",
	pauseSync: "Pause", stopSync: "Stop sync", runNow: "Run now", changeSchedule: "Change schedule",
	pickPreset: "Pick a sync schedule", every30m: "Every 30 minutes", everyHour: "Every hour", every6h: "Every 6 hours",
	daily2200: "Daily at 22:00", weeklyMWF: "Mon, Wed, Fri at 22:00", realtimePreset: "Realtime (change polling)",
	scheduleLabel: "Schedule", nextRun: "Next run", lastSuccess: "Last success", activeJob: "Active job",
	copyStateLabel: "Initial copy", keepCopies: "Keeps the destination if the source disappears",
	noticeCompleted: "Job finished", noticeFailed: "Job failed", noticeAction: "Job needs action",
	save: "Save", nextRuns: "Next runs",
	syncErrorNotice: "<b>Sync needs attention</b>\n<code>%s</code>\nOpen the menu to retry or check the account.",
	syncStates: map[string]string{
		"waiting_for_baseline": "Waiting for the initial copy", "bootstrapping": "Reconciling",
		"active": "On", "paused": "Paused", "stopping": "Stopping", "stopped": "Stopped",
		"needs_reconcile": "Needs reconcile", "error": "Error",
	},
	accountHint:  "Sign in on the daemon machine: 502drive account login",
	settingsHint: "Only affects your Telegram account.",
	autoCopy:     "Paste to copy", on: "On", off: "Off", language: "Language",
	totalUnknown:             "unknown",
	autoCopyNeedsDestination: "Set a default destination first (CLI: 502drive destinations).",
	notAvailable:             "Not available",
	destHint:                 "Add or change the default destination with the CLI: 502drive destinations list|add|default.",
	previewTitle:             "Copy %s", sourceLabel: "Source", targetLabel: "Target", seenLabel: "Seen",
	scanIncomplete: "(scan not finished)", confirmDest: "Confirm the destination again",
	copyStarted: "Queued", jobLabel: "Job", updated: "Updated", itemsLabel: "Items",
	resultLabel: "Result", problemLabel: "Problem", queueFull: "Not queued",
	linkOnly: "Only Drive links are accepted.",
	kbCopy:   "📥 Copy link", kbJobs: "📋 Jobs", kbSync: "🔄 Sync", kbDest: "📁 Destination",
	kbAccount: "👤 Account", kbSettings: "⚙️ Settings",
	kbPlaceholder: "📎 Paste a Google Drive link here…",
	kbReady:       "⌨️ The quick menu is on below. Paste a Drive link any time to copy it.",
	welcomeTitle:  "🌟 <b>502Drive</b> — copy and sync Google Drive",
	welcomeHello:  "👋 Hello <b>%s</b>!",
	welcomeFeatures: "📥 <b>Copy</b> — paste a link, Google copies inside Drive, folder tree preserved\n" +
		"🔄 <b>Sync</b> — on a schedule or in realtime, your destination is never lost\n" +
		"📋 <b>Jobs</b> — follow progress, pause or resume\n" +
		"📁 <b>Destination</b> — choose where copies go\n" +
		"⚙️ <b>Settings</b> — language, notifications, paste-to-copy",
	welcomeTail: "Paste a Drive link or pick an option below 👇",
	copyNow:     "✅ Copy now", renameBtn: "✏️ Rename", pickBtn: "📁 Choose where", saveHere: "✅ Save here",
	newFolderBtn: "➕ New folder", upBtn: "⬆️ Up", nextBtn: "⏭ Next page", backBtn: "↩️ Back",
	askName:   "✏️ Send the new name for the output folder (or /cancel).",
	askFolder: "➕ Send a name for the new folder (or /cancel).",
	pickTitle: "📁 <b>Choose where to save</b>", saveToLabel: "📂 Save to", nameLabel: "🏷 Output name",
	noFolders: "No sub-folders here yet.", renamed: "Renamed.",
	cancelAll:          "🧹 Cancel all",
	cancelAllAsk:       "<b>Cancel every open job?</b>\n%d job(s) will stop now. Anything already copied stays.",
	cancelAllDone:      "Cancelled %d, skipped %d, failed %d.",
	cancelAllNone:      "No open jobs.",
	auditCheck:         "🔍 Integrity check",
	auditDone:          "Integrity check: %d finding(s) (%d repairable)",
	auditClean:         "Integrity check: clean",
	auditRepair:        "🛠 Repair",
	auditStarted:       "🔍 Integrity audit started (job <code>%s</code>). Open this card again for the report.",
	auditRepairStarted: "🛠 Repair started (job <code>%s</code>).",
	errMsg: map[string]string{
		"destination_required":       "Choose a destination first (tap 📁 Choose where).",
		"ambiguous_source":           "That link is ambiguous. Send exactly one Drive link.",
		"source_not_found":           "That item was not found, or it is not shared with you.",
		"not_found":                  "That item was not found, or it is not shared with you.",
		"copy_not_permitted":         "The owner does not allow this file to be copied.",
		"unsupported_source":         "Shortcuts cannot be copied directly: open the target and send its link.",
		"destination_not_writable":   "This account cannot add files to that folder.",
		"capacity_exceeded":          "The queue is full, try again in a few minutes.",
		"rate_limited":               "Google is rate limiting, try again shortly.",
		"provider_unavailable":       "Google is not answering right now, try again later.",
		"account_reconnect_required": "Google needs reconnecting: press l in the TUI or run 502drive account login.",
		"forbidden":                  "You are not allowed to do that.",
		"not_a_folder":               "That item is not a folder.",
		"invalid_request":            "Invalid request.",
	},
	states: map[string]string{
		state.JobQueued: "Queued", state.JobRunning: "Copying", state.JobPausing: "Stop requested",
		state.JobPaused: "Paused", state.JobCancelling: "Cancel requested", state.JobReconciling: "Verifying",
		state.JobBlocked: "Needs action", state.JobSucceeded: "Done", state.JobPartial: "Partly done",
		state.JobFailed: "Failed", state.JobCancelled: "Cancelled",
	},
}

func (tx texts) stateLabel(s string) string {
	if v, ok := tx.states[s]; ok {
		return v
	}
	return s
}

// syncStateLabel names a subscription state; unknown values fall back to the
// raw state so a new server state is never hidden.
func (tx texts) syncStateLabel(s string) string {
	if v, ok := tx.syncStates[s]; ok {
		return v
	}
	return s
}

// scheduleSummary renders a stored schedule in the chat language.
func (tx texts) scheduleSummary(sp *schedule.Spec) string {
	if sp == nil {
		return tx.syncOff
	}
	switch sp.Kind {
	case schedule.KindInterval:
		if sp.IntervalMinutes%60 == 0 {
			return fmt.Sprintf("%s (%ds)", tx.every6h, sp.IntervalMinutes/60)
		}
		return fmt.Sprintf("%d phút", sp.IntervalMinutes)
	case schedule.KindDaily:
		return fmt.Sprintf("%s %s", sp.LocalTime, sp.Timezone)
	case schedule.KindWeekly:
		names := []string{"", "T2", "T3", "T4", "T5", "T6", "T7", "CN"}
		var days []string
		for _, d := range sp.Weekdays {
			if d >= 1 && d <= 7 {
				days = append(days, names[d])
			}
		}
		return fmt.Sprintf("%s %s %s", strings.Join(days, ","), sp.LocalTime, sp.Timezone)
	}
	return sp.Kind
}

func escape(s string) string { return html.EscapeString(s) }

// short clips a Drive name for a card title without breaking runes.
func short(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimSpace(string(r[:n])) + "…"
}

// jobText is the panel body for one job. Every Drive name is escaped, and the
// text only depends on the job row: the notification worker skips edits when
// the rendered text did not change.
// travelFrames is the loading animation of a running copy: a file travels from
// the source folder to the destination. The panel is edited at most once per
// edit interval, so each edit simply shows the next frame.
var travelFrames = []string{
	"📂 ➤ · · · · 📁",
	"📂 · ➤ · · · 📁",
	"📂 · · ➤ · · 📁",
	"📂 · · · ➤ · 📁",
	"📂 · · · · ➤ 📁",
}

func busyJob(state string) bool {
	switch state {
	case "queued", "running", "pausing", "cancelling", "reconciling":
		return true
	}
	return false
}

func stateEmoji(state string) string {
	switch state {
	case "succeeded":
		return "✅ "
	case "partial":
		return "⚠️ "
	case "failed":
		return "❌ "
	case "cancelled":
		return "⏹ "
	case "paused":
		return "⏸ "
	case "blocked":
		return "⛔ "
	}
	return ""
}

const barCells = 14

// progressBar is a real bar from counts: "▰▰▰▱▱ 42%".
func progressBar(done, total int) string {
	if total <= 0 {
		return ""
	}
	done = min(max(done, 0), total)
	fill := done * barCells / total
	return strings.Repeat("▰", fill) + strings.Repeat("▱", barCells-fill) + fmt.Sprintf(" %d%%", done*100/total)
}

func jobText(tx texts, j state.Job) string { return jobTextFrame(tx, j, 0) }

// jobTextFrame renders the job panel; frame picks the animation step.
func jobTextFrame(tx texts, j state.Job, frame int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<b>%s%s</b> — %s\n", stateEmoji(j.State), escape(short(j.RootName, 60)), tx.stateLabel(j.State))
	if busyJob(j.State) {
		fmt.Fprintf(&b, "%s\n", travelFrames[((frame%len(travelFrames))+len(travelFrames))%len(travelFrames)])
	}
	if j.Counts.Total != nil {
		if bar := progressBar(j.Counts.Completed, *j.Counts.Total); bar != "" {
			fmt.Fprintf(&b, "%s\n", bar)
		}
	}
	fmt.Fprintf(&b, "%s: <code>%s</code>\n", tx.jobLabel, escape(j.ID))
	total := tx.totalUnknown
	if j.Counts.Total != nil {
		total = fmt.Sprintf("%d", *j.Counts.Total)
	}
	fmt.Fprintf(&b, "%s: %d/%s · %s %d · %s %d\n", tx.itemsLabel, j.Counts.Completed, total,
		tx.stateLabel(state.JobFailed), j.Counts.Failed, tx.stateLabel(state.JobBlocked), j.Counts.Blocked)
	if j.ResultURL != nil && *j.ResultURL != "" {
		fmt.Fprintf(&b, "%s: %s\n", tx.resultLabel, escape(*j.ResultURL))
	}
	if j.LastError != nil {
		fmt.Fprintf(&b, "%s: %s (%s)\n", tx.problemLabel, escape(short(j.LastError.Message, 160)), escape(j.LastError.Code))
	}
	fmt.Fprintf(&b, "%s: %s", tx.updated, j.UpdatedAt.UTC().Format("2006-01-02 15:04"))
	return b.String()
}

// hashText keeps unchanged panels from being edited (Telegram rejects them
// anyway, but the budget is better spent elsewhere).
func hashText(s string) string { return fmt.Sprintf("%x", fnv64(s)) }

func fnv64(s string) uint64 {
	var h uint64 = 14695981039346656037
	for i := range s {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	return h
}
