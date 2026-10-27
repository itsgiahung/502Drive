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
502drive LINK [LINK...] [flags]             # dạng trần: mỗi link = copy (một job/link)
502drive copy [SOURCE...|--stdin] [--to ID] [--name NAME|--auto-name] [--wait] [--idempotency-key K] [--shortcuts follow|skip] [--on-conflict skip|rename|update] [FILTER] [--update|--new] [--sync off|realtime|hourly|six_hourly|daily]
502drive pause|resume                        # S6: pause/resume the whole job queue (owner)
502drive jobs move ID [--before ID|--after ID] [--priority N]   # S6: reorder a queued job (-5..5, higher first)
502drive ls [FOLDER] [--all]                 # FOLDER = link/ID/root (mặc định root)
502drive info LINK|ID
502drive mkdir NAME [--in PARENT] [--idempotency-key K]
502drive jobs list|get|wait|events|pause|resume|cancel|retry
502drive jobs report ID [--format table|json|csv] [--max-rows N]   # X1: mục failed/blocked/skipped (mã K1, lý do, đường dẫn, số lần thử)
502drive history export [--since 30d] [--format csv|json|table] [--all] [--max-rows N]   # X1: lịch sử job, mới cập nhật trước
502drive wait ID                              # dạng ngắn của jobs wait
502drive watch [ID] [--interval 1s]           # một dòng/giây; --json = một envelope/giây
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
502drive doctor [--online] [--fix]             # --fix: quyền file bí mật, thư mục thiếu
502drive                       # trong TTY: tạo config lần đầu, bật daemon + tray (tách khỏi terminal), mở TUI
502drive up                    # như trên nhưng không mở UI (dùng cho autostart)
502drive stop [--yes]          # dừng daemon graceful qua HTTP; còn job thì hỏi (ngoài TTY cần --yes)
502drive audit COLLECTION [--repair [--apply|--dry-run]] [--json]   # đối chiếu nguồn↔đích; repair mặc định dry-run
502drive audit show AUDIT_ID [--kind K] [--all] [--json]            # xem lại báo cáo + findings
502drive menu                  # chỉ mở TUI, line menu khi plain/dumb/NO_COLOR
502drive logs [-n N] [-f] [--tray]
502drive autostart enable|disable|status
502drive autostart systemd status|install|remove   # unit --user (daemon tự restart sau crash)
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

Tham chiếu job nhận ID đầy đủ, tiền tố duy nhất, `@last` (mới nhất) và
`@running` (mới nhất chưa xong). Completion bash/zsh/fish gợi ý ID job/collection
thật. `502drive LINK...` tương đương `copy`.

Exit code: `0` ok, `1` lỗi nội bộ/provider, `2` input/config, `3` auth/reconnect,
`4` not found, `5` conflict/stale, `6` daemon/network/wait timeout, `7` wait kết
thúc partial/failed/blocked, `8` wait cancelled, `130` Ctrl-C. `copy --wait` chỉ
exit 0 khi `succeeded`.

### Báo cáo & export (X1)

- `jobs report ID` đọc `GET /v1/jobs/{id}/items?problems=true` theo trang 100, ghép tối đa
  `--max-rows` (mặc định 5000). Cột bảng: trạng thái, mã, lý do (`present.Reason`), đường
  dẫn, số lần thử; CSV thêm `source_id`/`destination_id`. `--format json` và `--json` cùng
  in một envelope `{"data":{"job","items","truncated"},"request_id"}`; `--json` + `--format csv`
  là lỗi dùng (exit 2). Cắt dòng ghi chú ở stderr, exit vẫn 0.
- `history export` đọc `GET /v1/history?since=…&limit=100&cursor=…` (sắp theo `updated_at`,
  nên job cũ chạy lại vẫn nằm trong cửa sổ), mặc định `--format csv`, `--since 30d`
  (nhận `30d`, `12h`, `2w`, RFC3339 hoặc `YYYY-MM-DD`). CSV: `job_id,state,kind,
  collection_id,source_name,root_name,created_at,updated_at,total_items,completed_items,
  failed_items,blocked_items,skipped_items`. Trần 5000 job, cắt thì ghi chú stderr.
- CSV dùng `encoding/csv` (quote phẩy/nháy/xuống dòng); ô bắt đầu bằng `=`, `+`, `-`,
  `@`, TAB hoặc CR được thêm `'` để spreadsheet không chạy công thức.

