# Cài đặt trên Windows

[English](install-windows.md) · Tiếng Việt

Bản phát hành cho Windows gồm `502drive.exe`, trình mở giao diện `502drive-tray.exe` và
lối tắt `502d.cmd`. Chọn file ZIP nếu muốn chạy không cần cài, hoặc `502drive-setup.exe`
để cài đặt.

Bộ cài **chưa được ký số**, nên SmartScreen có thể hiện cảnh báo "nhà phát hành không
xác định". Hãy đối chiếu SHA-256 của file với checksum đi kèm bản phát hành. Bộ cài đã
được build, cài và gỡ thử tự động trên máy Windows của hệ thống kiểm thử, nhưng chưa có
ai dùng ứng dụng thật trên Windows.

## Cài đặt và thiết lập

Bộ cài (Inno Setup) có các tùy chọn:

| Tùy chọn | Mặc định |
| --- | --- |
| Thêm thư mục cài vào PATH | Bật |
| Tự chạy khi đăng nhập | Tắt |
| Tạo lối tắt trên desktop | Tắt |

Lối tắt trong Start Menu mở khay hệ thống hoặc menu dòng lệnh. Gỡ cài đặt chỉ xóa
chương trình và lối tắt, giữ nguyên cấu hình, token và dữ liệu của bạn. File ZIP chỉ cần
giải nén rồi chạy. Ngoài ra có script PowerShell:

```powershell
irm https://raw.githubusercontent.com/itsgiahung/502Drive/main/scripts/install.ps1 | iex
```

Thiết lập lần đầu (cách dễ nhất là `502drive onboard`, nó dẫn từng bước):

```powershell
502drive onboard
```

Hoặc làm tay:

```powershell
502drive setup --oauth-client C:\duong\dan\client_secret.json
502drive up
502drive account login
502drive doctor --online
```

Cách tạo khóa Google: xem [hướng dẫn OAuth](oauth-setup_VI.md). Cấu hình mới nằm ở
`%AppData%\502drive`; máy cài từ trước bản 0.3 với `502drive-next` vẫn dùng bình
thường. Quyền truy cập các file riêng tư chỉ dành cho người dùng của bạn. Đừng bao giờ
công khai các thư mục này.

## Chạy và dừng

`502drive up` bật dịch vụ nền. Khay hệ thống có **Mở 502Drive** và **Dừng 502Drive…**.
Thoát ứng dụng hoặc khay thông thường không dừng dịch vụ nền. `502drive serve` chạy ở
cửa sổ hiện tại và dừng khi cửa sổ đóng.

Bật tự chạy khi đăng nhập bằng `502drive autostart enable`, xem trạng thái bằng
`autostart status`. Chưa có dịch vụ Windows (Windows service). Cập nhật bản mới dùng
`502drive update --install --restart`, hoặc tải bộ cài mới và chạy lại.

## Dành cho người build bộ cài

Muốn biên dịch file `.iss` cần máy Windows có **Inno Setup 6**; quy trình phát hành
trên GitHub tự làm việc này nên bạn không cần máy Windows riêng. Đặt gói phát hành đã
kiểm tra vào `dist/windows` rồi chạy:

```powershell
./packaging/windows/build-installer.ps1 -Version 0.4.5
```

Kết quả là `artifacts/windows/502drive-setup.exe` và file `.sha256` đi kèm. Thêm
`-SmokeTest` để cài thử im lặng, chạy `version`, gỡ rồi kiểm tra file mẫu ngoài thư mục
chương trình vẫn còn nguyên.
