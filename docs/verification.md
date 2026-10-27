# Verification — cập nhật 2026-10-28

## Phiên 2026-10-28 — Google Drive thật (lần đầu)

Môi trường: Fedora 44, GNOME 50, daemon `v0.3.0-dev` (9b16d2b) đăng nhập một tài
khoản cá nhân; nguồn là thư mục thử do **một tài khoản khác** tạo và chia sẻ quyền
Viewer (6 file gồm hai file trùng tên, một Google Doc, thư mục con, thư mục rỗng);
đích là thư mục thử riêng (`destinations add`, không phải mặc định). Chạy qua CLI,
không dùng harness `internal/live` (không cần đưa token ra ngoài daemon).

| Kịch bản | Kết quả |
| --- | --- |
| `doctor --online`, `onboard --no-input` | đạt; exit 0 |
| `mkdir`, `destinations add` | đạt |
| `inspect` / `inspect --deep` | 6 file, 2 thư mục, 107 B, 1 native, 0 không copy được — khớp nguồn |
| `copy --wait` | 9/9 mục trong 6,5 s; cây, thư mục rỗng, Google Doc giữ nguyên; trùng tên → `dup (2).txt` |
| `copy` lần hai (update) | **lỗi** → sửa: Google Doc không đổi bị copy lại (fingerprint dùng `version`) — `ddafacd` |
| `audit` | **lỗi** → sửa: báo nhầm tên `dup (2).txt` do N1 đặt (`a958d12`); lệnh `audit` lần sau phát lại báo cáo cũ (`24d7489`) |
| sync khi nguồn không đổi | 0 mục copy; fingerprint dạng cũ được nâng cấp, không copy lại |
| sync sau khi đổi nguồn | file mới ✓, move ✓, thư mục đổi tên ✓, xóa ở nguồn giữ bản đích ✓; đổi tên file = generation mới (giữ bản cũ theo `versioned_copy`) |
| `audit --repair --apply` | **lỗi** → sửa: repair không có gì để sửa vẫn trash lịch sử phiên bản (`9b16d2b`) |
| realtime với nguồn được chia sẻ | file mới được copy sau ~40–50 s |
| `sync stop`, `retire`, `cleanup` | dry-run ✓, sai tên xác nhận bị từ chối ✓, cleanup chỉ đưa gốc vào thùng rác ✓ |
| `kill -9` daemon giữa lúc copy | 5 mục `unknown_remote_outcome` (thực tế chưa được tạo) → `jobs retry` → 10/10, không trùng |

| Telegram thật: `/start`, `/status`, dán link, `/find crash` | đều `processed`; dán link → job 10/10 vào đích mặc định; thẻ tiến độ được sửa tới trạng thái cuối |
| Telegram sau `retry` | **lỗi** → sửa: job báo `blocked` rồi `retry` thành công không có tin thứ hai (`6c87102`) |

### Kiểm lại sau khi sửa (cùng ngày, daemon 0c32ecf)

Nguồn mới: 8 file nhỏ, do tài khoản khác tạo và chia sẻ Viewer.

| Kịch bản | Kết quả |
| --- | --- |
| Đổi tên thuần `r01.txt` → `r01-renamed.txt` rồi sync | cùng ID file ở đích được đổi tên tại chỗ; vẫn 8 mục; job 1/1 |
| `kill -9` daemon ở 1,0 / 1,8 / 2,2 s sau khi copy | lệnh ghi đã tới Drive: đối soát tìm thấy theo intent, 9/9, không chặn |
| `kill -9` ở 1,4 s (kèm một crash chồng sau đó) | 4 mục `unknown_remote_outcome`; daemon tự kiểm lại sau 2 phút 14 giây, 9/9 mà không ai `retry`; `auto_retried=true` |
| Thông báo Telegram cho job trên | không có tin "cần xử lý"; đúng một tin "đã xong" |
| Bốn bản copy sau crash | mỗi bản 8 mục, không trùng tên |
| `collections cleanup` (dry-run) | in "Would move "rename-test" … 9 copied items, 84 B" và lệnh chạy thật với `--confirm-name` |
| Cleanup thật 5 bản copy | 5/5 vào thùng rác |

