# Cài trên Linux (rpm/deb)

Gói được tạo bởi GoReleaser/nfpm ([.goreleaser.yaml](../.goreleaser.yaml)); nội
dung đã kiểm bằng `rpm -qlp` + `rpm -Kv` + `rpm -i --test` (local, Fedora).
**Chưa** cài/chạy trên desktop thật trong phiên này.

## Gói chứa gì

```text
/usr/bin/502drive
/usr/bin/502d                                 # symlink: gõ `502d` cho nhanh
/usr/lib/systemd/user/502drive.service      # user unit, KHÔNG tự enable
# hoặc cài unit cho riêng người dùng (không cần root):
502drive autostart systemd install          # ~/.config/systemd/user, enable + daemon-reload
/usr/share/applications/502drive-tray.desktop
/usr/share/icons/hicolor/256x256/apps/502drive.png
/usr/share/bash-completion/completions/502drive
/usr/share/zsh/vendor-completions/_502drive
/usr/share/fish/vendor_completions.d/502drive.fish
```

Cài một dòng (không cần sudo, kiểm checksum, cài vào `~/.local/bin`):

```bash
curl -fsSL https://raw.githubusercontent.com/itsgiahung/502Drive/main/scripts/install.sh | sh
```

Hoặc cài gói:

```bash
sudo rpm -i 502drive_<ver>_linux_amd64.rpm      # hoặc: sudo dnf install ./…rpm
sudo dpkg -i 502drive_<ver>_linux_amd64.deb     # Debian/Ubuntu
```

## Thiết lập lần đầu

```bash
502drive setup                                  # ~/.config/502drive/config.json
502drive setup --oauth-client ~/client_secret.json
502drive setup --telegram-token-file ~/bot-token --telegram-owner-id 123456789
systemctl --user start 502drive                 # unit không có [Install] ⇒ không tự bật
502drive account login
502drive doctor --online
502drive destinations add FOLDER_LINK && 502drive destinations default ID
```

Data mặc định nằm cạnh config (`data-v2`); unit cho phép ghi
`%h/.config/502drive` (hoặc `%h/.config/502drive-next` của bản trước 0.3) và `%h/.local/share/502drive`, đọc-only phần còn lại
của home (`ProtectHome=read-only`). Data dir khác cần drop-in thêm
`ReadWritePaths=`.

## Tray và TUI

- Menu ứng dụng: **502Drive** → chạy `502drive tray` (icon khay).
- Cần phiên desktop có StatusNotifier/D-Bus; GNOME cần extension AppIndicator.
  Không có `DBUS_SESSION_BUS_ADDRESS` → tray báo lỗi và gợi ý `502drive menu`.
- Autostart **không** bật mặc định. Chọn một cách:
  `502drive autostart enable` (XDG `.desktop` chạy `502drive up --quiet` lúc đăng
  nhập) hoặc `502drive autostart systemd install` (unit `--user` có
  `Restart=on-failure`, chỉ daemon). Đừng bật cả hai; `502drive doctor` kiểm unit,
  `502drive autostart systemd status` cho biết trạng thái.
- TUI: `502drive menu` (fallback line menu với `--plain`/`TERM=dumb`/`NO_COLOR`).

## Kiểm tra

```bash
502drive version
systemctl --user status 502drive
502drive status
502drive doctor                 # --online để gọi Google, --fix sửa quyền/thư mục cục bộ
journalctl --user -u 502drive -f
```

Bảo trì khi daemon dừng: `502drive prune --older-than 90d` (dry-run trước) rồi
`502drive vacuum`. Sao lưu trước khi nâng cấp: `502drive backup /backups/…` (xem
[runbooks](runbooks.md)).

Gỡ: `sudo rpm -e 502drive` / `sudo dpkg -r 502drive`. Config/token/data trong
home **không** bị xóa. Chưa có gói cho armhf, AppImage, flatpak, snap.

## Nâng cấp từ bản trước 0.3

Bản trước 0.3 lưu cấu hình ở `~/.config/502drive-next/`. Bản 0.3 vẫn tìm và dùng
thư mục đó khi `~/.config/502drive/` chưa có `config.json`, nên **không cần làm gì**;
`502drive doctor` ghi chú đang dùng tên cũ. `config.json` cũ chứa đường dẫn tuyệt
đối vào thư mục cũ, nên đừng chỉ đổi tên thư mục bằng tay.
