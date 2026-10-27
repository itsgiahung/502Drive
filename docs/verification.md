# Verification — phiên 2026-10-26

Môi trường: Linux x86_64 (Fedora), Go 1.27.3 tại
`<thư mục toolchain>/go1.27.3/go/bin`, SQLite thuần Go
(`CGO_ENABLED=0`). Mọi test dùng **fake provider** và **fake Bot API**; không có
Google Drive/Telegram thật trong phiên này.

## Đã chạy

| Lệnh | Kết quả |
| --- | --- |
| `gofmt -l cmd internal` | không in gì |
| `go vet ./...` | exit 0 |
| `go test -race -count=1 ./...` | tất cả package `ok` (app, cli, config, daemon, drive, engine, httpapi, i18n, maintenance, schedule, state, telegram, tray, tui) |
| `go test -run TestLoadTenThousandFiles -v ./internal/engine` (không `-race`) | PASS — xem số đo dưới |
| `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ./cmd/502drive` | ok |
| `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build ./cmd/502drive` | ok |
| `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./cmd/502drive ./cmd/502drive-tray` | ok (tray `-H=windowsgui`, subsystem 2) |
| `goreleaser release --snapshot --clean --skip=publish` (v2.19.0, local) | 4 archive + 4 gói rpm/deb + `checksums.txt` |
| `rpm -qlp/-Kv/-i --test dist/..._linux_amd64.rpm` | 7 file đúng, `digests OK`, install test exit 0 |
| `sha256sum -c checksums.txt` | 8/8 OK |
| go-winres + build tray Windows | resource dir 82 544 byte, 6/6 payload icon từ `packaging/assets/logo.ico` được nhúng |
| `desktop-file-validate packaging/502drive-tray.desktop` | OK |
| `systemd-analyze verify packaging/502drive.service` | chỉ báo thiếu `/usr/bin/502drive` (chưa cài), không lỗi cú pháp |
| `python3 scripts/check-docs-links.py .` | xem "Docs" dưới |
| `python3 -c "import json;json.load(open('docs/contracts/openapi.json'))"` | OK |
| `git diff --check` | xem "Docs" dưới |

### Load probe (fake provider, 4 worker)

```
copied 10501 nodes in 1m18.299s (134 items/s)
GetJob while busy: 3396 probes, max 45ms; pause action 0s
heap: before 6 MiB, after 16 MiB, delta 9 MiB
```

(Lần chạy trước, cùng máy: 1m50s, max GetJob 62 ms, pause 4 ms — dao động
theo tải máy.)

Đây là giới hạn của harness (fake Drive + một connection SQLite), **không** phải
throughput Google Drive. Probe bị skip dưới `-race` (chậm ~80x) và dưới `-short`.

## Phiên 2026-10-27 (Lát H: vòng đời, realtime, TUI, mèo, release tại máy)

| Lệnh / việc | Kết quả |
| --- | --- |
| `gofmt -l cmd internal`, `go vet ./...` | sạch |
| `go test -race -count=1 ./...` | tất cả `ok`, thêm `internal/mascot`, `internal/update` |
| cross-build linux/amd64, linux/arm64, windows/amd64 (+tray), darwin/arm64 | ok |
| `scripts/release-local.sh --fast` | gofmt/vet, docs (291 link, 0 hỏng), goreleaser snapshot, `sha256sum -c` 8/8, smoke binary |
| Smoke tay: `502drive setup` → `502drive up` → `curl /health/live` → `up` lần hai → `status` → `curl -N /v1/stream` → `POST /v1/shutdown` | daemon `started` rồi `running`, stream gửi `hello`, shutdown dừng sạch; daemon sống sau khi `up` thoát (setsid) |
| CPU nhàn rỗi của daemon (30 giây, không job) | 3 tick/30 s ≈ **0,1%** CPU, RSS ≈ 27 MB |
| `go test -bench` TUI, danh sách 10 000 job (cửa sổ 120×40, có màu) | ≈ 0,5 ms và 958 alloc mỗi `View()` — không phụ thuộc N vì chỉ vẽ cửa sổ |
| `go test -bench` mèo: `Step` + `Lines` một khung | ≈ 0,6 ms (≈ 0,8% một lõi ở 14 fps, chỉ khi ở màn Home) |

GitHub Actions (repo public từ 2026-10-27, runner của GitHub):

| Workflow | Kết quả |
| --- | --- |
| `Go CI` trên `main` | gofmt, vet, `go test -race` và build linux/amd64, linux/arm64, windows/amd64, darwin/arm64: **success** |
| `Go Release` (dry run `workflow_dispatch`, không đăng) | test, build GoReleaser, verify checksum, **Inno Setup tạo installer Windows**: success; RPM smoke trong container Fedora: success; job Publish bị bỏ qua đúng như thiết kế |

Chưa kiểm chứng ở phiên này: màu/hoạt ảnh thật trong KDE Konsole/GNOME Terminal/
Windows Terminal (đã xem GIF dựng từ cùng bộ vẽ, không phải terminal thật),
`notify-send` thật, tray thật, spawn tách rời trên Windows, autostart ở phiên
đăng nhập thật. GitHub Actions không chạy được (hết phút miễn phí với repo
private); toàn bộ kiểm tra chạy bằng `scripts/release-local.sh`.

## Chưa chạy

- Google Drive thật: copy/sync/Changes API, 429/quota, Shared Drive, resource key,
  reconnect. Fake provider không chứng minh hành vi provider.
- Telegram bot thật, thiết bị mobile, operator thật, 429/403 thật.
- Tray trên KDE Plasma/GNOME (Wayland/X11); TUI trong terminal thật (đã chạy
  chương trình Bubble Tea với input/output trong bộ nhớ).
- Windows 10/11: tray, installer `.exe`, SmartScreen; Inno Setup chưa chạy.
- Docker: build image, volume, OAuth callback qua cổng publish.
- Publish thật của `go-release.yml` (chỉ chạy khi đẩy tag `v*`; dry run đã qua).
- 100k item/100 collection/200 admission; soak 24h; disk full/DB busy.
- OAuth live (consent/refresh/revoke), macOS tray (ngoài scope).

## Docs

- Link tương đối: `python3 scripts/check-docs-links.py .` → **0 broken** trên
  tổng số link đã quét (script bỏ qua `legacy-backup/`, `.git`, `dist`, `build`).
- `grep -rniE "tauri|cargo|rust|react|npm" --include=*.md .` chỉ còn câu nói về
  việc đã gỡ Rust/Tauri/React.
- `docs/handoff/` đã xóa (spec nội bộ, không phải tài liệu sản phẩm).

## Đợt B (T3, T5, S11, R1) — đo S11 nhàn rỗi

- Binary build từ nhánh `batch-b`, daemon chạy `serve --maintenance` với data dir tạm
  (dữ liệu tổng hợp, không có Drive/token thật), đo 30 s sau khi lắng nghe:
  - CPU: 0 tick (0 ms) trong 30 s nhàn rỗi (đọc `/proc/<pid>/stat`).
  - RSS: 26872 kB (≈ 26.9 MB), 11 thread; không tăng giữa hai lần đo.
- **Chưa đo**: copy 10k file (cần Drive thật), `scripts/soak.sh` (chỉ kiểm `bash -n`).
- `systemd-analyze verify packaging/502drive.service`: không lỗi cú pháp; chỉ cảnh báo
  `/usr/bin/502drive` chưa được cài.
