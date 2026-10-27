# Thiết kế T3 — Autostart thống nhất

## Bối cảnh & Mục tiêu
502Drive cần khởi động cùng phiên đăng nhập của người dùng khi được yêu cầu, chạy ngầm daemon + tray launcher (`502drive up --quiet`), không mở terminal hay TUI.
Trước đây logic autostart nằm rải rác trong `internal/cli/extras.go` và lệch cấu hình với file Inno Setup Windows (`packaging/windows/502drive.iss`).
Mục tiêu của slice T3:
1. Tạo package dùng chung `internal/autostart` cung cấp API:
   - `Status() (enabled bool, where string)`
   - `Enable() error`
   - `Disable() error`
   - `Toggle() (bool, error)`
2. Thống nhất cơ chế OS:
   - **Linux**: XDG Autostart `~/.config/autostart/502drive.desktop` chạy `"<exe>" up --quiet`.
     Đánh giá `systemd --user`: khi có `systemd --user` và unit `502drive.service`, daemon được kiểm soát bởi systemd nhưng XDG autostart vẫn chạy `up --quiet`. Instance lock (`instance.lock`) đảm bảo không bị chạy trùng.
   - **Windows**: Khóa registry HKCU `Software\Microsoft\Windows\CurrentVersion\Run`, value `502Drive` = `"<exe>" up --quiet`.
     Installer Inno Setup đồng bộ dùng chính value này.
   - **Darwin (macOS)**: Báo chưa hỗ trợ (hướng dẫn launchd).
3. Tích hợp:
   - CLI `502drive autostart enable|disable|status` dùng `internal/autostart`.
   - Tray: Thêm mục menu/action `autostart` hiển thị trạng thái thật và cho phép toggle.
   - Installer: Cập nhật `502drive.iss` khớp với `502drive.exe up --quiet`.
   - TUI: Cung cấp "Móc UI cho Claude" trong `PROGRESS.md`.

## Mô hình & Interface
```go
package autostart

type Manager struct {
    GOOS       string
    ConfigDir  string // Cho XDG autostart
    ExePath    string
    ExecCmd    func(name string, args ...string) ([]byte, error)
}
```
- `Status()` đọc trạng thái thật từ hệ thống (stat file hoặc query registry).
- `Enable()` và `Disable()` là idempotent (chạy nhiều lần không lỗi).
- Không yêu cầu quyền root/admin (chỉ ghi vào user home / HKCU).

## Xử lý lỗi & Biên
- Thư mục `~/.config/autostart` chưa tồn tại: `Enable()` tự tạo với quyền `0700`.
- File `.desktop` bị hỏng hoặc rỗng: `Status()` kiểm tra file tồn tại và nội dung hợp lệ.
- File exe không tìm thấy: Báo lỗi `autostart_failed`.
- Idempotent delete: File/registry không tồn tại khi `Disable()` không được báo lỗi.
