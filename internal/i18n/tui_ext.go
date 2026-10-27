package i18n

// Labels added for the TUI features of batch T: lifecycle banners, the
// integrity audit, retire/forget/cleanup, copy options (filter, name clash,
// shortcuts), quota waiting, skip reasons and the extended Settings screen.
// They live in their own file and merge into the two tables at start-up so the
// completeness tests (both languages, same printf verbs) cover them too.

const (
	LifeReconnecting Key = "life_reconnecting"
	LifeAfterSleep   Key = "life_after_sleep"
	LifeLoginHint    Key = "life_login_hint"
	LifeSleepHint    Key = "life_sleep_hint"

	LibRetiredTag Key = "lib_retired_tag"
	LibRetiredOn  Key = "lib_retired_on"
	CollKeysExt   Key = "collection_keys_ext"
	FooterCollExt Key = "footer_collection_ext"
	FooterLibExt  Key = "footer_library_ext"

	AuditTitle        Key = "audit_title"
	AuditNone         Key = "audit_none"
	AuditRunning      Key = "audit_running"
	AuditClean        Key = "audit_clean"
	AuditFinding      Key = "audit_finding"
	AuditFailed       Key = "audit_failed"
	AuditStarted      Key = "audit_started"
	AuditRepairPlan   Key = "audit_repair_plan"
	AuditRepairing    Key = "audit_repairing"
	AuditConfirm      Key = "audit_confirm_repair"
	AuditMore         Key = "audit_more"
	AuditNothingFix   Key = "audit_nothing_to_fix"
	AuditPlanFirst    Key = "audit_plan_first"
	AuditNeedsReport  Key = "audit_needs_report"
	AuditKindMissing  Key = "audit_kind_missing"
	AuditKindExtra    Key = "audit_kind_extra"
	AuditKindName     Key = "audit_kind_name"
	AuditKindSize     Key = "audit_kind_size"
	AuditKindParent   Key = "audit_kind_parent"
	AuditKindDup      Key = "audit_kind_duplicate"
	AuditKindRemoved  Key = "audit_kind_removed"
	AuditKindUnread   Key = "audit_kind_unreadable"
	AuditRepairable   Key = "audit_repairable_tag"
	AuditRepairedTag  Key = "audit_repaired_tag"
	ConfirmRetire     Key = "confirm_retire"
	ConfirmForget     Key = "confirm_forget"
	NeedRetiredFirst  Key = "need_retired_first"
	NotRetired        Key = "not_retired"
	CleanupPlanning   Key = "cleanup_planning"
	CleanupPlan       Key = "cleanup_plan"
	CleanupEmpty      Key = "cleanup_empty"
	CleanupTypeName   Key = "cleanup_type_name"
	CleanupSent       Key = "cleanup_sent"
	CleanupKeys       Key = "cleanup_keys"
	FilterTitle       Key = "filter_title"
	FilterNone        Key = "filter_none"
	FilterNoVideo     Key = "filter_no_video"
	FilterDocsOnly    Key = "filter_docs_only"
	FilterUnder1GB    Key = "filter_under_1gb"
	FilterCustom      Key = "filter_custom"
	FilterPreview     Key = "filter_preview"
	FilterSamples     Key = "filter_samples"
	FilterTopLevel    Key = "filter_top_level"
	FilterApplyLater  Key = "filter_apply_later"
	FilterSaved       Key = "filter_saved"
	ConflictTitle     Key = "conflict_title"
	ConflictRename    Key = "conflict_rename"
	ConflictSkip      Key = "conflict_skip"
	ConflictUpdate    Key = "conflict_update"
	ShortcutsTitle    Key = "shortcuts_title"
	ShortcutsFollow   Key = "shortcuts_follow"
	ShortcutsSkip     Key = "shortcuts_skip"
	CopyOptsKeys      Key = "copy_opts_keys"
	ExistingCopyTitle Key = "existing_copy_title"
	ExistingCopyKeys  Key = "existing_copy_keys"
	ExistingCopyAsk   Key = "existing_copy_ask"

	QuotaWait     Key = "quota_wait"
	QuotaWaitShot Key = "quota_wait_short"
	QuotaWaitNote Key = "quota_wait_note"
	QuotaNoRetry  Key = "quota_no_retry"
	FooterJobWait Key = "footer_job_wait"
	SkippedTitle  Key = "skipped_title"
	KindLabel     Key = "kind_label"
	SizeLabel     Key = "size_label"
	JobKindAudit  Key = "job_kind_audit"
	JobKindRepair Key = "job_kind_repair"
	JobKindClean  Key = "job_kind_cleanup"

	SkipFiltered   Key = "skip_filtered"
	SkipConflict   Key = "skip_conflict"
	SkipShortUnres Key = "skip_shortcut_unresolved"
	SkipShortLoop  Key = "skip_shortcut_loop"
	SkipCannotCopy Key = "skip_cannot_copy"
	SkipNativeOnly Key = "skip_google_native_export_only"
	SkipTooLarge   Key = "skip_too_large_for_quota"
	SkipUnreadable Key = "skip_unreadable"
	SkipTrashed    Key = "skip_trashed_source"

	SetLangAuto     Key = "set_lang_auto"
	SetQuiet        Key = "set_quiet"
	SetQuietOff     Key = "set_quiet_off"
	SetQuietPrompt  Key = "set_quiet_prompt"
	SetQuietBad     Key = "set_quiet_bad"
	SetRetention    Key = "set_retention"
	SetDays         Key = "set_days"
	SetForever      Key = "set_forever"
	SetNotifyDone   Key = "set_notify_done"
	SetNotifyFailed Key = "set_notify_failed"
	SetNotifyAction Key = "set_notify_action"
	SetKeys         Key = "set_keys"
	SetKeysQuiet    Key = "set_keys_quiet"
	SetOn           Key = "set_on"
	SetOff          Key = "set_off"
)

