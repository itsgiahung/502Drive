// Package i18n holds the Vietnamese and English strings for the interactive
// terminal surfaces (TUI, line menu, tray). CLI command output and JSON stay
// English by contract; only these menus are translated.
package i18n

import (
	"fmt"
	"strings"
	"time"
)

// Key identifies one label. Keys are stable identifiers, not user text.
type Key string

const (
	Ready            Key = "ready"
	Offline          Key = "offline"
	ShownLast        Key = "showing_last_data"
	Google           Key = "google"
	Telegram         Key = "telegram"
	Running          Key = "running"
	ThrottleLine     Key = "throttle_line"
	ThrottleRetry    Key = "throttle_retry"
	Queued           Key = "queued"
	Blocked          Key = "blocked"
	Home             Key = "home"
	Jobs             Key = "jobs"
	Library          Key = "library"
	Destination      Key = "destination"
	Account          Key = "account"
	Settings         Key = "settings"
	Help             Key = "help"
	Job              Key = "job"
	Checks           Key = "checks"
	Source           Key = "source"
	State            Key = "state"
	Phase            Key = "phase"
	Progress         Key = "progress"
	Counts           Key = "counts"
	Result           Key = "result"
	Problem          Key = "problem"
	CopyState        Key = "copy"
	Sync             Key = "sync"
	Revision         Key = "revision"
	Schedule         Key = "schedule"
	LastSuccess      Key = "last_success"
	NextRun          Key = "next_run"
	NextRuns         Key = "next_runs"
	ActiveJob        Key = "active_job"
	Output           Key = "output"
	NoJobs           Key = "no_jobs"
	NoSources        Key = "no_sources"
	NoDest           Key = "no_destination"
	SelectJob        Key = "select_job"
	SelectSource     Key = "select_source"
	ItemsUnknown     Key = "items_unknown"
	HomeHint         Key = "home_hint"
	JobKeys          Key = "job_keys"
	CollKeys         Key = "collection_keys"
	DetailKeys       Key = "detail_navigation"
	DestKeys         Key = "dest_keys"
	AccountKeys      Key = "account_keys"
	SettingsKeys     Key = "settings_keys"
	FooterKeys       Key = "footer_keys"
	FooterJob        Key = "footer_job"
	FooterColl       Key = "footer_collection"
	FooterHelp       Key = "footer_help"
	FooterList       Key = "footer_list"
	FooterLib        Key = "footer_library"
	Selected         Key = "selected"
	ConfirmCan       Key = "confirm_cancel"
	ConfirmStop      Key = "confirm_stop"
	BulkResult       Key = "bulk_result"
	Live             Key = "live"
	Reconnecting     Key = "reconnecting"
	FilesPerSec      Key = "files_per_sec"
	ETALeft          Key = "eta_left"
	ScrollPos        Key = "scroll_pos"
	GreetMorning     Key = "greet_morning"
	GreetDay         Key = "greet_day"
	GreetEvening     Key = "greet_evening"
	StayRunning      Key = "stay_running"
	WorkingNow       Key = "working_now"
	AllDone          Key = "all_done"
	NoticeDone       Key = "notice_done"
	NoticePart       Key = "notice_partial"
	NoticeFail       Key = "notice_failed"
	LoginHome        Key = "login_home"
	LoginWaiting     Key = "login_waiting"
	LoginOpen        Key = "login_open"
	LoginDone        Key = "login_done"
	LoginFailed      Key = "login_failed"
	LoginExpired     Key = "login_expired"
	LoginHide        Key = "login_hide"
	Drive            Key = "drive"
	FooterDrive      Key = "footer_drive"
	FooterPick       Key = "footer_pick"
	FooterCopySrc    Key = "footer_copy_source"
	FooterCopyPlan   Key = "footer_copy_plan"
	DriveEmpty       Key = "drive_empty"
	Loading          Key = "loading"
	MoreBelow        Key = "more_below"
	NewFolderPrompt  Key = "new_folder_prompt"
	FolderCreated    Key = "folder_created"
	PickHint         Key = "pick_hint"
	DestSet          Key = "dest_set"
	CopyTitle        Key = "copy_title"
	CopyPaste        Key = "copy_paste"
	CopyChecking     Key = "copy_checking"
	CopyFrom         Key = "copy_from"
	CopyTo           Key = "copy_to"
	CopyOutName      Key = "copy_out_name"
	NameKeep         Key = "name_keep"
	NameAuto         Key = "name_auto"
	NameCustom       Key = "name_custom"
	NamePrompt       Key = "name_prompt"
	PreviewCount     Key = "preview_count"
	PreviewPartial   Key = "preview_partial"
	CopyStarted      Key = "copy_started"
	MetaSize         Key = "meta_size"
	MetaType         Key = "meta_type"
	MetaModified     Key = "meta_modified"
	MetaCreated      Key = "meta_created"
	MetaOwner        Key = "meta_owner"
	MetaID           Key = "meta_id"
	MetaLink         Key = "meta_link"
	MetaCopy         Key = "meta_copy"
	MetaWrite        Key = "meta_write"
	Yes              Key = "yes"
	No               Key = "no"
	TabHome          Key = "tab_home"
	TabJobs          Key = "tab_jobs"
	TabLibrary       Key = "tab_library"
	TabDest          Key = "tab_dest"
	TabAccount       Key = "tab_account"
	TabSettings      Key = "tab_settings"
	TabDrive         Key = "tab_drive"
	TabHint          Key = "tab_hint"
	TabFocusHint     Key = "tab_focus_hint"
	SyncOption       Key = "sync_option"
	SettingsKeys2    Key = "settings_keys2"
	PanelStart       Key = "panel_start"
	PanelTip1        Key = "panel_tip1"
	PanelTip2        Key = "panel_tip2"
	PanelRecent      Key = "panel_recent"
	NoRecent         Key = "no_recent"
	PromptHint       Key = "prompt_hint"
	DestLine         Key = "dest_line"
	NoDestLine       Key = "no_dest_line"
	FooterHome       Key = "footer_home"
	CmdCopy          Key = "cmd_copy"
	CmdJobs          Key = "cmd_jobs"
	CmdLibrary       Key = "cmd_library"
	CmdDest          Key = "cmd_dest"
	CmdAccount       Key = "cmd_account"
	CmdSettings      Key = "cmd_settings"
	CmdDrive         Key = "cmd_drive"
	CmdLogin         Key = "cmd_login"
	CmdHelp          Key = "cmd_help"
	CmdRefresh       Key = "cmd_refresh"
	CmdQuit          Key = "cmd_quit"
	UnknownCmd       Key = "unknown_cmd"
	Language         Key = "language"
	Timezone         Key = "timezone"
	Save             Key = "save"
	Cancel           Key = "cancel"
	Pause            Key = "pause"
	Resume           Key = "resume"
	Retry            Key = "retry"
	RunNow           Key = "run_now"
	EnableSync       Key = "enable_sync"
	ChangeSched      Key = "change_schedule"
	StopSync         Key = "stop_sync"
	PauseSync        Key = "pause_sync"
	ResumeSync       Key = "resume_sync"
	PresetDaily      Key = "preset_daily"
	PresetHourly     Key = "preset_hourly"
	PresetSixH       Key = "preset_six_hour"
	PresetReal       Key = "preset_realtime"
	SyncOff          Key = "sync_off"
	PickPreset       Key = "pick_preset"
	OpenTUI          Key = "open_tui"
	PasteLink        Key = "paste_link"
	JobsLibrary      Key = "jobs_library"
	OpenResult       Key = "open_result"
	StartDaemon      Key = "start_daemon"
	StopDaemon       Key = "stop_daemon"
	StopApp          Key = "stop_app"
	StopAppConfirm   Key = "stop_app_confirm"
	Quit             Key = "quit"
	TrayClone        Key = "tray_clone"
	TrayOpenApp      Key = "tray_open_app"
	TrayStartApp     Key = "tray_start_app"
	TrayOpenAll      Key = "tray_open_all"
	TrayPauseAll     Key = "tray_pause_all"
	TrayResumeAll    Key = "tray_resume_all"
	TrayReasonDaemon Key = "tray_reason_daemon"
	TrayReasonGoogle Key = "tray_reason_google"
	TrayPausedLine   Key = "tray_paused_line"
	TrayNeedsHint    Key = "tray_needs_hint"
	TrayStartHint    Key = "tray_start_hint"
	StateReconnect   Key = "state_reconnect"
	StateIdle        Key = "state_idle"
	StateBusy        Key = "state_busy"
	StateNeeds       Key = "state_needs"
	StateError       Key = "state_error"
	StateOffline     Key = "state_offline"
	StateQueued      Key = "state_queued"
	StateRunning     Key = "state_running"
	StateDone        Key = "state_done"
	StatePartial     Key = "state_partial"
	StateFailed      Key = "state_failed"
	StatePaused      Key = "state_paused"
	StatusLine       Key = "status_line"
	TrayGoogleRow    Key = "tray_google_row"
	TrayWorkRow      Key = "tray_work_row"
	TrayPausedRow    Key = "tray_paused_row"
	TrayBlockedRow   Key = "tray_blocked_row"
	OfflineLine      Key = "offline_line"
	JobLine          Key = "job_line"
	NoJobsTray       Key = "no_jobs_tray"
	MenuTitle        Key = "menu_title"
	MenuCopy         Key = "menu_copy"
	MenuJobs         Key = "menu_jobs"
	MenuLibrary      Key = "menu_library"
	MenuDest         Key = "menu_destination"
	MenuAccount      Key = "menu_account"
	MenuSettings     Key = "menu_settings"
	MenuBack         Key = "menu_back"
	Choice           Key = "choice"
	InvalidPick      Key = "invalid_choice"
	Back             Key = "back"
	PastePrompt      Key = "paste_prompt"
	Follow           Key = "follow"
	BackToMenu       Key = "back_to_menu"
	JobsLatest       Key = "jobs_latest"
	JobActions       Key = "job_actions"
	AddDest          Key = "add_destination"
	RunChecks        Key = "run_checks"
	LoginHint        Key = "login_hint"
	Revoke           Key = "revoke"
	SettingsHead     Key = "settings_head"
	LangVI           Key = "lang_vi"
	LangEN           Key = "lang_en"
	CopiedSrcs       Key = "copied_sources"
	NothingCopy      Key = "nothing_copied"
	WhyOff           Key = "why_off"
	Autostart        Key = "autostart"
)

