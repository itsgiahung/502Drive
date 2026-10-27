# N1 — Trùng tên ở đích (`on_conflict = skip|rename|update`)

Thiết kế cho §N1 (phase-13), đã cài theo phạm vi dưới đây. Quy tắc: không bao giờ ghi đè.

## 1. Mô hình

- `items.dest_name` (tên đã chọn) và `items.conflict_action` (`''`, `rename`, `update`, `skip`), migration
  `010_conflict_policy.sql`. Backfill: mục đã có `dest_id` giữ tên hiện tại.
- Chính sách: `plans.on_conflict` → `jobs.on_conflict` → `collections.on_conflict` (copy khi admit, như
  `shortcuts`). Mặc định `rename`; job cũ không có trường nào cũng là `rename`.

## 2. Giữ tên trong cơ sở dữ liệu (không liệt kê Drive cho từng mục)

Trước khi ghi, mục giữ tên `N` dưới `dest_parent_id` trong **một transaction**: nếu không có mục nào khác
(không phải `skip`, không phải `skipped`) đang giữ `N` dưới cùng cha thì ghi `dest_name=N`. Hai worker không thể
giữ cùng một tên vì SQLite tuần tự hóa transaction ghi. Index: `items_dest_name(dest_parent_id, dest_name)`.

Lý do: thư mục do 502Drive tạo chỉ có các mục do 502Drive ghi, nên DB là đủ. Chỉ **thư mục gốc** của một lần copy
(mục có `parent_source_id=''`) được hỏi Drive (`FindChildrenByName`), vì nó nằm trong thư mục đích người dùng chọn.
Đây là lựa chọn để không tăng `files.list` (đo 26 lệnh trên cây 10k/1k, như trước N1).

## 3. Quyết định

1. Tên còn trống → giữ tên gốc (quyết định rỗng).
2. Đã có mục khác giữ tên, là tệp, cùng size + cùng md5 (md5 khác rỗng), và mục đó đã xong, đang chờ, đang ghi
   hoặc đang đối soát (không tính mục lỗi/bị chặn) → `skipped`, `conflict`, `skip`. Mục đang ghi có thể lỗi sau đó;
   khi đó bản trùng không được copy và mục lỗi hiện ra để người dùng xử lý.
   Không "nhận" tệp đích làm bản của mình, để chính sách thay thế về sau không xóa tệp không phải của ta.
3. Khác nội dung:
   - `skip` → `skipped`, `conflict`.
   - `rename` → `conflictName(name, n)` với `n = 2…201`; giữ phần mở rộng, `.tar.gz`/`.tar.bz2`/`.tar.xz`/`.tar.zst`
     là đuôi kép. Wrapper: mỗi ứng viên được hỏi Drive.
   - `update` → ghi cùng tên, không xóa bản nào (xem giới hạn).
4. Thư mục (và wrapper) luôn `rename` (bỏ qua thư mục làm mất cây con).
5. Quyết định ghi trong cùng transaction với việc giữ tên (`ClaimDestName`, `SkipWithDecision`). Sau khi có
   `dest_name`, lần chạy lại không quyết định lại.

## 4. Giới hạn (cố ý, ghi trong báo cáo)

- `update` chưa xóa bản cũ (thiết kế gốc chuyển bản cũ vào thùng rác theo cơ chế replace). Chưa làm trong đợt C1.
- Mục `align` (đổi tên nguồn) không quyết định lại tên khi tên mới trùng.
- Tệp do người khác thêm vào thư mục đích sau khi copy không được phát hiện, trừ thư mục gốc.
- Chưa kiểm với Drive thật (trùng tên thật, quyền).
