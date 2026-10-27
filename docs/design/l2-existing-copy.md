# L2 — Copy lại cùng nguồn: cập nhật thay vì nhân đôi

## Audit trước khi làm

| Hạng mục | Hiện có | Thiếu |
| --- | --- | --- |
| Admission | `Store.AdmitGuarded` (plan → job, `consumed_job`, idempotency) | không biết đã có bản sao |
| Delta run | `Service.RunNow` / `admitRun` (coalesce vào run đang chạy) | chưa được gọi từ copy |
| Trùng | không có kiểm tra; copy 2 lần = 2 collection | toàn bộ L2 |

## Quyết định

- **Khóa trùng**: `(principal, source_id, destination_id)`. Đích khác → không phải trùng, copy bình thường.
- **Chế độ** (`POST /v1/jobs`, trường `mode`, CLI `copy --update|--new`):
  - bỏ trống: có bản sao → `409 already_copied` (nêu id, trạng thái); không có → copy;
  - `update`: bản sao gần nhất → một delta run trên cùng collection (không tạo cây mới); không có → copy;
    bản sao đã retired → `409 collection_retired`;
  - `new`: luôn copy (hành vi `Submit` cũ).
- **Script** (không TTY) và không có cờ → `update`. **Terminal** không cờ → hỏi `[u]/[n]/[c]` khi gặp 409.
- **Bản sao gần nhất** thắng khi có nhiều bản sao cùng khóa (do `new` tạo ra).
- Plan đã consumed (replay idempotent) bỏ qua kiểm tra, để trả đúng job cũ.
- Không migration mới ngoài chỉ mục biểu thức `collections_source_dest` trong `020_collection_lifecycle.sql`.

## Chế độ lỗi / test

1. Không có mode, đã có bản sao → 409, không tạo job, không đổi gì (`TestCopyAgainAsksWhatToDo`).
2. `update` → job cùng `collection_id`, kind run, không tạo cây mới.
3. `new` → collection mới.
4. Bản sao retired + `update` → 409 `collection_retired`.
5. Replay plan đã dùng → không bị coi là trùng (`ExistingCopy` trả false).
6. Truy vấn dùng chỉ mục (`EXPLAIN QUERY PLAN`).

## Chưa làm

- Telegram (nút "cập nhật/tạo mới" ở xem trước) và TUI (hộp thoại copy) vẫn gọi `Submit` = `new`; móc UI ghi ở PROGRESS.
- Tên thư mục đích khác không đổi kết quả: khóa chỉ là nguồn + đích.
