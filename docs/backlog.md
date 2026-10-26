# Backlog và trạng thái (2026-10-26)

Bảng dưới đây là trạng thái **thật** của code Go hiện tại, không phải kế hoạch.
Cột "Bằng chứng" trỏ tới file/test có thật. Gate chi tiết: [acceptance](acceptance.md).

## Lát phát hành R0–R5

R0–R5 là các lát giao hàng dùng trong kế hoạch trước; ý nghĩa được chốt lại ở đây
để không mơ hồ.

| Lát | Nội dung | Trạng thái | Bằng chứng |
| --- | --- | --- | --- |
| R0 | Module Go, SQLite/WAL, migration, lock data dir | Đã implement + test local | [store.go](../internal/state/store.go), [store_test.go](../internal/state/store_test.go) |
| R1 | HTTP core, admission bền vững, OpenAPI conformance | Đã implement + test local | [server.go](../internal/httpapi/server.go), [server_test.go](../internal/httpapi/server_test.go) |
| R2 | Engine copy, intent, retry, reconcile, result URL | Đã implement + test local | [run.go](../internal/engine/run.go), [engine_test.go](../internal/engine/engine_test.go) |
| R3 | CLI/JSON/exit code, TUI, tray | Đã implement + test local; tray/TUI cần acceptance máy thật | [cli](../internal/cli), [tui](../internal/tui), [tray](../internal/tray) |
| R4 | Library + scheduled/realtime sync | Đã implement + test local (fake clock/fake Drive); chưa live | [syncer.go](../internal/daemon/syncer.go), [syncer_test.go](../internal/daemon/syncer_test.go) |
| R5 | Telegram, notifications, packaging, release | Đã implement + test fake/local; cần acceptance thật | [telegram](../internal/telegram), [.goreleaser.yaml](../.goreleaser.yaml) |

## Milestone G0–G7

| Gate | Nội dung | Trạng thái | Còn lại |
| --- | --- | --- | --- |
| G0 | Go runtime + SQLite spike | Xong | — |
| G1 | Durable HTTP/application core (H1–H10) | Đã test local | live acceptance nhỏ |
| G2 | Drive copy/recovery (C1–C12) | Đã test fake provider | copy thật trên Drive tổng hợp, quota/429 thật |
| G3 | CLI/TUI/tray (L1–L10) | Đã test local | TUI/tray trên KDE/GNOME, Windows 10/11 |
| G4 | Telegram menu/auto-copy/notify (T1–T12) | Đã test fake Bot API | bot thật, mobile vi/en, operator thật |
| G5 | Backup/restore/cleanup, release (O1–O12) | Backup/restore/cleanup + rpm smoke local; release chưa chạy CI | chạy CI thật, installer Windows, ký code |
| G6 | Library + sync (S1–S12) | Đã test fake clock/Drive | Changes API thật, DST thật, soak |
| G7 | Destination browser/mkdir UX | **Chưa hỗ trợ** | thiết kế route + UI trước khi code |

## Việc còn lại (ưu tiên)

1. **Live acceptance**: Google Drive thật (copy/sync/rate limit/reconnect),
   Telegram bot thật, tray trên KDE Plasma + GNOME (Wayland/X11), Windows 10/11
   (tray, installer, SmartScreen), Docker volume + OAuth callback.
2. **CI/release thật**: `go.yml` + `go-release.yml` chưa từng chạy trên GitHub (repo
   private hết phút Actions). Đã có `scripts/release-local.sh` làm đủ build +
   verify tại máy; repo public thì CI chạy lại miễn phí.
3. **Ký code Windows** (hiện chưa ký, SmartScreen cảnh báo) và ghi chú phát hành.
4. **Sync nâng cao**: replay raw change page trong bootstrapping; full reconcile
   định kỳ; `after_copy_sync_mode` áp cho collection mới; digest/no-change quiet
   (no-change đã im lặng cho notification).
5. **Desktop**: thông báo desktop trên Windows, macOS tray (ngoài scope). Đã xong ở
   Lát H: notification Linux, autostart, tooltip tiến độ, kiểm tra bản mới opt-in.
6. **Identity**: bảng `api_tokens` cho nhiều token/operator HTTP (hiện một bearer).
7. **G7**: duyệt folder đích trong chat/API, tạo folder có intent.

## Không làm (non-goal)

Relay/download bytes, upload thay, bypass quyền/quota, hai chiều, public endpoint,
multi-account, GUI web. Xem [product](product.md).
