# S6 — Batch clone và hàng đợi công bằng

## Mô hình
- Một **lô** (`batch_id`) = nhiều job, mỗi link một job (bất biến: mỗi job vẫn là một copy riêng). Lô không có
  bảng riêng; `GET /v1/batches/{id}` tổng hợp từ các job có cùng `batch_id` (theo principal).
- `batch_id` do client sinh **từ nội dung lô** (hash của khóa lô), nên gửi lại cùng lệnh cho cùng `batch_id`.
- Idempotency lô: khóa lô `K` → plan của dòng `i` dùng `K:i-plan`, job dùng `K:i`. Gửi lại cả lô: mỗi dòng
  replay đúng plan/job cũ, không tạo job mới. Một dòng lỗi không làm hỏng các dòng khác (kết quả từng dòng).
- Dòng trùng trong cùng lô bị bỏ (chỉ giữ lần đầu). Dòng trống bị bỏ.

## Cột sinh từ body (migration 021)
`batch_id`, `queue_priority`, `queue_order`, `fair_group` là cột `VIRTUAL` lấy từ `body`. Không có cột ghi
thêm, nên mọi nơi đang `json_set` body vẫn đúng. Index: `jobs_batch`, `jobs_queue(state, priority, order)`,
`jobs_fair(state, fair_group)`.

## Thứ tự hàng đợi
Chọn job kế tiếp (`ClaimQueued`) trong một transaction:
1. bỏ qua hoàn toàn nếu hàng đợi đang tạm dừng (`settings.queue_paused = 1`);
2. `queue_priority` giảm dần (ưu tiên);
3. **fair share**: số job đang chạy (`running|reconciling|pausing|cancelling`) của cùng `fair_group` tăng dần,
   nên một lô 20 link không chiếm hết hai slot khi có job đơn lẻ đang chờ;
4. `queue_order` tăng dần (FIFO), rồi `rowid`.

`queue_order` là số nguyên: lúc admit là `UnixNano`; `move` đánh số lại toàn bộ hàng đợi `1..N` trong cùng
transaction (N ≤ 100 do giới hạn admission), giữ đúng thứ tự tương đối.

Ưu tiên: `priority` trong khoảng -5..5, mặc định 0. Đổi ưu tiên không kèm vị trí: job xuống cuối nhóm ưu tiên mới.
`before`/`after` lấy ưu tiên của job đích nếu không chỉ định `priority`.

## Tạm dừng hàng đợi
- `POST /v1/queue/pause`: đặt cờ bền `settings.queue_paused=1`; mọi job `running|reconciling|pausing` chuyển
  sang `pausing` (desired `pause`), đánh dấu `queue_paused=true` trong body. Engine dừng ở điểm an toàn và đặt
  `paused`. Không job nào mất tiến độ (item, intent, cursor giữ nguyên). Job `queued` giữ nguyên trạng thái và
  không được nhận.
- `POST /v1/queue/resume`: xóa cờ; job `paused` có `queue_paused=true` quay về `queued`. Job người dùng tự tạm dừng
  không bị ảnh hưởng.
- Resume thủ công một job (`/resume`) xóa cờ `queue_paused` của job đó.
- Cả hai thao tác idempotent theo trạng thái (gọi lại không đổi gì), chỉ owner.

## Chế độ lỗi
- Restart giữa lô: job đã admit nằm trong DB; lần gửi lại cùng khóa replay đúng job, job chưa admit được tạo.
- Admit lỗi giữa lô: dòng đó báo lỗi, các dòng khác vẫn được admit; chạy lại chỉ admit dòng còn thiếu.
- Pause giữa chừng: job `pausing` được `StrandedJobs` đưa lại engine sau restart để chốt `paused`.
