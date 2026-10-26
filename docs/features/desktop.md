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
- Thiết kế/giới hạn: [design/t1-power-lifecycle.md](../design/t1-power-lifecycle.md);
  kiểm tay: [runbooks](../runbooks.md#kiểm-tra-laptop-thật).

## Tray

`502drive tray` là một process riêng, không cửa sổ chính, chỉ gọi HTTP API.

| Mục menu | Hành vi |
| --- | --- |
| dòng trạng thái | `Google: … · N chạy · N chờ · N cần xử lý` (tooltip theo trạng thái) |
| Open TUI | mở terminal chạy `502drive menu` (argv cố định) |
| Copy link trong clipboard | đọc clipboard (`wl-paste`/`xclip`/`xsel`, PowerShell `Get-Clipboard`), đúng **một** link qua `drive.Parse`, rồi submit copy |
| Jobs & thư viện | 3 job gần nhất (state · tên · tiến độ) + Open TUI |
| Open latest result folder | mở URL kết quả (chỉ https, host `drive.google.com`) |
| Start/Stop daemon | Start: spawn `502drive serve` tách khỏi terminal (Unix `setsid`, Windows `DETACHED_PROCESS`), log ở `<config>/logs/daemon.log`; Stop: `POST /v1/shutdown` |
| Quit | dừng daemon (`POST /v1/shutdown`, job đang chạy tiếp tục lần sau) rồi thoát tray |

- Icon vẽ bằng `image` stdlib, 5 trạng thái (idle/busy/needs/error/offline), PNG
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

- Notification desktop từ tray (chỉ tooltip/status).
- Autostart mặc định (desktop file kèm hướng dẫn copy vào `~/.config/autostart`).
- macOS tray (systray cần CGO; CLI/daemon vẫn chạy).
- Ký code, auto-update, packaging AppImage/flatpak/snap.
- Acceptance thật trên KDE/GNOME/Windows 10/11 chưa chạy trong phiên này.
