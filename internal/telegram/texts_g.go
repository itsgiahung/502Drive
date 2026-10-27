package telegram

import "fmt"

// Strings added by batch G (collection lifecycle, copy options, job management,
// notification settings, daily report, /find, error advice). They follow the
// same rule as render.go: every key exists in Vietnamese and English (a test
// compares both), and Drive names are escaped by the caller.

// gTexts is the batch-G string table.
type gTexts struct {
	// Preview options: selective-sync presets (S5) and name conflicts (N1).
	presetNoVideo, presetDocs, presetSmall string
	filterLine, filterScope, filterSamples string
	conflictFmt                            string
	conflictNames                          map[string]string

	// Copying a source that was already copied (L2).
	alreadyCopied, updateOld, createNew string

	// Collection lifecycle (L1).
	retireBtn, retireAsk, retireConfirm, retireDone       string
	resumeBtn, resumeDone                                 string
	forgetBtn, forgetAsk, forgetConfirm, forgetDone       string
	cleanupBtn, cleanupAsk, cleanupConfirm, cleanupBegan  string
	confirmExpired, retiredTag, retiredList, retiredEmpty string
	retiredButton                                         string

	// Results.
	skipTitle  string
	skipLabels map[string]string
	quotaWait  string
	quotaDue   string

	// Job management (S10).
	manageBtn, manageTitle, manageEmpty, pagePrev string
	filterNames                                   map[string]string
	batchPause, batchResume, batchRetry           string
	batchCancel, batchCancelAsk, batchCancelNo    string
	batchResult                                   string
	batchActions                                  map[string]string

	// Notification settings, quiet hours and the daily report (S10).
	notifTitle, notifFmt string
	notifNames           map[string]string
	quietFmt, quietNone  string
	reportFmt, reportOff string
	reportBody           reportTexts
	reportSettingsHint   string
	reportBtnOff         string

	// /find (S10).
	findUsage, findTitle, findNone, findScope, findMore string

	// Error advice.
	busyMsg        string
	errMsg         map[string]string
	errHint        map[string]string
	errHintDefault string
}

// reportTexts are the lines of the daily report.
type reportTexts struct {
	title, done, failed, blocked, sync, bytes, speed, footer string
}

func (tx texts) g() gTexts {
	if tx.lang == langEN {
		return gEN
	}
	return gVI
}

func (g gTexts) skipLabel(code string) string {
	if v, ok := g.skipLabels[code]; ok {
		return v
	}
	return code
}

func (g gTexts) conflictName(policy string) string {
	name, ok := g.conflictNames[policy]
	if !ok {
		name = g.conflictNames["rename"]
	}
	return fmt.Sprintf(g.conflictFmt, name)
}

