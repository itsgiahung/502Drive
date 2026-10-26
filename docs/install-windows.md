# Cài trên Windows

Artifact: `502drive_<ver>_windows_amd64.zip` (`502drive.exe` console +
`502drive-tray.exe` GUI) và `502drive-setup.exe` (Inno Setup) kèm `.sha256`.
**Installer chưa ký code** — SmartScreen sẽ cảnh báo "unknown publisher"; hãy
kiểm SHA256 trước khi chạy. Chưa acceptance trên Windows 10/11 trong phiên này.

## Cài

- **Installer:** chạy `502drive-setup.exe`; chọn "Start 502Drive in the
  notification area when I sign in" nếu muốn autostart (mặc định tắt). Shortcut
  Start Menu: tray + menu. Gỡ cài **không** xóa config/token/data người dùng.
- **Một dòng (PowerShell):** `irm https://raw.githubusercontent.com/itsgiahung/502Drive/main/scripts/install.ps1 | iex`
  — kiểm checksum, cài vào `%LOCALAPPDATA%\Programs\502Drive`, thêm vào PATH, tạo
  alias `502`.
- **Installer** cũng có tùy chọn thêm vào PATH (mặc định bật) và cài `502.cmd`. Chưa
  chạy ISCC thật nên phần này chưa được kiểm chứng.
- **Zip:** giải nén, chạy `502drive.exe setup` trong terminal (config/token file
  private), sau đó `502drive.exe serve` hoặc `502drive-tray.exe`.

Xác minh: `Get-FileHash .\502drive-setup.exe -Algorithm SHA256` so với file
`.sha256` trong release.

## Cấu hình

- `502drive setup` tạo config JSON (`%AppData%\502drive-next\config.json`) và
  API token 0600-tương đương (ACL user). Data dir mặc định cạnh config
  (`data-v2\state-v2.db`, `secrets\`).
- `502drive setup --oauth-client client_secret.json`,
  `--telegram-token-file bot-token --telegram-owner-id N`.
- `502drive account login` mở browser trên máy này; nếu daemon ở máy khác, dùng
  `ssh -L <port>:127.0.0.1:<port>` như hướng dẫn CLI in ra.

## Chạy nền

- Cách 1: chạy `502drive-tray.exe`; mục **Start daemon** của tray spawn
  `502drive.exe serve` ẩn console.
- Cách 2: `502drive.exe serve` trong một terminal (đóng terminal = dừng daemon),
  hoặc tạo Task Scheduler "At log on" trỏ tới `502drive.exe serve`.
- **Chưa hỗ trợ**: Windows service (`sc create`/`nssm`) và auto-update. Tray và
  daemon là hai process riêng; đóng tray không dừng daemon.

## Tray

- Icon 5 trạng thái; menu: Open TUI (mở `wt.exe`/`cmd` chạy `502drive menu`),
  Copy link trong clipboard (`Get-Clipboard`), Jobs & thư viện, mở folder kết quả
  gần nhất, Start/Stop daemon, Quit.
- `Stop daemon` dùng `taskkill /IM 502drive.exe /F` (chỉ daemon console, không
  phải tray).

## Gỡ

- Installer: Settings → Apps → 502Drive → Uninstall (chỉ xóa thư mục cài + shortcut).
- Zip: xóa thư mục giải nén. Muốn xóa dữ liệu: xóa tay config/data dir (không có
  lệnh purge).
