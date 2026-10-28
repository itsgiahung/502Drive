package i18n

// Plain-language texts for the stable error codes of the daemon, shown in the
// app instead of the daemon's English sentence. A code without an entry keeps
// the daemon's own message.

var errorsVI = map[string]string{
	"invalid_transition":         "Thao tác này chưa dùng được ở trạng thái hiện tại (ví dụ job đang chạy hoặc bị chặn, hoặc bản copy đã ngừng theo dõi). Xem trạng thái ở Thư viện hoặc Công việc rồi thử lại.",
	"stale_revision":             "Dữ liệu vừa đổi ở nơi khác. Màn hình đã được làm mới, hãy thử lại.",
	"collection_busy":            "Bản copy này đang có job chạy, tạm dừng hoặc bị chặn. Chờ nó xong hoặc hủy job trước.",
	"not_found":                  "Không tìm thấy mục này (có thể đã bị xóa hoặc không còn quyền).",
	"forbidden":                  "Chỉ chủ sở hữu mới làm được việc này.",
	"unauthenticated":            "Phiên làm việc không hợp lệ. Chạy lại 502drive up.",
	"account_reconnect_required": "Cần đăng nhập lại Google (phím l ở màn hình chính).",
	"destination_required":       "Chưa chọn thư mục đích. Thêm một thư mục ở tab Đích.",
	"capacity_exceeded":          "Hàng đợi đang đầy. Chờ bớt job rồi thử lại.",
	"plan_expired":               "Bản xem trước đã hết hạn. Hãy xem trước lại.",
	"plan_changed":               "Nguồn đã đổi sau bản xem trước. Hãy xem trước lại.",
	"copy_not_permitted":         "Nguồn không cho phép copy; không có cách vượt qua.",
	"destination_not_writable":   "Không ghi được vào thư mục đích. Xin quyền thêm file hoặc chọn thư mục khác.",
	"daemon_unavailable":         "Chưa kết nối được dịch vụ nền. Chạy 502drive up.",
	"not_ready":                  "Dịch vụ chưa sẵn sàng. Thử lại sau vài giây.",
	"unknown_remote_outcome":     "Chưa biết thao tác trước đã xong chưa; hệ thống đang đối soát. Không cần làm gì.",
	"snapshot_protected":         "Còn điểm khôi phục cần bản copy này nên chưa dọn được.",
	"destination_root_trashed":   "Thư mục chứa bản copy đang trong thùng rác Drive. Khôi phục nó rồi chạy lại.",
	"destination_root_missing":   "Thư mục chứa bản copy không còn. Khôi phục nó hoặc ngừng theo dõi bản copy này.",
	"already_copied":             "Nguồn này đã được copy. Chọn cập nhật hoặc tạo bản mới.",
	"invalid_request":            "Yêu cầu không hợp lệ.",
}

var errorsEN = map[string]string{
	"invalid_transition":         "That action is not available right now (for example a job is running or blocked, or the copy is no longer tracked). Check Library or Jobs and try again.",
	"stale_revision":             "The data changed elsewhere. The screen was refreshed, try again.",
	"collection_busy":            "This copy has a job running, paused or blocked. Wait for it or cancel it first.",
	"not_found":                  "That item was not found (it may be gone, or you lost access).",
	"forbidden":                  "Only the owner can do this.",
	"unauthenticated":            "The session is not valid. Run 502drive up again.",
	"account_reconnect_required": "Sign in to Google again (press l on Home).",
	"destination_required":       "No destination folder yet. Add one on the Destination tab.",
	"capacity_exceeded":          "The queue is full. Wait for jobs to finish and try again.",
	"plan_expired":               "The preview expired. Preview again.",
	"plan_changed":               "The source changed after the preview. Preview again.",
	"copy_not_permitted":         "The source does not allow copying; there is no way around that.",
	"destination_not_writable":   "Cannot write to the destination. Ask for edit access or pick another folder.",
	"daemon_unavailable":         "Cannot reach the background service. Run 502drive up.",
	"not_ready":                  "The service is not ready. Try again in a few seconds.",
	"unknown_remote_outcome":     "It is not known whether the last step finished; it is being reconciled. Nothing to do.",
	"snapshot_protected":         "A restore point still needs this copy, so it cannot be cleaned up yet.",
	"destination_root_trashed":   "The folder holding the copy is in the Drive trash. Restore it and run again.",
	"destination_root_missing":   "The folder holding the copy is gone. Restore it or stop tracking this copy.",
	"already_copied":             "This source was already copied. Choose update or a new copy.",
	"invalid_request":            "The request is not valid.",
}

// ErrorText is the plain-language sentence for a stable error code in lang
// ("vi" or "en"), and whether one exists.
func ErrorText(lang, code string) (string, bool) {
	m := errorsEN
	if lang != "en" {
		m = errorsVI
	}
	s, ok := m[code]
	return s, ok
}