var gVI = gTexts{
	presetNoVideo: "🎬 Bỏ video", presetDocs: "📄 Chỉ tài liệu", presetSmall: "📦 Bỏ file > 1 GB",
	filterLine:    "🧮 Đạt: %d · Bị loại: %d",
	filterScope:   "Chỉ tính cấp cao nhất của nguồn",
	filterSamples: "Mẫu bị loại",
	conflictFmt:   "🔁 Trùng tên: %s",
	conflictNames: map[string]string{"rename": "đổi tên", "skip": "bỏ qua", "update": "ghi cùng tên"},

	alreadyCopied: "<b>Nguồn này đã được copy tới thư mục đích.</b>\nCập nhật bản cũ (chỉ chép phần thay đổi) hay tạo thêm một bản mới?",
	updateOld:     "🔄 Cập nhật bản cũ", createNew: "🆕 Tạo bản mới",

	retireBtn:      "⏹ Ngừng theo dõi",
	retireAsk:      "<b>Ngừng theo dõi «%s»?</b>\nTắt đồng bộ và lịch chạy. File trên Drive, lịch sử và ánh xạ vẫn giữ; có thể theo dõi lại sau.\nXác nhận trong 5 phút.",
	retireConfirm:  "⏹ Xác nhận ngừng",
	retireDone:     "Đã ngừng theo dõi.",
	resumeBtn:      "▶️ Theo dõi lại",
	resumeDone:     "Đã theo dõi lại.",
	forgetBtn:      "🗑 Quên",
	forgetAsk:      "<b>Quên «%s»?</b>\nXoá bản ghi nội bộ (lịch sử, ánh xạ). Google Drive không bị thay đổi và việc này không hoàn tác được.\nXác nhận trong 5 phút.",
	forgetConfirm:  "🗑 Xác nhận quên",
	forgetDone:     "Đã quên bản ghi.",
	cleanupBtn:     "🧹 Dọn đích (vào thùng rác)",
	cleanupAsk:     "<b>Dọn đích «%s»?</b>\nThư mục đích do 502Drive tạo sẽ được chuyển vào thùng rác Google Drive (khôi phục được trong 30 ngày), không xoá vĩnh viễn.\nĐã chạy bản xem trước (công việc <code>%s</code>): mở công việc để xem số mục.\nXác nhận trong 5 phút.",
	cleanupConfirm: "🗑 Chuyển vào thùng rác",
	cleanupBegan:   "🧹 Đã bắt đầu dọn đích (công việc <code>%s</code>).",
	confirmExpired: "Xác nhận đã hết hạn, chưa có gì thay đổi. Mở lại thẻ để làm lại.",
	retiredTag:     "⏹ Đã ngừng theo dõi",
	retiredList:    "Nguồn đã ngừng theo dõi",
	retiredEmpty:   "Không có nguồn nào đã ngừng.",
	retiredButton:  "🗄 Đã ngừng (%d)",

	skipTitle: "Bỏ qua",
	skipLabels: map[string]string{
		"filtered": "bị bộ lọc loại", "conflict": "trùng tên/nội dung", "shortcut_unresolved": "shortcut không mở được",
		"shortcut_loop": "shortcut lặp vòng", "cannot_copy": "chủ sở hữu không cho copy",
		"google_native_export_only": "định dạng chỉ export", "too_large_for_quota": "quá lớn so với dung lượng",
		"unreadable": "không đọc được", "trashed_source": "nguồn trong thùng rác", "unknown": "không rõ lý do",
	},
	quotaWait: "⏳ Chờ quota tới %s (%s). Sẽ tự tiếp tục, không cần thử lại.",
	quotaDue:  "⏳ Đang chờ quota, sắp tự tiếp tục.",

	manageBtn: "🗂 Quản lý", manageTitle: "<b>Quản lý công việc</b> — %s (%d)",
	manageEmpty: "Không có công việc nào ở nhóm này.", pagePrev: "⬅️ Trang trước",
	filterNames: map[string]string{"all": "Tất cả", "running": "Đang chạy", "queued": "Chờ", "paused": "Tạm dừng",
		"blocked": "Cần xử lý", "failed": "Lỗi", "done": "Xong"},
	batchPause: "⏸ Tạm dừng %d", batchResume: "▶️ Tiếp tục %d", batchRetry: "🔁 Thử lại %d", batchCancel: "🧹 Huỷ %d",
	batchCancelAsk: "<b>Huỷ %d công việc (%s)?</b>\nBản đã copy xong vẫn giữ nguyên. Xác nhận trong 5 phút.",
	batchCancelNo:  "Không còn công việc nào để huỷ.",
	batchResult:    "%s: xong %d, bỏ qua %d, lỗi %d.",
	batchActions:   map[string]string{"pause": "Tạm dừng", "resume": "Tiếp tục", "retry": "Thử lại", "cancel": "Huỷ"},

	notifTitle: "🔔 Thông báo", notifFmt: "%s: %s",
	notifNames: map[string]string{"completed": "Xong", "failed": "Lỗi", "action": "Cần xử lý"},
	quietFmt:   "🌙 Giờ yên tĩnh %s–%s: %s", quietNone: "Giờ yên tĩnh: tắt",
	reportFmt: "📰 Báo cáo hằng ngày: %s", reportOff: "Tắt", reportBtnOff: "Tắt báo cáo",
	reportSettingsHint: "Báo cáo chỉ gửi khi có việc đáng nói, theo múi giờ %s.",
	reportBody: reportTexts{
		title: "📰 <b>Báo cáo 24 giờ</b> — %s", done: "✅ Xong: %d", failed: "❌ Lỗi hoặc xong một phần: %d",
		blocked: "⛔ Cần xử lý: %d", sync: "🔄 Lần đồng bộ: %d", bytes: "📊 Đã copy: %s",
		speed: "⚡ Tốc độ trung bình: %s/s", footer: "Mở Công việc để xử lý.",
	},

	findUsage: "Gửi <code>/find tên</code> để tìm trong Drive, ví dụ <code>/find báo cáo</code>.",
	findTitle: "🔎 <b>Kết quả cho «%s»</b>", findNone: "Không thấy mục nào khớp.",
	findScope: "Tìm trong toàn bộ Drive, tối đa 20 kết quả.", findMore: "+%d mục nữa, hãy gõ tên cụ thể hơn.",

	busyMsg: "Đang có công việc chạy trên mục này (%s).",
	errMsg: map[string]string{
		"stale_revision":         "Thẻ này đã cũ vì mục vừa thay đổi.",
		"plan_changed":           "Thư mục đích đổi sau khi xem trước.",
		"plan_expired":           "Bản xem trước đã hết hạn.",
		"collection_busy":        "Đang có công việc chạy trên mục này.",
		"collection_not_retired": "Mục này chưa ngừng theo dõi.",
		"collection_retired":     "Bản copy cũ đã ngừng theo dõi.",
		"already_copied":         "Nguồn này đã được copy tới đích.",
		"invalid_transition":     "Thao tác này không dùng được ở trạng thái hiện tại.",
		"needs_action":           "Bản copy cần được xử lý trước khi đồng bộ.",
		"permission_denied":      "Tài khoản này không có quyền với mục đó.",
		"invalid_filter":         "Bộ lọc không hợp lệ.",
		"internal_error":         "Có lỗi nội bộ.",
		"quota_exceeded":         "Google đã hết hạn mức tạm thời.",
	},
	errHint: map[string]string{
		"stale_revision":             "Mở lại thẻ rồi thử lại.",
		"plan_changed":               "Gửi lại link để xem trước với đích mới.",
		"plan_expired":               "Gửi lại link để xem trước lại.",
		"capacity_exceeded":          "Đợi vài phút hoặc huỷ bớt công việc ở trang Công việc.",
		"rate_limited":               "Không cần làm gì; thử lại sau ít phút.",
		"provider_unavailable":       "Kiểm tra mạng của máy chạy 502Drive rồi thử lại.",
		"account_reconnect_required": "Chạy 502drive account login trên máy chạy daemon.",
		"collection_busy":            "Đợi công việc đó xong hoặc huỷ nó ở trang Công việc, rồi thử lại.",
		"collection_not_retired":     "Ngừng theo dõi trước rồi mới quên hoặc dọn.",
		"collection_retired":         "Theo dõi lại ở mục Đã ngừng, hoặc chọn Tạo bản mới.",
		"already_copied":             "Chọn Cập nhật bản cũ hoặc Tạo bản mới.",
		"invalid_transition":         "Mở lại thẻ để xem trạng thái mới nhất.",
		"needs_action":               "Mở thẻ nguồn và chạy kiểm tra toàn vẹn.",
		"forbidden":                  "Hỏi chủ sở hữu bot nếu bạn cần quyền này.",
		"permission_denied":          "Nhờ chủ sở hữu chia sẻ mục đó cho tài khoản Google đã kết nối.",
		"destination_required":       "Nhấn Chọn nơi lưu rồi chọn thư mục.",
		"destination_not_writable":   "Chọn thư mục khác hoặc xin quyền chỉnh sửa.",
		"copy_not_permitted":         "Nhờ chủ sở hữu bật quyền copy hoặc cấp quyền chỉnh sửa.",
		"not_found":                  "Kiểm tra lại link hoặc quyền chia sẻ.",
		"source_not_found":           "Kiểm tra lại link hoặc quyền chia sẻ.",
		"quota_exceeded":             "Công việc tự tiếp tục khi Google cho phép; không cần thử lại.",
		"invalid_filter":             "Tắt bộ lọc rồi bật lại.",
		"internal_error":             "Thử lại; nếu vẫn lỗi, chạy 502drive doctor trên máy daemon.",
	},
	errHintDefault: "Thử lại; nếu vẫn lỗi, chạy 502drive doctor trên máy daemon.",
}

