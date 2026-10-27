# Runbooks

Quy trình cho daemon Go. Không chạy `backup/restore/cleanup` trên data thật khi
`serve` còn chạy.

## Kiểm tra source

```bash
gofmt -l cmd internal
go vet ./...
go test -race ./...
go build ./cmd/502drive
```

Test đắt: load probe (`go test -run TestLoadTenThousandFiles ./internal/engine`,
không `-race`), conformance OpenAPI ([openapi_test.go](../internal/httpapi/openapi_test.go)).
Kết quả phiên gần nhất: [verification](verification.md).

## Bootstrap

1. `502drive setup` (config + api-token 0600, data dir riêng).
2. `502drive setup --oauth-client client_secret.json`.
3. `502drive setup --telegram-token-file bot-token --telegram-owner-id 123456789`
   nếu dùng bot; `--oauth-callback-port 50200` khi cần cổng callback cố định.
4. `502drive serve` (foreground; systemd user/container quản lý).
5. `502drive account login` → approve; kiểm `502drive doctor --online`.
6. `502drive destinations add FOLDER_LINK` + `destinations default ID`.
7. `502drive copy LINK --wait`; `502drive collections list`; `502drive sync enable …`.

Config là JSON private; relative path resolve theo thư mục config. Không đưa
token/secret vào args/log/JSON output.

## Sức khỏe

| Kiểm tra | Lệnh |
| --- | --- |
| Process sống, schema, worker | `502drive status`, `GET /health/ready` |
| Account Google + destination | `502drive doctor` (offline), `--online` để gọi Google |
| Bot Telegram | `status` → `bot_state` (`disabled/starting/polling/error`) |
| Queue | `status` (running/queued/blocked), `jobs list --state blocked` |
| Sync | `sync status COLLECTION`, `collections get ID` |
| Log | stderr của `serve` (systemd journal/container log), đã redact secret |

Nếu `ready=false`: kiểm lock data dir (daemon khác), quyền file config/token,
schema (binary cũ hơn DB). Dead bot không làm core unready.

## Backup / restore / cleanup

Source: [internal/maintenance](../internal/maintenance). Test:
[maintenance_test.go](../internal/maintenance/maintenance_test.go).

```bash
systemctl --user stop 502drive        # hoặc dừng container
502drive backup /backups/502drive-$(date +%F)   # đích mới, 0700/0600 + MANIFEST.json
502drive restore /backups/... --to /srv/502drive-new   # dir mới/trống
502drive cleanup                # dry-run mặc định
502drive cleanup --apply        # chỉ file *.tmp cũ, không symlink
```

- `backup`: từ chối nếu daemon giữ `instance.lock`; `VACUUM INTO` DB + `secrets/master.key`
  + `secrets/google-token.enc` (nếu có) + `config.json`; thư mục 0700, file 0600,
  `MANIFEST.json` (schema_version, sha256 từng file, thời điểm, version binary).
- `restore`: chỉ vào data dir **mới/trống**; kiểm sha256, `integrity_check`,
  `foreign_key_check`, schema ≤ binary, giải mã token bằng key trong backup; lỗi
  → không để lại thư mục dở dang; `config.json` được sửa `data_dir` sang đích.
- `cleanup`: chỉ xóa `*.tmp` do chương trình tạo, cũ hơn ngưỡng, không theo symlink, không
  ra ngoài data dir, từ chối khi có writer. Không bao giờ xóa DB/WAL/`master.key`/backup/
  idempotency/mapping.

## Tải và phản hồi chậm

- Probe 10 000 file/500 folder (fake) trên máy này: 1m50s, `GetJob` tối đa 62 ms,
  pause 4 ms, heap +9 MiB ([load_test.go](../internal/engine/load_test.go)).
  Đây là harness, không phải Google.
- Khi Drive 429: engine backoff + `Retry-After`, trạng thái `reconciling/retry_wait`;
  syncer cooldown bằng `next_poll_at`. Không tăng trần âm thầm.
- Queue đầy: admission trả `429 capacity_exceeded` (retryable), slot sync giữ
  due. Không xóa row để "giải phóng".
