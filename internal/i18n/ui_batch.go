package i18n

const (
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
)

func init() {
	vi := map[Key]string{
		AuditBrowserKeys:       "t lọc loại · n/p trang sau/trước · f xem trước sửa · F áp dụng · esc về nguồn",
		AuditAllKinds:          "Tất cả loại",
		AuditPreviewInfo:       "Bản xem trước dry-run: %d thao tác; chỉ mục của 502Drive.",
		QueueAhead:             "Đang chờ (còn %d trước)",
		SetupTitle:             "Bắt đầu",
		SetupDaemon:            "Daemon: chạy 502drive up",
		SetupOAuth:             "OAuth: tạo client Desktop tại https://console.cloud.google.com/apis/credentials rồi nhập file JSON (0600)",
		SetupAccount:           "Google: nhấn 5 rồi l để đăng nhập",
		SetupDestination:       "Thư mục đích: nhấn 7, chọn thư mục rồi d; hoặc nhấn 4 để đặt mặc định",
		SetupOAuthPrompt:       "Đường dẫn file JSON OAuth Desktop (0600)",
		SetupDestinationPrompt: "Link hoặc ID thư mục đích",
		SetupRestart:           "Đã lưu OAuth. Chạy 502drive stop rồi 502drive up để nạp cấu hình; chạy lại onboard.",
		SetupIncomplete:        "Chưa đủ điều kiện; hoàn thành các bước trong checklist.",
	}
	en := map[Key]string{
		AuditBrowserKeys:       "t filter kind · n/p next/previous · f preview repair · F apply · esc back",
		AuditAllKinds:          "All kinds",
		AuditPreviewInfo:       "Dry-run preview: %d actions; only 502Drive objects.",
		QueueAhead:             "Queued (%d ahead)",
		SetupTitle:             "Get started",
		SetupDaemon:            "Daemon: run 502drive up",
		SetupOAuth:             "OAuth: create a Desktop client at https://console.cloud.google.com/apis/credentials and supply its JSON file (0600)",
		SetupAccount:           "Google: press 5 then l to sign in",
		SetupDestination:       "Destination: press 7, choose a folder then d; or press 4 to set the default",
		SetupOAuthPrompt:       "Path to the OAuth Desktop JSON file (0600)",
		SetupDestinationPrompt: "Destination folder link or ID",
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
