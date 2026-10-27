# S9 — Snapshots & Điểm khôi phục cấp Collection

Tài liệu thiết kế cho §S9 (phase-12-spec).
Trạng thái: **Thiết kế chờ duyệt (Design Only — Không viết code trong đợt này)**.

---

## 1. Bối cảnh & Vấn đề

502Drive hiện tại đã có cơ chế lưu trữ lịch sử các bản sao cấp tập tin thông qua bảng `item_versions` (chế độ `versioned_copy`). Khi một file nguồn thay đổi, phiên bản cũ ở đích được đổi tên/lưu lại trước khi bản mới được tạo.

Tuy nhiên, hệ thống chưa có **Điểm khôi phục cấp Collection (Collection-Level Restore Point / Snapshot)**:
- Người dùng không thể quay ngược trạng thái của toàn bộ thư mục đích về đúng thời điểm một lần đồng bộ (run) thành công trong quá khứ.
- Nếu một đợt đồng bộ vô tình ghi đè hoặc tạo ra hàng loạt thay đổi không mong muốn (ví dụ: ransomware mã hoá thư mục nguồn, thao tác nhầm trên nguồn), việc khôi phục thủ công từng file từ `item_versions` là bất khả thi với các cây thư mục hàng nghìn tập tin.
- Cần một cơ chế chụp nhanh trạng thái cây thư mục tại các mốc thành công, cho phép xem trước (preview) và khôi phục (rollback) toàn diện, an toàn, không bao giờ gây mất mát dữ liệu vĩnh viễn.

---

## 2. Bất biến & Ràng buộc cốt lõi

1. **Tuyệt đối không xóa vĩnh viễn**: Thao tác rollback không bao giờ gọi API xóa vĩnh viễn (`files.delete`). Bất kỳ tập tin nào cần được thay thế hoặc loại bỏ khỏi cây đích sẽ được chuyển vào thùng rác của Google Drive (`files.update` với `trashed=true`), đảm bảo người dùng luôn có thể can thiệp khôi phục thủ công nếu cần.
2. **Điểm khôi phục dựa trên run thành công**: Mỗi snapshot ghi nhận danh sách ánh xạ `(source_id → dest_id + version)` tại thời điểm một sync run hoàn tất thành công (`JobSucceeded`), liên kết chặt chẽ với `item_versions`.
3. **Chính sách lưu trữ (Retention Policy)**: Mặc định giữ tối đa **30 ngày** hoặc **10 điểm khôi phục** gần nhất cho mỗi collection (cấu hình được trong preferences/collection). Cho phép người dùng "ghim" (pin) các snapshot quan trọng để không bị dọn dẹp tự động.
4. **Khôi phục như một Job bền vững**: Quá trình rollback chạy như một job chính thức (kind `rollback`), có đầy đủ tính năng:
   - Preview kế hoạch (dry-run) trước khi áp dụng.
   - Admission bền vững, idempotency key.
   - Phân trang, checkpoint tiến độ, hỗ trợ pause/resume/cancel.
   - Sống sót qua crash / kill process / restart daemon / mất mạng mà không sinh lỗi trùng lặp.
5. **Tôn trọng giới hạn của Google Drive**:
   - **Thùng rác tự dọn sau 30 ngày**: Google Drive tự động xoá vĩnh viễn các mục trong thùng rác sau 30 ngày. Tài liệu và giao diện phải cảnh báo rõ ràng: các bản cũ chỉ có thể phục hồi nếu còn nằm trong thùng rác hoặc trong `item_versions`.
   - **Quyền hạn (Permissions)**: Chỉ có thể phục hồi các đối tượng mà tài khoản hiện tại có quyền chỉnh sửa (`canAddChildren`, `capabilities.canDelete`, v.v.). Với file chỉ đọc (read-only) hoặc thuộc sở hữu tài khoản khác trong Shared Drive, hệ thống đánh dấu `permission_denied` và yêu cầu can thiệp.
   - **Giới hạn tốc độ & Quota**: Tôn trọng cơ chế AIMD throttle và sổ byte 750 GB/ngày.

---

## 3. Mô hình dữ liệu & Migration

### Schema đề xuất (Migration `009_snapshots.sql`)