// VI is the Vietnamese table (the default language).
var VI = map[Key]string{
	Ready: "sẵn sàng", Offline: "MẤT KẾT NỐI", ShownLast: "(đang hiện dữ liệu cuối)",
	Google: "Google", Telegram: "Telegram", Running: "đang chạy", Queued: "đang chờ", Blocked: "cần xử lý",
	Home: "Trang chính", Jobs: "Công việc", Library: "Thư viện & đồng bộ", Destination: "Thư mục đích",
	Account: "Tài khoản", Settings: "Cài đặt", Help: "Phím tắt", Job: "Công việc", Checks: "Kiểm tra",
	Source: "Nguồn", State: "Trạng thái", Phase: "Giai đoạn", Progress: "Tiến độ", Counts: "Số lượng",
	Result: "Kết quả", Problem: "Lỗi", CopyState: "Bản copy", Sync: "Đồng bộ", Revision: "Revision",
	Schedule: "Lịch", LastSuccess: "Thành công gần nhất", NextRun: "Lần tới", NextRuns: "Lần chạy tới",
	ActiveJob: "Công việc đang chạy", Output: "Kết quả",
	NoJobs: "Chưa có công việc nào. Hãy copy một link.", NoSources: "Chưa copy nguồn nào.",
	NoDest:    "Chưa có thư mục đích: 502drive destinations add FOLDER_LINK",
	SelectJob: "Chọn một công việc trước.", SelectSource: "Chọn một nguồn đã copy trước.",
	ItemsUnknown:    "mục: %d xong · chưa rõ tổng",
	HomeHint:        "Dán link bằng `502drive copy`, hoặc chọn màn bên dưới.",
	JobKeys:         "p tạm dừng · u tiếp tục · c huỷ · t thử lại · R báo cáo · E xuất CSV · esc quay lại",
	DetailKeys:      "↑↓ chọn · enter mở · ← / esc quay lại",
	CollKeys:        "↑↓ chọn · enter mở · ← / esc quay lại · d hằng ngày · h mỗi giờ · e mỗi 6 giờ · t realtime · n chạy ngay · p tạm dừng · x dừng",
	DestKeys:        "enter đặt mặc định · esc quay lại",
	AccountKeys:     "l đăng nhập / kết nối lại Google · d chạy kiểm tra offline",
	SettingsKeys:    "l đổi ngôn ngữ · esc quay lại",
	FooterKeys:      "1–7 chuyển màn · ↑↓ di chuyển · enter mở · esc quay lại · ? trợ giúp · q thoát",
	FooterJob:       "p tạm dừng · u tiếp tục · c huỷ · t thử lại · R báo cáo · E CSV · esc quay lại",
	FooterColl:      "d/h/e/t bật lịch · n chạy · p tạm dừng · x dừng · esc quay lại · q thoát",
	FooterHelp:      "esc quay lại · q thoát",
	FooterList:      "space chọn · a chọn hết · p tạm dừng · u tiếp tục · c huỷ · t thử lại · enter mở · q thoát",
	FooterLib:       "space chọn · a chọn hết · n chạy · p tạm dừng · x dừng · d/h/e/t lịch · enter mở · q thoát",
	Selected:        "%d đã chọn",
	ConfirmCan:      "Huỷ %d công việc? [y/N]",
	ConfirmStop:     "Dừng đồng bộ %d nguồn? [y/N]",
	BulkResult:      "%s: %d xong, %d bỏ qua, %d lỗi",
	Live:            "trực tiếp",
	Reconnecting:    "đang nối lại…",
	FilesPerSec:     "%.0f mục/giây",
	ETALeft:         "còn %s",
	ScrollPos:       "%d–%d / %d",
	GreetMorning:    "Chào buổi sáng! Sẵn sàng copy.",
	GreetDay:        "Chào bạn! Dán link để bắt đầu.",
	GreetEvening:    "Chào buổi tối! Mọi thứ đang ổn.",
	ThrottleLine:    "Luồng: %d · bị giới hạn %d lần/5 phút",
	ThrottleRetry:   "Google giới hạn tới %s",
	StayRunning:     "502Drive vẫn chạy nền ở khay hệ thống. Chọn Thoát trên khay để dừng hẳn.",
	WorkingNow:      "Đang copy %d / %d mục",
	AllDone:         "Xong rồi — không còn việc nào đang chạy.",
	NoticeDone:      "Đã copy xong",
	NoticePart:      "Copy xong một phần",
	NoticeFail:      "Copy thất bại",
	LoginHome:       "Google chưa kết nối — nhấn l để đăng nhập.",
	LoginWaiting:    "Đang chờ bạn cho phép trên trang Google…",
	LoginOpen:       "▶ Bấm vào đây để mở trang đăng nhập Google",
	LoginDone:       "Đã kết nối Google.",
	LoginFailed:     "Đăng nhập không hoàn tất. Nhấn l để thử lại.",
	LoginExpired:    "Link đăng nhập đã hết hạn. Nhấn l để tạo link mới.",
	LoginHide:       "Link còn hiệu lực khoảng 10 phút · esc để ẩn",
	Drive:           "Drive",
	FooterDrive:     "↑↓ chọn · enter/→ mở · ←/⌫ lên · / tìm · i thông tin · n thư mục mới · c copy · d đặt làm đích · esc quay lại",
	FooterPick:      "↑↓ chọn · enter/→ mở · ←/⌫ lên · y chọn thư mục đang mở làm đích · n thư mục mới · esc huỷ",
	FooterCopySrc:   "enter kiểm tra · ctrl+v dán · esc huỷ",
	FooterCopyPlan:  "enter bắt đầu copy · 1/2/3 tên · d nơi lưu · s đồng bộ · f/o/k tùy chọn · esc hủy",
	DriveEmpty:      "Thư mục trống.",
	Loading:         "Đang tải…",
	MoreBelow:       "… còn nữa, cuộn xuống để tải thêm",
	NewFolderPrompt: "Tên thư mục mới (Enter tạo, Esc huỷ)",
	FolderCreated:   "Đã tạo thư mục “%s”.",
	PickHint:        "CHỌN THƯ MỤC ĐÍCH — vào thư mục rồi nhấn y",
	DestSet:         "Đã đặt “%s” làm thư mục đích mặc định.",
	CopyTitle:       "Copy một link",
	CopyPaste:       "Dán link Google Drive hoặc ID",
	CopyChecking:    "Đang kiểm tra…",
	CopyFrom:        "Nguồn",
	CopyTo:          "Thư mục đích",
	CopyOutName:     "Tên thư mục kết quả",
	NameKeep:        "1 giữ tên gốc",
	NameAuto:        "2 tự đặt tên mới",
	NameCustom:      "3 tự nhập tên",
	NamePrompt:      "Tên thư mục kết quả (Enter xác nhận, Esc huỷ)",
	PreviewCount:    "%d tệp, %d thư mục%s",
	PreviewPartial:  " (mới thấy một phần)",
	CopyStarted:     "Đã bắt đầu copy — công việc %s.",
	MetaSize:        "Kích thước",
	MetaType:        "Loại",
	MetaModified:    "Sửa lần cuối",
	MetaCreated:     "Tạo lúc",
	MetaOwner:       "Chủ sở hữu",
	MetaID:          "ID",
	MetaLink:        "Link",
	MetaCopy:        "Cho phép copy",
	MetaWrite:       "Thêm file vào được",
	Yes:             "có",
	No:              "không",
	TabHome:         `Trang chủ`,
	TabJobs:         `Công việc`,
	TabLibrary:      `Thư viện`,
	TabDest:         `Đích`,
	TabAccount:      `Tài khoản`,
	TabSettings:     `Cài đặt`,
	TabDrive:        `Drive`,
	TabHint:         `Tab/←→ chuyển màn`,
	TabFocusHint:    `←/→ đổi tab · ↓ hoặc Enter vào nội dung`,
	SyncOption:      `Đồng bộ sau khi copy`,
	SettingsKeys2:   `←/→ hoặc Enter đổi ngay · ↑ lên thanh tab · esc quay lại`,
	PanelStart:      `Bắt đầu`,
	PanelTip1:       `Dán link Drive vào ô dưới, nhấn Enter để copy`,
	PanelTip2:       `Gõ / xem lệnh · nhấn 1–7 để chuyển màn`,
	PanelRecent:     `Hoạt động gần đây`,
	NoRecent:        `Chưa có công việc nào`,
	PromptHint:      `Dán link Drive, hoặc gõ / để xem lệnh`,
	DestLine:        `Thư mục đích`,
	NoDestLine:      `chưa chọn (nhấn 7 → d)`,
	FooterHome:      `/ lệnh · 1–7 chuyển màn · c copy · l đăng nhập · ? trợ giúp · q thoát`,
	CmdCopy:         `Copy một link (chọn tên và thư mục đích)`,
	CmdJobs:         `Danh sách công việc`,
	CmdLibrary:      `Thư viện và đồng bộ`,
	CmdDest:         `Thư mục đích`,
	CmdAccount:      `Tài khoản Google và kiểm tra`,
	CmdSettings:     `Ngôn ngữ và múi giờ`,
	CmdDrive:        `Duyệt Google Drive`,
	CmdLogin:        `Đăng nhập / kết nối lại Google`,
	CmdHelp:         `Phím tắt`,
	CmdRefresh:      `Làm mới ngay`,
	CmdQuit:         `Thoát TUI (daemon vẫn chạy)`,
	UnknownCmd:      `Không có lệnh “%s”. Gõ / để xem danh sách.`,
	Language:        "Ngôn ngữ", Timezone: "Múi giờ", Save: "Lưu", Cancel: "Huỷ",
	Pause: "Tạm dừng", Resume: "Tiếp tục", Retry: "Thử lại", RunNow: "Chạy ngay",
	EnableSync: "Bật đồng bộ", ChangeSched: "Đổi lịch", StopSync: "Dừng đồng bộ",
	PauseSync: "Tạm dừng", ResumeSync: "Bật lại đồng bộ",
	PresetDaily: "Hằng ngày 22:00", PresetHourly: "Mỗi giờ", PresetSixH: "Mỗi 6 giờ",
	PresetReal: "Realtime (theo dõi thay đổi)", SyncOff: "chưa bật", PickPreset: "Chọn lịch đồng bộ",
	OpenTUI: "Mở menu TUI", PasteLink: "Copy link trong clipboard", JobsLibrary: "Công việc & thư viện",
	OpenResult: "Mở thư mục kết quả gần nhất", StartDaemon: "Bật daemon", StopDaemon: "Dừng daemon",
	StopApp:        "Dừng 502Drive…",
	StopAppConfirm: "Dừng 502Drive (%d việc đang chạy)? Bấm lần nữa để xác nhận.",
	Quit:           "Thoát khay",
	TrayClone:      "Clone từ clipboard", TrayOpenApp: "Mở 502Drive", TrayStartApp: "Bật 502Drive",
	TrayOpenAll: "Mở tất cả", TrayPauseAll: "Tạm dừng tất cả", TrayResumeAll: "Tiếp tục tất cả",
	TrayReasonDaemon: "— daemon chưa chạy", TrayReasonGoogle: "— chưa đăng nhập Google",
	TrayPausedLine: "Google: %s · hàng đợi đang tạm dừng · %d cần xử lý",
	StateReconnect: "đang kết nối lại",
	TrayNeedsHint:  "Có việc cần bạn xử lý: mở 502Drive › Công việc để xem lý do và cách sửa.",
	TrayStartHint:  "Bấm “Bật 502Drive” để bắt đầu.",
	StateIdle:      "rảnh", StateBusy: "đang sao chép", StateNeeds: "cần xử lý", StateError: "lỗi", StateOffline: "ngoại tuyến",
	StateQueued: "đang chờ", StateRunning: "đang chạy", StateDone: "xong", StatePartial: "xong một phần",
	StateFailed: "thất bại", StatePaused: "tạm dừng",
	StatusLine:    "Google: %s · %d đang chạy · %d đang chờ · %d cần xử lý",
	TrayGoogleRow: "Google: %s", TrayWorkRow: "%d đang chạy · %d đang chờ", TrayPausedRow: "Hàng đợi đang tạm dừng", TrayBlockedRow: "%d cần xử lý",
	OfflineLine: "Daemon không chạy — bật lên để copy hoặc đồng bộ",
	JobLine:     "%s · %s · %s", NoJobsTray: "Chưa có công việc nào",
	MenuTitle: "Bạn muốn làm gì?", MenuCopy: "Copy link Drive", MenuJobs: "Công việc",
	MenuLibrary: "Nguồn đã copy / đồng bộ", MenuDest: "Thư mục đích", MenuAccount: "Tài khoản & sức khỏe",
	MenuSettings: "Cài đặt", MenuBack: "Quay lại / Thoát", Choice: "Chọn",
	InvalidPick: "Hãy nhập số từ 0 đến %d.", Back: "Quay lại",
	PastePrompt: "Dán link hoặc ID Google Drive", Follow: "Theo dõi tới khi xong",
	BackToMenu: "Về menu", JobsLatest: "Công việc (mới nhất 10)", JobActions: "Thao tác công việc",
	AddDest: "Thêm thư mục làm đích", RunChecks: "Chạy kiểm tra sức khỏe",
	LoginHint: "Kết nối lại Google (in ra link)", Revoke: "Ngắt kết nối và thu hồi quyền",
	SettingsHead: "Cài đặt (ngôn ngữ hiện tại: %s)", LangVI: "Ngôn ngữ: Tiếng Việt", LangEN: "Ngôn ngữ: English",
	CopiedSrcs: "Nguồn đã copy", NothingCopy: "Chưa copy gì.", WhyOff: "Đồng bộ đang tắt",
	Autostart: "Khởi động cùng hệ thống",
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
	ItemsUnknown:    "items: %d done · total unknown",
	HomeHint:        "Paste a link with `502drive copy`, or pick a screen below.",
	JobKeys:         "p pause · u resume · c cancel · t retry · R report · E export CSV · esc back",
	DetailKeys:      "↑↓ select · enter open · ← / esc back",
	CollKeys:        "↑↓ select · enter open · ← / esc back · d daily · h hourly · e every 6h · t realtime · n run now · p pause · x stop",
	DestKeys:        "enter set as default · esc back",
	AccountKeys:     "l sign in / reconnect Google · d run the offline doctor checks",
	SettingsKeys:    "l switch language · esc back",
	FooterKeys:      "1-6 screens · ↑↓ move · enter open · esc back · ? help · q quit",
	FooterJob:       "p pause · u resume · c cancel · t retry · R report · E CSV · esc back",
	FooterColl:      "d/h/e/t enable · n run · p pause · x stop · esc back · q quit",
	FooterHelp:      "esc back · q quit",
	FooterList:      "space select · a all · p pause · u resume · c cancel · t retry · enter open · q quit",
	FooterLib:       "space select · a all · n run · p pause · x stop · d/h/e/t schedule · enter open · q quit",
	Selected:        "%d selected",
	ConfirmCan:      "Cancel %d job(s)? [y/N]",
	ConfirmStop:     "Stop sync for %d source(s)? [y/N]",
	BulkResult:      "%s: %d done, %d skipped, %d failed",
	Live:            "live",
	Reconnecting:    "reconnecting…",
	FilesPerSec:     "%.0f items/s",
	ETALeft:         "%s left",
	ScrollPos:       "%d–%d / %d",
	GreetMorning:    "Good morning! Ready to copy.",
	GreetDay:        "Hello! Paste a link to start.",
	GreetEvening:    "Good evening! All quiet.",
	ThrottleLine:    "Workers: %d · throttled %d/5m",
	ThrottleRetry:   "Google limit until %s",
	StayRunning:     "502Drive keeps running in the tray. Choose Quit in the tray to stop it.",
	WorkingNow:      "Copying %d / %d items",
	AllDone:         "All done — nothing is running.",
	NoticeDone:      "Copy finished",
	NoticePart:      "Copy finished partly",
	NoticeFail:      "Copy failed",
	LoginHome:       "Google is not connected — press l to sign in.",
	LoginWaiting:    "Waiting for you to approve on the Google page…",
	LoginOpen:       "▶ Click here to open the Google sign-in page",
	LoginDone:       "Google is connected.",
	LoginFailed:     "Sign-in did not complete. Press l to try again.",
	LoginExpired:    "The sign-in link expired. Press l for a new one.",
	LoginHide:       "The link is valid for about 10 minutes · esc hides it",
	Drive:           "Drive",
	FooterDrive:     "↑↓ move · enter/→ open · ←/⌫ up · / search · i info · n new folder · c copy · d make destination · esc back",
	FooterPick:      "↑↓ move · enter/→ open · ←/⌫ up · y use this open folder · n new folder · esc cancel",
	FooterCopySrc:   "enter check · ctrl+v paste · esc cancel",
	FooterCopyPlan:  "enter start copy · 1/2/3 name · d where · s sync · f/o/k options · esc cancel",
	DriveEmpty:      "This folder is empty.",
	Loading:         "Loading…",
	MoreBelow:       "… more below, scroll down to load",
	NewFolderPrompt: "New folder name (Enter creates, Esc cancels)",
	FolderCreated:   "Created folder “%s”.",
	PickHint:        "CHOOSE A DESTINATION — open a folder, then press y",
	DestSet:         "“%s” is now the default destination.",
	CopyTitle:       "Copy a link",
	CopyPaste:       "Paste a Google Drive link or ID",
	CopyChecking:    "Checking…",
	CopyFrom:        "Source",
	CopyTo:          "To",
	CopyOutName:     "Output folder name",
	NameKeep:        "1 keep the source name",
	NameAuto:        "2 make a new name",
	NameCustom:      "3 type a name",
	NamePrompt:      "Output folder name (Enter confirms, Esc cancels)",
	PreviewCount:    "%d files, %d folders%s",
	PreviewPartial:  " (seen so far, not a full scan)",
	CopyStarted:     "Copy started — job %s.",
	MetaSize:        "Size",
	MetaType:        "Type",
	MetaModified:    "Modified",
	MetaCreated:     "Created",
	MetaOwner:       "Owner",
	MetaID:          "ID",
	MetaLink:        "Link",
	MetaCopy:        "Copy allowed",
	MetaWrite:       "Can add files",
	Yes:             "yes",
	No:              "no",
	TabHome:         `Home`,
	TabJobs:         `Jobs`,
	TabLibrary:      `Library`,
	TabDest:         `Folders`,
	TabAccount:      `Account`,
	TabSettings:     `Settings`,
	TabDrive:        `Drive`,
	TabHint:         `Tab/←→ switch`,
	TabFocusHint:    `←/→ switch tab · ↓ or Enter to go into the page`,
	SyncOption:      `Keep in sync after copy`,
	SettingsKeys2:   `←/→ or Enter changes it at once · ↑ to the tab bar · esc back`,
	PanelStart:      `Get started`,
	PanelTip1:       `Paste a Drive link below, press Enter to copy`,
	PanelTip2:       `Type / for commands · 1–7 switch screens`,
	PanelRecent:     `Recent activity`,
	NoRecent:        `No jobs yet`,
	PromptHint:      `Paste a Drive link, or type / for commands`,
	DestLine:        `Destination`,
	NoDestLine:      `not set (press 7 → d)`,
	FooterHome:      `/ commands · 1–7 screens · c copy · l sign in · ? help · q quit`,
	CmdCopy:         `Copy a link (choose the name and destination)`,
	CmdJobs:         `Jobs list`,
	CmdLibrary:      `Library and sync`,
	CmdDest:         `Destination folders`,
	CmdAccount:      `Google account and checks`,
	CmdSettings:     `Language and timezone`,
	CmdDrive:        `Browse Google Drive`,
	CmdLogin:        `Sign in / reconnect Google`,
	CmdHelp:         `Key map`,
	CmdRefresh:      `Refresh now`,
	CmdQuit:         `Leave the TUI (the daemon keeps running)`,
	UnknownCmd:      `No command “%s”. Type / for the list.`,
	Language:        "Language", Timezone: "Timezone", Save: "Save", Cancel: "Cancel",
	Pause: "Pause", Resume: "Resume", Retry: "Retry", RunNow: "Run now",
	EnableSync: "Enable sync", ChangeSched: "Change schedule", StopSync: "Stop sync",
	PauseSync: "Pause sync", ResumeSync: "Resume sync",
	PresetDaily: "Daily at 22:00", PresetHourly: "Every hour", PresetSixH: "Every 6 hours",
	PresetReal: "Realtime (change polling)", SyncOff: "off", PickPreset: "Pick a sync schedule",
	OpenTUI: "Open TUI", PasteLink: "Copy link in clipboard", JobsLibrary: "Jobs & library",
	OpenResult: "Open latest result folder", StartDaemon: "Start daemon", StopDaemon: "Stop daemon",
	StopApp:        "Stop 502Drive…",
	StopAppConfirm: "Stop 502Drive (%d job(s) running)? Click again to confirm.",
	Quit:           "Quit tray",
	TrayClone:      "Clone from clipboard", TrayOpenApp: "Open 502Drive", TrayStartApp: "Start 502Drive",
	TrayOpenAll: "Open all", TrayPauseAll: "Pause all", TrayResumeAll: "Resume all",
	TrayReasonDaemon: "— daemon not running", TrayReasonGoogle: "— Google not connected",
	TrayPausedLine: "Google: %s · queue paused · %d need action",
	StateReconnect: "reconnecting",
	TrayNeedsHint:  "Something needs you: open 502Drive › Jobs to see why and how to fix it.",
	TrayStartHint:  "Click “Start 502Drive” to begin.",
	StateIdle:      "idle", StateBusy: "copying", StateNeeds: "needs action", StateError: "error", StateOffline: "offline",
	StateQueued: "queued", StateRunning: "running", StateDone: "done", StatePartial: "partly done",
	StateFailed: "failed", StatePaused: "paused",
	StatusLine:    "Google: %s · %d running · %d queued · %d blocked",
	TrayGoogleRow: "Google: %s", TrayWorkRow: "%d running · %d queued", TrayPausedRow: "Queue paused", TrayBlockedRow: "%d need action",
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
	Autostart: "Start on login",
}

