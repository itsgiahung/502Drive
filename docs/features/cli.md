# CLI, TUI, tray

Source: [internal/cli](../../internal/cli) — [cmd.go](../../internal/cli/cmd.go) (root,
flags), [commands.go](../../internal/cli/commands.go), [sync.go](../../internal/cli/sync.go),
[maintenance.go](../../internal/cli/maintenance.go), [menu.go](../../internal/cli/menu.go),
[tui.go](../../internal/cli/tui.go), [tray.go](../../internal/cli/tray.go),
[render.go](../../internal/cli/render.go), [client.go](../../internal/cli/client.go).
TUI: [internal/tui](../../internal/tui); duyệt Drive: [drive.go](../../internal/cli/drive.go).
Test: [cli_test.go](../../internal/cli/cli_test.go), [drive_test.go](../../internal/cli/drive_test.go),
[sync_test.go](../../internal/cli/sync_test.go), [tray_test.go](../../internal/cli/tray_test.go),
[setup_test.go](../../internal/cli/setup_test.go), [tui_test.go](../../internal/tui/tui_test.go).

## Command tree (thật)

```text
502drive setup [--oauth-client FILE] [--telegram-token-file FILE]
               [--telegram-owner-id N] [--oauth-callback-port N]
               [--env-file PATH] [--no-env]   # keys from .env/env; hỏi trong terminal nếu còn thiếu
502drive serve [--maintenance]
502drive status
502drive inspect SOURCE [--to ID] [--name NAME] [--shortcuts follow|skip] [--on-conflict skip|rename|update] [FILTER] [--deep]
502drive copy [SOURCE...|--stdin] [--to ID] [--name NAME|--auto-name] [--wait] [--idempotency-key K] [--shortcuts follow|skip] [--on-conflict skip|rename|update] [FILTER] [--update|--new]
502drive pause|resume                        # S6: pause/resume the whole job queue (owner)
502drive jobs move ID [--before ID|--after ID] [--priority N]   # S6: reorder a queued job (-5..5, higher first)
502drive ls [FOLDER] [--all]                 # FOLDER = link/ID/root (mặc định root)
502drive info LINK|ID
502drive mkdir NAME [--in PARENT] [--idempotency-key K]
502drive jobs list|get|wait|events|pause|resume|cancel|retry
502drive collections list|get
502drive collections run|pause|audit COLLECTION          # L3: quick actions (resume: retired → back; else sync resume)
502drive collections retire|resume COLLECTION            # L1: stop following / bring back (no Drive change)
502drive collections forget COLLECTION --yes             # L1: delete local records of a retired copy only
502drive collections cleanup COLLECTION [--dry-run=false --yes --confirm-name NAME]   # L1: destination → Drive trash
502drive collections open COLLECTION [--dest]            # L3: print the source (or destination) link
502drive sync enable|status|run|pause|resume|stop COLLECTION
502drive sync filter show COLLECTION
502drive sync filter set COLLECTION [FILTER] [--keep-empty-folders] | --clear
502drive destinations list|add|default
502drive account status|login|revoke
502drive doctor [--online]
502drive                       # trong TTY: tạo config lần đầu, bật daemon + tray (tách khỏi terminal), mở TUI
502drive up                    # như trên nhưng không mở UI (dùng cho autostart)
502drive stop [--yes]          # dừng daemon graceful qua HTTP; còn job thì hỏi (ngoài TTY cần --yes)
502drive audit COLLECTION [--repair [--apply|--dry-run]] [--json]   # đối chiếu nguồn↔đích; repair mặc định dry-run
502drive audit show AUDIT_ID [--kind K] [--all] [--json]            # xem lại báo cáo + findings
502drive menu                  # chỉ mở TUI, line menu khi plain/dumb/NO_COLOR
502drive logs [-n N] [-f] [--tray]
502drive autostart enable|disable|status
502drive update [--enable-check|--disable-check]   # chỉ báo bản mới, không tự cài
502drive tray [--write-icons DIR]
502drive backup PATH | restore PATH [--to DIR] | cleanup [--dry-run|--apply] [--older-than D]
502drive completion bash|zsh|fish|powershell
502drive version
```

Global flags: `--config`, `--endpoint`, `--token-file`, `--json`, `--quiet`,
`--no-input`, `--timeout`, `--ascii`, `--plain`, `--color`, `--no-motion`.
Precedence: flags > `DRIVE502_*` > config > default. CLI **không** mở DB (trừ
`backup/restore/cleanup`, offline, có kiểm tra lock).

## Hợp đồng output

