# Desktop: tray, TUI, đóng gói

Source: [internal/tray](../../internal/tray), [internal/tui](../../internal/tui),
[internal/cli/tray.go](../../internal/cli/tray.go), [cmd/502drive-tray/main.go](../../cmd/502drive-tray/main.go),
[packaging](../../packaging), [.goreleaser.yaml](../../.goreleaser.yaml),
[go-release.yml](../../.github/workflows/go-release.yml). Test:
[tray/tray_test.go](../../internal/tray/tray_test.go),
[tui/tui_test.go](../../internal/tui/tui_test.go), [cli/tray_test.go](../../internal/cli/tray_test.go).

## Máy ngủ, mất mạng

Daemon theo dõi vòng đời máy: `running → suspending → sleeping → resuming →
running`, cộng `reconnect_required` khi token Google hỏng. Trước khi ngủ nó dừng
nhận việc mới (503 `suspending` cho plan/job), checkpoint WAL (≤2s), ghi
`heartbeat`; sau khi thức nó kiểm DB, đóng idle connection, probe mạng có backoff
rồi mới mở lại hàng đợi. Phát hiện ngủ **không phụ thuộc OS**: lệch đồng hồ
tường/đơn điệu > 30s (logind/Power API là lớp phụ, chỉ thêm độ chính xác).

- Lỗi mạng không làm item `failed` và **không** tiêu attempt; job chờ thay vì
  báo hỏng. Ghi có kết quả không rõ vẫn reconcile bằng intent trước khi gửi lại.
- `GET /v1/status` có `lifecycle {state, online, since, last_gap_seconds,
  reconnect_required}` để tray/TUI/CLI hiển thị "đang kết nối lại…".
- `keep_awake_while_copying: true` (mặc định **tắt**) giữ máy thức khi có job —
  tốn pin/nhiệt; Linux dùng inhibitor `sleep:idle` kiểu block, Windows
  `SetThreadExecutionState`.