Năm điểm UX ghi nhận trong phiên (tên xác nhận cleanup, dry-run cleanup không báo
số mục, `--wait` không thống nhất, đổi tên thuần tạo bản mới, phải `retry` tay sau
crash) đã được sửa sau phiên; xem `docs/acceptance.md` (dòng "Live 2026-10-28").

Chưa chạy trong phiên này: tray/thông báo bấm tay, Shared Drive,
cây lớn (throttle/AIMD, quota ngày), Windows/macOS, mất mạng/ngủ máy thật.


## Phiên 2026-10-27 (Đợt D — đồng bộ tài liệu với code)

Môi trường: Linux x86_64 (Fedora), Go 1.27.3 (`go1.27.3/go/bin`), không có
Google Drive/Telegram thật. Chỉ sửa tài liệu; **không** đổi code Go, migration
hay `openapi.json`.

| Lệnh | Kết quả |
| --- | --- |
| `gofmt -l cmd internal` | không in gì |
| `go vet ./...` | exit 0 |
| `staticcheck ./...` | exit 0 (không cảnh báo) |
| `go test -race -count=1 -timeout 900s ./...` | 21 package `ok`; tổng ≈104 s (engine 101 s, telegram 60 s, cli 43 s, state 36 s, httpapi 34 s, daemon 32 s) |
| `python3 scripts/check-docs-links.py .` | 483 link tương đối, **0 hỏng** |
| `python3 -c "import json;json.load(open('docs/contracts/openapi.json'))"` | OK |
| `git diff --check` | sạch (không khoảng trắng thừa) |

Package đã chạy: app, autostart, cli, config, daemon, drive, engine, filter,
httpapi, i18n, live, maintenance, mascot, power, present, schedule, state,
telegram, tray, tui, update.

**Chưa chạy trong phiên này:** Google Drive/Telegram thật, tray trên desktop
thật, Windows 10/11 và installer, Docker image, cross-build (thay đổi chỉ ở tài
liệu). Các số đo hiệu năng bên dưới là của các phiên trước, giữ nguyên làm mốc.

## Phiên 2026-10-26

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

## Đợt A2 (S2 → S3 → F1 → Q1)

Fake provider/DB, không phải Google thật. Lệnh: xem báo cáo
`docs/handoff/BATCH-A2-REPORT.md` (dán nguyên văn).

### S2 — replay + full reconcile

- Thay đổi trong baseline: lưu `sync_pending_changes`, cursor đứng yên; baseline
  xong replay một transaction (xoá buffer + tiến cursor + admit run). Test
  `TestPollStoresChangesDuringBaselineAndReplaysAfterIt`.
- Crash matrix 12 seed (chết trước poll, sau khi lưu trang, sau baseline): restart
  vẫn kết thúc ở đầu feed, đúng một run, không lặp trang.
- Trần 5000 dòng / 4 MiB: append dừng đúng trần, `replay_overflow` chuyển
  subscription sang `needs_reconcile` (không phát lại tập thiếu).
- Full reconcile: slot đầu và slot kế trong ±10% của 7 ngày; tick thứ hai không
  tạo run; `0` tắt.
- Ngưỡng 30%: 9/20 file mất → job `partial`, `source_removed=9`, 0 lệnh ghi
  Drive, 20 bản đích còn nguyên, audit `reconcile_watch` được tạo; 5/20 → sync
  thường.
- Migration 006→007 trên DB có dữ liệu: `user_version≥7`,
  `PRAGMA foreign_key_check` rỗng.


### S3 — AIMD, token bucket, phân loại lỗi

- `drive.Throttle`: 2 → 3 → 4 sau 2/4 success, không vượt 4; 429 → 2 → 1, sàn
  1; lỗi mạng/5xx/quota/quyền không đổi limit; Acquire chờ đúng 1 slot và huỷ
  theo ctx; bucket 20 req/s trễ ≥20 ms ở call thứ 21; cửa sổ 5 phút + `next_retry_at`.
- Engine: 3 lần 429 liên tiếp trên copy → item `done`, `attempts=1`, 4 call
  copy, throttle đếm 3; provider paced làm limit 1 → ≥2.
- `/v1/status` có `throttle` (6, 3, `2026-10-27T12:00:30Z`) và conformance
  OpenAPI; service chưa nối dây trả 4/0/null.