| Chế độ | stdout | stderr |
| --- | --- | --- |
| Mặc định | kết quả, bảng/key-value | progress, cảnh báo, lỗi |
| `--json` | một envelope `{"data","request_id"}` (hoặc NDJSON cho `jobs events`) | một JSON error envelope |
| `--quiet` | chỉ ID/kết quả | lỗi |

Exit code: `0` ok, `1` lỗi nội bộ/provider, `2` input/config, `3` auth/reconnect,
`4` not found, `5` conflict/stale, `6` daemon/network/wait timeout, `7` wait kết
thúc partial/failed/blocked, `8` wait cancelled, `130` Ctrl-C. `copy --wait` chỉ
exit 0 khi `succeeded`.

`--json`, `--quiet`, `--plain`, `TERM=dumb` tắt màu/mascot/cursor; pipe mặc định
plain. Tên Drive qua `Sanitize` (ESC/control/bidi). Lệnh nghiệp vụ in tiếng Anh
(theo contract); **menu/TUI/tray** dùng vi/en từ `Preferences.Language`.

## Sync CLI

Bộ lọc (`FILTER`, S5) dùng chung cho `inspect`, `copy` và `sync filter set`:

- `--include GLOB|ext:.pdf|class:doc` và `--exclude ...` (lặp được; exclude luôn thắng include);
  `--ext pdf,docx` là `ext:` vào include; `--min-size 10MB`, `--max-size 1GB`;
- `--filter-file FILE`: mỗi dòng một luật `include|exclude|ext|min-size|max-size|modified-after|keep-empty-folders`,
  dòng bắt đầu bằng `#` là chú thích.
- Lớp MIME: `image|video|audio|doc|archive|native|other`. Glob không có `/` khớp tên; có `/` khớp đường dẫn
  từ gốc, `**` khớp nhiều cấp.

`--on-conflict` (N1) quyết định khi tên đã có ở đích: `rename` (mặc định, thêm ` (2)`), `skip` (không ghi),
`update` (cùng tên, giữ cả hai). Không bao giờ ghi đè.

```text
502drive sync enable ID --daily 22:00 [--timezone IANA] [--policy versioned_copy|replace_copy] [--dry-run]
502drive sync enable ID --weekly mon,wed@07:30 [--timezone IANA]
502drive sync enable ID --every 30m
502drive sync enable ID --mode realtime
502drive sync status|pause|resume|stop|run ID
```

- Timezone mặc định = preference (đổi bằng PATCH `/v1/preferences`); `--timezone`
  explicit thắng.
- `--dry-run` in `Spec.Describe` + **3 lần chạy tới** (giờ theo zone), không lưu.
- Conflict `stale_revision` được retry một lần với revision mới (key mới).

## Lệnh duyệt Drive (`ls`, `info`, `mkdir`)

- `ls [FOLDER]`: một dòng breadcrumb, rồi mỗi mục một dòng — thư mục trước (tên
  kết thúc `/`), sau đó file; trong mỗi nhóm sắp theo tên không phân biệt hoa
  thường; cột `size` (B/KB/MB, thư mục `-`), ngày sửa `YYYY-MM-DD`, tên. `--all`
  đi hết `next_page_token` (dừng ở 200 trang); `--json` in nguyên envelope của
  trang, còn `--json --all` in **một** envelope đã gộp (`next_page_token: null`,
  `path` giữ từ trang đầu). `--quiet` không in gì.
- `info LINK|ID`: `kind`, `size`, `modified`, `created`, `owner`, `can copy`,
  `can add children`, `id`, `link`; `--json` nguyên envelope
  `GET /v1/drive/files/{id}`. Link chia sẻ mang `resourcekey` → gửi
  `resource_key`, không in key.
- `mkdir NAME [--in PARENT]`: `POST /v1/drive/folders` với `Idempotency-Key`
  (cờ riêng hoặc key ngẫu nhiên); in `id` + `link`; `--quiet` chỉ `id`; 409 →
  exit 5. `PARENT` nhận link/ID/`root`; key của folder cha không đi kèm body.
- `copy` nhiều link (S6): đối số lặp hoặc `--stdin` một link mỗi dòng (dòng trống và trùng bị bỏ). Mỗi link là
  một job, chung `batch_id` (`batch-…`, suy ra từ nội dung lệnh, nên chạy lại lệnh không tạo job thứ hai). Lô
  không hỏi; mặc định `--update` (như script). Đầu ra: `Batch … N accepted, M failed` và một dòng cho từng link;
  `--json` trả `{"data":{"batch_id","accepted","failed","items":[…]}}`. Có link lỗi thì exit code là mã của lỗi
  đầu tiên, các link khác vẫn được nhận.
