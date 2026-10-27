# Sản phẩm

502Drive copy thư mục/file Google Drive **phía server** (không tải rồi upload)
và giữ bản đã copy đồng bộ theo lịch hoặc bằng Drive Changes polling. Có batch,
hàng đợi công bằng, lọc nội dung, audit/repair và vòng đời collection. Một daemon
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
| Preview metadata → submit bền vững → job + event + idempotency; deep preflight nền (S7) | [service.go](../internal/app/service.go), [jobs.go](../internal/state/jobs.go), [server_test.go](../internal/httpapi/server_test.go) |
| Copy cây server-side, giữ tên/cây, retry, reconcile write mơ hồ; K1 skipped, N1 trùng tên, shortcut, Shared Drive (D1) | [run.go](../internal/engine/run.go), [engine_test.go](../internal/engine/engine_test.go) |
| Lọc khi copy/sync (S5: path/tên/size/mime, `keep_empty_folders`) | [filter](../internal/filter), [s5-selective-sync.md](design/s5-selective-sync.md) |
| Batch nhiều link, hàng đợi ưu tiên + fair share, di chuyển job, tạm dừng cả hàng đợi (S6) | [queue.go](../internal/state/queue.go), [service.go](../internal/app/service.go), [queue.go](../internal/app/queue.go) |
| Copy lại cùng nguồn (`mode=update|new`, L2) và vòng đời collection retire/resume/forget/cleanup (L1) | [lifecycle.go](../internal/app/lifecycle.go), [collection_lifecycle.go](../internal/httpapi/collection_lifecycle.go) |
| Audit/repair cây đích, dry-run mặc định, chỉ trash object có intent (S1) | [audit.go](../internal/engine/audit.go), [audits.go](../internal/httpapi/audits.go) |
| Sync theo lịch (interval/daily/weekly, DST), misfire coalesce | [schedule.go](../internal/schedule/schedule.go), [schedule_test.go](../internal/schedule/schedule_test.go), [syncer_test.go](../internal/daemon/syncer_test.go) |
| Realtime: cursor, lọc thay đổi, replay trong baseline, full reconcile định kỳ (S2) | [syncer.go](../internal/daemon/syncer.go), [s2-sync-replay.md](design/s2-sync-replay.md) |
| Tự bật sync sau copy đầu (S8), quota ledger Q1, throttle AIMD S3 | [aftercopy.go](../internal/state/aftercopy.go), [copy_ledger.sql](../internal/state/migrations/008_copy_ledger.sql), [throttle.go](../internal/drive/throttle.go) |
| Telegram: private chat, allowlist, callback one-shot, outbox, quiet hours, báo cáo ngày | [telegram](../internal/telegram), [notifier.go](../internal/telegram/notifier.go) |
| CLI (link trần, wait/watch, jobs report, history export), TUI, tray; i18n vi/en | [cli](../internal/cli), [tui](../internal/tui), [tray](../internal/tray), [i18n](../internal/i18n) |
| Autostart (XDG/Run key), systemd `--user`, power/sleep/resume, keep-awake | [autostart](../internal/autostart), [power](../internal/power), [desktop.md](features/desktop.md) |
| Backup/restore/cleanup/prune/vacuum offline; doctor `--fix` | [maintenance](../internal/maintenance), [maintenance_test.go](../internal/maintenance/maintenance_test.go) |
| Đóng gói rpm/deb/zip/installer | [.goreleaser.yaml](../.goreleaser.yaml), [packaging](../packaging) |

## Non-goal (không làm ở bản này)

- Không relay/download file bytes, không upload thay, không bypass quyền/quota.
- Không tạo link public, không hứa giữ owner/permission/revision của bản gốc.
- Không hai chiều (destination → source), không xóa bản đích khi nguồn mất.
- Không public HTTP endpoint: bind loopback, auth bearer bắt buộc, remote qua SSH.
- Không GUI web; tray + TUI + Telegram là bề mặt desktop.
- Không multi-account Google, không team/tenant, không billing.

## Chưa hỗ trợ (ghi nhận)

- Notification desktop ngoài Linux (`notify-send`); Windows/macOS chưa có.
- Lịch tuỳ biến trong chat Telegram (chỉ preset); sync delta so metadata, không so
  md5 nội dung (audit mới so size/md5).
- Token HTTP riêng cho operator (`api_tokens` không có; operator chỉ qua Telegram).
- Ký code Windows; auto-update (chỉ kiểm tra bản mới); watch/webhook endpoint.
- Google Drive/Telegram/Shared Drive thật chưa được acceptance trong phiên này;
  D1 mới kiểm bằng fake provider.

## Trạng thái phát hành

Bản release cần acceptance trên máy thật (Google Drive, Telegram, KDE/GNOME,
Windows 10/11) — [acceptance](acceptance.md) và [verification](verification.md).
Test fake provider **không** chứng minh hành vi Google thật.