```sql
-- Điểm khôi phục cấp collection
CREATE TABLE snapshots (
    id TEXT PRIMARY KEY,
    collection_id TEXT NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
    run_id TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    expires_at TIMESTAMP,
    pinned INTEGER NOT NULL DEFAULT 0,
    total_items INTEGER NOT NULL DEFAULT 0,
    total_bytes INTEGER NOT NULL DEFAULT 0,
    root_folder_id TEXT NOT NULL,
    metadata_json TEXT
);

CREATE INDEX idx_snapshots_collection ON snapshots(collection_id, created_at DESC);
CREATE INDEX idx_snapshots_expires ON snapshots(expires_at) WHERE pinned = 0;

-- Chi tiết từng mục trong điểm khôi phục
CREATE TABLE snapshot_items (
    snapshot_id TEXT NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
    source_id TEXT NOT NULL,
    dest_id TEXT NOT NULL,
    version_id TEXT NOT NULL,
    relative_path TEXT NOT NULL,
    name TEXT NOT NULL,
    mime_type TEXT NOT NULL,
    size INTEGER,
    md5 TEXT,
    trashed INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (snapshot_id, source_id)
);

CREATE INDEX idx_snapshot_items_dest ON snapshot_items(dest_id);
```

### Giải thích quan hệ:
- `snapshots`: Đại diện cho một điểm khôi phục tại mốc thời gian hoàn thành của `run_id`.
- `snapshot_items`: Lưu lại snapshot cây thư mục tại mốc đó. `dest_id` và `version_id` tham chiếu tới phiên bản cụ thể trong Drive hoặc trong `item_versions`.
- `pinned`: Cờ đánh dấu snapshot đặc biệt không bị dọn dẹp khi hết hạn retention.

---

## 4. Vòng đời & Chính sách lưu trữ (Retention)

### Tạo Snapshot tự động:
- Khi một sync run (hoặc clone initial run) kết thúc với trạng thái `JobSucceeded`:
  1. Engine kiểm tra cấu hình collection (`snapshots_enabled`, mặc định `true`).
  2. Tạo bản ghi `snapshots` mới với `expires_at = now() + 30 days`.
  3. Quét bảng ánh xạ hiện thời (`mapping` và `item_versions`) của collection để kết xuất danh sách `snapshot_items` trong một database transaction.

### Dọn dẹp tự động (Pruning):
- Định kỳ (trong maintenance cycle hoặc ngay sau khi tạo snapshot mới):
  1. Tìm các snapshot `pinned = 0` có `expires_at < now()`.
  2. Tìm các snapshot `pinned = 0` vượt quá số lượng tối đa cấu hình (mặc định > 10 snapshot cũ nhất).
  3. Xoá bản ghi snapshot và snapshot_items tương ứng (CASCADE).
  4. Lưu ý: Xoá snapshot trong DB không làm xoá file trên Drive.

---

## 5. Quy trình Rollback & Kế hoạch phục hồi

Rollback được chia làm 2 giai đoạn rõ ràng:

### Giai đoạn 1: Preflight & Xem trước (Dry-run Preview)
Trước khi thực hiện, người dùng yêu cầu kế hoạch phục hồi (`POST /v1/snapshots/{id}/restore?dry_run=true`):
1. **Đối soát hiện trạng**: Quét cây thư mục đích hiện tại trên Drive.
2. **So sánh với snapshot**:
   - `ActionRestoreDeleted`: Tập tin có trong snapshot nhưng hiện đang bị đưa vào thùng rác (hoặc mất khỏi đích) → cần khôi phục từ thùng rác (`trashed=false`) và đưa về đúng thư mục cha.
   - `ActionRevertVersion`: Tập tin có phiên bản khác với snapshot → tìm bản ghi tương ứng trong `item_versions`, hoán đổi phiên bản (bản hiện tại chuyển vào thùng rác, bản cũ đưa ra thế chỗ).
   - `ActionTrashExtra`: Tập tin xuất hiện ở đích nhưng không tồn tại trong snapshot (do các lần sync sau tạo ra) → chuyển vào thùng rác Google Drive.
   - `ActionNoOp`: Tập tin đã khớp hoàn toàn (cùng ID, cùng MD5/version).
3. **Phát hiện xung đột & rủi ro**:
   - File trong thùng rác đã bị Google xoá vĩnh viễn (> 30 ngày) → đánh dấu `missing_unrecoverable`.
   - File đích bị khoá quyền sửa đổi → đánh dấu `permission_denied`.
