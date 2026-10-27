package i18n

const (
	FilterYes              Key = "filter_yes"
	FilterNo               Key = "filter_no"
	FilterBadSize          Key = "filter_bad_size"
	FilterBadDate          Key = "filter_bad_date"
	FilterMinOverMax       Key = "filter_min_over_max"
	NotificationOpen       Key = "notification_open"
	LibraryFindTitle       Key = "library_find_title"
	LibraryFindScope       Key = "library_find_scope"
	BatchSummary           Key = "batch_summary"
	BatchViewIssues        Key = "batch_view_issues"
	BatchRetryFailed       Key = "batch_retry_failed"
	BatchRetryAccepted     Key = "batch_retry_accepted"
	QuotaCopied24h         Key = "quota_copied_24h"
	QuotaResumeAt          Key = "quota_resume_at"
	DeepTotalUnknown       Key = "deep_total_unknown"
	JobStatesChoose        Key = "job_states_choose"
	FilterInclude          Key = "filter_include"
	FilterExclude          Key = "filter_exclude"
	FilterMin              Key = "filter_min"
	FilterMax              Key = "filter_max"
	FilterModified         Key = "filter_modified"
	FilterKeep             Key = "filter_keep"
	FilterEditorHelp       Key = "filter_editor_help"
	FilterPreviewCounts    Key = "filter_preview_counts"
	FilterPreviewScope     Key = "filter_preview_scope"
	AuditBrowserKeys       Key = "audit_browser_keys"
	AuditAllKinds          Key = "audit_all_kinds"
	AuditPreviewInfo       Key = "audit_preview_info"
	QueueAhead             Key = "queue_ahead"
	SetupTitle             Key = "setup_title"
	SetupDaemon            Key = "setup_daemon"
	SetupOAuth             Key = "setup_oauth"
	SetupAccount           Key = "setup_account"
	SetupDestination       Key = "setup_destination"
	SetupOAuthPrompt       Key = "setup_oauth_prompt"
	SetupDestinationPrompt Key = "setup_destination_prompt"
	SetupRestart           Key = "setup_restart"
	SetupIncomplete        Key = "setup_incomplete"
	SetupReady             Key = "setup_ready"
)

