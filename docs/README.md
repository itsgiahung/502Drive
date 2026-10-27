# 502Drive — bản đồ tài liệu

Go 1.27, một module (bản Rust/Tauri cũ đã gỡ ngày 2026-10-26). Tài liệu này mô
tả **cái đang chạy**; phần chưa có ghi rõ "Chưa hỗ trợ".

## Đọc trước

1. [Kiến trúc](architecture.md) — package thật, luồng job/sync/audit, bất biến.
2. [HTTP contract](control-contracts.md) — route, quyền, lỗi, idempotency.
3. [Data contracts](data-contracts.md) — schema `internal/state/migrations` (001–011, 020–025).
4. Đúng feature bạn chạm trong [features/](features/).

## Bảng tài liệu

| Tài liệu | Nội dung | Trạng thái |
| --- | --- | --- |
| [product.md](product.md) | Phạm vi, non-goal, trạng thái từng phần | Khớp code |
| [architecture.md](architecture.md) | Package, job/sync/audit/lifecycle, bất biến | Khớp code |
| [control-contracts.md](control-contracts.md) | 58 route (56 dưới `/v1` + 2 health), quyền, lỗi | Khớp router + OpenAPI |
| [data-contracts.md](data-contracts.md) | Bảng SQLite 001–011, 020–025 | Khớp migrations |
| [features/clone-jobs.md](features/clone-jobs.md) | Copy, admission, N1/K1/S5/D1, L1/L2, batch/queue, deep preflight, audit | Đã test local |
| [features/automation.md](features/automation.md) | Sync lịch + realtime, replay, full reconcile, quota, S8 | Đã test local, chưa live |
| [features/telegram.md](features/telegram.md) | Bot menu, paste-to-copy, thông báo, quiet hours | Đã test fake API |
| [features/cli.md](features/cli.md) | Command tree, TUI, exit code, JSON | Đã test local |
| [features/identity-control.md](features/identity-control.md) | Principal, token, operator, quyền owner/admin | Một phần (chưa có `api_tokens`) |
| [features/desktop.md](features/desktop.md) | Tray, TUI, autostart/systemd, đóng gói | Đã test local |
| [acceptance.md](acceptance.md) | Gate H/C/L/T/S/O/I + test tương ứng | Có cột trạng thái |
| [verification.md](verification.md) | Lệnh đã chạy, kết quả, việc chưa làm | Cập nhật 2026-10-27 |
| [runbooks.md](runbooks.md) | Vận hành: serve, backup, prune/vacuum, nâng cấp, sự cố | Khớp code |
| [oauth-setup.md](oauth-setup.md) | Login Google, callback, SSH forward, Docker | Khớp code |
| [install-linux.md](install-linux.md) | rpm/deb, systemd user, tray, autostart | Cần acceptance máy thật |
| [install-windows.md](install-windows.md) | zip + installer, không ký code | Cần acceptance máy thật |
| [install-docker.md](install-docker.md) | Image distroless, volume, nâng cấp | Cần acceptance máy thật |
| [threat-model.md](threat-model.md) | Ranh giới tin cậy hiện tại | Khớp code |
| [troubleshooting.md](troubleshooting.md) | Triệu chứng → kiểm tra | Khớp code |
| [design/](design/) | Quyết định cho từng slice (S1, S2, S3, S5, S6, S7, N1, K1, L1, L2, T1–T5, H1–H6, Q1, R1) | Lịch sử thiết kế |

## Trạng thái tổng quan (2026-10-27)

- **Đã implement + test local:** copy/admission/retry/reconcile, N1 trùng tên, K1
  skipped, S5 lọc, D1 Shared Drive (fake), L1/L2 lifecycle + copy lại, audit/repair,
  batch + hàng đợi fair + pause queue, deep preflight, HTTP contract, CLI/JSON,
  report/history export, TUI, tray/autostart/systemd, Telegram (fake Bot API),
  scheduled + realtime sync (replay/full reconcile), quota ledger, backup/restore/
  cleanup/prune/vacuum, đóng gói rpm/deb/zip/installer.
- **Cần acceptance máy thật:** Google Drive/Telegram thật, tray trên KDE/GNOME,
  Windows 10/11, installer SmartScreen, Docker volume/OAuth callback.
- **Chưa hỗ trợ:** xem danh sách ở từng feature và [acceptance](acceptance.md).

## Kiểm chứng

```bash
gofmt -l cmd internal
go vet ./...
staticcheck ./...
go test -race -count=1 ./...
python3 scripts/check-docs-links.py .
python3 -c "import json;json.load(open('docs/contracts/openapi.json'))"
git diff --check
```

Kết quả lần chạy gần nhất và phần chưa làm: [verification](verification.md).
