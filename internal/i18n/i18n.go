// Package i18n holds the Vietnamese and English strings for the interactive
// terminal surfaces (TUI, line menu, tray). CLI command output and JSON stay
// English by contract; only these menus are translated.
package i18n

import (
	"fmt"
	"strings"
)

// Key identifies one label. Keys are stable identifiers, not user text.
type Key string

const (
	Ready        Key = "ready"
	Offline      Key = "offline"
	ShownLast    Key = "showing_last_data"
	Google       Key = "google"
	Telegram     Key = "telegram"
	Running      Key = "running"
	Queued       Key = "queued"
	Blocked      Key = "blocked"
	Home         Key = "home"
	Jobs         Key = "jobs"
	Library      Key = "library"
	Destination  Key = "destination"
	Account      Key = "account"
	Settings     Key = "settings"
	Help         Key = "help"
	Job          Key = "job"
	Checks       Key = "checks"
	Source       Key = "source"
	State        Key = "state"
	Phase        Key = "phase"
	Progress     Key = "progress"
	Counts       Key = "counts"
	Result       Key = "result"
	Problem      Key = "problem"
	CopyState    Key = "copy"
	Sync         Key = "sync"
	Revision     Key = "revision"
	Schedule     Key = "schedule"
	LastSuccess  Key = "last_success"
	NextRun      Key = "next_run"
	NextRuns     Key = "next_runs"
	ActiveJob    Key = "active_job"
	Output       Key = "output"
	NoJobs       Key = "no_jobs"
	NoSources    Key = "no_sources"
	NoDest       Key = "no_destination"
	SelectJob    Key = "select_job"
	SelectSource Key = "select_source"
	ItemsUnknown Key = "items_unknown"
	HomeHint     Key = "home_hint"
	JobKeys      Key = "job_keys"
	CollKeys     Key = "collection_keys"
	DestKeys     Key = "dest_keys"
	AccountKeys  Key = "account_keys"
	SettingsKeys Key = "settings_keys"
	FooterKeys   Key = "footer_keys"
	FooterJob    Key = "footer_job"
	FooterColl   Key = "footer_collection"
	FooterHelp   Key = "footer_help"
	FooterList   Key = "footer_list"
	FooterLib    Key = "footer_library"
	Selected     Key = "selected"
	ConfirmCan   Key = "confirm_cancel"
	ConfirmStop  Key = "confirm_stop"
	BulkResult   Key = "bulk_result"
	Live         Key = "live"
	Reconnecting Key = "reconnecting"
	FilesPerSec  Key = "files_per_sec"
	ETALeft      Key = "eta_left"
	ScrollPos    Key = "scroll_pos"
	GreetMorning Key = "greet_morning"
	GreetDay     Key = "greet_day"
	GreetEvening Key = "greet_evening"
	StayRunning  Key = "stay_running"
	WorkingNow   Key = "working_now"
	AllDone      Key = "all_done"
	NoticeDone   Key = "notice_done"
	NoticePart   Key = "notice_partial"
	NoticeFail   Key = "notice_failed"
	LoginHome    Key = "login_home"
	LoginWaiting Key = "login_waiting"
	LoginOpen    Key = "login_open"
	LoginDone    Key = "login_done"
	LoginFailed  Key = "login_failed"
	LoginExpired Key = "login_expired"
	LoginHide    Key = "login_hide"
	Language     Key = "language"
	Timezone     Key = "timezone"
	Save         Key = "save"
	Cancel       Key = "cancel"
	Pause        Key = "pause"
	Resume       Key = "resume"
	Retry        Key = "retry"
	RunNow       Key = "run_now"
	EnableSync   Key = "enable_sync"
	ChangeSched  Key = "change_schedule"
	StopSync     Key = "stop_sync"
	PauseSync    Key = "pause_sync"
	ResumeSync   Key = "resume_sync"
	PresetDaily  Key = "preset_daily"
	PresetHourly Key = "preset_hourly"
	PresetSixH   Key = "preset_six_hour"
	PresetReal   Key = "preset_realtime"
	SyncOff      Key = "sync_off"
	PickPreset   Key = "pick_preset"
	OpenTUI      Key = "open_tui"
	PasteLink    Key = "paste_link"
	JobsLibrary  Key = "jobs_library"
	OpenResult   Key = "open_result"
	StartDaemon  Key = "start_daemon"
	StopDaemon   Key = "stop_daemon"
	Quit         Key = "quit"
	StateIdle    Key = "state_idle"
	StateBusy    Key = "state_busy"
	StateNeeds   Key = "state_needs"
	StateError   Key = "state_error"
	StateOffline Key = "state_offline"
	StateQueued  Key = "state_queued"
	StateRunning Key = "state_running"
	StateDone    Key = "state_done"
	StatePartial Key = "state_partial"
	StateFailed  Key = "state_failed"
	StatePaused  Key = "state_paused"
	StatusLine   Key = "status_line"
	OfflineLine  Key = "offline_line"
	JobLine      Key = "job_line"
	NoJobsTray   Key = "no_jobs_tray"
	MenuTitle    Key = "menu_title"
	MenuCopy     Key = "menu_copy"
	MenuJobs     Key = "menu_jobs"
	MenuLibrary  Key = "menu_library"
	MenuDest     Key = "menu_destination"
	MenuAccount  Key = "menu_account"
	MenuSettings Key = "menu_settings"
	MenuBack     Key = "menu_back"
	Choice       Key = "choice"
	InvalidPick  Key = "invalid_choice"
	Back         Key = "back"
	PastePrompt  Key = "paste_prompt"
	Follow       Key = "follow"
	BackToMenu   Key = "back_to_menu"
	JobsLatest   Key = "jobs_latest"
	JobActions   Key = "job_actions"
	AddDest      Key = "add_destination"
	RunChecks    Key = "run_checks"
	LoginHint    Key = "login_hint"
	Revoke       Key = "revoke"
	SettingsHead Key = "settings_head"
	LangVI       Key = "lang_vi"
	LangEN       Key = "lang_en"
	CopiedSrcs   Key = "copied_sources"
	NothingCopy  Key = "nothing_copied"
	WhyOff       Key = "why_off"
)