- `inspect --deep` (S7): quét đầy đủ nguồn chỉ đọc, chờ đến khi xong (Ctrl-C chỉ ngừng theo dõi, quét vẫn chạy).
  In số file/thư mục/byte, native, shortcut, không copy được, thư mục không đọc được, thời gian ước tính và cảnh
  báo. Kết quả thiếu `complete` (đang chạy/hủy/hết hạn) không được coi là tổng.
- `pause` / `resume`: tạm dừng hoặc tiếp tục toàn bộ hàng đợi. Job đang chạy chuyển *pausing* rồi *paused* ở điểm an
  toàn, không mất tiến độ; job chờ không được nhận cho đến khi `resume`. Trạng thái bền qua khởi động lại.
- `jobs move ID --before OTHER` / `--after OTHER` / `--priority N`: chỉ với job `queued`; phải kèm đúng `revision`
  (CLI tự lấy).
- `copy --auto-name`: đặt root `<tên nguồn> (copy YYYY-MM-DD)`, lấy tên nguồn
  bằng một plan chỉ đọc trước khi submit (hai plan, một job); không dùng cùng
  `--name` (exit 2). `--dry-run --auto-name` in plan với tên tự đặt.
- Mọi tên qua `Sanitize`; màu chỉ khi TTY.

## TUI (`502drive menu`)

- Chạy khi stdin/stdout TTY, không `--plain`/`TERM=dumb`/`NO_COLOR` và không
  `--json`/`--no-input`. Ngược lại giữ line menu (cùng API). `--no-motion` vẫn
  dùng TUI nhưng mọi thứ đứng yên.
- Màn: Home (mèo hoạt hình + trạng thái + tốc độ/ETA tổng), Jobs (thanh tiến
  trình gradient từ counts thật, tốc độ mục/giây và thời gian còn lại ước từ các
  snapshot liên tiếp), Library & sync, Destination, Account (doctor offline),
  Settings (ngôn ngữ + timezone), Help.
- Chọn nhiều: `space` chọn dòng, `a` chọn hết, `esc` bỏ chọn. `p/u/c/t` (Jobs) và
  `n/p/x/d/h/e/t` (Library) áp dụng cho mục đã chọn (hoặc dòng hiện tại); chỉ gửi
  cho mục hợp lệ (ví dụ resume chỉ job `paused`), còn lại tính là "bỏ qua"; một
  mục lỗi không dừng cả lô; `c` và `x` hỏi `y/N`. Chạy nền, giao diện không bị chặn.
- Danh sách dài được cắt cửa sổ theo chiều cao terminal và view luôn đúng bằng
  chiều cao (đệm dòng trống), nên không còn dòng cũ sót lại hay header bị đẩy ra
  ngoài; cắt theo độ rộng hiển thị (ANSI + chữ rộng như CJK/emoji).
- Mèo (`internal/mascot`): idle/busy/happy/wait/error/sleep, vẽ bằng nửa ô `▀▄`
  truecolor/256 màu; chỉ khi có màu, TTY, đủ cao (≥24 dòng) — nếu không dùng bot
  chữ nhỏ; không có mèo với `--plain`/`NO_COLOR`/`--ascii`.
- Realtime qua `/v1/stream` (làm mới ≤ ~8 lần/giây, gộp sự kiện); poll 8s làm lưới an
  toàn khi stream sống, 2s khi stream mất; các mục ít đổi (đích, tài khoản, cài
  đặt) được cache 30s ở CLI. Mất kết nối giữ dữ liệu cuối + nhãn OFFLINE; resize 40/80/120 cột
  (mỗi dòng bị cắt theo bề rộng); phím `↑↓ jk enter esc q ? 1..7 r` + phím hành
  động; alt-screen được vào/ra kể cả khi lỗi/panic (Bubble Tea + Kill).

### Duyệt Drive trong TUI

`7` mở trình duyệt ở My Drive (chỉ đọc tới khi bấm một phím hành động):

| Phím | Tác dụng |
| --- | --- |
| `↑↓ jk`, `pgup/pgdown`, `g/G` | di chuyển, cuộn trang (tự nạp trang kế) |
| `enter`/`→`/`l` | vào thư mục; file: bật/tắt dòng metadata |
| `←`/`⌫`/`h` | lên thư mục cha (giữ con trỏ) |
| `i` | bật/tắt metadata của mục đang chọn |
| `r` | tải lại thư mục |
| `n` | tạo thư mục mới (chỉ khi `can_add_children`) |
| `d` | đặt thư mục đang chọn (hoặc thư mục đang mở) làm destination mặc định |
| `c` | mở hộp copy cho mục đang chọn |
| `esc`/`q` | về Home |