func init() {
	for k, v := range tuiExtVI {
		VI[k] = v
	}
	for k, v := range tuiExtEN {
		EN[k] = v
	}
}

var tuiExtVI = map[Key]string{
	LifeReconnecting: "Đang kết nối lại…",
	LifeAfterSleep:   "sau %s ngủ",
	LifeLoginHint:    "nhấn l để đăng nhập lại Google",
	LifeSleepHint:    "việc đang chạy sẽ tiếp tục khi máy dậy",

	LibRetiredTag: "đã ngừng theo dõi",
	LibRetiredOn:  "đang hiện cả nguồn đã ngừng theo dõi (H để ẩn)",
	CollKeysExt:   "a kiểm tra toàn vẹn · v kết quả · f lập kế hoạch sửa · F sửa · b bộ lọc · o ngừng theo dõi · u theo dõi lại · c dọn đích · w quên",
	FooterCollExt: "a kiểm tra · v kết quả · f/F sửa · b lọc · o ngừng · u theo dõi lại · c dọn đích · w quên · esc",
	FooterLibExt:  "space chọn · n chạy · p tạm dừng · x dừng · d/h/e/t lịch · H cả nguồn đã ngừng · enter mở · q",

	AuditTitle:        "Kiểm tra toàn vẹn",
	AuditNone:         "chưa kiểm tra (nhấn a để quét)",
	AuditRunning:      "đang quét… %d thư mục",
	AuditClean:        "không có sai lệch",
	AuditFinding:      "%d sai lệch (%d sửa được)",
	AuditFailed:       "lần quét thất bại",
	AuditStarted:      "Đã bắt đầu kiểm tra (job %s).",
	AuditRepairPlan:   "Đã lập kế hoạch sửa (job %s). Chưa thay đổi gì.",
	AuditRepairing:    "Đang sửa (job %s)…",
	AuditConfirm:      "Sửa %d mục? Chỉ chạm vào bản do 502Drive tạo, không xóa vĩnh viễn. [y/N]",
	AuditMore:         "…và %d mục nữa (xem `502drive audit show %s`)",
	AuditNothingFix:   "Không có gì sửa được.",
	AuditPlanFirst:    "Hãy lập kế hoạch sửa trước (nhấn f) rồi mới sửa.",
	AuditNeedsReport:  "Hãy kiểm tra trước (nhấn a) và đợi kết quả.",
	AuditKindMissing:  "thiếu ở đích",
	AuditKindExtra:    "thừa ở đích (chỉ báo)",
	AuditKindName:     "sai tên",
	AuditKindSize:     "khác dung lượng hoặc md5",
	AuditKindParent:   "sai thư mục cha",
	AuditKindDup:      "trùng ở đích",
	AuditKindRemoved:  "nguồn đã bị xóa",
	AuditKindUnread:   "không đọc được nguồn",
	AuditRepairable:   "sửa được",
	AuditRepairedTag:  "đã sửa",
	ConfirmRetire:     "Ngừng theo dõi “%s”? Bản sao trên Drive và lịch sử được giữ, đồng bộ tắt. [y/N]",
	ConfirmForget:     "Quên “%s”? Xóa lịch sử và ánh xạ trong máy, không đụng Drive. [y/N]",
	NeedRetiredFirst:  "Cần ngừng theo dõi trước (nhấn o).",
	NotRetired:        "Nguồn này chưa ngừng theo dõi.",
	CleanupPlanning:   "Đang lập kế hoạch dọn đích (chưa xóa gì)…",
	CleanupPlan:       "Dọn đích: %d mục · %s sẽ vào thùng rác Drive (khôi phục trong 30 ngày). Nhấn C để xác nhận.",
	CleanupEmpty:      "Dọn đích: không có gì trong thư mục đích để chuyển vào thùng rác.",
	CleanupTypeName:   "Gõ lại tên thư mục “%s” để chuyển vào thùng rác:",
	CleanupSent:       "Đã gửi lệnh dọn đích (job %s).",
	CleanupKeys:       "Enter xác nhận · esc hủy",
	FilterTitle:       "Bộ lọc",
	FilterNone:        "không lọc",
	FilterNoVideo:     "bỏ video",
	FilterDocsOnly:    "chỉ tài liệu",
	FilterUnder1GB:    "bỏ file trên 1 GB",
	FilterCustom:      "tùy chỉnh (%d luật)",
	FilterPreview:     "Đạt: %d · Bị loại: %d",
	FilterSamples:     "Mẫu bị loại",
	FilterTopLevel:    "chỉ tính cấp cao nhất",
	FilterApplyLater:  "áp dụng cho lần chạy sau",
	FilterSaved:       "Đã lưu bộ lọc: %s",
	ConflictTitle:     "Trùng tên",
	ConflictRename:    "đổi tên",
	ConflictSkip:      "bỏ qua",
	ConflictUpdate:    "cập nhật",
	ShortcutsTitle:    "Shortcut",
	ShortcutsFollow:   "đi theo",
	ShortcutsSkip:     "bỏ qua",
	CopyOptsKeys:      "f lọc · o trùng tên · k shortcut",
	ExistingCopyTitle: "Nguồn này đã được sao chép tới đích này.",
	ExistingCopyKeys:  "u cập nhật bản cũ · n tạo bản mới · esc hủy",
	ExistingCopyAsk:   "Bạn muốn làm gì?",

	QuotaWait:     "Chờ quota tới %s",
	QuotaWaitShot: "chờ tới %s",
	QuotaWaitNote: "tự tiếp tục, không cần thử lại",
	QuotaNoRetry:  "Job tự tiếp tục lúc %s, không cần thử lại.",
	FooterJobWait: "p tạm dừng · c huỷ · esc quay lại · q thoát",
	SkippedTitle:  "Bỏ qua",
	KindLabel:     "Loại",
	SizeLabel:     "Dung lượng",
	JobKindAudit:  "kiểm tra toàn vẹn",
	JobKindRepair: "sửa sai lệch",
	JobKindClean:  "dọn đích",

	SkipFiltered:   "bị bộ lọc loại",
	SkipConflict:   "trùng tên, đã bỏ qua",
	SkipShortUnres: "shortcut không mở được",
	SkipShortLoop:  "shortcut quay vòng",
	SkipCannotCopy: "không có quyền copy",
	SkipNativeOnly: "tệp Google, chỉ xuất được",
	SkipTooLarge:   "quá lớn so với quota",
	SkipUnreadable: "không đọc được",
	SkipTrashed:    "nguồn nằm trong thùng rác",

	SetLangAuto:     "Tự động",
	SetQuiet:        "Giờ yên tĩnh",
	SetQuietOff:     "tắt",
	SetQuietPrompt:  "Giờ yên tĩnh dạng 22:00-07:00 (để trống = tắt):",
	SetQuietBad:     "Nhập dạng 22:00-07:00, hoặc để trống để tắt.",
	SetRetention:    "Giữ lịch sử",
	SetDays:         "%d ngày",
	SetForever:      "mãi mãi",
	SetNotifyDone:   "Báo khi xong",
	SetNotifyFailed: "Báo khi lỗi",
	SetNotifyAction: "Báo khi cần xử lý",
	SetKeys:         "↑↓ chọn dòng · ←/→ hoặc Enter đổi · ↑ ở dòng đầu lên thanh tab · esc quay lại",
	SetKeysQuiet:    "Enter lưu · esc hủy",
	SetOn:           "bật",
	SetOff:          "tắt",
}