- Thiết kế/giới hạn: [design/t1-power-lifecycle.md](../design/t1-power-lifecycle.md) (ngủ/mạng),
  [design/t2-lifecycle.md](../design/t2-lifecycle.md) (3 hành động, single instance); kiểm tay:
  [runbooks](../runbooks.md#kiểm-tra-laptop-thật).

## Tray

`502drive tray` là một process riêng, không cửa sổ chính, chỉ gọi HTTP API.

| Mục menu | Hành vi |
| --- | --- |
| dòng trạng thái | `Google: … · N chạy · N chờ · N cần xử lý` (không bấm); tooltip có tài khoản, số việc và hướng dẫn khi cần |
| Bật 502Drive | chỉ hiện khi daemon tắt: spawn `502drive serve` tách khỏi terminal (Unix `setsid`, Windows `DETACHED_PROCESS`), log ở `<config>/logs/daemon.log` |
| Clone từ clipboard | đọc clipboard **khi bấm** (`wl-paste`/`xclip`/`xsel`, PowerShell `Get-Clipboard`), đúng **một** link qua `drive.Parse`, rồi clone mặc định; mờ kèm lý do khi daemon tắt hoặc chưa đăng nhập Google |
| Mở 502Drive | mở terminal chạy `502drive menu` (argv cố định); double-click icon trên Windows cũng mở |
| Tác vụ & thư viện › | 3 job gần nhất (trạng thái · tên · tiến độ), Mở tất cả, Mở thư mục kết quả gần nhất |
| Tạm dừng tất cả / Tiếp tục tất cả | `POST /v1/queue/pause|resume` (S6); mờ khi daemon tắt |
| Cài đặt › Khởi động cùng hệ thống | bật/tắt autostart, dấu ✓ là trạng thái thật |
| Thoát khay | chỉ thoát khay — **daemon tiếp tục chạy**; opt-in `tray_quit_stops_daemon: true` trả lại hành vi cũ (thoát khay cũng dừng daemon) |
| Dừng 502Drive… | chỉ bật khi daemon đang chạy: `POST /v1/shutdown` (graceful, không kill theo tên), chờ tối đa 6s rồi báo kết quả; khi còn job phải **bấm xác nhận lần hai trong 10s** |

Icon có bảy trạng thái (offline, error, warning, reconnecting, paused, running, idle); chi tiết và ma trận click:
[design/tray.md](../design/tray.md). Icon giữ logo nền đen; trạng thái chỉ thêm một overlay nhỏ.

Ba hành động dùng chung từ vựng ở tray/CLI/docs: *Đóng TUI* (daemon tiếp tục), *Thoát
khay* (daemon tiếp tục), *Dừng 502Drive…* — CLI tương ứng `502drive stop [--yes]`
(hỏi `[y/N]` trong TTY khi còn job, ngoài TTY bắt buộc `--yes`, `--json` giữ nguyên
hợp đồng).

## Một daemon, một tray; chống restart loop

- Khóa instance: daemon giữ `instance.lock`, tray giữ `tray.lock` (flock chết cùng
  process — tray/daemon crash không chặn lần mở sau). `502drive`/`up` ping
  `/health/live` trước khi spawn; tray đang chạy thì không spawn thêm.
- **Không có giám sát hồi sinh từ tray**: daemon tự hồi sinh qua systemd user unit
  (`Restart=on-failure`, `RestartSec=5`, `StartLimitIntervalSec=300`,
  `StartLimitBurst=5` — dừng sau 5 lần crash/5 phút; tray lúc đó hiện offline, người
  dùng bấm Start daemon). Poll tray backoff 5→30s khi daemon tắt.
- Windows: dừng daemon **luôn** qua HTTP (`POST /v1/shutdown`), không còn
  `taskkill`; icon tự dựng lại khi Explorer restart (`TaskbarCreated` do
  `fyne.io/systray` xử lý sẵn).
- Linux thiếu `StatusNotifierWatcher` (GNOME thuần, không AppIndicator): `502drive
  up` **không spawn tray**, ghi note rõ; `502drive doctor` có check `tray_host`;
  chạy tay `502drive tray` vẫn được — thư viện tự đăng ký lại khi watcher xuất hiện
  (shell restart). Daemon + CLI vẫn chạy (chế độ không tray).

- Icon vẽ bằng `image` stdlib, 7 trạng thái (offline, error, warning/needs, reconnecting, paused, running/busy, idle), PNG
  cho Linux, ICO (PNG bên trong) cho Windows; busy tối đa 4fps, `--no-motion` tắt.
- Poll: 3s khi có job chạy, 10s khi idle, backoff 5→30s khi daemon tắt.
- Headless: thiếu `DBUS_SESSION_BUS_ADDRESS` (Linux) hoặc platform không hỗ trợ
  (macOS) → exit 1 + hướng dẫn `502drive menu`.
- Ngôn ngữ menu theo `Preferences.Language` (vi/en).
- Tooltip khi busy: `Đang copy 312 / 1284 mục` (chỉ khi total thật được biết).
- Thông báo desktop (Linux): khi job đã thấy trước đó chuyển sang succeeded/partial/
  failed, chạy `notify-send -a 502Drive -- TITLE BODY` (argv cố định, escape markup,
  không qua shell); lần poll đầu chỉ lập mốc nên không phát lại kết quả cũ. Tắt
  bằng `DRIVE502_NO_NOTIFY=1`. Windows chưa có thông báo.
- Kiểm tra bản mới (opt-in `502drive update --enable-check`): mỗi 24 giờ gọi GitHub
  releases/latest, ghi một dòng vào menu nếu có bản mới; không tải, không cài.
- Lệnh `502drive` (TTY) và `502drive up` bật daemon + tray tách rời; `502drive
  autostart enable` đặt `~/.config/autostart/502drive.desktop` (Linux) hoặc khóa Run
  (Windows) chạy `502drive up --quiet`.

## TUI

Chi tiết phím/màn: [cli](cli.md#tui-502drive-menu). Terminal được khôi phục qua
Bubble Tea kể cả khi panic ([run.go](../../internal/tui/run.go)).

## Đóng gói

| Gói | Nội dung |
| --- | --- |
| `.rpm` / `.deb` (nfpm) | `/usr/bin/502drive`, unit **user** `502drive.service` (không tự enable), `502drive-tray.desktop`, icon 256px, completion bash/zsh/fish |
| Linux/macOS tar.gz, Windows zip | binary `502drive`; zip Windows thêm `502drive-tray.exe` (subsystem GUI) |
| `502drive-setup.exe` | Inno Setup, shortcut Start Menu, autostart tuỳ chọn, uninstall giữ `%APPDATA%`/`%LOCALAPPDATA%` |
| Dockerfile | image 2 tầng distroless/static, user non-root, volume `/data` |

- Icon Windows nhúng bằng go-winres (pin `v0.3.3`) trong hook GoReleaser; `.syso`
  sinh ở `build/`, copy cạnh package lúc build, gitignored
  ([winres.json](../../packaging/windows/winres.json)).
- Installer **chưa ký code**: SmartScreen cảnh báo; release footer ghi rõ.
- Release chỉ publish file đã verify (checksum + smoke linux + rpm + installer);
  test job phải pass trước publish ([go-release.yml](../../.github/workflows/go-release.yml)).

## Chưa hỗ trợ

- Notification desktop từ tray: chỉ có trên Linux (`notify-send`), Windows chưa có.
- Autostart **không tự bật khi cài**: phải chạy `502drive autostart enable`
  (XDG/Run key) hoặc `502drive autostart systemd install` (unit `--user` tự khởi động lại sau crash).
- macOS tray (systray cần CGO; CLI/daemon vẫn chạy).
- Ký code, auto-update, packaging AppImage/flatpak/snap.
- Acceptance thật trên KDE/GNOME/Windows 10/11 chưa chạy trong phiên này.

U1: Home displays the Doctor core readiness checklist while first-run setup is
incomplete. Tray names the first missing prerequisite; Open 502Drive opens Home.
No client exit stops the daemon. Older servers use the existing Doctor checks.

U4: progress uses one shared 30-second observation window, with at least three
samples before displaying a rate. Bytes are copied logical bytes, unknown totals
stay unknown; ETA is explicitly estimated and withheld on stalls/unstable rates.
Tray keeps the detailed shared progress in its tooltip and bounds menu row width.

U5: on a collection, **A** opens the audit findings screen: **t** filters kind,
**n/p** follows next/previous cursors, **f** previews repair, **F** asks before
apply. Apply requires a successful dry-run; its plan is shown. A missing or
failed dry-run does not enable apply. Confirmation states only 502Drive-owned
objects may change. Relative paths come from that collection's mapping; unmapped
extra objects use detail/ID. Existing inline audit (`v`) remains available.

U6: collection **B** edits the full S5 filter. Include/exclude accept path/name
globs, `ext:` and `class:` rules separated by `;`; size fields accept byte counts
or KB/MB/GB/TB. Modified-after and empty-folder settings are preserved. Syntax
errors appear at the active field. **Ctrl+P** previews matching counts through
inspect, explicitly top-level scope; **Ctrl+S** saves by collection revision.
The three presets remain on **b**. Preview creates no copy job.

U8: while SSE is connected, deep preview refreshes with stream-driven snapshots;
the separate 700ms poll is suppressed. Stream loss resumes the existing fallback.
Partial scans explicitly show unknown total; completion updates without a keypress.

U9: Home and tray tooltip show Q1 copied bytes over the last 24 hours and the
actual resume time of a quota-blocked job, in the user's timezone. Unknown Google
limits stay unknown. Old servers without quota retain the existing display.

U12: Linux tray notifies on blocked/quota/reconnect transitions, coalesced by cause
for 60 seconds, with user notification settings and quiet hours. Unknown settings
suppress notifications. A Freedesktop notification action uses fixed `open` and the
captured job ID (wired to `menu --job` in U13). notify-send is the plain fallback.
Windows notification delivery and actual desktop action clicks remain unverified;
the existing Windows backend has no native notification implementation.