// VI is the Vietnamese table (the default language).
var VI = map[Key]string{
	Ready: "sẵn sàng", Offline: "MẤT KẾT NỐI", ShownLast: "(đang hiện dữ liệu cuối)",
	Google: "Google", Telegram: "Telegram", Running: "đang chạy", Queued: "đang chờ", Blocked: "cần xử lý",
	Home: "Trang chính", Jobs: "Công việc", Library: "Thư viện & sync", Destination: "Thư mục đích",
	Account: "Tài khoản", Settings: "Cài đặt", Help: "Phím tắt", Job: "Job", Checks: "Kiểm tra",
	Source: "Nguồn", State: "Trạng thái", Phase: "Giai đoạn", Progress: "Tiến độ", Counts: "Số lượng",
	Result: "Kết quả", Problem: "Lỗi", CopyState: "Bản copy", Sync: "Đồng bộ", Revision: "Revision",
	Schedule: "Lịch", LastSuccess: "Thành công gần nhất", NextRun: "Lần tới", NextRuns: "Lần chạy tới",
	ActiveJob: "Job đang chạy", Output: "Kết quả",
	NoJobs: "Chưa có job. Hãy copy một link.", NoSources: "Chưa copy nguồn nào.",
	NoDest:    "Chưa có thư mục đích: 502drive destinations add FOLDER_LINK",
	SelectJob: "Chọn một job trước.", SelectSource: "Chọn một nguồn đã copy trước.",
	ItemsUnknown: "mục: %d xong · chưa rõ tổng",
	HomeHint:     "Dán link bằng `502drive copy`, hoặc chọn màn bên dưới.",
	JobKeys:      "p tạm dừng · u tiếp tục · c hủy · t thử lại · esc quay lại",
	CollKeys:     "d hằng ngày · h mỗi giờ · e mỗi 6 giờ · t realtime · n chạy ngay · p tạm dừng · x dừng · esc quay lại",
	DestKeys:     "enter đặt mặc định · esc quay lại",
	AccountKeys:  "l đăng nhập / kết nối lại Google · d chạy kiểm tra offline",
	SettingsKeys: "l đổi ngôn ngữ · esc quay lại",
	FooterKeys:   "1-6 chuyển màn · ↑↓ di chuyển · enter mở · esc quay lại · ? trợ giúp · q thoát",
	FooterJob:    "p tạm dừng · u tiếp tục · c hủy · t thử lại · esc quay lại · q thoát",
	FooterColl:   "d/h/e/t bật lịch · n chạy · p tạm dừng · x dừng · esc quay lại · q thoát",
	FooterHelp:   "esc quay lại · q thoát",
	FooterList:   "space chọn · a chọn hết · p tạm dừng · u tiếp tục · c hủy · t thử lại · enter mở · q thoát",
	FooterLib:    "space chọn · a chọn hết · n chạy · p tạm dừng · x dừng · d/h/e/t lịch · enter mở · q thoát",
	Selected:     "%d đã chọn",
	ConfirmCan:   "Hủy %d job? [y/N]",
	ConfirmStop:  "Dừng đồng bộ %d nguồn? [y/N]",
	BulkResult:   "%s: %d xong, %d bỏ qua, %d lỗi",
	Live:         "trực tiếp",
	Reconnecting: "đang nối lại…",
	FilesPerSec:  "%.0f mục/giây",
	ETALeft:      "còn %s",
	ScrollPos:    "%d–%d / %d",
	GreetMorning: "Chào buổi sáng! Sẵn sàng copy.",
	GreetDay:     "Chào bạn! Dán link để bắt đầu.",
	GreetEvening: "Chào buổi tối! Mọi thứ đang ổn.",
	StayRunning:  "502Drive vẫn chạy nền ở khay hệ thống. Chọn Thoát trên khay để dừng hẳn.",
	WorkingNow:   "Đang copy %d / %d mục",
	AllDone:      "Xong rồi — không còn việc nào đang chạy.",
	NoticeDone:   "Đã copy xong",
	NoticePart:   "Copy xong một phần",
	NoticeFail:   "Copy thất bại",
	LoginHome:    "Google chưa kết nối — nhấn l để đăng nhập.",
	LoginWaiting: "Đang chờ bạn cho phép trên trang Google…",
	LoginOpen:    "▶ Bấm vào đây để mở trang đăng nhập Google",
	LoginDone:    "Đã kết nối Google.",
	LoginFailed:  "Đăng nhập không hoàn tất. Nhấn l để thử lại.",
	LoginExpired: "Link đăng nhập đã hết hạn. Nhấn l để tạo link mới.",
	LoginHide:    "Link còn hiệu lực khoảng 10 phút · esc để ẩn",
	Language:     "Ngôn ngữ", Timezone: "Múi giờ", Save: "Lưu", Cancel: "Hủy",
	Pause: "Tạm dừng", Resume: "Tiếp tục", Retry: "Thử lại", RunNow: "Chạy ngay",
	EnableSync: "Bật sync", ChangeSched: "Đổi lịch", StopSync: "Dừng sync",
	PauseSync: "Tạm dừng", ResumeSync: "Bật lại sync",
	PresetDaily: "Hằng ngày 22:00", PresetHourly: "Mỗi giờ", PresetSixH: "Mỗi 6 giờ",
	PresetReal: "Realtime (theo dõi thay đổi)", SyncOff: "chưa bật", PickPreset: "Chọn lịch đồng bộ",
	OpenTUI: "Mở menu TUI", PasteLink: "Copy link trong clipboard", JobsLibrary: "Jobs & thư viện",
	OpenResult: "Mở folder kết quả gần nhất", StartDaemon: "Bật daemon", StopDaemon: "Dừng daemon",
	Quit:      "Thoát (dừng 502Drive nền)",
	StateIdle: "rảnh", StateBusy: "đang sao chép", StateNeeds: "cần xử lý", StateError: "lỗi", StateOffline: "ngoại tuyến",
	StateQueued: "đang chờ", StateRunning: "đang chạy", StateDone: "xong", StatePartial: "xong một phần",
	StateFailed: "thất bại", StatePaused: "tạm dừng",
	StatusLine:  "Google: %s · %d đang chạy · %d đang chờ · %d cần xử lý",
	OfflineLine: "Daemon không chạy — bật lên để copy hoặc sync",
	JobLine:     "%s · %s · %s", NoJobsTray: "Chưa có job",
	MenuTitle: "Bạn muốn làm gì?", MenuCopy: "Copy link Drive", MenuJobs: "Công việc",
	MenuLibrary: "Nguồn đã copy / sync", MenuDest: "Thư mục đích", MenuAccount: "Tài khoản & sức khỏe",
	MenuSettings: "Cài đặt", MenuBack: "Quay lại / Thoát", Choice: "Chọn",
	InvalidPick: "Hãy nhập số từ 0 đến %d.", Back: "Quay lại",
	PastePrompt: "Dán link hoặc ID Google Drive", Follow: "Theo dõi tới khi xong",
	BackToMenu: "Về menu", JobsLatest: "Công việc (mới nhất 10)", JobActions: "Hành động job",
	AddDest: "Thêm folder làm đích", RunChecks: "Chạy kiểm tra sức khỏe",
	LoginHint: "Kết nối lại Google (in ra link)", Revoke: "Ngắt kết nối và thu hồi quyền",
	SettingsHead: "Cài đặt (ngôn ngữ hiện tại: %s)", LangVI: "Ngôn ngữ: Tiếng Việt", LangEN: "Ngôn ngữ: English",
	CopiedSrcs: "Nguồn đã copy", NothingCopy: "Chưa copy gì.", WhyOff: "Đồng bộ đang tắt",
}