`--json`, `--quiet`, `--plain`, `TERM=dumb` tắt màu/mascot/cursor; pipe mặc định
plain. Tên Drive qua `Sanitize` (ESC/control/bidi). Lệnh nghiệp vụ in tiếng Anh
(theo contract); **menu/TUI/tray** dùng vi/en từ `Preferences.Language`.
Giá trị trong output người đọc theo locale/múi giờ của preferences: ngày giờ qua
`i18n.FormatDate/FormatTime`, dung lượng qua `present.HumanBytes`, lý do mã K1 qua
`present.Reason`. JSON luôn giữ RFC3339 gốc; lệnh chỉ đọc preferences khi thật sự
in ngày giờ (không thêm request cho JSON/`status`).

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

### Library, Jobs, hộp copy và Settings (Đợt T)

Mọi phím dưới đây chỉ gọi API có sẵn của daemon (TUI không ghi DB, không mở engine riêng); thiếu action
nào thì dòng/phím đó không hiện thay vì hiện mà chết. Chuỗi hiển thị vi/en nằm ở
[tui_ext.go](../../internal/i18n/tui_ext.go); nhãn trạng thái, thời lượng, dung lượng dùng
[internal/present](../../internal/present).

**Vòng đời máy (T1).** `GET /v1/status.lifecycle` hiện thành một dòng trên Home, Jobs, Library
("Đang ngủ · việc đang chạy sẽ tiếp tục khi máy dậy", "Đang kết nối lại… (sau 1g00p ngủ)" khi ngủ ≥ 60 giây,
"Cần đăng nhập lại Google · nhấn l") và một dấu ngắn `zZ`/`~` trên thanh tab. `l` mở đăng nhập ở mọi tab có dòng
này. Daemon cũ không báo `lifecycle` thì không có dòng nào.

**Jobs.** Job `blocked` có `resume_after` ở tương lai hiện "Chờ quota tới HH:MM" (múi giờ trong Settings) thay cho
thanh tiến độ và **không** được gửi `retry` (daemon tự tiếp tục; `p` và `c` vẫn dùng được). Chi tiết job liệt kê số mục
bỏ qua theo mã K1 (`filtered`, `conflict`, `shortcut_unresolved`, `shortcut_loop`, `cannot_copy`,
`google_native_export_only`, `too_large_for_quota`, `unreadable`, `trashed_source`; mã lạ hiện nguyên văn), loại job
(`audit`/`repair`/`cleanup`) và dung lượng.

**Library.** `H` hiện thêm nguồn đã ngừng theo dõi (đọc `GET /v1/collections?include_retired=true`, chỉ của người dùng này). Màn một nguồn (`enter`):

| Phím | Tác dụng | API |
| --- | --- | --- |
| `a` | quét toàn vẹn (chỉ đọc), mỗi lần một lượt | `POST /v1/collections/{id}/audits`, rồi đọc `GET /v1/audits/{id}` theo mỗi lần làm mới |
| `v` | bật/tắt danh sách sai lệch (6 dòng đầu) | `GET /v1/audits/{id}/findings` (đọc tối đa 500) |
| `f` / `F` | lập kế hoạch sửa (dry-run, không đổi gì) / sửa thật (phải có kế hoạch trước, hỏi `y/N`) | `POST /v1/audits/{id}/repair` với `expected_revision` của báo cáo |
| `b` | đổi bộ lọc: không lọc → bỏ video → chỉ tài liệu → bỏ file > 1 GB | `PUT /v1/collections/{id}/filter` (áp dụng cho lần chạy sau) |
| `o` | ngừng theo dõi, hỏi trước và nói rõ cái gì được giữ | `POST …/retire` |
| `u` | theo dõi lại (nguồn đã ngừng) hoặc tiếp tục sync đang tạm dừng | `POST …/resume` / sync resume |
| `w` | quên bản ghi cục bộ (chỉ khi đã ngừng theo dõi), hỏi `y/N` | `POST …/forget` |
| `c` rồi `C` | dọn đích: `c` chạy dry-run và hiện "N mục · X sẽ vào thùng rác Drive"; `C` bắt gõ lại đúng tên thư mục rồi mới gửi | `POST …/cleanup` (`dry_run` rồi `confirm_name`) |

Mọi lệnh gửi `expected_revision` mà người dùng đang thấy; lỗi của daemon (ví dụ `collection_busy`) hiện ở dòng trạng thái.