var gEN = gTexts{
	presetNoVideo: "🎬 Skip videos", presetDocs: "📄 Documents only", presetSmall: "📦 Skip files > 1 GB",
	filterLine:    "🧮 Kept: %d · Excluded: %d",
	filterScope:   "Counts the top level of the source only",
	filterSamples: "Excluded samples",
	conflictFmt:   "🔁 If the name exists: %s",
	conflictNames: map[string]string{"rename": "rename", "skip": "skip", "update": "same name"},

	alreadyCopied: "<b>This source was already copied to this destination.</b>\nUpdate the old copy (only changes are copied) or make another new copy?",
	updateOld:     "🔄 Update the old copy", createNew: "🆕 Make a new copy",

	retireBtn:      "⏹ Stop following",
	retireAsk:      "<b>Stop following «%s»?</b>\nTurns off sync and the schedule. Files on Drive, history and mapping are kept; you can follow it again later.\nConfirm within 5 minutes.",
	retireConfirm:  "⏹ Confirm stop",
	retireDone:     "Stopped following.",
	resumeBtn:      "▶️ Follow again",
	resumeDone:     "Following again.",
	forgetBtn:      "🗑 Forget",
	forgetAsk:      "<b>Forget «%s»?</b>\nDeletes the internal records (history, mapping). Google Drive is not changed and this cannot be undone.\nConfirm within 5 minutes.",
	forgetConfirm:  "🗑 Confirm forget",
	forgetDone:     "Records forgotten.",
	cleanupBtn:     "🧹 Clean up destination (to trash)",
	cleanupAsk:     "<b>Clean up «%s»?</b>\nThe destination folder created by 502Drive moves to the Google Drive trash (restorable for 30 days); nothing is deleted permanently.\nA preview already ran (job <code>%s</code>): open the job to see the item count.\nConfirm within 5 minutes.",
	cleanupConfirm: "🗑 Move to trash",
	cleanupBegan:   "🧹 Cleanup started (job <code>%s</code>).",
	confirmExpired: "The confirmation expired and nothing changed. Open the card again to retry.",
	retiredTag:     "⏹ Stopped following",
	retiredList:    "Sources no longer followed",
	retiredEmpty:   "No stopped sources.",
	retiredButton:  "🗄 Stopped (%d)",

	skipTitle: "Skipped",
	skipLabels: map[string]string{
		"filtered": "excluded by the filter", "conflict": "same name/content", "shortcut_unresolved": "shortcut cannot be opened",
		"shortcut_loop": "shortcut loop", "cannot_copy": "owner disallows copying",
		"google_native_export_only": "export-only format", "too_large_for_quota": "too large for the quota",
		"unreadable": "unreadable", "trashed_source": "source in trash", "unknown": "unknown reason",
	},
	quotaWait: "⏳ Waiting for quota until %s (%s). It resumes by itself; no retry needed.",
	quotaDue:  "⏳ Waiting for quota, resuming shortly.",

	manageBtn: "🗂 Manage", manageTitle: "<b>Manage jobs</b> — %s (%d)",
	manageEmpty: "No jobs in this group.", pagePrev: "⬅️ Previous page",
	filterNames: map[string]string{"all": "All", "running": "Running", "queued": "Queued", "paused": "Paused",
		"blocked": "Needs action", "failed": "Failed", "done": "Done"},
	batchPause: "⏸ Pause %d", batchResume: "▶️ Resume %d", batchRetry: "🔁 Retry %d", batchCancel: "🧹 Cancel %d",
	batchCancelAsk: "<b>Cancel %d job(s) (%s)?</b>\nAnything already copied stays. Confirm within 5 minutes.",
	batchCancelNo:  "No jobs left to cancel.",
	batchResult:    "%s: done %d, skipped %d, failed %d.",
	batchActions:   map[string]string{"pause": "Pause", "resume": "Resume", "retry": "Retry", "cancel": "Cancel"},

	notifTitle: "🔔 Notifications", notifFmt: "%s: %s",
	notifNames: map[string]string{"completed": "Finished", "failed": "Failed", "action": "Needs action"},
	quietFmt:   "🌙 Quiet hours %s–%s: %s", quietNone: "Quiet hours: off",
	reportFmt: "📰 Daily report: %s", reportOff: "Off", reportBtnOff: "Turn the report off",
	reportSettingsHint: "The report is only sent when there is something to say, in the %s time zone.",
	reportBody: reportTexts{
		title: "📰 <b>24-hour report</b> — %s", done: "✅ Finished: %d", failed: "❌ Failed or partly done: %d",
		blocked: "⛔ Needs action: %d", sync: "🔄 Sync runs: %d", bytes: "📊 Copied: %s",
		speed: "⚡ Average speed: %s/s", footer: "Open Jobs to deal with them.",
	},

	findUsage: "Send <code>/find name</code> to search Drive, for example <code>/find report</code>.",
	findTitle: "🔎 <b>Results for «%s»</b>", findNone: "Nothing matched.",
	findScope: "Searches all of Drive, up to 20 results.", findMore: "+%d more, type a more specific name.",

	busyMsg: "A job is running on this item (%s).",
	errMsg: map[string]string{
		"stale_revision":         "This card is out of date because the item just changed.",
		"plan_changed":           "The destination changed after the preview.",
		"plan_expired":           "The preview expired.",
		"collection_busy":        "A job is running on this item.",
		"collection_not_retired": "This item is not stopped yet.",
		"collection_retired":     "The old copy is no longer followed.",
		"already_copied":         "This source was already copied to the destination.",
		"invalid_transition":     "That action is not available in the current state.",
		"needs_action":           "The copy needs attention before it can sync.",
		"permission_denied":      "This account has no access to that item.",
		"invalid_filter":         "The filter is not valid.",
		"internal_error":         "Something went wrong inside.",
		"quota_exceeded":         "Google is out of quota for now.",
	},
	errHint: map[string]string{
		"stale_revision":             "Open the card again and retry.",
		"plan_changed":               "Send the link again for a fresh preview with the new destination.",
		"plan_expired":               "Send the link again for a fresh preview.",
		"capacity_exceeded":          "Wait a few minutes or cancel some jobs on the Jobs page.",
		"rate_limited":               "Nothing to do; try again in a few minutes.",
		"provider_unavailable":       "Check the network of the machine running 502Drive and try again.",
		"account_reconnect_required": "Run 502drive account login on the daemon machine.",
		"collection_busy":            "Wait for that job to finish or cancel it on the Jobs page, then retry.",
		"collection_not_retired":     "Stop following it first, then forget or clean up.",
		"collection_retired":         "Follow it again from the Stopped list, or choose Make a new copy.",
		"already_copied":             "Choose Update the old copy or Make a new copy.",
		"invalid_transition":         "Open the card again to see the latest state.",
		"needs_action":               "Open the source card and run the integrity check.",
		"forbidden":                  "Ask the bot owner if you need this permission.",
		"permission_denied":          "Ask the owner to share that item with the connected Google account.",
		"destination_required":       "Tap Choose where and pick a folder.",
		"destination_not_writable":   "Pick another folder or ask for edit access.",
		"copy_not_permitted":         "Ask the owner to allow copying or to grant edit access.",
		"not_found":                  "Check the link and the sharing permissions.",
		"source_not_found":           "Check the link and the sharing permissions.",
		"quota_exceeded":             "The job resumes by itself when Google allows; no retry needed.",
		"invalid_filter":             "Turn the filter off and on again.",
		"internal_error":             "Retry; if it persists, run 502drive doctor on the daemon machine.",
	},
	errHintDefault: "Retry; if it still fails, run 502drive doctor on the daemon machine.",
}

