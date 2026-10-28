# Xử lý sự cố

[English](troubleshooting.md) · Tiếng Việt

Bắt đầu bằng `502drive status`, `502drive doctor` và nhật ký (`502drive logs`).
`doctor --online` có gọi tới Google; `doctor` thường chỉ kiểm tra trên máy.

| Hiện tượng | Kiểm tra và cách xử lý |
| --- | --- |
| `daemon_unavailable` | Chạy `502drive up`; kiểm tra địa chỉ và file token riêng tư |
| `unauthenticated` | Dùng đúng file token hiện tại của dịch vụ, đừng dùng token cũ đã chép |
| `not_ready` | Kiểm tra khóa thư mục dữ liệu, quyền file, và phiên bản chương trình có khớp dữ liệu |
| `account_reconnect_required` | Chạy `account login`; cấp lại quyền nếu đã bị thu hồi. Nếu cứ khoảng 7 ngày lại bị, ứng dụng Google còn ở trạng thái Testing: hãy Publish (xem [hướng dẫn OAuth](oauth-setup_VI.md)) rồi đăng nhập lại |
| `destination_required` | Thêm một thư mục rồi chọn nó làm đích mặc định |
| Đích nằm trong nguồn / không xác minh được | Chọn đích khác; 502Drive từ chối khi không chắc chắn |
| `destination_not_writable` | Xin quyền thêm file hoặc chọn thư mục ghi được |
| `copy_not_permitted` | Nguồn cấm copy; không có cách vượt qua |
| `capacity_exceeded` | Chờ hàng đợi bớt; đừng gửi lại job trùng |
| Hàng đợi đang tạm dừng | `502drive resume`; job bạn tạm dừng riêng vẫn giữ nguyên |
| `suspending` | Chờ máy thức dậy và phục hồi; đây không phải lỗi cấu hình |
| `collection_busy` | Chờ hoặc hủy job đang chạy; job tạm dừng hay bị chặn vẫn có thể giữ bản copy |
| Báo "đã ngừng theo dõi"/"chưa ngừng" | Tiếp tục theo dõi trước khi cập nhật; "quên" cần ngừng theo dõi trước |
| `already_copied` | Chọn rõ `copy --update` hoặc `copy --new` |
| Kế hoạch quét sâu bị gián đoạn | Tạo bản xem trước sâu mới; số liệu dở dang không phải tổng |
| Audit báo "thừa" ở đích | Xem thủ công; file lạ ở đích chỉ được báo, không bị đụng |
| Repair không ghi gì | Lệnh repair mặc định chỉ xem trước; xem kế hoạch rồi thêm `--apply` |
| Không thấy thông báo | Kiểm tra giờ yên tĩnh, múi giờ và tùy chọn thông báo |
| `plan_expired` / `plan_changed` | Xem trước lại; đừng dùng kế hoạch cũ |
| `stale_revision` | Làm mới và xem trạng thái hiện tại trước khi thao tác lại |
| `unknown_remote_outcome` | Để hệ thống đối soát tự chạy; không gửi thêm lệnh copy mù quáng |
| Bị chặn vì hạn mức | Xem `resume_after`; hạn mức ngày và dung lượng khác với giới hạn tốc độ |
| Đồng bộ ở trạng thái `needs_reconcile` | Hệ thống tự lấy mốc mới và quét lại đầy đủ |
| `waiting_for_baseline` | Hoàn tất hoặc thử lại lần copy đầu trước khi mong đồng bộ |
| Bot Telegram lỗi | Kiểm tra file token bot và kết nối; các job copy vẫn chạy độc lập |
| Không có khay hệ thống (Linux) | Kiểm tra D-Bus và tiện ích AppIndicator; dùng ứng dụng terminal |
| Bảo trì ngoại tuyến bị từ chối | Dừng dịch vụ/container; đừng xóa file khóa |
| Doctor báo lỗi quyền file | `doctor --fix` sửa được lỗi cục bộ an toàn; đăng nhập Google thì phải đăng nhập lại |
| Cơ sở dữ liệu phình to hoặc danh sách job quá dài | `history clear --older-than 30d` chạy được khi dịch vụ đang chạy; muốn nén sâu thì sao lưu, xem trước `prune` ngoại tuyến, rồi `vacuum` |
| Khôi phục báo thư mục đích đã có | Chọn thư mục mới và trống; lệnh khôi phục không ghi đè |
| Lỗi toàn vẹn/khóa khi khôi phục sao lưu | Dùng bản sao lưu đầy đủ đã kiểm tra; thiếu khóa thì không giải mã được token |
| Đăng nhập OAuth chờ mãi | Kiểm tra đúng cổng callback, chuyển tiếp SSH hoặc cổng container |
| Mirror bị chặn | Kiểm tra dấu sở hữu, lần quét nguồn, ngưỡng mất hàng loạt, và thư mục đích còn file lạ |
| Snapshot không dùng được | Bản copy đã lưu có thể bị sửa, bị xóa hoặc không truy cập được nữa |
| Đồng bộ đang tạm dừng sau khi khôi phục | Đúng thiết kế; xem kết quả rồi tự bật lại khi muốn theo nguồn |
| Dọn dẹp bản copy bị chặn `snapshot_protected` | Còn điểm khôi phục cần bản copy; giữ lại hoặc xem lại thời hạn giữ snapshot |
| `destination_root_trashed` / `destination_root_missing` | Thư mục chứa bản copy đã vào thùng rác hoặc bị xóa: khôi phục nó từ thùng rác Drive rồi chạy lại, hoặc ngừng theo dõi bản copy này |
| Phím mũi tên đổi tab thay vì mở hàng đang chọn | Cập nhật lên bản mới nhất; con trỏ giờ ở thanh tab hoặc trong trang, `←` ra một cấp, `→` mở |

Đừng bao giờ xóa file cơ sở dữ liệu/WAL/khóa chính để "gỡ kẹt" một job, xóa cả volume
cho nhanh, hay đổi khóa idempotency để lách một kết quả chưa rõ. Hãy xem trạng thái và
sự kiện của job thay vì chỉ kiểm tra tiến trình còn chạy hay không. Xem thêm
[sổ tay vận hành](runbooks.md).
