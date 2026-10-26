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

Chi tiết và invariant: [migration](migration.md#backup-và-restore).

```bash
systemctl --user stop 502drive        # hoặc dừng container
502drive backup /backups/502drive-$(date +%F)   # đích mới, 0700/0600 + MANIFEST.json
502drive restore /backups/... --to /srv/502drive-new   # dir mới/trống
502drive cleanup                # dry-run mặc định
502drive cleanup --apply        # chỉ file *.tmp cũ, không symlink
```

`backup` từ chối khi daemon giữ `instance.lock`; `cleanup` cũng vậy. Không bao
giờ xóa DB/WAL/`master.key`/backup/idempotency/mapping.

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

1. Stop daemon → backup → thay binary → `serve` → `status`/`doctor` → quan sát job.
2. Schema forward-only; binary cũ từ chối DB mới. Rollback: restore **cả bộ**
   backup vào dir mới + trỏ config, không chép DB đè.
3. Không rollback tác động Drive; freeze sync, reconcile output trước khi resume.

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
Windows. Repo public thì Actions miễn phí và workflow dưới đây dùng được.

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