var osLocaleHook func() string

// OSLocale returns the current operating system locale string.
func OSLocale() string {
	if osLocaleHook != nil {
		return osLocaleHook()
	}
	return defaultOSLocale()
}

// Resolve maps a language preference ("auto", "vi", "en", or custom locale)
// to "vi" or "en".
// "auto" or empty inspects the OS locale: only "vi*" maps to "vi", everything else maps to "en".
func Resolve(lang string) string {
	l := strings.ToLower(strings.TrimSpace(lang))
	if l == "" || l == "auto" {
		loc := strings.ToLower(OSLocale())
		if strings.HasPrefix(loc, "vi") {
			return "vi"
		}
		return "en"
	}
	if strings.HasPrefix(l, "vi") {
		return "vi"
	}
	return "en"
}

// For returns the table for a language code; unknown codes fall back to English.
func For(lang string) map[Key]string {
	if Resolve(lang) == "vi" {
		return VI
	}
	return EN
}

// FormatTime formats a timestamp according to locale and timezone.
// For Vietnamese: "02/01/2006 15:04".
// For English/default: "2006-01-02 15:04".
func FormatTime(t time.Time, loc *time.Location, lang string) string {
	if t.IsZero() {
		return ""
	}
	if loc != nil {
		t = t.In(loc)
	}
	if Resolve(lang) == "vi" {
		return t.Format("02/01/2006 15:04")
	}
	return t.Format("2006-01-02 15:04")
}

