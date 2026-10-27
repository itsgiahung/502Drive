# S7 — Preflight sâu (deep scan, chỉ đọc)

## Mục tiêu
`inspect --deep` / `POST /v1/plans` với `deep: true`: quét **toàn bộ** cây nguồn để biết số file, thư mục, byte,
mục Google-native, shortcut, mục không copy được (`cannot_copy`, cùng ý nghĩa mã K1), thư mục không đọc được, và
đưa ra cảnh báo — trước khi người dùng bấm copy. Không ghi gì lên Drive.

## Cách làm
- Yêu cầu trả về ngay với plan `deep.state = "running"`; quét chạy nền trong goroutine của daemon (context gốc là
  context của daemon, nên tắt daemon sẽ hủy quét). Client theo dõi bằng `GET /v1/plans/{id}`.
- Quét theo BFS: chỉ giữ hàng đợi id thư mục (O(số thư mục)), không nạp cây vào RAM; phân trang `ListChildren`.
- Mỗi lần list/get là lệnh đọc mà copy cũng sẽ làm; không có lệnh ghi (kiểm bằng `Fake.Writes == 0`).
- Ghi tiến độ vào body plan (`plans.body.deep`), tối đa mỗi 100 ms, để `Scanned` có giá trị khi hỏi.
- Hủy: `POST /v1/plans/{id}/preflight/cancel` → `state = cancelled`, `complete = false`. Số đã đếm vẫn hiện nhưng
  được đánh dấu là **không phải tổng**.
- Hết hạn: plan deep có TTL 1 giờ; quét còn chạy sau đó sẽ dừng với `state = expired`.
- Sau restart: quét đang chạy không còn goroutine → `GET` báo `interrupted`; hủy không đổi trạng thái đó.

## Ước thời gian và cảnh báo (không chặn)
- Thời gian = byte copy được / tốc độ đo của 5 job `succeeded` gần nhất (byte logic / khoảng `created→updated`).
  Chưa có số đo → `estimate_basis = "unknown"`, cảnh báo `estimate_unknown`, không đoán.
- `destination_storage_short`: nếu tài khoản có hạn mức (`About.storageQuota`, qua giao diện tùy chọn
  `drive.QuotaReader`) và byte copy được > dung lượng trống.
- `may_hit_daily_limit`: byte copy được > hạn mức copy trong ngày còn lại (`CopiedBytes24h`, sổ Q1; hạn mức mặc
  định 750 GB, cấu hình được qua `DailyCopyLimit`).
- Mọi cảnh báo chỉ hiển thị; không chặn copy.

## Mô hình dữ liệu
Không có migration mới: khối `deep` nằm trong body của plan (`state.DeepPreflight`). Plan vẫn bất biến ở các trường
gốc; chỉ khối `deep` được cập nhật.

## Chế độ lỗi
- Lỗi quyền/không tìm thấy trên một thư mục → đếm `unreadable_folders`, quét tiếp.
- Lỗi khác (mạng, server) → `state = failed`, `error = listing_failed`; không báo tổng.
- `Complete` chỉ true khi `state = complete`.

## Giới hạn đã biết
- Thời gian ước tính dựa trên một tài khoản; không tính độ trễ theo từng file.
- Hạn mức dung lượng đọc từ tài khoản của người dùng; với Shared Drive chưa kiểm (`limit` có thể bằng 0 → không cảnh báo).
- Chưa kiểm với Drive thật (chỉ fake provider).
