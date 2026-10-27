# L1 — Vòng đời collection: Ngừng theo dõi / Quên / Dọn đích

## 1. Mục tiêu và phạm vi

Bổ sung 3 mức vòng đời cho một collection đã sao chép, tách biệt an toàn theo nguyên tắc:
1. **retire** (ngừng theo dõi): Tắt sync (lịch và realtime), đánh dấu trạng thái `retired`, ẩn khỏi danh sách mặc định (chỉ hiện khi `--all`), có thể `resume` đưa lại. Không đụng đến file trên Drive, giữ nguyên lịch sử và mapping. Từ chối khi có job đang chạy (409 `collection_busy`, nêu rõ ID job đang chạy).
2. **forget** (quên): Xóa bản ghi collection, mapping (`items`, `item_versions`), lịch sử job (`jobs`, `events`), cấu hình sync và audit trong cơ sở dữ liệu nội bộ. Chỉ được thực hiện khi collection đã ở trạng thái `retired`. Không đụng vào Google Drive. Cần `--yes` trong CLI/script.
3. **cleanup** (dọn đích): Đưa thư mục đích do 502Drive tạo vào **thùng rác Google Drive** (`TrashFile`, khôi phục trong 30 ngày), tuyệt đối không xóa vĩnh viễn (`DeleteFile`). Chỉ thực hiện khi có dấu `502drive_intent`, chỉ khi collection đã `retired`. Mặc định `--dry-run` in kế hoạch (số mục, dung lượng ước tính). Chạy thật cần `--yes` + xác nhận tên thư mục. Thực hiện như một job bền vững (kind `cleanup`), an toàn qua restart và crash.

---

## 2. Mô hình dữ liệu và Migration (`020_collection_lifecycle.sql`)

```sql
-- Migration 020: Thêm retired_at vào collections để lọc danh sách nhanh và ghi nhận mốc thời gian ngừng theo dõi.
ALTER TABLE collections ADD COLUMN retired_at INTEGER;
CREATE INDEX IF NOT EXISTS collections_retired ON collections(retired_at);
```

- Struct `state.Collection`:
  - `RetiredAt *time.Time json:"retired_at,omitempty"`
  - `State`: Cho phép giá trị `"retired"`.
- Trạng thái danh sách:
  - `CollectionPage(ctx, principal, all, query, mode, limit, cursor)`:
    - Khi `all == false`: chỉ lấy các collection có `retired_at IS NULL`.
    - Khi `all == true`: lấy toàn bộ bao gồm cả `retired`.

---

## 3. Giao diện HTTP API

### 3.1 `POST /v1/collections/{id}/retire`
- Header: `Idempotency-Key` (bắt buộc)
- Body: `{"expected_revision": N}`
- Mã trả về:
  - `200 OK`: Trả về collection DTO đã cập nhật (`state: "retired"`, `retired_at: ...`).
  - `404 Not Found`: Không tìm thấy collection.
  - `409 Conflict` (`collection_busy`): Có job đang chạy trên collection, body chứa `{"code": "collection_busy", "message": "...", "active_job_id": "job-xxx"}`.
  - `409 Conflict` (`stale_revision`): Sai `expected_revision`.

### 3.2 `POST /v1/collections/{id}/resume`
- Header: `Idempotency-Key` (bắt buộc)
- Body: `{"expected_revision": N}`
- Mã trả về:
  - `200 OK`: Trả về collection DTO đã kích hoạt lại (`state: "ready"`, `retired_at: null`).
  - `409 Conflict` (`collection_not_retired`): Collection không ở trạng thái `retired`.
  - `409 Conflict` (`stale_revision`): Sai `expected_revision`.