var tuiExtEN = map[Key]string{
	LifeReconnecting: "Reconnecting…",
	LifeAfterSleep:   "after %s asleep",
	LifeLoginHint:    "press l to sign in to Google again",
	LifeSleepHint:    "running jobs continue when the computer wakes",

	LibRetiredTag: "retired",
	LibRetiredOn:  "showing retired sources too (H to hide)",
	CollKeysExt:   "a integrity check · v result · f plan repair · F repair · b filter · o stop following · u follow again · c clean up destination · w forget",
	FooterCollExt: "a check · v result · f/F repair · b filter · o retire · u resume · c clean up · w forget · esc",
	FooterLibExt:  "space select · n run · p pause · x stop · d/h/e/t schedule · H retired too · enter open · q",

	AuditTitle:        "Integrity check",
	AuditNone:         "not checked yet (press a to scan)",
	AuditRunning:      "scanning… %d folders",
	AuditClean:        "no differences",
	AuditFinding:      "%d difference(s) (%d repairable)",
	AuditFailed:       "the scan failed",
	AuditStarted:      "Check started (job %s).",
	AuditRepairPlan:   "Repair plan made (job %s). Nothing changed.",
	AuditRepairing:    "Repairing (job %s)…",
	AuditConfirm:      "Repair %d item(s)? Only objects made by 502Drive are touched; nothing is deleted for good. [y/N]",
	AuditMore:         "…and %d more (see `502drive audit show %s`)",
	AuditNothingFix:   "Nothing can be repaired.",
	AuditPlanFirst:    "Plan the repair first (press f), then repair.",
	AuditNeedsReport:  "Run a check first (press a) and wait for the result.",
	AuditKindMissing:  "missing at destination",
	AuditKindExtra:    "extra at destination (report only)",
	AuditKindName:     "name differs",
	AuditKindSize:     "size or md5 differs",
	AuditKindParent:   "wrong parent folder",
	AuditKindDup:      "duplicate at destination",
	AuditKindRemoved:  "removed at source",
	AuditKindUnread:   "source unreadable",
	AuditRepairable:   "repairable",
	AuditRepairedTag:  "repaired",
	ConfirmRetire:     "Stop following “%s”? The copy on Drive and its history stay; sync is turned off. [y/N]",
	ConfirmForget:     "Forget “%s”? Removes its history and mapping on this machine, Drive is untouched. [y/N]",
	NeedRetiredFirst:  "Stop following first (press o).",
	NotRetired:        "This source is not retired.",
	CleanupPlanning:   "Planning the cleanup (nothing is deleted)…",
	CleanupPlan:       "Cleanup: %d item(s) · %s will go to the Drive trash (restorable for 30 days). Press C to confirm.",
	CleanupEmpty:      "Cleanup: nothing in the destination folder to move to the trash.",
	CleanupTypeName:   "Type the folder name “%s” to move it to the trash:",
	CleanupSent:       "Cleanup sent (job %s).",
	CleanupKeys:       "Enter confirm · esc cancel",
	FilterTitle:       "Filter",
	FilterNone:        "no filter",
	FilterNoVideo:     "no video",
	FilterDocsOnly:    "documents only",
	FilterUnder1GB:    "skip files over 1 GB",
	FilterCustom:      "custom (%d rules)",
	FilterPreview:     "Kept: %d · Excluded: %d",
	FilterSamples:     "Excluded samples",
	FilterTopLevel:    "top level only",
	FilterApplyLater:  "applies to later runs",
	FilterSaved:       "Filter saved: %s",
	ConflictTitle:     "Name clash",
	ConflictRename:    "rename",
	ConflictSkip:      "skip",
	ConflictUpdate:    "update",
	ShortcutsTitle:    "Shortcuts",
	ShortcutsFollow:   "follow",
	ShortcutsSkip:     "skip",
	CopyOptsKeys:      "f filter · o name clash · k shortcuts",
	ExistingCopyTitle: "This source is already copied to this destination.",
	ExistingCopyKeys:  "u update the old copy · n make a new copy · esc cancel",
	ExistingCopyAsk:   "What would you like to do?",

	QuotaWait:     "Waiting for quota until %s",
	QuotaWaitShot: "wait until %s",
	QuotaWaitNote: "resumes by itself, no retry needed",
	QuotaNoRetry:  "The job resumes by itself at %s, no retry needed.",
	FooterJobWait: "p pause · c cancel · esc back · q quit",
	SkippedTitle:  "Skipped",
	KindLabel:     "Type",
	SizeLabel:     "Size",
	JobKindAudit:  "integrity check",
	JobKindRepair: "repair",
	JobKindClean:  "destination cleanup",

	SkipFiltered:   "excluded by the filter",
	SkipConflict:   "name taken, skipped",
	SkipShortUnres: "shortcut could not be opened",
	SkipShortLoop:  "shortcut loops back",
	SkipCannotCopy: "copying not allowed",
	SkipNativeOnly: "Google file, export only",
	SkipTooLarge:   "too large for the quota",
	SkipUnreadable: "cannot be read",
	SkipTrashed:    "source is in the trash",

	SetLangAuto:     "Auto",
	SetQuiet:        "Quiet hours",
	SetQuietOff:     "off",
	SetQuietPrompt:  "Quiet hours as 22:00-07:00 (empty = off):",
	SetQuietBad:     "Use the form 22:00-07:00, or leave empty to turn off.",
	SetRetention:    "Keep history",
	SetDays:         "%d days",
	SetForever:      "forever",
	SetNotifyDone:   "Notify when done",
	SetNotifyFailed: "Notify on failure",
	SetNotifyAction: "Notify when action is needed",
	SetKeys:         "↑↓ pick a row · ←/→ or Enter changes it · ↑ on the first row to the tab bar · esc back",
	SetKeysQuiet:    "Enter save · esc cancel",
	SetOn:           "on",
	SetOff:          "off",
}