- Hộp copy: `enter` submit, `1` giữ tên, `2` tên tự đặt `<tên> (copy ngày)`,
  `3` tự nhập, `d` mở trình duyệt chọn đích (`y` xác nhận thư mục đang mở),
  `esc` hủy. Submit tạo job thật; lỗi hiện trong hộp, không đóng.
- Test: [explorer_test.go](../../internal/tui/explorer_test.go);
  JSON request/response: [drive_test.go](../../internal/httpapi/drive_test.go).

## Tray (`502drive tray`)

- Chỉ client HTTP; menu: trạng thái, Open TUI, Copy link trong clipboard, Jobs &
  thư viện, mở folder kết quả gần nhất, Start/Stop daemon, **Quit (dừng cả daemon)**.
- Một tray một lúc (khóa `tray.lock`), tooltip hiện `Đang copy 312 / 1284 mục` từ
  counts thật, thông báo desktop khi job xong/lỗi (Linux `notify-send`).
- Poll 3s khi busy, 10s khi idle, backoff khi daemon tắt; icon 5 trạng thái vẽ
  bằng code, animation busy ≤4fps, tắt bằng `--no-motion`.
- Linux không có `DBUS_SESSION_BUS_ADDRESS` → exit 1 kèm hướng dẫn dùng `menu`.
- `--write-icons DIR` ghi `502drive.png`/`502drive.ico` (build helper).

## Chưa hỗ trợ

- `--verbose`, `watch`, `sync configure` (đổi lịch qua `enable`), pagination cho
  `destinations list`.
- Tray: notification desktop trên Windows, macOS.
- `jobs events` chưa có filter theo kind.

## Setup nhanh (`502drive setup`, lần chạy đầu của `502drive`)

- Nguồn khóa, theo thứ tự ưu tiên: cờ > biến môi trường > `--env-file` hoặc `./.env` >
  `<config>/.env`. Tên hiểu được: `GOOGLE_OAUTH_CLIENT_ID/SECRET`,
  `TELEGRAM_BOT_TOKEN`, `TELEGRAM_OWNER_ID` (và tiền tố `DRIVE502_`, hoặc tên cũ
  `GDCLONE__…` của file `.env` thời Rust).
- Mọi giá trị được **kiểm tra trước** khi tạo bất cứ thứ gì (JSON phải là client *Desktop*,
  token bot đúng dạng `123456:ABC…`, owner id là số dương, id Google kết thúc
  `.apps.googleusercontent.com`), rồi chép vào `<config>/secrets/` với quyền 0600. File
  nguồn (`.env`, JSON) không bị sửa; giá trị không bao giờ được in ra, chỉ in *nguồn*.
- Trong terminal, nếu còn thiếu và không có cờ nào: hỏi lần lượt (Enter để bỏ qua; token
  ẩn khi gõ). `502drive up --quiet` (autostart) không hỏi nhưng vẫn đọc env/`.env`.
- Test: [setup_env_test.go](../../internal/cli/setup_env_test.go).

## Màn Home của TUI (kiểu Claude Code)

- Khung viền: mèo nhỏ (≈9 dòng, `mascot.NewMini`) | lời chào, tài khoản Google, thư mục
  đích mặc định, đếm job, Telegram | gợi ý bắt đầu. Dưới khung là "Hoạt động gần đây"
  (job mới nhất) và **ô lệnh** ghim sát đáy.
- Hai thư mục và các file bay chỉ hiện khi đang có job `running` (dải 4 dòng
  `mascot.Strip`, kèm tốc độ/ETA), không còn chiếm chỗ lúc rảnh.
- Ô lệnh luôn nhận chữ: dán link rồi Enter → hộp thoại copy. Gõ `/` để mở danh sách lệnh
  (`/copy`, `/drive`, `/jobs`, `/library`, `/dest`, `/account`, `/login`, `/settings`,
  `/refresh`, `/help`, `/quit`), ↑↓ chọn, Tab hoàn thành, Enter chạy; `/copy LINK` đi
  thẳng vào bản xem trước. Khi ô trống, các phím `1`–`7`, `c`, `l`, `r`, `q`, `?` vẫn là
  phím tắt; khi đang gõ, `q` chỉ là chữ q. Esc xóa ô trước khi làm gì khác.
- Test: [home_test.go](../../internal/tui/home_test.go).