// EN is the English table.
var EN = map[Key]string{
	Ready: "ready", Offline: "OFFLINE", ShownLast: "(showing last data)",
	Google: "Google", Telegram: "Telegram", Running: "running", Queued: "queued", Blocked: "blocked",
	Home: "Home", Jobs: "Jobs", Library: "Library & sync", Destination: "Destination",
	Account: "Account", Settings: "Settings", Help: "Keys", Job: "Job", Checks: "Checks",
	Source: "Source", State: "State", Phase: "Phase", Progress: "Progress", Counts: "Counts",
	Result: "Result", Problem: "Problem", CopyState: "Copy", Sync: "Sync", Revision: "Revision",
	Schedule: "Schedule", LastSuccess: "Last success", NextRun: "Next run", NextRuns: "Next runs",
	ActiveJob: "Active job", Output: "Output",
	NoJobs: "No jobs yet. Copy a link to start.", NoSources: "Nothing copied yet.",
	NoDest:    "No destination yet: 502drive destinations add FOLDER_LINK",
	SelectJob: "Select a job first.", SelectSource: "Select a copied source first.",
	ItemsUnknown: "items: %d done · total unknown",
	HomeHint:     "Paste a link with `502drive copy`, or pick a screen below.",
	JobKeys:      "p pause · u resume · c cancel · t retry · esc back",
	CollKeys:     "d daily · h hourly · e every 6h · t realtime · n run now · p pause · x stop · esc back",
	DestKeys:     "enter set as default · esc back",
	AccountKeys:  "l sign in / reconnect Google · d run the offline doctor checks",
	SettingsKeys: "l switch language · esc back",
	FooterKeys:   "1-6 screens · ↑↓ move · enter open · esc back · ? help · q quit",
	FooterJob:    "p pause · u resume · c cancel · t retry · esc back · q quit",
	FooterColl:   "d/h/e/t enable · n run · p pause · x stop · esc back · q quit",
	FooterHelp:   "esc back · q quit",
	FooterList:   "space select · a all · p pause · u resume · c cancel · t retry · enter open · q quit",
	FooterLib:    "space select · a all · n run · p pause · x stop · d/h/e/t schedule · enter open · q quit",
	Selected:     "%d selected",
	ConfirmCan:   "Cancel %d job(s)? [y/N]",
	ConfirmStop:  "Stop sync for %d source(s)? [y/N]",
	BulkResult:   "%s: %d done, %d skipped, %d failed",
	Live:         "live",
	Reconnecting: "reconnecting…",
	FilesPerSec:  "%.0f items/s",
	ETALeft:      "%s left",
	ScrollPos:    "%d–%d / %d",
	GreetMorning: "Good morning! Ready to copy.",
	GreetDay:     "Hello! Paste a link to start.",
	GreetEvening: "Good evening! All quiet.",
	StayRunning:  "502Drive keeps running in the tray. Choose Quit in the tray to stop it.",
	WorkingNow:   "Copying %d / %d items",
	AllDone:      "All done — nothing is running.",
	NoticeDone:   "Copy finished",
	NoticePart:   "Copy finished partly",
	NoticeFail:   "Copy failed",
	LoginHome:    "Google is not connected — press l to sign in.",
	LoginWaiting: "Waiting for you to approve on the Google page…",
	LoginOpen:    "▶ Click here to open the Google sign-in page",
	LoginDone:    "Google is connected.",
	LoginFailed:  "Sign-in did not complete. Press l to try again.",
	LoginExpired: "The sign-in link expired. Press l for a new one.",
	LoginHide:    "The link is valid for about 10 minutes · esc hides it",
	Language:     "Language", Timezone: "Timezone", Save: "Save", Cancel: "Cancel",
	Pause: "Pause", Resume: "Resume", Retry: "Retry", RunNow: "Run now",
	EnableSync: "Enable sync", ChangeSched: "Change schedule", StopSync: "Stop sync",
	PauseSync: "Pause sync", ResumeSync: "Resume sync",
	PresetDaily: "Daily at 22:00", PresetHourly: "Every hour", PresetSixH: "Every 6 hours",
	PresetReal: "Realtime (change polling)", SyncOff: "off", PickPreset: "Pick a sync schedule",
	OpenTUI: "Open TUI", PasteLink: "Copy link in clipboard", JobsLibrary: "Jobs & library",
	OpenResult: "Open latest result folder", StartDaemon: "Start daemon", StopDaemon: "Stop daemon",
	Quit:      "Quit (stops 502Drive in the background)",
	StateIdle: "idle", StateBusy: "copying", StateNeeds: "needs action", StateError: "error", StateOffline: "offline",
	StateQueued: "queued", StateRunning: "running", StateDone: "done", StatePartial: "partly done",
	StateFailed: "failed", StatePaused: "paused",
	StatusLine:  "Google: %s · %d running · %d queued · %d blocked",
	OfflineLine: "Daemon offline — start it to copy or sync",
	JobLine:     "%s · %s · %s", NoJobsTray: "No jobs yet",
	MenuTitle: "What would you like to do?", MenuCopy: "Copy a Drive link", MenuJobs: "Jobs",
	MenuLibrary: "Copied sources / sync", MenuDest: "Destination", MenuAccount: "Account and health",
	MenuSettings: "Settings", MenuBack: "Back / Exit", Choice: "Choice",
	InvalidPick: "Please enter a number from 0 to %d.", Back: "Back",
	PastePrompt: "Paste a Google Drive link or ID", Follow: "Follow progress until it finishes",
	BackToMenu: "Back to menu", JobsLatest: "Jobs (latest 10)", JobActions: "Job actions",
	AddDest: "Add a folder as destination", RunChecks: "Run health checks",
	LoginHint: "Connect / reconnect Google (prints a link)", Revoke: "Disconnect and revoke",
	SettingsHead: "Settings (current language: %s)", LangVI: "Language: Tiếng Việt", LangEN: "Language: English",
	CopiedSrcs: "Copied sources", NothingCopy: "Nothing copied yet.", WhyOff: "Sync is off",
}