- Provider stall: xem `status`/`doctor`/events; nếu cần dừng nhận việc, chạy
  `serve --maintenance` (đọc được, từ chối job/sync mới).

## Nâng cấp / rollback

- Migration forward-only trong [internal/state/migrations](../internal/state/migrations), áp
  trong transaction khi `state.Open`; **không sửa migration đã áp**, chỉ thêm file mới. Binary cũ
  gặp DB mới hơn sẽ từ chối mở (`unsupported database version`): nâng cấp một chiều.
- Quy trình: dừng daemon → backup → thay binary → khởi động → `502drive status` / `doctor` →
  quan sát job và sync. `serve --maintenance` đọc được status/doctor nhưng từ chối job mới,
  cấu hình sync và run now (`503 maintenance`).
- **Binary và DB phải khớp.** Schema không tương thích: restore **cả bộ** backup vào data dir
  mới rồi trỏ config sang; không chép DB cũ đè lên DB mới.
- **Không rollback tác động trên Drive.** Restore DB cũ không xóa file đã tạo trên Drive kể từ
  backup. Sau rollback: đóng băng lịch/realtime, đối soát output bằng operation identity, chỉ
  tiếp tục khi trạng thái rõ. Không tự bỏ output vào thùng rác để "undo". Nếu không thể đối
  soát an toàn, để job/sync `blocked`/`needs_reconcile` và xử lý tiến, không hứa rollback một cú bấm.

## Release tại máy (không cần CI)

GitHub Actions có thể hết phút miễn phí (repo private) nên mọi bước chạy được ở máy:

```bash
make check                       # gofmt, vet, test -race
scripts/release-local.sh         # kiểm tra + goreleaser snapshot + sha256sum -c + smoke binary
scripts/release-local.sh --tag   # build thật cho tag ở HEAD (cây sạch), vẫn không đăng
scripts/release-local.sh --rpm-smoke   # cài rpm trong container Fedora (podman/docker)
```

Kết quả ở `dist/` kèm `checksums.txt`. Đăng bản phát hành là bước tay riêng
(`gh release create …`, in ở cuối script). Installer Windows cần Inno Setup trên
Windows. Repo đã public nên GitHub Actions chạy miễn phí; thử toàn bộ pipeline mà **không
đăng gì** bằng `gh workflow run "Go Release" --ref main` (bản dry run: version snapshot,
artifact nằm ở trang run, job Publish bị bỏ qua). Đăng bản thật chỉ khi đẩy tag `v*`.

## Release (khi chạy CI thật)

- `go.yml`: gofmt/vet/test race + build matrix.
- `go-release.yml` (tag `v*`): test → goreleaser build → verify checksum/smoke →
  rpm smoke (container Fedora) → Windows installer → publish file đã verify.
- Installer Windows chưa ký code: nêu rõ trong release notes; không hứa "đã ký".
- Docs/checks: [verification](verification.md).

## Retention và dọn rác

- Job/event/idempotency/mapping giữ mặc định; chưa có auto-prune.
- `cleanup` chỉ dọn file tạm `*.tmp` cũ hơn ngưỡng trong data dir.
- Log: để systemd journal/container runtime xoay vòng; app không tự xóa log.
- Backup: owner tự quản lý, giữ ít nhất bản đã restore-test.

## Kiểm tra laptop thật

