package i18n

// One-line explanations shown under the highlighted row of a menu, so each
// option says what it does and what it will not do. Vietnamese and English.

const (
	DescPresetDaily  Key = "desc_preset_daily"
	DescPresetHourly Key = "desc_preset_hourly"
	DescPresetSixH   Key = "desc_preset_six_hour"
	DescPresetReal   Key = "desc_preset_realtime"
	DescRunNow       Key = "desc_run_now"
	DescPauseSync    Key = "desc_pause_sync"
	DescResumeSync   Key = "desc_resume_sync"
	DescStopSync     Key = "desc_stop_sync"

	DescRowCopyState  Key = "desc_row_copy_state"
	DescRowSync       Key = "desc_row_sync"
	DescRowOutput     Key = "desc_row_output"
	DescRowFilter     Key = "desc_row_filter"
	DescRowIntegrity  Key = "desc_row_integrity"
	DescRowProtection Key = "desc_row_protection"

	DescSetLanguage   Key = "desc_set_language"
	DescSetAutostart  Key = "desc_set_autostart"
	DescSetQuiet      Key = "desc_set_quiet"
	DescSetRetention  Key = "desc_set_retention"
	DescSetNotifyDone Key = "desc_set_notify_done"
	DescSetNotifyFail Key = "desc_set_notify_failed"
	DescSetNotifyAct  Key = "desc_set_notify_action"
	DescSetAfterSync  Key = "desc_set_after_sync"
	DescSetClearHist  Key = "desc_set_clear_history"
)

var describeVI = map[Key]string{
	DescPresetDaily:  "Mỗi ngày lúc 22:00 (giờ của bạn), 502Drive copy thêm những gì mới ở nguồn sang bản copy.",
	DescPresetHourly: "Mỗi giờ kiểm tra nguồn và copy phần mới. Hợp với nguồn thay đổi thường xuyên.",
	DescPresetSixH:   "Mỗi 6 giờ kiểm tra nguồn và copy phần mới. Nhẹ hơn \"Mỗi giờ\".",
	DescPresetReal:   "Theo dõi thay đổi ở nguồn và copy gần như ngay. Tốn nhiều lượt gọi Google hơn lịch cố định.",
	DescRunNow:       "Chạy một lượt đồng bộ ngay, không chờ lịch.",
	DescPauseSync:    "Tạm dừng các lượt tự chạy. Những gì đã copy được giữ nguyên.",
	DescResumeSync:   "Cho đồng bộ chạy lại; phần nguồn đã đổi trong lúc dừng sẽ được copy.",
	DescStopSync:     "Ngừng đồng bộ tự động. Bản copy và file vẫn còn, bật lại bằng cách chọn một lịch.",

	DescRowCopyState:  "Trạng thái của bản copy: sẵn sàng, đang copy hoặc đã ngừng theo dõi.",
	DescRowSync:       "Đồng bộ: tự động copy phần mới ở nguồn theo lịch. Enter để chọn lịch hoặc chạy ngay.",
	DescRowOutput:     "Thư mục chứa bản copy trên Drive. Enter để mở trên trình duyệt.",
	DescRowFilter:     "Bộ lọc: chọn loại file, dung lượng, ngày hoặc thư mục nào được copy. Không xóa gì đã copy.",
	DescRowIntegrity:  "Kiểm tra toàn vẹn: so bản copy với nguồn, chỉ đọc. Có thể sửa lỗi sau khi xem trước.",
	DescRowProtection: "Bảo vệ: mirror (file xóa ở nguồn vào thùng rác Drive) và điểm khôi phục. Mặc định tắt.",

	DescSetLanguage:   "Ngôn ngữ của ứng dụng.",
	DescSetAutostart:  "Tự chạy 502Drive khi bạn đăng nhập máy.",
	DescSetQuiet:      "Trong khoảng giờ này không gửi thông báo tự phát (Telegram/máy tính).",
	DescSetRetention:  "Tự xóa lịch sử job cũ hơn số ngày này mỗi ngày một lần. Không đụng tới bản copy.",
	DescSetNotifyDone: "Báo khi một job copy hoàn tất.",
	DescSetNotifyFail: "Báo khi một job thất bại.",
	DescSetNotifyAct:  "Báo khi cần bạn làm gì đó (đăng nhập lại, hạn mức, job bị chặn).",
	DescSetAfterSync:  "Sau mỗi lần copy xong, tự bật đồng bộ theo chế độ này.",
	DescSetClearHist:  "Xóa job đã xong khỏi lịch sử ngay (xem trước, rồi xác nhận). Không đụng tới bản copy và Drive.",
}

var describeEN = map[Key]string{
	DescPresetDaily:  "Every day at 22:00 (your time) 502Drive copies what is new at the source into the copy.",
	DescPresetHourly: "Checks the source every hour and copies what is new. Good for sources that change often.",
	DescPresetSixH:   "Checks the source every 6 hours and copies what is new. Lighter than every hour.",
	DescPresetReal:   "Watches the source for changes and copies almost at once. Uses more Google calls than a fixed schedule.",
	DescRunNow:       "Runs one sync pass now, without waiting for the schedule.",
	DescPauseSync:    "Pauses the automatic passes. What was already copied stays.",
	DescResumeSync:   "Lets sync run again; what changed at the source meanwhile is copied.",
	DescStopSync:     "Stops automatic sync. The copy and its files stay; pick a schedule to turn it on again.",

	DescRowCopyState:  "State of the copy: ready, copying or no longer tracked.",
	DescRowSync:       "Sync: automatically copies what is new at the source on a schedule. Enter to pick a schedule or run now.",
	DescRowOutput:     "The folder holding the copy on Drive. Enter opens it in the browser.",
	DescRowFilter:     "Filter: choose which file types, sizes, dates or folders are copied. Never deletes what was copied.",
	DescRowIntegrity:  "Integrity check: compares the copy with the source, read-only. Fixes only after a preview.",
	DescRowProtection: "Protection: mirror (files deleted at the source go to the Drive trash) and restore points. Off by default.",

	DescSetLanguage:   "The language of the app.",
	DescSetAutostart:  "Start 502Drive when you log in.",
	DescSetQuiet:      "During these hours no unsolicited notifications (Telegram/desktop) are sent.",
	DescSetRetention:  "Once a day, old job history beyond this many days is deleted. Your copies are not touched.",
	DescSetNotifyDone: "Tell me when a copy job finishes.",
	DescSetNotifyFail: "Tell me when a job fails.",
	DescSetNotifyAct:  "Tell me when something needs you (sign in again, quota, a blocked job).",
	DescSetAfterSync:  "After each copy finishes, turn sync on in this mode.",
	DescSetClearHist:  "Clear finished jobs from the history now (preview, then confirm). Copies and Drive are untouched.",
}

func init() {
	for k, v := range describeVI {
		VI[k] = v
	}
	for k, v := range describeEN {
		EN[k] = v
	}
}
