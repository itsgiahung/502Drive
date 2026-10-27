# K1 — Phân loại mục không copy được (mã skipped cố định) & Shortcut policy

Thiết kế cho §K1 (phase-13).

## 1. Mục tiêu

1. Định danh các lý do không copy được thành tập mã `skipped` cố định:
   - `shortcut_unresolved`: shortcut trỏ tới mục không tồn tại, bị xoá, trong thùng rác hoặc không truy cập được.
   - `shortcut_loop`: phát hiện vòng lặp shortcut (A trỏ A, hoặc A trỏ B trỏ A).
   - `cannot_copy`: chủ sở hữu không cho phép copy (`canCopy=false`, `copyRequiresWriterPermission`).
   - `google_native_export_only`: định dạng tài liệu Google native chỉ export, không hỗ trợ server-side copy (`application/vnd.google-apps.map`, `site`, `script`, `fusiontable`, `jam`).
   - `too_large_for_quota`: file vượt quá dung lượng hạn mức khả dụng của Drive/người dùng.
   - `unreadable`: lỗi quyền đọc hoặc file nguồn không đọc được.
   - `trashed_source`: file nguồn nằm trong thùng rác.
   - `filtered`: dành cho S5 (bị loại bởi bộ lọc).
2. Tổng hợp số lượng mục bị bỏ qua theo mã trong `Counts` của Job (`skipped_by_code`) ở cả Job DTO và OpenAPI schema.
3. Chính sách shortcut: cờ `--shortcuts follow|skip` (mặc định `follow`: theo đích 1 cấp, phát hiện vòng; `skip`: bỏ qua shortcut với mã `shortcut_unresolved`).

## 2. Mô hình & Lưu trữ

- `internal/state/model.go`:
  - Thêm hằng số `SkipShortcutUnresolved`, `SkipShortcutLoop`, `SkipCannotCopy`, `SkipGoogleNativeExport`, `SkipTooLargeForQuota`, `SkipUnreadable`, `SkipTrashedSource`, `SkipFiltered`.
  - `Counts` thêm `SkippedByCode map[string]int `json:"skipped_by_code,omitempty"``.
  - `Plan` và `Job` thêm trường `Shortcuts string `json:"shortcuts,omitempty"`` (lưu trong JSON body, không đổi cột SQL).
  - `fillCounts` tổng hợp `SELECT coalesce(nullif(error_code,''),'unknown'), count(*) FROM items WHERE job_id=? AND kind<>'wrapper' AND state IN ('skipped','cancelled') GROUP BY error_code`.
- `internal/httpapi/dto.go`:
  - `jobDTO` thêm `skipped_by_code` từ `j.Counts.SkippedByCode`.
- `docs/contracts/openapi.json`:
  - `JobCounts` và `Job` thêm thuộc tính `skipped_by_code`.

## 3. Engine & Shortcut Resolution

Khi worker xử lý `it.Kind == state.KindShortcut`:
- Nếu `Shortcuts == "skip"`:
  - Cập nhật item `state='skipped', error_code='shortcut_unresolved'`.
- Nếu `Shortcuts == "follow"` (mặc định):
  - Kiểm tra `targetID = it.ShortcutTarget`. Nếu rỗng -> `shortcut_unresolved`.
  - Nếu `targetID == it.SourceID` -> `shortcut_loop`.
  - Gọi `p.GetFile(ctx, targetID, it.ResourceKey)`. Nếu lỗi/trashed -> `shortcut_unresolved`.
  - Nếu target là shortcut (`target.IsShortcut()`):
    - Kiểm tra `target2ID = target.ShortcutTargetID`.
    - Nếu `target2ID == it.SourceID` hoặc `target2ID == target.ID` -> `shortcut_loop`.
    - Nếu `target2ID == ""` -> `shortcut_unresolved`.
    - Gọi `p.GetFile(ctx, target2ID, target.ShortcutTargetKey)`. Nếu lỗi/trashed -> `shortcut_unresolved`.
    - Nếu target2 vẫn là shortcut -> nếu trỏ về source/target -> `shortcut_loop`, ngược lại do Drive cấm shortcut trỏ shortcut và chính sách chỉ theo 1 cấp -> `shortcut_unresolved`.
    - Nếu target2 hợp lệ -> tạo shortcut đích trỏ tới `target2.ID`.
  - Nếu target là file/folder hợp lệ -> tạo shortcut đích trỏ tới `target.ID`.

## 4. Các mã skipped khác khi copy file

- Khi `IsGoogleNativeExportOnly(it.MimeType)` -> đánh dấu `skipped`, `error_code='google_native_export_only'`.
- Khi `CopyFile` trả lỗi `cannot_copy` hoặc `copyRequiresWriterPermission` -> đánh dấu `skipped`, `error_code='cannot_copy'`.
- Khi `CopyFile` trả lỗi `too_large_for_quota` -> đánh dấu `skipped`, `error_code='too_large_for_quota'`.
- Khi đọc metadata nguồn gặp file bị trashed -> đánh dấu `skipped`, `error_code='trashed_source'`.
- Khi file nguồn unreadable -> đánh dấu `skipped`, `error_code='unreadable'`.

## 5. Bề mặt CLI & HTTP

- CLI: `502drive copy [SOURCE] --shortcuts follow|skip` (mặc định `follow`). `502drive inspect [SOURCE] --shortcuts follow|skip`.
- HTTP: `POST /v1/plans` nhận trường `shortcuts` trong body JSON.

## 6. Cập nhật đợt C1 (đã cài)

- Tra cứu đích shortcut: chỉ lỗi định nghĩa (`not_found`, `permission`, `invalid`) mới thành `shortcut_unresolved`.
  Lỗi tạm (mất mạng, throttle, auth, 5xx) đưa mục về hàng đợi (`releaseTransient`), không bao giờ thành skip.
- `skipSafe`: ghi trạng thái skip lỗi thì ghi nhật ký và trả mục về hàng đợi (`skip_write_failed`).
- Mã mới do N1: `conflict`. Mã do S5: `filtered`.
- `too_large_for_quota`: chỉ khớp khi API trả lý do đúng tên đó; chưa xác nhận với Google thật.