Mọi kịch bản dưới đây **chưa chạy trên OS thật** trong phiên 2026-10-27 (xem
cột "OS thật" ở [acceptance](acceptance.md#vòng-đời-laptop-t1-ngủmạng-chống-trùng)).
Chạy tay và ghi log trước khi đánh dấu "chạy".

Chuẩn bị:

```bash
# daemon ở foreground để đọc log trực tiếp
502drive serve            # hoặc: journalctl --user -u 502drive -f
# terminal khác: theo dõi trạng thái
watch -n2 '502drive --json status | python3 -m json.tool | grep -A6 lifecycle'
```

Checklist (đánh dấu khi có log):

1. **Gập máy 1 phút** giữa lúc đang copy một thư mục vài chục file.
   Kỳ vọng log: `power: sleeping` → (mở máy) `power: resuming` → `power: online`;
   `lifecycle.state` đi `running → sleeping → resuming → running`; job kết thúc
   `succeeded`, **không** có hai bản của cùng một file ở đích (`ls` thư mục đích).
2. **Gập máy 1 giờ** (hoặc chỉnh đồng hồ nhảy) với lịch sync hằng ngày đã quá giờ.
   Kỳ vọng: đúng **một** run catch-up cho mỗi collection trong log syncer.
3. **Rút Wi-Fi / tắt VPN** khi đang copy: log `power: network unreachable; waiting
   for connectivity`; job vẫn `running` (không `failed`), item không tăng attempt
   (`502drive jobs get ID --json` → `counts.failed` không tăng). Bật mạng lại →
   job tiếp tục.
4. **Bật VPN** khi đang rảnh: log `power: network changed; resetting idle connections`;
   lệnh `502drive status` vẫn thành công ngay.
5. **Copy dở rồi gập máy**, mở lại: không có lỗi "unknown_remote_outcome" nếu máy
   chỉ ngủ; nếu có (mạng chết đúng lúc ghi) thì item `blocked` kèm lý do và
   `retry` xác nhận lại trước khi gửi.
6. **Restart máy** khi daemon đang chạy: sau khi bật lại, log có
   `power: resuming, gap=…` (gap ≥ thời gian tắt máy) và engine `ActiveJobs`
   đưa item `executing` về `reconciling` trước khi ghi tiếp.
7. **Linux**: kiểm tra delay lock không bị giữ:
   `systemd-inhibit --list | grep 502Drive` chỉ thấy inhibitor `block` khi
   `keep_awake_while_copying: true` và đang có job; sau khi sleep, lệnh trả về ngay.
8. **Windows**: `powercfg /requests` không có 502Drive khi tắt keep-awake; sau khi
   resume từ hibernate, tray hiện trạng thái mới trong ≤ 10s.

Ghi lại: OS + phiên bản, thời điểm, đoạn log `power:` + `lifecycle`, kết quả
`jobs get`, ảnh/`ls` đích nếu có nghi ngờ trùng. Không dùng dữ liệu thật để thử
"xóa/ghi đè"; chỉ copy trong thư mục test.

## Chạy live acceptance

Harness kiểm thử trực tiếp trên Google Drive thật (`internal/live`) được bảo vệ bằng build tag `live` và cờ môi trường `DRIVE502_LIVE=1`.

### 1. Nguyên tắc an toàn
- **Cô lập hoàn toàn**: Test tự tạo thư mục gốc duy nhất theo định dạng `502drive-live-test-<unix>` trong My Drive.
- **Tự động dọn dẹp**: Mọi thao tác copy/tạo mới chỉ diễn ra trong thư mục này. Khi test kết thúc (`t.Cleanup`), thư mục test được tự động chuyển vào thùng rác Google Drive (`TrashFile`). Tuyệt đối không đụng chạm đến bất kỳ dữ liệu nào ngoài thư mục test.
- **Mô phỏng lỗi kết nối**: Sử dụng `drive.FaultInjector` (bọc `http.RoundTripper`) để mô phỏng mất phản hồi sau ghi hoặc chèn HTTP 429 mà không can thiệp logic của engine.

### 2. Biến môi trường
- `DRIVE502_LIVE=1`: Bắt buộc để chạy test live.
- Khóa xác thực (chọn một trong các cách):
  - `DRIVE502_LIVE_ACCESS_TOKEN`: Bearer token Google OAuth còn hạn.
  - `DRIVE502_LIVE_CLIENT_SECRET` và `DRIVE502_LIVE_TOKEN_DATA`: Đường dẫn hoặc JSON client_secret và token (tự làm mới khi hết hạn).
- `DRIVE502_LIVE_TELEGRAM_TOKEN`: (Tuỳ chọn) Bot token để kiểm tra API getMe của Telegram.

### 3. Lệnh chạy
```bash
export DRIVE502_LIVE=1
export DRIVE502_LIVE_ACCESS_TOKEN="<access-token>"
go test -v -tags=live -timeout 600s ./internal/live
```