// reportChoices are the report times offered as buttons (wall clock in the
// preferences time zone).
var reportChoices = []string{"07:00", "12:00", "18:00", "21:00"}

// Advice for the failures that jobs and the copy flow report. They are merged
// into the tables above at start-up so each language reads as one list here.
var (
	moreErrMsgVI = map[string]string{
		"destination_inside_source": "Thư mục đích không được nằm trong nguồn.",
		"destination_unverifiable":  "Chưa kiểm tra được thư mục đích.",
		"retry_exhausted":           "Đã thử nhiều lần mà chưa được.",
		"source_mass_removal":       "Nguồn mất nhiều mục so với lần trước.",
		"unknown_remote_outcome":    "Chưa rõ Drive đã nhận lệnh ghi hay chưa.",
		"name_exhausted":            "Hết tên khả dụng ở thư mục đích.",
		"intent_missing":            "Thư mục đích không do 502Drive tạo.",
	}
	moreErrHintVI = map[string]string{
		"ambiguous_source":          "Gửi đúng một link Drive.",
		"unsupported_source":        "Mở mục đích của shortcut rồi gửi link của nó.",
		"not_a_folder":              "Chọn một thư mục.",
		"invalid_request":           "Kiểm tra lại nội dung vừa gửi rồi thử lại.",
		"destination_inside_source": "Chọn thư mục đích nằm ngoài nguồn.",
		"destination_unverifiable":  "Chọn lại thư mục đích bằng Chọn nơi lưu.",
		"idempotency_mismatch":      "Mở lại thẻ rồi thử lại.",
		"retry_exhausted":           "Nhấn Thử lại; nếu vẫn lỗi, kiểm tra quyền và dung lượng của thư mục đích.",
		"source_mass_removal":       "Chạy Kiểm tra toàn vẹn trên thẻ nguồn trước khi chạy lại.",
		"unknown_remote_outcome":    "Nhấn Thử lại: 502Drive đối chiếu với Drive trước nên không copy trùng.",
		"name_exhausted":            "Đổi tên thư mục kết quả hoặc dọn các tên trùng ở đích.",
		"intent_missing":            "Không dọn tự động được; nếu cần, xoá tay trên Drive.",
		"trash_failed":              "Thử lại sau; nếu vẫn lỗi, kiểm tra kết nối và quyền ghi.",
		"drive_error":               "Thử lại sau; nếu vẫn lỗi, kiểm tra kết nối và quyền.",
		"audit_missing":             "Mục liên quan đã bị xoá; mở lại thẻ nguồn.",
		"collection_missing":        "Mục liên quan đã bị xoá; mở lại thẻ nguồn.",
	}
	moreErrMsgEN = map[string]string{
		"destination_inside_source": "The destination cannot be inside the source.",
		"destination_unverifiable":  "The destination could not be verified.",
		"retry_exhausted":           "It was tried several times without success.",
		"source_mass_removal":       "The source lost many items since the last run.",
		"unknown_remote_outcome":    "It is not known whether Drive accepted the write.",
		"name_exhausted":            "No free name is left in the destination.",
		"intent_missing":            "The destination folder was not created by 502Drive.",
	}
	moreErrHintEN = map[string]string{
		"ambiguous_source":          "Send exactly one Drive link.",
		"unsupported_source":        "Open the shortcut's target and send its link.",
		"not_a_folder":              "Pick a folder.",
		"invalid_request":           "Check what you sent and try again.",
		"destination_inside_source": "Pick a destination outside the source.",
		"destination_unverifiable":  "Pick the destination again with Choose where.",
		"idempotency_mismatch":      "Open the card again and retry.",
		"retry_exhausted":           "Tap Retry; if it still fails, check permissions and space in the destination.",
		"source_mass_removal":       "Run the integrity check on the source card before running again.",
		"unknown_remote_outcome":    "Tap Retry: 502Drive reconciles with Drive first, so nothing is copied twice.",
		"name_exhausted":            "Rename the output folder or clean up the duplicate names in the destination.",
		"intent_missing":            "It cannot be cleaned up automatically; delete it by hand on Drive if needed.",
		"trash_failed":              "Try again later; if it still fails, check the connection and write access.",
		"drive_error":               "Try again later; if it still fails, check the connection and access.",
		"audit_missing":             "The related item was removed; open the source card again.",
		"collection_missing":        "The related item was removed; open the source card again.",
	}
)

func init() {
	for k, v := range moreErrMsgVI {
		gVI.errMsg[k] = v
	}
	for k, v := range moreErrHintVI {
		gVI.errHint[k] = v
	}
	for k, v := range moreErrMsgEN {
		gEN.errMsg[k] = v
	}
	for k, v := range moreErrHintEN {
		gEN.errHint[k] = v
	}
}