func init() {
	vi := map[Key]string{
		NotificationOpen: "Mở",
		LibraryFindTitle: "Tìm thư viện: %s", LibraryFindScope: "Chỉ đọc · chỉ bộ sưu tập trong phạm vi được xem.",
		BatchSummary: "Lô %s: xong %d/%d; %d cần xử lý", BatchViewIssues: "Xem vấn đề", BatchRetryFailed: "Thử lại các job lỗi", BatchRetryAccepted: "Đã nhận yêu cầu thử lại các job lỗi.",
		QuotaCopied24h: "24 giờ qua đã copy %s", QuotaResumeAt: "tiếp tục lúc %s",
		DeepTotalUnknown: "chưa biết tổng",
		JobStatesChoose:  "Chọn trạng thái · Space bật/tắt · Enter áp dụng · Esc hủy (không chọn = tất cả)",
		FilterInclude:    "Bao gồm", FilterExclude: "Loại trừ", FilterMin: "Nhỏ nhất (vd 10MB)", FilterMax: "Lớn nhất (vd 2GB)", FilterModified: "Sửa sau ngày (YYYY-MM-DD)", FilterKeep: "Giữ thư mục rỗng",
		FilterYes: "Có", FilterNo: "Không", FilterBadSize: "Kích thước chưa đúng: nhập số kèm KB, MB, GB hoặc TB (vd 500KB, 10MB), hoặc để trống.",
		FilterBadDate: "Ngày chưa đúng: nhập dạng 2026-01-31, hoặc để trống.", FilterMinOverMax: "Nhỏ nhất đang lớn hơn lớn nhất.",
		FilterEditorHelp:    "Tab đổi ô · luật cách nhau ; (đường dẫn/glob, ext:, class:) · Ctrl+P xem trước · Ctrl+S lưu · Esc về",
		FilterPreviewCounts: "Khớp %d · loại %d", FilterPreviewScope: "Phạm vi xem trước (không phải tổng cây):",
		AuditBrowserKeys:       "t lọc loại · n/p trang sau/trước · f xem trước sửa · F áp dụng · esc về nguồn",
		AuditAllKinds:          "Tất cả loại",
		AuditPreviewInfo:       "Bản xem trước dry-run: %d thao tác; chỉ mục của 502Drive.",
		QueueAhead:             "Đang chờ (còn %d trước)",
		SetupTitle:             "Bắt đầu",
		SetupDaemon:            "Daemon: chạy 502drive up",
		SetupOAuth:             "OAuth: tạo client Desktop tại https://console.cloud.google.com/apis/credentials rồi nhập file JSON vừa tải về",
		SetupAccount:           "Google: nhấn 5 rồi l để đăng nhập",
		SetupDestination:       "Thư mục đích: nhấn 7, chọn thư mục rồi d; hoặc nhấn 4 để đặt mặc định",
		SetupOAuthPrompt:       "Đường dẫn file JSON OAuth Desktop (kéo thả vào đây cũng được)",
		SetupDestinationPrompt: "Link thư mục đích (Enter = My Drive)",
		SetupReady:             "Xong! Dán một link Drive để copy: 502d LINK, hoặc mở giao diện: 502drive",
		SetupRestart:           "Đã lưu OAuth. Chạy 502drive stop rồi 502drive up để nạp cấu hình; chạy lại onboard.",
		SetupIncomplete:        "Chưa đủ điều kiện; hoàn thành các bước trong checklist.",
	}
	en := map[Key]string{
		NotificationOpen: "Open",
		LibraryFindTitle: "Library search: %s", LibraryFindScope: "Read only · collections within your permitted scope.",
		BatchSummary: "Batch %s: done %d/%d; %d need action", BatchViewIssues: "View issues", BatchRetryFailed: "Retry failed jobs", BatchRetryAccepted: "Retry requests accepted for failed jobs.",
		QuotaCopied24h: "Copied %s in the last 24 hours", QuotaResumeAt: "resumes at %s",
		DeepTotalUnknown: "total unknown",
		JobStatesChoose:  "Choose states · Space toggle · Enter apply · Esc cancel (none = all)",
		FilterInclude:    "Include", FilterExclude: "Exclude", FilterMin: "Minimum size (e.g. 10MB)", FilterMax: "Maximum size (e.g. 2GB)", FilterModified: "Modified after (YYYY-MM-DD)", FilterKeep: "Keep empty folders",
		FilterYes: "Yes", FilterNo: "No", FilterBadSize: "Enter a size with KB, MB, GB or TB (e.g. 500KB, 10MB), or leave it empty.",
		FilterBadDate: "Enter a date like 2026-01-31, or leave it empty.", FilterMinOverMax: "The minimum size is larger than the maximum.",
		FilterEditorHelp:    "Tab next field · separate rules by ; (path/name glob, ext:, class:) · Ctrl+P preview · Ctrl+S save · Esc back",
		FilterPreviewCounts: "Matched %d · excluded %d", FilterPreviewScope: "Preview scope (not a whole-tree total):",
		AuditBrowserKeys:       "t filter kind · n/p next/previous · f preview repair · F apply · esc back",
		AuditAllKinds:          "All kinds",
		AuditPreviewInfo:       "Dry-run preview: %d actions; only 502Drive objects.",
		QueueAhead:             "Queued (%d ahead)",
		SetupTitle:             "Get started",
		SetupDaemon:            "Daemon: run 502drive up",
		SetupOAuth:             "OAuth: create a Desktop client at https://console.cloud.google.com/apis/credentials and give the downloaded JSON file",
		SetupAccount:           "Google: press 5 then l to sign in",
		SetupDestination:       "Destination: press 7, choose a folder then d; or press 4 to set the default",
		SetupOAuthPrompt:       "Path to the OAuth Desktop JSON file (drag and drop works)",
		SetupDestinationPrompt: "Destination folder link (Enter = My Drive)",
		SetupReady:             "Ready. Copy a Drive link with: 502d LINK, or open the app: 502drive",
		SetupRestart:           "OAuth saved. Run 502drive stop then 502drive up to reload configuration; run onboard again.",
		SetupIncomplete:        "Setup incomplete; finish the checklist steps.",
	}
	for k, v := range vi {
		VI[k] = v
	}
	for k, v := range en {
		EN[k] = v
	}
}
