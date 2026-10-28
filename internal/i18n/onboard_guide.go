package i18n

// Onboard guide strings: the step-by-step Google Cloud walkthrough printed by
// `502drive onboard` in a terminal. Kept in their own file so the guide can
// change without touching the main tables.
const (
	OnboardAskPath       Key = "onboard_ask_path"
	OnboardDownloadsUse  Key = "onboard_downloads_use"
	OnboardGuideIntro    Key = "onboard_guide_intro"
	OnboardGuideKeys     Key = "onboard_guide_keys"
	OnboardGuideProject  Key = "onboard_guide_project"
	OnboardGuideDriveAPI Key = "onboard_guide_drive_api"
	OnboardGuideConsent  Key = "onboard_guide_consent"
	OnboardGuideClient   Key = "onboard_guide_client"
	OnboardGuideDownload Key = "onboard_guide_download"
	OnboardGuideQuit     Key = "onboard_guide_quit"
	OnboardPathOnly      Key = "onboard_path_only"
	OnboardPathRetry     Key = "onboard_path_retry"
	OnboardPathFailed    Key = "onboard_path_failed"
)

func init() {
	vi := map[Key]string{
		OnboardAskPath:       "Dán đường dẫn file JSON (nếu đã có), hoặc nhấn g để được hướng dẫn từng bước",
		OnboardDownloadsUse:  "Tìm thấy file OAuth mới trong ~/Downloads:\n  %s\nDùng file này? [Y/n]",
		OnboardGuideIntro:    "Hướng dẫn tạo khóa Google (5 bước). Mỗi bước mở một trang Google Cloud; làm xong thì quay lại đây.",
		OnboardGuideKeys:     "Enter = xong bước này · s = bỏ qua · q = thoát · hoặc dán đường dẫn file JSON để kết thúc",
		OnboardGuideProject:  "1/5 Tạo hoặc chọn project: nếu đã có project dùng được thì chỉ cần chọn nó.",
		OnboardGuideDriveAPI: "2/5 Bật Google Drive API cho project (bấm Enable).",
		OnboardGuideConsent:  "3/5 Màn hình đồng ý: chọn External ở Branding. Thêm email của bạn làm Test user tại https://console.cloud.google.com/auth/audience, rồi bấm Publish app (In production) để đăng nhập không hết hạn sau 7 ngày. Khi đăng nhập Google sẽ cảnh báo ứng dụng chưa xác minh: chọn Advanced > Go to ... (đây là khóa của chính bạn).",
		OnboardGuideClient:   "4/5 Tạo OAuth client loại Desktop app (Create client → Desktop app).",
		OnboardGuideDownload: "5/5 Tải file JSON của client vừa tạo rồi kéo-thả file đó vào terminal này.",
		OnboardGuideQuit:     "Đã dừng hướng dẫn. Chạy lại `502drive onboard` khi bạn có file JSON.",
		OnboardPathOnly:      "Đường dẫn file client_secret….json vừa tải",
		OnboardPathRetry:     "Không dùng được file: %v\nDán lại đường dẫn (còn %d lần thử).",
		OnboardPathFailed:    "Không dùng được file OAuth sau 3 lần: %v",
	}
	en := map[Key]string{
		OnboardAskPath:       "Paste the JSON file path (if you already have one), or press g for the step-by-step guide",
		OnboardDownloadsUse:  "Found a recent OAuth file in ~/Downloads:\n  %s\nUse this file? [Y/n]",
		OnboardGuideIntro:    "Google key guide (5 steps). Each step opens a Google Cloud page; come back here when it is done.",
		OnboardGuideKeys:     "Enter = step done · s = skip · q = quit · or paste the JSON file path to finish now",
		OnboardGuideProject:  "1/5 Create or pick a project; if you already have a usable project, just select it.",
		OnboardGuideDriveAPI: "2/5 Enable the Google Drive API for that project (press Enable).",
		OnboardGuideConsent:  "3/5 Consent screen: choose External under Branding. Add your email as a Test user at https://console.cloud.google.com/auth/audience, then press Publish app (In production) so sign-in does not expire after 7 days. Google will warn that the app is unverified when you sign in: choose Advanced > Go to ... (it is your own key).",
		OnboardGuideClient:   "4/5 Create an OAuth client of type Desktop app (Create client → Desktop app).",
		OnboardGuideDownload: "5/5 Download the client JSON, then drag and drop that file into this terminal.",
		OnboardGuideQuit:     "Guide stopped. Run `502drive onboard` again when you have the JSON file.",
		OnboardPathOnly:      "Path to the downloaded client_secret….json",
		OnboardPathRetry:     "That file cannot be used: %v\nPaste the path again (%d attempts left).",
		OnboardPathFailed:    "The OAuth file still cannot be used after 3 attempts: %v",
	}
	for k, v := range vi {
		VI[k] = v
	}
	for k, v := range en {
		EN[k] = v
	}
}
