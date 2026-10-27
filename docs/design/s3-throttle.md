# S3 — AIMD toàn tài khoản, token bucket, phân loại lỗi

Thiết kế cho đặc tả §S3 (phase-12). Gốc: commit `41c7c37` + S2 trong working tree.

## Vấn đề

Engine hiện có `Workers=4` cố định cho mỗi job và một channel `MaxCalls=8` toàn
cục; syncer/audit gọi Drive không qua giới hạn nào. Không có phản hồi khi Google
throttle: 429 bị coi như lỗi item (đốt `attempts`), không có số đo cho người dùng.

## Thành phần

### `drive.Throttle` (`internal/drive/throttle.go`)

Một đối tượng dùng chung cho **mọi** thành phần nói chuyện với cùng tài khoản
(engine, syncer, audit) — đó là nghĩa "toàn tài khoản" (daemon một tài khoản).

- **AIMD**: `limit` bắt đầu `workers` (mặc định 4), trần `max_workers` (8).
  - `Observe(nil)` +1 chuỗi; đủ `throttle_successes` (8) liên tiếp → `limit+1`.
  - `Observe(KindThrottled)` → `limit/2`, sàn 1, reset chuỗi; ghi mốc thời gian
    và `next_retry_at` nếu có `Retry-After`.
  - Lỗi khác (mạng, 5xx, quota, quyền) không đổi `limit`.
- **Acquire/Release**: hàng đợi có thể huỷ theo `ctx`; worker chỉ giữ permit
  quanh claim → gọi provider → ghi kết quả, nên số item `executing` không vượt
  trần.
- **Token bucket đọc**: `read_requests_per_second` (mặc định 8), burst = 1 giây.
  Áp cho `ListChildren`, `GetFile`, `FindByIntent`, `StartPageToken`,
  `ListChanges`; ghi (copy/create/rename/move/trash) không bị bóp bởi bucket,
  chỉ chịu AIMD.
- **`Snapshot`**: `workers_now`, `throttled_last_5m` (cửa sổ trượt 5 phút),
  `next_retry_at` (chỉ khi còn ở tương lai).

### `drive.Paced(provider, throttle)`

Wrapper mỏng: đọc qua bucket, mọi method báo `Observe(err)`. Không tự
`Acquire` để tránh đếm hai lần với engine (engine đã giữ permit quanh call).

### Engine

- `Workers` = điểm bắt đầu AIMD; `MaxWorkers` = trần; `Throttle` chia sẻ với
  provider. `copyAll` spawn `MaxWorkers` goroutine; permit quyết định bao nhiêu
  call chạy thật.
- Bỏ channel `calls`/`MaxCalls`; `acquire/release` gọi throttle.
- `throttleWait()`: backoff mũ + jitter dùng chung cho retry do throttle, tách
  khỏi `attempts` của item. `throttleClear()` sau call thành công.
- 429: `ReleaseItemThrottled` (trả `attempts`, `pending`, hẹn giờ) — không đốt
  ngân sách thử; `listPage`/`reconcile` không tăng `attempt/try` khi throttle.
- Giữ nguyên: unreachable không tiêu attempt, shutdown không huỷ write in-flight.

### Phân loại lỗi

Thêm `KindServer` cho 5xx, tách khỏi `KindUnavailable` (transport/timeout) —
đủ năm nhóm đặc tả yêu cầu: throttle, quota (Q1 tách daily/storage), mạng
(unreachable/unavailable), 5xx, quyền. `KindOf` giữ nguyên cho lỗi lạ
(`unavailable`, giả định an toàn cho write).

### HTTP

`GET /v1/status` thêm object `throttle {workers_now, throttled_last_5m,
next_retry_at}` (OpenAPI `Throttle`, required). Không có tài khoản → vẫn trả
shape (4, 0, null). Đây là dữ liệu additive; CLI/TUI cũ bỏ qua.

## Cấu hình

| Khóa | Mặc định | Ý nghĩa |
| --- | --- | --- |
| `workers` | 4 | concurrency khởi điểm (1–64) |
| `max_workers` | 8 | trần AIMD, không nhỏ hơn `workers` |
| `read_requests_per_second` | 8 | token bucket cho list/get/changes (0 = mặc định) |
| `throttle_successes` | 8 | số call thành công liên tiếp để +1 |

## Cửa sổ lỗi

| Cửa sổ | Ứng xử |
| --- | --- |
| 429 giữa copy | item về `pending`, `attempts-1`, hẹn `max(throttleWait, Retry-After)`; AIMD halve; không fail item |
| 429 khi list page | đọc lại trang, không tăng attempt; backoff mũ có jitter |
| 429 khi reconcile | không tiêu `try`; chờ rồi tìm lại |
| `Retry-After` lớn | mọi retry tôn trọng `max(...)`; status có `next_retry_at` |
| Nhiều job cùng lúc | một throttle chung; permit toàn account |
| Tắt mạng | như cũ: gate đóng, không tiêu attempt; bucket/AIMD không bị phạt |
| 5xx | `KindServer`, retry có backoff, write vào đường reconcile (có thể đã xảy ra) |
| 403 quyền | `KindPermission`, không retry mù |

## Test

- `drive/throttle_test.go`: lớn dần theo chuỗi, halve, sàn 1, Acquire chờ/huỷ,
  bucket chia nhịp, tắt bucket, cửa sổ 5 phút + `next_retry_at`.
- `engine/throttle_test.go`: provider paced làm `limit` lớn lên; 429 ba lần
  không đốt attempt, item `done` với `attempts<=1`, đếm đúng 3 throttle.
- `httpapi/lifecycle_test.go`: `throttle` đúng shape + conformance; service
  chưa nối dây vẫn trả 4/0/null.
- `drive/client_test.go`: 5xx → `KindServer` (cập nhật kỳ vọng cũ).
- Toàn bộ engine cũ chạy lại: gating mới không đổi kết quả job.