### 3.3 `POST /v1/collections/{id}/forget`
- Header: `Idempotency-Key` (bắt buộc)
- Body: `{"expected_revision": N}`
- Điều kiện tiên quyết: Collection phải ở trạng thái `retired`. Nếu chưa retire, trả về `409 Conflict` (`collection_not_retired`).
- Mã trả về:
  - `200 OK`: `{"deleted": true, "id": "col-xxx"}`.
  - `409 Conflict`: Chưa retire hoặc có job đang chạy.

### 3.4 `POST /v1/collections/{id}/cleanup`
- Header: `Idempotency-Key` (bắt buộc)
- Body: `{"expected_revision": N, "dry_run": bool, "confirm_name": string}`
- Điều kiện: Collection phải ở trạng thái `retired`. Nếu không phải dry-run, `confirm_name` phải khớp với `collection.root_name`.
- Mã trả về:
  - `202 Accepted`: Trả về job DTO (kind `cleanup`, state `queued`).
  - `409 Conflict`: Chưa retire, hoặc sai tên xác nhận, hoặc đang có job khác đang chạy (`one_active_run`).

---

## 4. Cơ chế thực thi Job Cleanup (`internal/engine`)

1. Job kind `cleanup` được nhận diện tại `r.executeCleanup(ctx, job)`.
2. Kiểm tra `col.RootID`:
   - Nếu `col.RootID == nil`: Không có thư mục đích trên Drive → Kết thúc `JobSucceeded`.
   - Nếu `col.RootID != nil`:
     - Gọi `r.p.GetFile(ctx, *col.RootID, "")`.
     - Nếu lỗi 404 (Not Found) hoặc `f.Trashed == true`: Đã nằm trong thùng rác hoặc không tồn tại → Idempotent, kết thúc `JobSucceeded`.
     - Nếu `f.AppProperties[drive.IntentProperty] == ""`:
       **HÀNG RÀO AN TOÀN TUYỆT ĐỐI**: Thư mục không có đánh dấu intent của 502Drive.
       Hủy thao tác, đánh dấu job `JobFailed` với lỗi `intent_missing`. Tuyệt đối không gọi `TrashFile`.
     - Nếu có `502drive_intent`:
       - Nếu `job.DryRun == true`:
         Đếm số lượng mục và dung lượng từ bảng `items`, cập nhật kết quả vào job, kết thúc `JobSucceeded`.
       - Nếu `job.DryRun == false`:
         Gọi `r.p.TrashFile(ctx, *col.RootID)`.
         Nếu mất kết nối mạng hoặc timeout, bước sau kiểm tra lại bằng `GetFile`: nếu đã vào trash thì coi như thành công (reconcile).
         Kết thúc `JobSucceeded`.

---

## 5. Chế độ lỗi và Kiểm thử bắt buộc (TDD)

1. **Retire khi đang chạy**: Khởi tạo copy/sync đang chạy, gọi retire → phải nhận lỗi 409 `collection_busy` kèm ID của job.
2. **Resume**: Gọi resume trên collection đã retired → chuyển về `ready`, xuất hiện lại trong danh sách mặc định.
3. **Forget khi chưa retire**: Gọi forget trên collection `ready` → bị từ chối 409 `collection_not_retired`.
4. **Cleanup chỉ đối tượng có dấu**: Thư mục đích giả lập không có `502drive_intent` → cleanup từ chối và báo lỗi, không xóa/trash.
5. **Crash giữa cleanup**: Giả lập crash trước/sau khi gọi Drive → restart engine tiếp tục và hoàn tất sạch sẽ.
6. **Cleanup hai lần**: Chạy cleanup lần 2 khi thư mục đã ở thùng rác → thành công idempotent, không gây lỗi.
7. **Retired không bị `ClaimDueSchedules` bắt**: Collection có lịch khi chuyển sang retired sẽ bị dừng lịch, `ClaimDueSchedules` không kích hoạt run mới.
8. **Thông báo sync không gửi cho retired**: Bộ notifier bỏ qua thông báo lỗi/tiến độ sync khi collection đã ở trạng thái `retired`.
