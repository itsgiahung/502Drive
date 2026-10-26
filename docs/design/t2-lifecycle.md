# T2 — Lifecycle & single instance

Thiết kế cho `PHẦN T` mục T2 của đặc tả. Trạng thái khi audit: commit `5f11840` + working tree.

## Audit hiện trạng

| Hạng mục | Hiện có | Thiếu |
| --- | --- | --- |
| 3 hành động | TUI thoát không đụng daemon (`CmdQuit`); tray có toggle Start/Stop daemon (`ActionDaemon`) | Tray **Quit vừa dừng daemon vừa thoát** (`tray.go` `ActionQuit` gọi `StopDaemon`) — sai chính sách; không có "Dừng 502Drive…" riêng, không có xác nhận khi còn job; CLI không có lệnh stop |
| Single instance | `instance.lock` (daemon, [state/store.go](../../internal/state/store.go)), `tray.lock` (tray, [tray/instance.go](../../internal/tray/instance.go), flock chết cùng process); `ensureDaemon` ping `/health/live` trước khi spawn; `ensureTray` kiểm `InstanceRunning` | Chưa có test cho khóa tray và nhánh "đã chạy rồi" của `ensureTray` |
| Chống restart loop | Poll backoff 5→30s; systemd unit `Restart=on-failure`, `RestartSec=5` | Tray không tự giám sát daemon (không có loop spawn) — nhưng unit thiếu `StartLimitBurst/IntervalSec` |
| Windows | Đã dừng qua HTTP ở đường chính (`stopDaemon`) | `ControlDaemon(DaemonStop)` trên Windows vẫn `taskkill /IM 502drive.exe /F` (theo tên, force) — vi phạm "không dùng kill theo tên" |
| Linux không tray | Headless (thiếu `DBUS_SESSION_BUS_ADDRESS`) → exit 1 rõ ràng | Chưa phát hiện thiếu `StatusNotifierWatcher` (GNOME thuần không AppIndicator): icon không bao giờ hiện mà không ai biết vì sao; `doctor` không kiểm |
| TaskbarCreated / re-register | `fyne.io/systray` v1.12.2 xử lý sẵn: `systray_windows.go:334` (`wmTaskbarCreated` → dựng lại icon khi Explorer restart), `systray_unix.go` `stayRegistered()` (theo dõi `NameOwnerChanged` của `org.kde.StatusNotifierWatcher`, tự đăng ký lại khi shell/watcher xuất hiện) | Chỉ cần ghi nhận, không viết thêm |

## Thiết kế

### 1. Ba hành động, một từ vựng (tray + CLI + docs; TUI đã đúng)

| Hành động | Tray | CLI | Daemon |
| --- | --- | --- | --- |
| Đóng TUI | — | thoát `502drive menu` / Ctrl+C | vẫn chạy |
| Thoát khay (`ActionQuit`) | "Quit tray" — chỉ thoát tray | — | vẫn chạy |
| Dừng 502Drive… (`ActionStopDaemon` / `502drive stop`) | mục menu riêng, xác nhận khi còn job | `stop [--yes]`, xác nhận khi còn job | `POST /v1/shutdown` (graceful, 202 rồi dừng sau 200ms) |

- **Quit tray mặc định không đụng daemon.** Opt-in `tray_quit_stops_daemon` (config, mặc định
  tắt) trả lại hành vi cũ cho ai muốn. Đọc ở `cli/tray.go` khi dựng `Options`.
- **Xác nhận khi còn job:**
  - CLI: giống `jobs cancel --all` — TTY hỏi `[y/N]` (đếm `running_jobs + queued_jobs` từ
    `/v1/status`); ngoài TTY bắt buộc `--yes`; `--json` trả envelope.
  - Tray: **xác nhận 2 bước** thay hộp thoại native (không có API dialog thống nhất trên
    SNI/Windows trong `fyne.io/systray`): lần bấm đầu khi `Running+Queued > 0` chỉ *giương*
    mục thành "Xác nhận dừng 502Drive (N việc)?"; lần bấm thứ hai trong cửa sổ 10s mới dừng.
    Bấm khi không còn job thì dừng ngay. Trạng thái giương tự hạ ở poll kế (>10s) — dùng
    `Options.Now` để test được đồng hồ.
- **Dừng có chờ và báo kết quả**: sau `POST /v1/shutdown`, poll `/health/live` mỗi 200ms
  tối đa 6s (`daemonStopWait`). Xong → note "502Drive stopped"/exit 0; chưa dừng → note/
  lỗi `daemon_stop_failed` kèm đường dẫn log. Không kill theo tên, không SIGKILL.
