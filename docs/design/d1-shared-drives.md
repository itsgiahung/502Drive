# D1 — Shared Drives (Drive dùng chung)

Thiết kế cho §D1 (phase-13).

## 1. Hiện trạng và Scope OAuth

- Phạm vi OAuth hiện tại trong `internal/drive/auth.go` là `https://www.googleapis.com/auth/drive` (toàn quyền truy cập Drive). Scope này đã bao gồm đầy đủ quyền đối với Shared Drives (Team Drives).
- Quyết định: **Không thay đổi OAuth scope**.

## 2. API Google Drive v3 Client

Mọi lệnh gọi Drive API trong `internal/drive/client.go` đã được bật cờ:
- `SupportsAllDrives(true)` trên `Files.Get`, `Files.List`, `Files.Create`, `Files.Copy`, `Files.Update`.
- `IncludeItemsFromAllDrives(true)` và `Corpora("allDrives")` trên `Files.List`, `Changes.List`.
- Trích xuất `driveId` từ metadata `f.DriveId` gán vào `File.DriveID`.

## 3. Quản lý quyền Shared Drive & Báo lỗi rõ ràng

- **Quyền xem nguồn**:
  - Nếu tài khoản không có quyền truy cập vào Shared Drive: trả về 403/404 với thông báo rõ ràng (`permission_denied` hoặc `not_found`).
  - Nếu tài khoản chỉ có quyền xem (viewer) và file bị khoá quyền copy (`canCopy=false` hoặc `copyRequiresWriterPermission`):
    - Khi `Inspect` một file nguồn: từ chối 403 `copy_not_permitted` ("Tài khoản chỉ có quyền xem trong Drive dùng chung và không được phép sao chép tệp này").
    - Khi copy cây thư mục: các tệp không có quyền sao chép được phân loại `skipped` với mã `cannot_copy` (theo K1).
- **Quyền ghi đích**:
  - Khi thư mục đích nằm trong Shared Drive (`dst.DriveID != ""` hoặc parent có `DriveID`):
    - Kiểm tra `dst.CanAddChildren`. Nếu `false`: từ chối 409 `destination_not_writable` ("Thư mục đích trong Drive dùng chung không có quyền ghi cho tài khoản này").
    - Khi tạo folder/copy/shortcut: nếu đích thiếu quyền, báo lỗi `destination_not_writable` / `permission_denied`.

## 4. Fake Provider: Hỗ trợ Shared Drive

Cập nhật `internal/drive/drivetest/fake.go`:
- Thêm `SharedDrive(name string) drive.File`: tạo một Shared Drive với `DriveID` riêng.
- Hỗ trợ chế độ phân quyền trên Shared Drive (`SetSharedDriveRole(driveID, role string)`):
  - `viewer`: `CanCopy = false`, `CanAddChildren = false`.
  - `contributor` / `organizer`: `CanCopy = true`, `CanAddChildren = true`.
- Tự động kế thừa `DriveID` từ thư mục cha khi tạo thư mục/sao chép tệp/tạo shortcut bên trong Shared Drive.