// For returns the table for a language code; unknown codes fall back to English.
func For(lang string) map[Key]string {
	if strings.HasPrefix(strings.ToLower(lang), "vi") {
		return VI
	}
	return EN
}

// T returns the label for key, falling back to English, then to the key.
func T(lang string, key Key) string {
	if v, ok := For(lang)[key]; ok && v != "" {
		return v
	}
	if v, ok := EN[key]; ok && v != "" {
		return v
	}
	return string(key)
}

// Tf formats a label with arguments.
func Tf(lang string, key Key, args ...any) string {
	return fmt.Sprintf(T(lang, key), args...)
}

// Keys returns every declared key, for completeness tests and documentation.
func Keys() []Key {
	out := make([]Key, 0, len(EN))
	for k := range EN {
		out = append(out, k)
	}
	return out
}

// HelpLines is the key map shown on the TUI help screen.
func HelpLines(lang string) []string {
	if strings.HasPrefix(strings.ToLower(lang), "vi") {
		return []string{
			"1..6     chuyển màn (Trang chính, Công việc, Thư viện, Đích, Tài khoản, Cài đặt)",
			"↑↓ / j k di chuyển lựa chọn",
			"enter    mở job, nguồn hoặc thư mục đang chọn",
			"esc      quay lại màn trước",
			"q        thoát (daemon vẫn chạy)",
			"?        trợ giúp này",
			"r        làm mới ngay",
			"",
			"space    chọn nhiều mục · a chọn hết · esc bỏ chọn",
			"Jobs:       p tạm dừng · u tiếp tục · c hủy · t thử lại (áp dụng cho mục đã chọn)",
			"Library:    d hằng ngày 22:00 · h mỗi giờ · e mỗi 6 giờ · t realtime",
			"            n chạy ngay · p tạm dừng sync · x dừng sync",
			"Destination: enter đặt làm mặc định",
			"Account:    l đăng nhập / kết nối lại Google · d chạy kiểm tra offline",
			"Settings:   l đổi ngôn ngữ (vi/en)",
		}
	}
	return []string{
		"1..6     switch screen (Home, Jobs, Library, Destination, Account, Settings)",
		"↑↓ / j k move the selection",
		"enter    open the selected job, collection or destination",
		"esc      back to the previous screen",
		"q        quit (the daemon keeps running)",
		"?        this help",
		"r        refresh now",
		"",
		"space    select several rows · a select all · esc clear",
		"Jobs:       p pause · u resume · c cancel · t retry (applies to the selection)",
		"Library:    d daily 22:00 · h hourly · e every 6h · t realtime",
		"            n run now · p pause sync · x stop sync",
		"Destination: enter sets the default folder",
		"Account:    l sign in / reconnect Google · d run the offline doctor checks",
		"Settings:   l switch language (vi/en)",
	}
}
