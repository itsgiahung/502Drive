package present

import "strings"

type Explanation struct {
	Code    string `json:"code"`
	Title   string `json:"title"`
	Message string `json:"message"`
	Action  string `json:"action"`
	Known   bool   `json:"-"`
}

type explanationText struct{ title, message, action string }

var explanations = map[string][2]explanationText{
	"filtered":                   {{"excluded by the filter", "Excluded by the collection filter.", "Review the collection filter."}, {"bị bộ lọc loại", "Mục này bị bộ lọc của bộ sưu tập loại.", "Kiểm tra bộ lọc của bộ sưu tập."}},
	"shortcut_unresolved":        {{"shortcut could not be opened", "The shortcut target is missing, trashed or unreadable.", "Check access to the shortcut target."}, {"shortcut không mở được", "Không đọc được đích của shortcut.", "Kiểm tra đích shortcut và quyền truy cập."}},
	"shortcut_loop":              {{"shortcut loop", "The shortcut points back into itself.", "Skip this shortcut or fix its target."}, {"shortcut bị vòng lặp", "Shortcut trỏ vòng về chính nó.", "Bỏ qua shortcut hoặc sửa đích."}},
	"cannot_copy":                {{"copying not allowed", "The owner does not allow copying this item.", "Ask the owner to allow copying."}, {"không có quyền copy", "Chủ sở hữu không cho phép copy mục này.", "Nhờ chủ sở hữu cấp quyền copy."}},
	"google_native_export_only":  {{"export only", "Google-native document; server-side copy is not supported.", "Export manually outside 502Drive if permitted."}, {"chỉ hỗ trợ xuất", "Định dạng Google này không hỗ trợ copy phía máy chủ.", "Xuất thủ công bên ngoài 502Drive nếu được phép."}},
	"too_large_for_quota":        {{"insufficient space", "Larger than the destination's remaining quota.", "Free space or choose another destination."}, {"thiếu dung lượng", "Mục này lớn hơn dung lượng đích còn lại.", "Dọn dung lượng hoặc chọn đích khác."}},
	"unreadable":                 {{"cannot be read", "Permission denied or the source cannot be read.", "Check the source and its sharing settings."}, {"không đọc được nguồn", "Không đọc được mục nguồn hoặc thiếu quyền.", "Kiểm tra nguồn và quyền chia sẻ."}},
	"trashed_source":             {{"source is in the trash", "The source item is in the Drive trash.", "Restore the source if it is still needed."}, {"nguồn nằm trong thùng rác", "Mục nguồn đang ở thùng rác Drive.", "Khôi phục nguồn nếu vẫn cần."}},
	"conflict":                   {{"name taken, skipped", "A name conflict was skipped by policy.", "Review the name conflict policy."}, {"trùng tên", "Mục trùng tên được bỏ qua theo chính sách.", "Kiểm tra chính sách trùng tên."}},
	"quota_exceeded":             {{"quota reached", "Google reported a quota limit.", "Wait until resume_after, or check Google quota and destination space."}, {"đã chạm hạn mức", "Google báo đã chạm hạn mức.", "Chờ mốc tiếp tục hoặc kiểm tra hạn mức Google và dung lượng đích."}},
	"permission_denied":          {{"access denied", "This account cannot access the item.", "Ask the owner for the required access."}, {"không có quyền", "Tài khoản này không được truy cập mục.", "Nhờ chủ sở hữu cấp quyền cần thiết."}},
	"unknown_remote_outcome":     {{"write not confirmed", "A remote write may have happened but is not confirmed.", "Retry to reconcile the existing operation; do not submit a new copy."}, {"chưa xác nhận kết quả", "Thao tác có thể đã ghi lên Drive nhưng chưa xác nhận được.", "Thử lại để đối soát thao tác cũ; đừng tạo bản copy mới."}},
	"multiple_matches":           {{"multiple copies found", "More than one remote item matches this operation.", "Inspect the audit before retrying."}, {"tìm thấy nhiều bản", "Có nhiều mục từ xa khớp với thao tác này.", "Xem báo cáo kiểm tra trước khi thử lại."}},
	"account_reconnect_required": {{"sign in again", "Google access needs to be connected again.", "Run 502drive account login."}, {"cần đăng nhập lại", "Cần kết nối lại quyền truy cập Google.", "Chạy 502drive account login."}},
	"source_not_found":           {{"source not found", "The source is missing or not visible to this account.", "Check the source link and sharing permissions."}, {"không thấy nguồn", "Nguồn không tồn tại hoặc tài khoản này không nhìn thấy.", "Kiểm tra link nguồn và quyền chia sẻ."}},
	"rate_limited":               {{"requests limited", "Google is limiting requests temporarily.", "Wait for the reported retry time."}, {"đang bị giới hạn nhịp", "Google tạm giới hạn số yêu cầu.", "Chờ mốc thử lại được báo."}},
	"provider_unavailable":       {{"Drive unavailable", "Drive could not be reached or did not answer successfully.", "Check the connection and retry later."}, {"chưa kết nối được Drive", "Không kết nối được Drive hoặc Drive chưa trả lời thành công.", "Kiểm tra mạng và thử lại sau."}},
	"request_rejected":           {{"request rejected", "Drive rejected this request.", "Check the source and destination, then run doctor."}, {"yêu cầu bị từ chối", "Drive từ chối yêu cầu này.", "Kiểm tra nguồn, đích rồi chạy doctor."}},
	"retry_exhausted":            {{"retries exhausted", "The operation could not finish after repeated attempts.", "Check the problem report, then retry this job."}, {"đã hết lượt thử", "Thao tác chưa hoàn tất sau nhiều lần thử.", "Xem báo cáo vấn đề rồi thử lại job này."}},
	"waiting_for_network":        {{"waiting for network", "The machine could not connect to Drive.", "Restore the connection; the job keeps its progress."}, {"đang chờ mạng", "Máy chưa kết nối được Drive.", "Khôi phục kết nối; job vẫn giữ tiến độ."}},
	"destination_missing":        {{"destination missing", "The recorded destination is missing or trashed.", "Inspect the collection and run an audit."}, {"không thấy đích", "Đích đã lưu không tồn tại hoặc đang ở thùng rác.", "Xem bộ sưu tập và chạy kiểm tra."}},
	"destination_not_owned":      {{"ownership changed", "The destination no longer has the expected 502Drive intent.", "Inspect the audit; do not modify this item automatically."}, {"dấu sở hữu đã đổi", "Đích không còn mang dấu ý định 502Drive như dự kiến.", "Xem kiểm tra; không tự sửa mục này."}},
	"root_not_created":           {{"destination not created", "The destination folder could not be created.", "Check write permission and retry the job."}, {"chưa tạo được đích", "Chưa tạo được thư mục đích.", "Kiểm tra quyền ghi rồi thử lại job."}},
	"items_failed":               {{"some items failed", "Some items could not be copied.", "Open the problem report and retry failed items."}, {"một số mục bị lỗi", "Một số mục chưa copy được.", "Mở báo cáo vấn đề rồi thử lại các mục lỗi."}},
	"source_mass_removal":        {{"source changed widely", "Many previously mapped source items are no longer visible.", "Review the source and audit before continuing."}, {"nguồn thay đổi nhiều", "Nhiều mục nguồn đã theo dõi không còn nhìn thấy.", "Kiểm tra nguồn và báo cáo trước khi tiếp tục."}},
	"invalid_filter":             {{"invalid filter", "The saved filter is not valid.", "Edit the collection filter."}, {"bộ lọc không hợp lệ", "Bộ lọc đã lưu không hợp lệ.", "Sửa bộ lọc của bộ sưu tập."}},
	"collection_missing":         {{"collection missing", "The collection record is missing.", "Run doctor and inspect local state."}, {"thiếu bộ sưu tập", "Không tìm thấy bản ghi bộ sưu tập.", "Chạy doctor và kiểm tra trạng thái cục bộ."}},
	"audit_missing":              {{"audit missing", "The audit record or its collection is missing.", "Start a fresh audit."}, {"thiếu báo cáo kiểm tra", "Không tìm thấy kiểm tra hoặc bộ sưu tập của nó.", "Chạy kiểm tra mới."}},
	"drive_error":                {{"Drive error", "A Drive operation failed.", "Run doctor and review the job report."}, {"lỗi Drive", "Một thao tác Drive bị lỗi.", "Chạy doctor và xem báo cáo job."}},
	"trash_failed":               {{"trash not confirmed", "Moving an owned object to Drive trash was not confirmed.", "Run an audit before retrying cleanup."}, {"chưa xác nhận chuyển thùng rác", "Chưa xác nhận được việc chuyển mục sở hữu vào thùng rác Drive.", "Chạy kiểm tra trước khi thử dọn lại."}},
	"unknown":                    {{"unknown reason", "The daemon did not record a reason.", "Refresh the report and run doctor if needed."}, {"chưa rõ lý do", "Daemon chưa ghi lại lý do.", "Đọc lại báo cáo và chạy doctor nếu cần."}},
	"destination_not_ours":       {{"ownership changed", "The destination no longer has the expected 502Drive intent.", "Inspect the audit before continuing."}, {"dấu sở hữu đã đổi", "Đích không còn mang dấu ý định 502Drive như dự kiến.", "Xem kiểm tra trước khi tiếp tục."}},
	"skip_write_failed":          {{"local state not saved", "The skip decision could not be saved locally.", "Check disk space and run doctor."}, {"chưa lưu trạng thái", "Chưa lưu được quyết định bỏ qua vào trạng thái cục bộ.", "Kiểm tra dung lượng máy và chạy doctor."}},
	"name_exhausted":             {{"no available name", "No available destination name was found.", "Choose a different root name or review the destination."}, {"hết tên khả dụng", "Chưa tìm được tên khả dụng ở đích.", "Chọn tên gốc khác hoặc kiểm tra đích."}},
	"scan_incomplete":            {{"scan incomplete", "Some folders could not be read.", "Review permissions and run the audit again."}, {"quét chưa đủ", "Một số thư mục chưa đọc được.", "Kiểm tra quyền và chạy kiểm tra lại."}},
	"findings_unverified":        {{"findings unverified", "Some findings could not be verified against Drive.", "Run the audit again before repair."}, {"chưa xác minh phát hiện", "Một số phát hiện chưa xác minh được với Drive.", "Kiểm tra lại trước khi sửa."}},
	"cleanup_failed":             {{"cleanup incomplete", "Some superseded copies could not be moved to trash.", "Run an audit before retrying repair."}, {"dọn chưa xong", "Một số bản cũ chưa chuyển được vào thùng rác.", "Chạy kiểm tra trước khi thử sửa lại."}},
	"needs_action":               {{"needs action", "The collection needs attention before syncing.", "Review its latest job and audit."}, {"cần xử lý", "Bộ sưu tập cần xử lý trước khi đồng bộ.", "Xem job và kiểm tra gần nhất."}},
	"replay_overflow":            {{"changes need reconciliation", "Buffered changes exceeded the replay limit.", "Wait for the full reconciliation."}, {"cần đối soát thay đổi", "Thay đổi đã vượt giới hạn bộ đệm phát lại.", "Chờ lần đối soát đầy đủ."}},
	"throttled":                  {{"requests limited", "Google is limiting requests temporarily.", "Wait for the reported retry time."}, {"đang bị giới hạn nhịp", "Google tạm giới hạn số yêu cầu.", "Chờ mốc thử lại được báo."}},
	"unavailable":                {{"Drive unavailable", "Drive could not be reached or did not answer successfully.", "Check the connection and retry later."}, {"chưa kết nối được Drive", "Không kết nối được Drive hoặc Drive chưa trả lời thành công.", "Kiểm tra mạng và thử lại sau."}},
	"server":                     {{"Drive unavailable", "Drive could not be reached or did not answer successfully.", "Check the connection and retry later."}, {"chưa kết nối được Drive", "Không kết nối được Drive hoặc Drive chưa trả lời thành công.", "Kiểm tra mạng và thử lại sau."}},
	"unreachable":                {{"waiting for network", "The machine could not connect to Drive.", "Restore the connection; the job keeps its progress."}, {"đang chờ mạng", "Máy chưa kết nối được Drive.", "Khôi phục kết nối; job vẫn giữ tiến độ."}},
	"auth":                       {{"sign in again", "Google access needs to be connected again.", "Run 502drive account login."}, {"cần đăng nhập lại", "Cần kết nối lại quyền truy cập Google.", "Chạy 502drive account login."}},
	"permission":                 {{"access denied", "This account cannot access the item.", "Ask the owner for the required access."}, {"không có quyền", "Tài khoản này không được truy cập mục.", "Nhờ chủ sở hữu cấp quyền cần thiết."}},
	"not_found":                  {{"source not found", "The source is missing or not visible to this account.", "Check the source link and sharing permissions."}, {"không thấy nguồn", "Nguồn không tồn tại hoặc tài khoản này không nhìn thấy.", "Kiểm tra link nguồn và quyền chia sẻ."}},
	"quota":                      {{"quota reached", "Google reported a quota limit.", "Wait until resume_after, or check Google quota and destination space."}, {"đã chạm hạn mức", "Google báo đã chạm hạn mức.", "Chờ mốc tiếp tục hoặc kiểm tra hạn mức Google và dung lượng đích."}},
	"invalid":                    {{"request rejected", "Drive rejected this request.", "Check the source and destination, then run doctor."}, {"yêu cầu bị từ chối", "Drive từ chối yêu cầu này.", "Kiểm tra nguồn, đích rồi chạy doctor."}},
	"cancelled":                  {{"unknown reason", "The daemon did not record a reason.", "Refresh the report and run doctor if needed."}, {"chưa rõ lý do", "Daemon chưa ghi lại lý do.", "Đọc lại báo cáo và chạy doctor nếu cần."}},
}

func Explain(code, lang string) Explanation {
	idx := 0
	if strings.HasPrefix(strings.ToLower(lang), "vi") {
		idx = 1
	}
	texts, ok := explanations[code]
	if !ok {
		texts = explanations["unknown"]
	}
	v := texts[idx]
	if !ok {
		v.message += " (" + code + ")"
	}
	return Explanation{Code: code, Title: v.title, Message: v.message, Action: v.action, Known: ok}
}
func Advice(code, lang string) string { e := Explain(code, lang); return e.Message + " " + e.Action }
