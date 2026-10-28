# Cài đặt trên Linux

[English](install-linux.md) · Tiếng Việt

Tải gói RPM/DEB hoặc file nén ở trang [Releases](https://github.com/itsgiahung/502Drive/releases).
Hãy kiểm tra checksum (và từ bản 0.4.5 là cả chữ ký: xem
[Kiểm tra bản tải về](#kiểm-tra-bản-tải-về)) trước khi cài.

```sh
sudo dnf install ./502drive_PHIÊN_BẢN_linux_amd64.rpm
# Debian/Ubuntu:
sudo dpkg -i ./502drive_PHIÊN_BẢN_linux_amd64.deb
```

Hoặc dùng script cài trong thư mục người dùng (không cần sudo), nó kiểm tra checksum
rồi cài vào `~/.local/bin`:

```sh
curl -fsSL https://raw.githubusercontent.com/itsgiahung/502Drive/main/scripts/install.sh | sh
```

## Gói cài gồm những gì

- `/usr/bin/502drive` và lối tắt `502d`.
- `/usr/lib/systemd/user/502drive.service` (cài xong chưa tự bật).
- Mục trong menu ứng dụng, icon và gợi ý lệnh cho bash/zsh/fish.

## Thiết lập lần đầu

Cách dễ nhất là chạy một lệnh, nó dẫn từng bước:

```sh
502drive onboard
```

Hoặc làm tay:

```sh
502drive setup --oauth-client /duong/dan/client_secret.json
502drive up
502drive account login
502drive destinations add LINK_THU_MUC
502drive destinations default MÃ_ĐÍCH
502drive doctor --online
```

`MÃ_ĐÍCH` là mã do `destinations add` hoặc `destinations list` in ra (khác với ID thư
mục của Google). Cách tạo khóa Google: xem [hướng dẫn OAuth](oauth-setup_VI.md).

Cấu hình thường nằm ở `~/.config/502drive`, dữ liệu nằm cạnh đó. Máy đã cài từ trước
bản 0.3 với thư mục `~/.config/502drive-next` vẫn dùng bình thường. Đừng tự đổi tên
thư mục đó: file cấu hình chứa đường dẫn tuyệt đối trỏ vào nó.

## Tự chạy khi đăng nhập và khay hệ thống

Chọn một cách khởi động:

- `502drive autostart enable`: mỗi lần đăng nhập màn hình, chạy `502drive up --quiet`.
- `502drive autostart systemd install`: cài và bật một dịch vụ systemd của người dùng,
  tự chạy lại khi lỗi. Dùng `systemctl --user start 502drive` để bật ngay.

Đừng bật cả hai cùng lúc. Khóa tiến trình ngăn hai bản ghi dữ liệu chồng nhau, nhưng
chạy trùng là không cần thiết.

Khay hệ thống cần phiên D-Bus của desktop và hỗ trợ StatusNotifier/AppIndicator. GNOME
có thể cần cài tiện ích AppIndicator. Nếu không có khay, dùng `502drive menu`; dịch vụ
nền và dòng lệnh vẫn chạy bình thường.

## Kiểm tra và bảo trì

```sh
502drive status
502drive doctor
502drive autostart systemd status
journalctl --user -u 502drive -f
```

Hãy dừng dịch vụ và sao lưu trước khi nâng cấp. Xem [sổ tay vận hành](runbooks.md).
Gỡ gói bằng `dnf remove` / `dpkg -r` giữ nguyên cấu hình và dữ liệu của bạn. Chưa có
gói AppImage, Flatpak, Snap hay armhf.

**Cập nhật lên bản mới:** `502drive update --install --restart` tải bản mới, kiểm
checksum, thay chương trình, sao lưu dữ liệu rồi khởi động lại dịch vụ. Với bản cài
bằng rpm/deb, lệnh này chỉ in cách nâng cấp bằng `dnf`/`apt`.

## Kiểm tra bản tải về

`checksums.txt` liệt kê mã SHA-256 của mọi file nén và gói, nên phát hiện được file
tải về bị hỏng. Từ bản 0.4.5, mỗi bản phát hành còn có `checksums.txt.sigstore.json`:
chữ ký [Sigstore](https://www.sigstore.dev/) do quy trình phát hành của repo này tạo ra.
Chữ ký chứng minh các checksum đến từ quy trình đó, chứ không phải do ai đó thay file.
Không có khóa ký nào để bị đánh cắp: chữ ký gắn với danh tính của quy trình qua một
chứng chỉ ngắn hạn.

```sh
# 1. chữ ký trên file checksum
cosign verify-blob \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp '^https://github\.com/itsgiahung/502Drive/\.github/workflows/go-release\.yml@refs/tags/v.+$' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt

# 2. checksum của file bạn tải
sha256sum --ignore-missing -c checksums.txt
```

`cosign` miễn phí ([cách cài](https://docs.sigstore.dev/cosign/installation/)). Bộ cài
Windows (`502drive-setup.exe`) có file `.sha256` riêng và chưa được ký số, nên Windows
hiện cảnh báo "nhà phát hành không xác định". Lệnh `502drive update --install` chỉ kiểm
checksum; muốn bắt buộc có chữ ký, hãy cài bằng script với `REQUIRE_SIGNATURE=1`. Các
bản trước 0.4.5 không có chữ ký.
