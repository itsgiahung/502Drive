<div align="center">
<img src="docs/assets/mascot.gif" alt="Chú mèo 502Drive chuyển file giữa hai thư mục" width="480">

# 502Drive

**Sao chép và đồng bộ thư mục Google Drive mà không cần tải về.**

[![License: GPL-3.0-only](https://img.shields.io/badge/license-GPL--3.0--only-blue.svg)](LICENSE)
![Go](https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white)
![Platforms](https://img.shields.io/badge/platforms-Linux%20%7C%20Windows%20%7C%20macOS%20CLI-lightgrey)

[English](README.md) · Tiếng Việt
</div>

Dán một link Google Drive, 502Drive sẽ copy cả thư mục vào Drive của bạn, giữ
nguyên cây thư mục và tên file. Việc copy diễn ra **ngay trong Google**: không có
gì bị tải về máy rồi tải lên lại, nên một thư mục 50 GB gần như không tốn mạng của
bạn. Nếu laptop ngủ hay mất mạng, việc copy chạy tiếp từ chỗ dừng. Sau đó 502Drive
có thể giữ bản copy luôn khớp với bản gốc.

Chương trình chạy trên máy của bạn, đăng nhập bằng tài khoản Google của bạn.
Không có máy chủ 502Drive nào, cũng không cần tạo tài khoản.

## Vì sao dùng

- **Thư mục lớn, mạng yếu vẫn được.** Google tự copy; máy bạn chỉ gửi lệnh.
- **Ngắt giữa chừng cũng không sao.** Gập máy, mất Wi-Fi, khởi động lại: job chạy
  tiếp từ chỗ dừng, và 502Drive kiểm tra trên Drive trước khi gửi lại lệnh copy,
  nên không sinh file trùng.
- **Tự đồng bộ.** Kiểm tra lại bản copy theo lịch (hằng ngày, vài giờ một lần)
  hoặc gần như ngay khi bản gốc thay đổi.
- **Không bao giờ xóa file của bạn.** Dọn dẹp chỉ đưa bản copy do 502Drive tạo vào
  thùng rác Drive, và chỉ sau khi bạn xác nhận.
- **Dùng theo cách bạn thích:** ứng dụng trong terminal (có mèo), lệnh cho script,
  biểu tượng khay hệ thống, hoặc bot Telegram.

## Bắt đầu nhanh

**1. Cài đặt.** Tải gói cho máy bạn ở
[Releases](https://github.com/itsgiahung/502Drive/releases), hoặc:

```sh
# Linux hoặc macOS
curl -fsSL https://raw.githubusercontent.com/itsgiahung/502Drive/main/scripts/install.sh | sh
```

```powershell
# Windows (PowerShell)
irm https://raw.githubusercontent.com/itsgiahung/502Drive/main/scripts/install.ps1 | iex
```

Cả hai script đều kiểm tra file tải về với checksum đã công bố.

**2. Tạo khóa Google (làm một lần, khoảng 5 phút).** Google yêu cầu mọi ứng dụng
copy file Drive phải có khóa. Bạn tự tạo khóa của mình trên Google Cloud, nên file
của bạn không đi qua dự án của ai khác:
[hướng dẫn từng bước](docs/oauth-setup.md) (tiếng Anh). Kết quả là một file nhỏ
`client_secret….json`.

**3. Chạy trình hướng dẫn.**

```sh
502drive onboard
```

Trình này hỏi file JSON ở trên (kéo thả vào terminal cũng được), bật 502Drive chạy
nền, mở trang đăng nhập Google, rồi hỏi bản copy sẽ lưu vào đâu (nhấn Enter để
chọn My Drive). Khi nó in **Xong!** là đã sẵn sàng.

**4. Copy thử.**

```sh
502d 'https://drive.google.com/drive/folders/SOURCE_ID' --wait
```

Hoặc chỉ cần gõ `502drive` để mở ứng dụng rồi dán link vào đó.

## Dùng hằng ngày

| Tôi muốn… | Lệnh |
| --- | --- |
| Mở ứng dụng | `502drive` |
| Copy một link và chờ xong | `502d LINK --wait` |
| Xem trước link có gì | `502drive inspect LINK --deep` |
| Copy nhiều link một lúc | `502d LINK1 LINK2 LINK3` |
| Xem các job | `502drive jobs list` |
| Lấy danh sách file bị lỗi | `502drive jobs report JOB_ID --format csv` |
| Xem các bản copy | `502drive collections list` |
| Cập nhật một bản copy ngay | `502drive sync run COPY_ID` |
| Tự cập nhật mỗi tối lúc 22:00 | `502drive sync enable COPY_ID --daily 22:00` |
| Kiểm tra bản copy có khớp bản gốc | `502drive audit COPY_ID` |
| Tạm dừng hoặc chạy lại tất cả | `502drive pause` · `502drive resume` |
| Kiểm tra cài đặt có ổn không | `502drive doctor` |
| Tắt 502Drive | `502drive stop` |
| Cập nhật lên bản mới nhất | `502drive update --install --restart` |

`COPY_ID` lấy từ `collections list`. Lệnh nào cũng có `--help`, và `--json` cho
script. Danh sách đầy đủ ở [tài liệu CLI](docs/features/cli.md) (tiếng Anh).

### Trong ứng dụng

Phím số `1`–`7` đổi tab, mũi tên để di chuyển, `Enter` để mở, `←` hoặc `Esc` để
quay lại, `q` để đóng ứng dụng. Đóng ứng dụng không dừng việc copy: 502Drive vẫn
chạy nền cho tới khi bạn gõ `502drive stop` hoặc chọn **Dừng 502Drive…** trên khay.
Ứng dụng có tiếng Việt và tiếng Anh.

### Telegram (tùy chọn)

Gửi link Drive cho bot của riêng bạn để bắt đầu copy và nhận tin nhắn khi xong.
Chỉ những tài khoản bạn cho phép mới dùng được. Xem
[hướng dẫn Telegram](docs/features/telegram.md) (tiếng Anh).

## Chuyện gì xảy ra với file

- **Khi bản gốc thay đổi**, lần cập nhật sau sẽ copy file mới và file đã sửa, di
  chuyển và đổi tên theo bản gốc, và giữ lại phiên bản cũ của file đã sửa (trừ khi
  bạn chọn thay thế).
- **Khi một file bị xóa ở bản gốc**, bản copy vẫn giữ file đó. Nếu muốn bản copy
  xóa theo, bật *mirror* cho bản copy đó; file bị xóa sẽ vào thùng rác Drive, không
  bao giờ bị xóa vĩnh viễn.
- **Khi bạn tự tay xóa file trong bản copy**, 502Drive không tự chép lại.
  `502drive audit COPY_ID` liệt kê những gì bị thiếu, còn `audit COPY_ID --repair`
  cho xem trước những gì sẽ khôi phục rồi mới làm.
- **Điểm khôi phục** (*snapshot*) giữ các phiên bản cũ của bản copy để quay lại khi
  cần. Mặc định tắt và chiếm dung lượng Drive. Xem
  [mirror và snapshot](docs/features/protection.md) (tiếng Anh).
- Muốn bỏ hẳn một bản copy: `502drive collections retire COPY_ID`, sau đó
  `502drive collections cleanup COPY_ID` cho xem những gì sẽ vào thùng rác và câu
  lệnh chính xác để xác nhận.

## Nên biết

- Bản copy có ID mới và bạn là chủ sở hữu. Quyền chia sẻ, bình luận và lịch sử phiên
  bản không được copy; Google quyết định file nào copy được.
- Hạn mức copy hằng ngày của Google vẫn áp dụng. Khi chạm hạn mức, job tự chờ và
  chạy tiếp khi Google cho phép.
- Thông tin đăng nhập Google được mã hóa trên máy bạn. Bản sao lưu tạo bằng
  `502drive backup` có chứa nó, nên hãy giữ kín.
- 502Drive chỉ kết nối tới tài khoản Google của bạn, Telegram (nếu bạn bật) và
  GitHub khi bạn kiểm tra cập nhật. Không gửi dữ liệu thống kê, không tự cập nhật,
  không tạo link chia sẻ công khai.

**Đã kiểm thật:** copy, đồng bộ, phục hồi sau crash, audit và Telegram trên Linux với
tài khoản Google thật. **Chưa kiểm thật:** cài trên Windows và macOS, Shared Drive,
thư mục rất lớn, mirror và snapshot. Chi tiết: [verification](docs/verification.md).

## Cho lập trình viên

Bắt đầu từ [mục lục tài liệu](docs/README.md) và [kiến trúc](docs/architecture.md).
Đóng góp: [CONTRIBUTING](CONTRIBUTING.md). Tài liệu kỹ thuật viết bằng tiếng Anh.

```sh
make check           # format, vet và test race
make docs            # link tài liệu và schema API
make release-local   # build gói phát hành trên máy; không đăng gì
```

[Changelog](CHANGELOG.md) · [Quyền riêng tư](PRIVACY.md) · [Bảo mật](SECURITY.md) ·
[Thư viện bên thứ ba](THIRD_PARTY_NOTICES.md)

502Drive là dự án độc lập, không do Google làm hay bảo trợ. Google Drive là thương hiệu
của Google LLC. Giấy phép [GPL-3.0-only](LICENSE).