- Menu tray mới (id ổn định để presenter không phải dựng lại):
  `… , sep, daemon(start, chỉ bật khi offline), stop-daemon(only online), sep, quit`.

### 2. Single instance

Đã có khóa; T2 chỉ bổ sung test và thứ tự kiểm trong `ensureTray`:
`trayPossible` → `InstanceRunning(tray.lock)` → (Linux) watcher probe → spawn.
Tray và daemon restart độc lập: cả hai khóa đều flock chết cùng process; tray chết
không ảnh hưởng daemon (chỉ HTTP client) và ngược lại (poll fail → StateOffline).

### 3. Chống restart loop

Quyết định: **tray không tự giám sát/tự hồi sinh daemon** — người dùng bấm "Bật daemon"
hoặc systemd làm việc đó. Lý do: hồi sinh tự động từ tray tạo đúng loop mà đặc tả muốn
tránh, trong khi unit đã có `Restart=on-failure`. Bổ sung `StartLimitIntervalSec=300`,
`StartLimitBurst=5` vào unit (tắt daemon sau 5 lần crash/5 phút; người dùng thấy tray
offline — chính là "trạng thái lỗi với hành động khắc phục"). Poll backoff giữ nguyên.

### 4. Windows

- Bỏ `taskkill`: `ControlDaemon(DaemonStop)` trên Windows trả lỗi hướng dẫn dùng
  `Dừng 502Drive…`/`POST /v1/shutdown`; đường chính trong tray vẫn là HTTP.
- `TaskbarCreated`: đã có trong thư viện (evidence trên) — không code thêm.
- Hồi phục khi session quay lại: poll loop backoff 5→30s tự nối lại khi daemon trả lời
  (đã có); probe mạng/wake là việc T1.

### 5. Linux thiếu StatusNotifierWatcher

- `internal/tray/watcher.go`: `var WatcherProbe func(ctx) (present bool, reason string)`
  + `StatusNotifierWatcherPresent(ctx)` wrapper. `watcher_linux.go` (build tag linux):
  godbus session bus, `GetNameOwner "org.kde.StatusNotifierWatcher"`, timeout 1s;
  lỗi bus/thiếu watcher → `false` + lý do. `watcher_other.go`: `true, ""` (Windows/darwin
  không khái niệm này).
- `ensureTray` (Linux): watcher vắng → **không spawn**, trả `unavailable` + note
  "no StatusNotifierWatcher — the daemon and `502drive menu` still work". Daemon + CLI
  vẫn chạy (chế độ không tray). Chạy tay `502drive tray` vẫn được: thư viện tự
  đăng ký lại khi watcher xuất hiện (`stayRegistered`).
- `doctor` (CLI, client-side vì nói về *desktop session nơi lệnh chạy*, không phải daemon):
  thêm check `tray_host` trên Linux — passed/failed kèm lý do. Hook `WatcherProbe` để test
  được mà không cần bus thật.

## Chế độ lỗi

| Cửa sổ lỗi | Ứng xử |
| --- | --- |
| POST shutdown xong nhưng daemon treo | chờ 6s, báo `daemon_stop_failed` + log path; không force kill |
| Người dùng bấm nhầm "Dừng…" khi còn job | cần bấm xác nhận thứ hai trong 10s; hết hạn tự hạ |
| Hai tray cùng lúc | flock `tray.lock`; nhánh sau thoát 0 với thông báo (giữ nguyên) |
| Tray crash giữa xác nhận | trạng thái giương nằm trong memory của process chết → tự mất |
| Daemon chết giữa poll | backoff, StateOffline, item Start daemon bật lên |
| GNOME không AppIndicator | không spawn tray, note rõ; `doctor` failed; bật lại khi shell có watcher (thư viện tự re-register) |

## Test (viết trước phần cài đặt)

- tray: quit không dừng daemon; opt-in thì dừng; stop-daemon 2 bước (giương → xác nhận →
  dừng), hết hạn cửa sổ, không job thì dừng ngay; cấu trúc menu ổn định với id mới;
  Windows stop không chạy `taskkill`; `PollDelay` giữ nguyên.
- cli: `stop` (TTY hỏi, `--yes`, ngoài TTY không `--yes` là lỗi usage, `--json`, daemon
  đang chết → "not running" exit 0, daemon treo → exit `daemon_stop_failed`); `stopDaemon`
  chờ xuống rồi mới báo; `ensureTray` vắng watcher → không spawn + note; `ensureTray`
  khi khóa đang giữ → "running"; `doctor` có `tray_host`.
- i18n: đủ khóa vi/en mới (`quit` đổi nghĩa → "Thoát khay"; thêm `stop_app`,
  `stop_app_confirm`).