- 5xx đổi từ `unavailable` sang `server`; test phân loại cũ được cập nhật.
- Chưa đo trên Google thật: 429 thật, `Retry-After` thật, tài khoản nhiều job.


### F1 — fast-list

- Cây 45 folder nhỏ (1 file/folder), fake pageSize 1000:
  **trước** 46 lệnh `files.list` (1 gốc + 45 folder), **sau** 3 lệnh (1 list gốc
  + 2 batch 40/5) → **15,3×**; cây đích đủ 45 folder đúng tên. Test
  `TestBatchListCutsFileListCalls`.
- Fallback khi query gộp bị 400 → list từng folder, job vẫn xong; thứ tự cha
  trước con được kiểm bằng hook `folder`; sync không đổi chỉ tốn ≤3 lệnh list và
  0 ghi.
- 10k file / 1k folder (pageSize 1000, riêng tiến trình, `FASTLIST_MODE`):

| Chỉ số | Trước F1 (`before`) | Sau F1 (`after`) | Tỉ lệ |
| --- | --- | --- | --- |
| `files.list` | **1001** | **26** | **38,5×** ít hơn |
| Thời gian discovery | **4,67 s** | **2,35 s** | **1,99×** nhanh hơn |
| Heap sau khi copy | 21 MiB | 21 MiB | — |
| RSS đỉnh (`VmHWM`) | 71 440 kB | 71 924 kB | — |
| Kết quả | 11 001 node, succeeded | 11 001 node, succeeded | — |

  Lệnh: `FASTLIST_MODE=before|after go test -count=1 -run TestFastListTenThousand
  -timeout 900s -v ./internal/engine`. Cùng một tiến trình riêng cho mỗi mode nên
  RSS đỉnh không bị cộng dồn; cây giả dựng lại mỗi lần.
- `EXPLAIN QUERY PLAN` cho truy vấn lặp mới:

```
pending-batch:  SEARCH sync_pending_changes USING INDEX sqlite_autoindex_sync_pending_changes_1 (collection_id=?)
pending-last:   SEARCH sync_pending_changes USING INDEX sqlite_autoindex_sync_pending_changes_1 (collection_id=?)
pending-collections: SCAN sync_pending_changes USING COVERING INDEX sqlite_autoindex_sync_pending_changes_1
due-polls:      SEARCH sync_subscriptions USING INDEX sync_due_poll (mode=? AND state=? AND next_poll_at<?) | USE TEMP B-TREE FOR ORDER BY
full-reconcile: SEARCH sync_subscriptions USING INDEX sync_due_full (mode=? AND state=?)
next-batch:     MULTI-INDEX OR | SEARCH items USING INDEX items_job_list (job_id=? AND kind=? AND list_state=?) | SEARCH items USING INDEX items_scan_list (scan_job=? AND kind=? AND list_state=?) | USE TEMP B-TREE FOR ORDER BY
next-partial:   MULTI-INDEX OR | INDEX 1 | SEARCH items USING INDEX items_job_list (job_id=? AND kind=? AND list_state=?) | INDEX 2 | SEARCH items USING INDEX items_scan_list (scan_job=? AND kind=? AND list_state=?) | USE TEMP B-TREE FOR ORDER BY
item-totals:    SEARCH items USING INDEX items_parent (collection_id=?)
copied-24h:     SEARCH copy_ledger USING INDEX sqlite_autoindex_copy_ledger_1 (principal=? AND hour>?)
```

- Số câu SQL mỗi tick syncer (đếm tĩnh từ code): **5** khi rảnh
  (`ClaimDueSchedules`, `ClaimDueFullReconciles`, `PendingCollections`, `DuePolls`,
  `NeedsReconcileSubs`). Một poll realtime yên tĩnh thêm `LastPendingCursor` +
  transaction `AdvancePoll` (`getSub`, đọc collection, `putSub`) ⇒ **~9**; poll
  bootstrapping thêm `getSub` + N insert + `putSub`, **không `COUNT(*)`** (trần
  buffer đọc từ cột `replay_count/replay_bytes`).
- Không nạp cả cây: batch tối đa 40 cha, trần 20 000 file/batch rồi fallback.


### Q1 — quota và sổ byte

