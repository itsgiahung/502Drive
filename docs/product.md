# Sản phẩm

502Drive copy thư mục/file Google Drive **phía server** (không tải rồi upload)
và giữ bản đã copy đồng bộ theo lịch hoặc bằng Drive Changes polling. Một daemon
Go sở hữu DB, worker, token; CLI/TUI/tray/Telegram là client HTTP hoặc adapter
dùng chung `internal/app`.

## Người dùng và bề mặt điều khiển

- **Chủ máy (owner):** `setup`, chạy `serve`, đăng nhập Google, chọn đích, xem
  job/sync/doctor, cấu hình bot Telegram (token file, owner id), operator.
- **CLI:** toàn bộ nghiệp vụ; JSON ổn định cho script ([cli](features/cli.md)).
- **TUI:** `502drive menu` trong terminal ([desktop](features/desktop.md)).
- **Tray:** icon khay, thao tác nhanh, mở TUI, copy link từ clipboard.
- **Telegram:** menu nút, dán link tự copy (opt-in), job/sync, thông báo
  ([telegram](features/telegram.md)).

## Phạm vi hiện có (có code + test local)

| Năng lực | Bằng chứng |
| --- | --- |
| Preview metadata → submit bền vững → job + event + idempotency | [service.go](../internal/app/service.go), [jobs.go](../internal/state/jobs.go), [server_test.go](../internal/httpapi/server_test.go) |
| Copy cây server-side, giữ tên/cây, retry, reconcile write mơ hồ | [run.go](../internal/engine/run.go), [engine_test.go](../internal/engine/engine_test.go) |
| Sync theo lịch (interval/daily/weekly, DST), misfire coalesce | [schedule.go](../internal/schedule/schedule.go), [schedule_test.go](../internal/schedule/schedule_test.go), [syncer_test.go](../internal/daemon/syncer_test.go) |
| Realtime: cursor, lọc thay đổi, token hết hạn → reconcile | [syncer.go](../internal/daemon/syncer.go), [atomic với run](../internal/state/sync.go) |
| Telegram: private chat, allowlist, callback one-shot, outbox, thông báo job | [telegram](../internal/telegram), [notifier.go](../internal/telegram/notifier.go) |
| CLI, TUI, tray; i18n vi/en cho menu | [cli](../internal/cli), [tui](../internal/tui), [tray](../internal/tray), [i18n](../internal/i18n) |
| Backup/restore/cleanup offline | [maintenance](../internal/maintenance), [maintenance_test.go](../internal/maintenance/maintenance_test.go) |
| Đóng gói rpm/deb/zip/installer | [.goreleaser.yaml](../.goreleaser.yaml), [packaging](../packaging) |

## Non-goal (không làm ở bản này)

- Không relay/download file bytes, không upload thay, không bypass quyền/quota.
- Không tạo link public, không hứa giữ owner/permission/revision của bản gốc.
- Không hai chiều (destination → source), không xóa bản đích khi nguồn mất.
- Không public HTTP endpoint: bind loopback, auth bearer bắt buộc, remote qua SSH.
- Không GUI web; tray + TUI + Telegram là bề mặt desktop.
- Không multi-account Google, không team/tenant, không billing.

## Chưa hỗ trợ (ghi nhận)

- Notification desktop từ tray; lịch tuỳ biến trong chat (chỉ preset).
- `after_copy_sync_mode` mới lưu preference, chưa tự bật sync cho collection mới.
- Realtime chưa replay raw change page trong lúc bootstrapping.
- Token HTTP riêng cho operator (`api_tokens` không có; operator chỉ qua Telegram).
- Ký code Windows; auto-update; watch endpoint.

## Trạng thái phát hành

Bản release cần acceptance trên máy thật (Google Drive, Telegram, KDE/GNOME,
Windows 10/11) — [acceptance](acceptance.md) và [verification](verification.md).
Test fake provider **không** chứng minh hành vi Google thật.