**Hộp copy.** Thêm `f` (lọc), `o` (xử lý trùng tên: đổi tên → bỏ qua → cập nhật), `k` (shortcut: đi theo/bỏ qua);
mỗi lần đổi, bản xem trước chạy lại với `filter`, `on_conflict`, `shortcuts` nên con số hiển thị luôn khớp với
cái sẽ chạy: "Đạt: N · Bị loại: M (chỉ tính cấp cao nhất)" kèm tối đa 3 mẫu bị loại. Khi daemon trả `409
already_copied` hộp hỏi "`u` cập nhật bản cũ · `n` tạo bản mới · `esc` hủy" rồi gửi lại `mode=update|new`
(trước đây TUI gửi không có `mode` và chỉ hiện lỗi).

**Settings.** Con trỏ đi qua các dòng (↑ ở dòng đầu lên thanh tab): ngôn ngữ (English → Tiếng Việt → Tự động theo
locale máy), tự chạy khi đăng nhập (T3, công tắc của máy này, không phải của daemon), giờ yên tĩnh (gõ
`22:00-07:00`, để trống = tắt), giữ lịch sử (7/30/90/180/365 ngày hoặc mãi mãi), ba công tắc thông báo. Mỗi thay đổi hiện
ngay và lưu qua `PATCH /v1/preferences`; lưu lỗi thì hiện lỗi và lần làm mới sau trả về giá trị thật.

Test: [batch_t_test.go](../../internal/tui/batch_t_test.go), [tui_ext_test.go](../../internal/cli/tui_ext_test.go).

### Lô, hàng đợi, quét sâu, báo cáo, tìm Drive (Đợt IT)

Cũng chỉ gọi API có sẵn (không ghi DB); chuỗi vi/en ở [tui_it.go](../../internal/i18n/tui_it.go), adapter ở
[tui_it.go](../../internal/cli/tui_it.go). Thiếu action nào thì phím/dòng đó không hiện.

**Hộp copy.**

| Phím / thao tác | Tác dụng | API |
| --- | --- | --- |
| dán nhiều link (mỗi dòng một link) | một **lô**: không xem trước từng link, `1/2` tên, `d` đích, `f/o/k` tùy chọn áp cho cả lô; link lỗi báo riêng | `POST /v1/plans` + `POST /v1/jobs` với `batch_id` cho từng link (khóa idempotency riêng, mỗi lần Enter là một lô mới) |
| `i` / `x` | bật/tắt quét sâu cùng bản xem trước / hủy quét đang chạy | `POST /v1/plans` với `deep`, `GET /v1/plans/{id}` mỗi ~0,7 giây, `POST /v1/plans/{id}/preflight/cancel` |
| `s` | chọn đồng bộ sau copy: tắt / realtime / mỗi giờ / 6 giờ / hằng ngày (bắt đầu ở tùy chọn đã lưu) | `after_copy_sync_mode` trong `POST /v1/jobs`; chỉ gửi khi khác tùy chọn đã lưu |
| gõ chữ không phải link/ID | tìm trong Drive theo tên (tối đa 20) | `GET /v1/drive/search?q=` |

Quét sâu: số đang quét ghi "Chưa đủ để là tổng"; xong mới hiện tổng, Google/shortcut/không copy được/thư mục không đọc
được, ước tính (từ các lần copy đã đo) và cảnh báo `estimate_unknown`, `destination_storage_short`,
`may_hit_daily_limit`. Đích mà tài khoản không ghi được (`destination_not_writable`) có câu riêng và chặn Enter tới khi
chọn lại.

**Jobs.**

| Phím | Tác dụng | API |
| --- | --- | --- |
| `f` | lọc theo trạng thái (tất cả → queued → running → paused → blocked → failed → succeeded) | `GET /v1/jobs?state=` |
| `s` | chỉ nguồn của dòng đang chọn / bỏ giới hạn | `GET /v1/jobs?collection_id=` |
| cuối danh sách (`↓`, `End`) | tải trang kế | `GET /v1/jobs?cursor=` |
| `K` / `J` | đưa job chờ lên / xuống một chỗ | `POST /v1/jobs/{id}/move` (`before`/`after`, `expected_revision`) |
| `+` / `-` | đổi ưu tiên -5..5 của job chờ | `POST /v1/jobs/{id}/move` (`priority`) |
| `P` / `U` | tạm dừng (hỏi `y/N`) / tiếp tục cả hàng đợi | `POST /v1/queue/pause`, `POST /v1/queue/resume` |
| `E` | xuất lịch sử 30 ngày ra CSV (hỏi đường dẫn) | `GET /v1/history` |

