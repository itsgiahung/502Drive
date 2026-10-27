<div align="center">
<img src="docs/assets/mascot.gif" alt="Chú mèo 502Drive chuyển file giữa hai thư mục" width="480">

# 502Drive

**Sao chép Google Drive phía máy chủ. Quản lý bản sao ngay trong terminal.**

[English](README.md) · Tiếng Việt
</div>

502Drive là công cụ tự chạy trên máy của bạn để sao chép và đồng bộ Google Drive.
Một daemon Go quản lý cơ sở dữ liệu, thông tin đăng nhập, hàng đợi và worker;
CLI, TUI, khay hệ thống và bot Telegram tùy chọn cùng điều khiển dịch vụ đó.

Nội dung file được Google sao chép bên trong Drive, không tải xuống rồi tải lên qua
máy bạn. Các yêu cầu metadata vẫn dùng mạng; quyền truy cập và hạn mức Google vẫn áp dụng.

## Có thể làm gì?

- Copy file hoặc cả cây thư mục bằng link, trong phạm vi loại file Google cho phép.
- Xem trước, lọc đường dẫn/loại/kích thước, nhận nhiều link và điều khiển hàng đợi.
- Đồng bộ theo lịch hoặc theo luồng thay đổi của Drive.
- Kiểm tra toàn vẹn và xem kế hoạch sửa trước khi áp dụng.
- Bật **mirror** riêng từng collection để đưa bản sao thuộc 502Drive vào thùng rác
  khi nguồn mất. Mặc định tắt; không xóa vĩnh viễn.
- Chụp **snapshot**, ghim điểm khôi phục và xem trước phục hồi. Tự chụp mặc định
  tắt; các bản được giữ lại vẫn chiếm dung lượng Drive.
- Dùng giao diện tiếng Việt/Anh; script nhận JSON và mã thoát ổn định.

## Cài đặt

Tải đúng gói tại [Releases](https://github.com/itsgiahung/502Drive/releases) và kiểm
SHA-256 trước khi chạy. Tài liệu chi tiết còn lại dùng tiếng Anh.

| Nền tảng | Cách cài |
| --- | --- |
| Linux | RPM, DEB hoặc archive; [hướng dẫn](docs/install-linux.md) |
| Windows | ZIP hoặc Inno Setup chưa ký; [hướng dẫn](docs/install-windows.md) |
| macOS | CLI/daemon; chưa có tray |
| Docker / Podman | Build từ repo; [hướng dẫn](docs/install-docker.md) |
| Mã nguồn | Go 1.27; `go install github.com/itsgiahung/502Drive/cmd/502drive@latest` |

Script cài có kiểm checksum và tạo lệnh ngắn `502d`:

```sh
# Linux hoặc macOS
curl -fsSL https://raw.githubusercontent.com/itsgiahung/502Drive/main/scripts/install.sh | sh
```

```powershell
# Windows
irm https://raw.githubusercontent.com/itsgiahung/502Drive/main/scripts/install.ps1 | iex
```

## Lần copy đầu

Chuẩn bị tài khoản Google và OAuth client loại **Desktop app** của bạn. Xem
[hướng dẫn OAuth](docs/oauth-setup.md). Không có máy chủ hay tài khoản Google dùng
chung của dự án; khóa và dữ liệu nằm trên máy bạn.

```sh
502drive setup --oauth-client /duong/dan/client_secret.json
502drive up
502drive account login
502drive destinations add 'https://drive.google.com/drive/folders/DESTINATION_ID'
502drive destinations list
502drive destinations default DESTINATION_ID_TRONG_DANH_SACH
502drive copy 'https://drive.google.com/drive/folders/SOURCE_ID' --wait
```

Lệnh `destinations default` dùng **ID của 502Drive** được `add/list` trả về, khác
với ID thư mục Google. Chạy `502drive` để mở giao diện hoặc `502drive onboard` để
kiểm tra lần lượt các bước thiết lập.

## Dùng hằng ngày

```sh
502drive jobs list
502drive wait JOB_ID
502drive sync enable COLLECTION_ID --daily 22:00
502drive audit COLLECTION_ID
502drive pause
502drive resume
502drive doctor
```

TUI: `1`–`7` chọn tab; `↑↓` chọn dòng, `Enter` mở, `←` hoặc `Esc` quay lại từ màn
chi tiết. Trong collection, xuống **Đồng bộ** rồi Enter để chọn lịch/thao tác bằng
mũi tên. Trong ô nhập, mũi tên vẫn sửa chữ; trong trình duyệt Drive, `←` lên thư mục cha.
`q` thoát TUI, daemon tách nền vẫn chạy. Dừng bằng `502drive stop` hoặc **Dừng
502Drive…** trên khay. Lệnh `serve` chạy foreground và kết thúc theo process của nó.

```sh
502drive sync options COLLECTION_ID --snapshots=true --keep-days 30 --keep-count 10
502drive snapshots create COLLECTION_ID
502drive sync options COLLECTION_ID --mirror=true
502drive snapshots restore SNAPSHOT_ID             # chỉ xem kế hoạch
502drive snapshots restore SNAPSHOT_ID --apply     # áp dụng và tạm dừng sync
```

Đọc [giới hạn bảo vệ dữ liệu](docs/features/protection.md) trước khi bật. Phục hồi
xong vẫn tạm dừng sync; bạn chủ động resume khi muốn bám nguồn hiện tại trở lại.

## Bảo mật và giới hạn

- Không giữ nguyên ID, chủ sở hữu, quyền chia sẻ hay lịch sử phiên bản.
- Mặc định nguồn mất vẫn giữ bản đích; mirror là lựa chọn có chủ đích.
- Ghi không rõ kết quả phải đối soát; không gửi lại mù.
- API dùng loopback và bearer token riêng tư; truy cập từ xa qua SSH forwarding.
  Token Google được mã hóa; backup chứa cả khóa nên phải giữ kín.
- Snapshot giữ bản sao vật lý, không phải backup byte độc lập. Người khác sửa/xóa
  file có thể làm điểm khôi phục không còn dùng được; không phục hồi xóa vĩnh viễn.
- Không tự public link, vượt quota, telemetry hay tự cài bản cập nhật.

Đã có bản ghi kiểm thật nhỏ trên Linux cho copy/sync/phục hồi sau crash và Telegram.
Mirror/snapshot mới kiểm bằng fixture cục bộ; Windows/installer, Shared Drive và
cây lớn vẫn cần nghiệm thu. Xem [verification](docs/verification.md).

## Tài liệu và đóng góp

[Bản đồ tài liệu](docs/README.md) · [CLI](docs/features/cli.md) ·
[Telegram](docs/features/telegram.md) · [Đóng góp](CONTRIBUTING.md) ·
[Changelog](CHANGELOG.md) · [Riêng tư](PRIVACY.md) · [Bảo mật](SECURITY.md)

502Drive là dự án độc lập, không được Google bảo trợ. Google Drive là nhãn hiệu của
Google LLC. Giấy phép [GPL-3.0-only](LICENSE).