4. Trả về bảng tổng kết kế hoạch: số file cần khôi phục, số file cần hoán đổi version, số file cần chuyển vào thùng rác, và các cảnh báo rủi ro.

### Giai đoạn 2: Thực thi (Execution)
Sau khi người dùng xác nhận (`POST /v1/snapshots/{id}/restore` kèm `expected_revision`):
1. Khởi tạo một job loại `rollback` trong `state.Store`.
2. Worker thực thi từng thao tác đã định trong kế hoạch theo thứ tự an toàn:
   - Thư mục cha được khôi phục trước thư mục con.
   - File được di chuyển/khôi phục trước khi đưa các file thừa vào thùng rác.
   - Mọi thao tác đều mang Intent tag (`502drive_intent: rollback_<snapshot_id>_<dest_id>`) để đảm bảo tính idempotent nếu bị ngắt quãng giữa chừng.
3. Khi hoàn thành, cập nhật trạng thái job thành `succeeded` và ghi nhận một mốc audit log.

---

## 6. Giao diện & Hợp đồng điều khiển (API & CLI)

### HTTP Endpoints
- `GET /v1/collections/{id}/snapshots`: Danh sách các điểm khôi phục của collection (phân trang, sắp xếp theo thời gian mới nhất).
- `GET /v1/snapshots/{id}`: Chi tiết một snapshot (số lượng file, dung lượng, trạng thái ghim, thời hạn).
- `GET /v1/snapshots/{id}/items`: Danh sách các tập tin trong snapshot (hỗ trợ tìm kiếm theo đường dẫn/tên).
- `POST /v1/snapshots/{id}/restore`: Yêu cầu khôi phục.
  - Body: `{"dry_run": bool, "target_folder_id": string, "expected_revision": int}`
  - Trả về 200 kèm kế hoạch nếu `dry_run=true`, hoặc 202 Accepted kèm Job ID nếu thực thi.
- `POST /v1/snapshots/{id}/pin`: Ghim snapshot để ngăn tự động dọn dẹp.
- `DELETE /v1/snapshots/{id}/pin`: Bỏ ghim snapshot.

### CLI Commands
```bash
# Liệt kê các snapshot của một collection
502drive snapshots list col_123

# Xem chi tiết một snapshot
502drive snapshots show snap_456

# Xem trước kế hoạch rollback (mặc định an toàn, không thay đổi gì)
502drive restore snap_456 --dry-run

# Thực hiện rollback sau khi đã xem xét kỹ
502drive restore snap_456 --apply

# Ghim hoặc bỏ ghim snapshot
502drive snapshots pin snap_456
502drive snapshots unpin snap_456
```

---

## 7. Giới hạn vật lý & Khuyến nghị kỹ thuật

1. **Google Drive Trash Retention (30 ngày)**:
   - Google Drive tự động thanh trừng các file trong thùng rác sau 30 ngày kể từ lúc bị trash. Do đó, các snapshot cũ hơn 30 ngày có thể không thể phục hồi trọn vẹn nếu các file tương ứng đã bị xoá khỏi thùng rác.
   - Hệ thống cần hiển thị cảnh báo rõ: *"Khôi phục tốt nhất trong vòng 30 ngày kể từ thời điểm tạo snapshot"*.
2. **Quyền hạn trên Shared Drives**:
   - Nếu thư mục đích nằm trên Shared Drive (Google Workspace), tài khoản chạy 502Drive phải có vai trò tối thiểu là `Content Manager` (Người quản lý nội dung) để có thể di chuyển và đưa file vào thùng rác. Với vai trò `Contributor`, tài khoản không thể trash file.
3. **Google-native Docs/Sheets/Slides**:
   - Các file định dạng riêng của Google không có MD5 checksum và kích thước byte vật lý. Việc khôi phục chúng cần dựa trên `version_id` hoặc tạo bản sao độc lập thay vì so sánh byte thô.
4. **Không can thiệp vào mã nguồn khác trong đợt này**:
   - Thiết kế này hoàn toàn độc lập, sẵn sàng để cài đặt trong giai đoạn sau mà không làm xáo trộn kiến trúc engine hiện tại.
