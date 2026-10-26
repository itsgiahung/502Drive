# 502Drive — bản đồ tài liệu

Go 1.27, một module. Rust/Tauri/React đã bị gỡ ngày 2026-10-26; bản nén ngoài
git ở `legacy-backup/`, không khôi phục và không tham chiếu trong code. Tài liệu
này mô tả **cái đang chạy**; phần chưa có ghi rõ "Chưa hỗ trợ".

## Đọc trước

1. [Kiến trúc](architecture.md) — package thật, luồng chạy, bất biến.
2. [HTTP contract](control-contracts.md) — route, auth, lỗi, idempotency.
3. [Data contracts](data-contracts.md) — schema `internal/state/migrations`.
4. Đúng feature bạn chạm trong [features/](features/).

## Bảng tài liệu

| Tài liệu | Nội dung | Trạng thái |
| --- | --- | --- |
| [product.md](product.md) | Phạm vi, non-goal, trạng thái từng phần | Đã implement một phần |
| [architecture.md](architecture.md) | Package, luồng request/job/sync, bất biến | Khớp code |
| [control-contracts.md](control-contracts.md) | Route HTTP v1, quyền, lỗi | Khớp `internal/httpapi` |
| [data-contracts.md](data-contracts.md) | Bảng SQLite 001–005 | Khớp migrations |
| [features/clone-jobs.md](features/clone-jobs.md) | Copy server-side, admission, retry, kết quả | Đã test local |
| [features/automation.md](features/automation.md) | Sync theo lịch + realtime polling | Đã test local, chưa live |
| [features/telegram.md](features/telegram.md) | Bot menu, paste-to-copy, thông báo | Đã test fake API |
| [features/cli.md](features/cli.md) | Command tree, TUI, exit code, JSON | Đã test local |
| [features/identity-control.md](features/identity-control.md) | Principal, token, operator | Một phần |
| [features/desktop.md](features/desktop.md) | Tray, đóng gói rpm/deb/exe | Đã test local |
| [migration.md](migration.md) | Nâng cấp Go→Go, backup/restore, rollback | Khớp code |
| [backlog.md](backlog.md) | Trạng thái R0–R5, G0–G7 | Cập nhật 2026-10-26 |
| [acceptance.md](acceptance.md) | Gate H/C/L/T/S/O/I + test tương ứng | Có cột trạng thái |
| [verification.md](verification.md) | Lệnh đã chạy, kết quả, việc chưa làm | Cập nhật 2026-10-26 |
| [runbooks.md](runbooks.md) | Vận hành: serve, backup, nâng cấp, sự cố | Khớp code |
| [oauth-setup.md](oauth-setup.md) | Login Google, callback, SSH forward, Docker | Khớp code |
| [install-linux.md](install-linux.md) | rpm/deb, systemd user, tray | Cần acceptance máy thật |
| [install-windows.md](install-windows.md) | zip + installer, không ký code | Cần acceptance máy thật |
| [install-docker.md](install-docker.md) | Image distroless, volume | Cần acceptance máy thật |
| [threat-model.md](threat-model.md) | Ranh giới tin cậy hiện tại | Khớp code |
| [troubleshooting.md](troubleshooting.md) | Triệu chứng → kiểm tra | Khớp code |
| [research.md](research.md) | Nguồn tham khảo đã dùng | Rút gọn |

## Trạng thái tổng quan (2026-10-26)

- **Đã implement + test local:** copy/admission/retry, HTTP contract, CLI/JSON,
  TUI, tray logic, Telegram (fake Bot API), scheduled + realtime sync, backup/
  restore/cleanup, đóng gói rpm/deb/windows, notification owner/operator.
- **Cần acceptance máy thật:** Google Drive/Telegram thật, tray trên KDE/GNOME,
  Windows 10/11, installer SmartScreen, Docker volume/OAuth callback.
- **Chưa hỗ trợ:** xem danh sách ở từng feature và [backlog](backlog.md).

## Kiểm chứng

```bash
gofmt -l cmd internal
go vet ./...
go test -race ./...
go test -run TestLoadTenThousandFiles ./internal/engine   # chỉ khi không -race
```

Kết quả lần chạy gần nhất và phần chưa làm: [verification](verification.md).