// FormatDate formats a date according to locale and timezone.
// For Vietnamese: "02/01/2006".
// For English/default: "2006-01-02".
func FormatDate(t time.Time, loc *time.Location, lang string) string {
	if t.IsZero() {
		return ""
	}
	if loc != nil {
		t = t.In(loc)
	}
	if Resolve(lang) == "vi" {
		return t.Format("02/01/2006")
	}
	return t.Format("2006-01-02")
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
	if Resolve(lang) == "vi" {
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
			"            f lọc theo trạng thái · s chỉ nguồn này · K/J đổi chỗ job chờ · +/- ưu tiên",
			"            P/U tạm dừng/tiếp tục cả hàng đợi · E xuất lịch sử CSV · cuối danh sách: tải thêm",
			"Job:        R báo cáo lỗi/bỏ qua theo mã · E xuất báo cáo CSV (không ghi đè tệp có sẵn)",
			"Library:    d hằng ngày 22:00 · h mỗi giờ · e mỗi 6 giờ · t realtime",
			"            n chạy ngay · p tạm dừng sync · x dừng sync · H hiện cả nguồn đã ngừng",
			"Nguồn đã copy: a kiểm tra toàn vẹn · v kết quả · f lập kế hoạch sửa · F sửa",
			"            b bộ lọc · o ngừng theo dõi · u theo dõi lại · c dọn đích · w quên",
			"Destination: enter đặt làm mặc định",
			"Drive:      ↑↓ ←→ duyệt · / tìm theo tên · i thông tin · n thư mục mới · c copy · d đặt làm đích",
			"Home:       c copy một link (kèm chọn thư mục đích và cách đặt tên)",
			"Hộp copy:  f lọc · o trùng tên · k shortcut · gặp bản cũ: u cập nhật / n tạo mới",
			"            s đồng bộ sau copy · i quét đầy đủ · x hủy quét · dán nhiều link = một lô",
			"Account:    l đăng nhập / kết nối lại Google · d chạy kiểm tra offline",
			"Settings:   ↑↓ chọn dòng · ←/→ hoặc Enter đổi (ngôn ngữ, tự chạy, giờ yên tĩnh, giữ lịch sử, thông báo, đồng bộ sau copy)",
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
		"            f filter by state · s only this source · K/J move a queued job · +/- priority",
		"            P/U pause/resume the whole queue · E export history CSV · at the end: load more",
		"Job:        R report of failed/skipped items by code · E export the report as CSV (never overwrites)",
		"Library:    d daily 22:00 · h hourly · e every 6h · t realtime",
		"            n run now · p pause sync · x stop sync · H show retired too",
		"Collection: a integrity check · v result · f plan repair · F repair",
		"            b filter · o stop following · u follow again · c clean up destination · w forget",
		"Destination: enter sets the default folder",
		"Drive:      ↑↓ ←→ browse · / search by name · i info · n new folder · c copy · d make destination",
		"Home:       c copy a link (choose the destination folder and the name)",
		"Copy box:   f filter · o name clash · k shortcuts · on an old copy: u update / n make new",
		"            s sync after copy · i full scan · x cancel scan · several links pasted = one batch",
		"Account:    l sign in / reconnect Google · d run the offline doctor checks",
		"Settings:   ↑↓ pick a row · ←/→ or Enter changes it (language, autostart, quiet hours, history, notifications, sync after copy)",
	}
}