Rời tab Jobs thì bộ lọc được bỏ. Job trong một lô hiện "lô abc123 3/10" (đọc `GET /v1/batches/{id}`, mỗi lô một lần).
Thứ tự "chờ #N" là ước lượng: ưu tiên → thời điểm nhận → các lần đổi chỗ đã làm trong phiên này (API chưa trả vị trí).
`status.queue.paused` hiện dòng "Hàng đợi đang tạm dừng" trên Jobs/Home và `||` trên thanh tab.

**Chi tiết job.** `R` mở báo cáo lỗi/chặn/bỏ qua theo mã K1 (`GET /v1/jobs/{id}/items?problems=true`, đọc tối đa 500
dòng, báo "Đã cắt ở N dòng"); `E` xuất báo cáo ra CSV (tối đa 5000 dòng, như `jobs report --format csv`) tới đường dẫn gõ vào.
Tệp mới 0600, ô bắt đầu bằng `= + - @` được chặn công thức, **không ghi đè** tệp đã có và không tạo tệp khi đọc lỗi.

**Drive.** `/` tìm theo tên; kết quả dùng chung phím với thư mục (`enter` mở thư mục, `c` copy, `i` thông tin);
`esc`/`←` về thư mục đang đứng.

**Library.** Mỗi dòng đang thấy hiện audit gần nhất ("kiểm tra: sạch" / "N sai lệch (M sửa được)", đọc
`GET /v1/collections/{id}/audits/latest`, tối đa một lần mỗi phút mỗi nguồn); màn nguồn bắt đầu từ audit đó nên `f`
lập kế hoạch sửa được ngay.

**Settings.** Dòng cuối "Đồng bộ sau copy" (tắt/realtime/mỗi giờ/6 giờ/hằng ngày) lưu ngay `after_copy_sync_mode`.

Test: [batch_it_test.go](../../internal/tui/batch_it_test.go), [batch_it2_test.go](../../internal/tui/batch_it2_test.go),
[tui_it_test.go](../../internal/cli/tui_it_test.go).

## Tray (`502drive tray`)

- Chỉ client HTTP; menu: trạng thái, Open TUI, Copy link trong clipboard, Jobs &
  thư viện, mở folder kết quả gần nhất, Start/Stop daemon, **Quit (dừng cả daemon)**.
- Một tray một lúc (khóa `tray.lock`), tooltip hiện `Đang copy 312 / 1284 mục` từ
  counts thật, thông báo desktop khi job xong/lỗi (Linux `notify-send`).
- Poll 3s khi busy, 10s khi idle, backoff khi daemon tắt; icon 5 trạng thái vẽ
  bằng code, animation busy ≤4fps, tắt bằng `--no-motion`.
- Linux không có `DBUS_SESSION_BUS_ADDRESS` → exit 1 kèm hướng dẫn dùng `menu`.
- `--write-icons DIR` ghi `502drive.png`/`502drive.ico` (build helper).

### systemd `--user`

`502drive autostart systemd install` ghi `~/.config/systemd/user/502drive.service`
(hoặc dưới `XDG_CONFIG_HOME`), `daemon-reload` + `enable` — không tự `start`; unit chạy
`serve`, `Restart=on-failure`, giới hạn 5 lần/5 phút, hardening như unit đóng gói.
`status` in đường dẫn và enabled/disabled; `remove` disable rồi xóa. `doctor` báo
`systemd_user` failed nếu unit có nhưng chưa enable; không có unit thì passed
(tray/XDG vẫn khởi động daemon).

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

## First-run guide (U1)

`502drive onboard` guides a TTY through OAuth Desktop JSON (regular file, Unix
0600), detached daemon startup, Google login and a default destination. Existing
`setup` flags retain their provisioning behavior. Without a TTY, with `--json`
or `--no-input`, onboard never reads input or starts processes: it emits a JSON
envelope with `ready` and ordered `checks`, exit **9** when incomplete. An older
daemon falls back to Doctor. Unknown readiness is reported as unknown.

U3: human job errors and reports use `present.Explain` (title, explanation,
next action). Item JSON and failure JSON add `explanation`; codes and existing
fields remain stable. Unknown codes retain the raw code with generic advice.

U4: watch/wait use shared recent progress (completed/total, copied bytes, sampled
rate). No rate before three observations; no ETA for unknown/unstable totals.
JSON counters and exit codes retain their existing meaning.

U7: `jobs list --state queued,blocked,failed` accepts a comma-separated set.
Invalid/empty members return HTTP 400 `invalid_request`; missing `state` means all.
TUI keeps **f** for single-state cycling; **F**, Space, Enter selects several.