- `dailyLimitExceeded` → job `blocked`, `resume_after` = 2026-10-28T07:00:00Z
  (midnight PDT), 0 file trong đích, 2 item pending `attempts=0`; nâng đồng hồ
  qua mốc → tự chạy lại **cùng id** và succeeded.
- `storageQuotaExceeded` → `now+24h`, đúng 1 call copy, không attempt nào bị đốt.
- Ledger: 10+20+5 = **35 byte**; delta no-op vẫn 35; `now+25h` = 0.
- Migration 007→008 (áp dụng luôn 009): `user_version≥8`, `foreign_key_check` rỗng, ledger dùng được.
- Chưa chạy: quota/reset thật, Shared Drive hết dung lượng thật.


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

## Đợt C1 (K1 · D1 · N1 · S5)

Đo trên cây giả 10 000 file / 1 000 thư mục (pageSize 1000, mỗi mode một tiến trình riêng, fake provider, không phải Drive thật):

| Chỉ số | F1 (`after`, trước C1) | C1 `after` (không lọc) | C1 `filter` (loại 2/10 file) |
| --- | --- | --- | --- |
| `files.list` | 26 | **26** | **26** |
| Thời gian khám phá | 2,35 s | 2,41 s | 3,16 s |
| Heap sau copy | 21 MiB | 21 MiB | 19 MiB |
| RSS đỉnh (`VmHWM`) | 71 924 kB | 71 792 kB | 69 104 kB |
| Kết quả | succeeded | succeeded | succeeded, `filtered=2000` |

Lệnh: `FASTLIST_MODE=after|filter go test -count=1 -run TestFastListTenThousand -timeout 900s -v ./internal/engine`.
Lọc không thêm lệnh `files.list`. Thời gian khám phá tăng khoảng 30% ở chế độ lọc (một lần đo, chưa lặp nhiều lần).
Chưa đo tổng thời gian job (có bước giữ tên DB cho mỗi mục copy).

`EXPLAIN QUERY PLAN` cho truy vấn mới (nguyên văn từ `go test -v ./internal/state`):

```
name lookup (giữ tên N1):
SEARCH items USING INDEX items_dest_name (dest_parent_id=? AND dest_name=?)
prune (thư mục rỗng sau lọc S5):
SEARCH items USING INDEX items_collection_kind_state (collection_id=? AND kind=? AND state=?)
CORRELATED SCALAR SUBQUERY 1
SEARCH c USING INDEX items_parent (collection_id=? AND parent_source_id=?)
```
## Đợt B (T3, T5, S11, R1) — đo S11 nhàn rỗi

- Binary build từ nhánh `batch-b`, daemon chạy `serve --maintenance` với data dir tạm
  (dữ liệu tổng hợp, không có Drive/token thật), đo 30 s sau khi lắng nghe:
  - CPU: 0 tick (0 ms) trong 30 s nhàn rỗi (đọc `/proc/<pid>/stat`).
  - RSS: 26872 kB (≈ 26.9 MB), 11 thread; không tăng giữa hai lần đo.
- **Chưa đo**: copy 10k file (cần Drive thật), `scripts/soak.sh` (chỉ kiểm `bash -n`).
- `systemd-analyze verify packaging/502drive.service`: không lỗi cú pháp; chỉ cảnh báo
  `/usr/bin/502drive` chưa được cài.

## Đợt C2 (S6 → S7 → T4)

Lệnh kiểm chứng và kết quả: xem `docs/handoff/BATCH-C2-REPORT.md` (dán nguyên văn).

Số đo (hàng đợi): không đo thời gian trên bảng lớn trong đợt này. Chứng cứ đã có: `EXPLAIN QUERY PLAN` của truy vấn
chọn job (`TestQueueClaimUsesIndexes`) dùng `jobs_queue` và `jobs_fair`, không `SCAN jobs`. Đo thực tế với 10 000
job chờ và tốc độ `ClaimQueued` là việc còn lại.

Số đo (tray): **chưa đo CPU nhàn rỗi** — không có desktop thật trong môi trường đợt này. Phép đo đề xuất:
`/proc/<pid>/stat` của `502drive-tray` trong 60 s nhàn rỗi, trước và sau đợt T4.

Số đo (preflight): không đo; quét dùng BFS theo trang và chỉ giữ id thư mục, không nạp cây vào RAM. Chưa đo số
lệnh API trên cây lớn.
