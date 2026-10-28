<div align="center">
<img src="docs/assets/mascot.gif" alt="Chú mèo 502Drive chuyển file giữa hai thư mục" width="480">

# 502Drive

**Sao chép và đồng bộ thư mục Google Drive mà không cần tải về.**

[![License: GPL-3.0-only](https://img.shields.io/badge/license-GPL--3.0--only-blue.svg)](LICENSE)
![Go](https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white)
![Platforms](https://img.shields.io/badge/platforms-Linux%20%7C%20Windows%20%7C%20macOS%20CLI-lightgrey)

[English](README.md) · Tiếng Việt
</div>

## Nó làm gì

Dán một link Google Drive. 502Drive tạo bản sao của cả thư mục đó trong **Drive của
chính bạn**, giữ nguyên thư mục con và tên file. Việc sao chép diễn ra **bên trong
Google**: không có gì được tải về máy rồi tải lên lại, nên thư mục 50 GB gần như không
tốn mạng của bạn.

```
 một link Drive  ──►  502Drive nhờ Google sao chép  ──►  thư mục xuất hiện trong Drive của bạn
 (thư mục của người khác)   (máy bạn chỉ gửi lệnh)        (bạn là chủ bản sao)
```

Nó chạy trên máy của bạn, bằng tài khoản Google của bạn. Không có máy chủ 502Drive,
không phải tạo tài khoản.

**Hợp với:** copy thư mục chia sẻ lớn vào Drive của bạn, giữ bản sao luôn mới theo bản
gốc, dùng bằng terminal, biểu tượng khay hệ thống hoặc bot Telegram.

**Không dành cho:** sao lưu file trên máy tính, tải file Drive về ổ đĩa, hay sao chép
những thứ Google không cho phép copy.

## Vì sao nên dùng

- **Thư mục lớn, mạng yếu.** Google lo việc sao chép.
- **Ngắt giữa chừng không sao.** Đóng máy, mất Wi-Fi, khởi động lại: job tiếp tục từ chỗ
  đã dừng, và 502Drive hỏi Drive trước khi gửi lệnh copy lại nên không sinh bản trùng.
- **Luôn mới.** Làm mới bản sao theo lịch, hoặc gần như tức thì khi bản gốc đổi.
- **Không xóa file của bạn.** Dọn dẹp chỉ chuyển bản copy do chính 502Drive tạo vào
  thùng rác Drive, và chỉ sau khi bạn xác nhận.

## Trước khi bắt đầu

| Bạn cần | Mất bao lâu |
| --- | --- |
| Một tài khoản Google (tài khoản sẽ sở hữu các bản sao) | — |
| Khoảng **20 phút** lần đầu (khoảng 10 cho khóa Google, 5 để cài, 5 cho lần copy đầu) | một lần |
| Biết dùng terminal ở mức dán vài lệnh | — |
| *Tùy chọn:* tài khoản Telegram để điều khiển từ điện thoại | +5 phút |

## Thiết lập (4 bước)

### 1. Cài đặt (khoảng 5 phút)

Chọn một cách:

| Hệ điều hành | Cách dễ nhất |
| --- | --- |
| **Windows** | Chạy `irm https://raw.githubusercontent.com/itsgiahung/502Drive/main/scripts/install.ps1 \| iex` trong PowerShell, hoặc tải `502drive-setup.exe` ở [Releases](https://github.com/itsgiahung/502Drive/releases) (xem [hướng dẫn Windows](docs/install-windows_VI.md)). |
| **Linux** | `curl -fsSL https://raw.githubusercontent.com/itsgiahung/502Drive/main/scripts/install.sh \| sh`, hoặc gói `.rpm`/`.deb` (xem [hướng dẫn Linux](docs/install-linux_VI.md)). |
| **macOS** | Dùng đúng dòng `install.sh` như Linux (chỉ Apple Silicon, chỉ dòng lệnh, [chưa kiểm thử](#đã-kiểm-thử-đến-đâu)). |
| **Docker / máy chủ** | Xem [hướng dẫn Docker](docs/install-docker_VI.md). |

Cả hai script đều kiểm tra file tải về với checksum đã công bố. Từ bản 0.4.5, mỗi bản
phát hành còn có chữ ký của các checksum đó; script Linux/macOS kiểm chữ ký khi máy có
[`cosign`](https://docs.sigstore.dev/cosign/installation/). Muốn tự kiểm, xem
[Kiểm tra bản tải về](docs/install-linux_VI.md#kiểm-tra-bản-tải-về). Bộ cài Windows chưa
được ký số nên Windows có thể cảnh báo "nhà phát hành không xác định".

### 2. Tạo khóa Google (một lần, khoảng 10 phút)

Google yêu cầu mọi ứng dụng copy file Drive phải có một khóa. Bạn tự tạo khóa trong
Google Cloud, nên file của bạn không bao giờ đi qua project của người khác. Đây là bước
duy nhất cần cẩn thận một chút. Tóm tắt:

1. **Tạo project** tại <https://console.cloud.google.com/projectcreate>.
2. **Bật Google Drive API** cho project:
   <https://console.cloud.google.com/apis/library/drive.googleapis.com>.
3. **Màn hình đồng ý:** ở <https://console.cloud.google.com/auth/branding> chọn
   **External**, rồi ở mục **Audience** thêm Gmail của bạn làm *test user*.
4. **Publish ứng dụng** (nút **Publish app**, trạng thái **In production**).
   *Đừng bỏ qua:* ứng dụng để ở "Testing" sẽ bị Google đăng xuất sau mỗi 7 ngày. Dùng cho
   cá nhân thì không cần Google duyệt; lúc đăng nhập Google hiện màn hình "ứng dụng chưa
   được xác minh": chọn **Advanced → Go to … (unsafe)**. Đó là khóa của chính bạn.
5. **Tạo khóa:** <https://console.cloud.google.com/auth/clients/create>, loại
   **Desktop app**, rồi bấm **Download JSON**. Bạn nhận một file tên
   `client_secret….json`.

Hướng dẫn đầy đủ kèm mọi liên kết: [hướng dẫn khóa Google](docs/oauth-setup_VI.md).
`502drive onboard` (bước kế tiếp) mở từng trang giúp bạn và chờ bạn làm xong.

### 3. Chạy trình hướng dẫn (khoảng 3 phút)

```sh
502drive onboard
```

Nó hỏi file JSON (bạn có thể kéo thả vào terminal), bật 502Drive chạy nền, mở trang đăng
nhập Google, rồi hỏi bản sao nên nằm ở đâu (nhấn Enter để dùng Drive của tôi). Khi nó in
**Sẵn sàng**, bạn đã xong.

### 4. Copy lần đầu (khoảng 2 phút)

```sh
502d 'https://drive.google.com/drive/folders/ID_NGUON' --wait
```

Hoặc chạy `502drive` để mở ứng dụng rồi dán link vào đó. Bản sao xuất hiện trong Drive
của bạn với cùng tên như bản gốc.

### Tùy chọn: điều khiển bằng Telegram (khoảng 5 phút)

```sh
502drive onboard --telegram
```

Trình hướng dẫn mở **@BotFather** giúp bạn: gửi `/newbot`, đặt tên, rồi dán token nó trả
về. Sau đó mở bot mới và bấm **Start**; 502Drive đọc ID số của bạn từ tin nhắn đó nên bạn
không phải tự tra. Sau khi chạy `502drive stop` rồi `502drive up`, bot chỉ trả lời bạn.
Xem [hướng dẫn Telegram](docs/features/telegram_VI.md).

## Dùng hằng ngày

| Tôi muốn… | Lệnh |
| --- | --- |
| Mở ứng dụng | `502drive` |
| Copy một link và chờ xong | `502d LINK --wait` |
| Xem trước link có gì | `502drive inspect LINK --deep` |
| Copy nhiều link cùng lúc | `502d LINK1 LINK2 LINK3` |
| Xem các job | `502drive jobs list` |
| Lấy danh sách file bị lỗi | `502drive jobs report JOB_ID --format csv` |
| Xem các bản sao | `502drive collections list` |
| Làm mới một bản sao ngay | `502drive sync run MA_BAN_SAO` |
| Làm mới mỗi đêm lúc 22:00 | `502drive sync enable MA_BAN_SAO --daily 22:00` |
| Kiểm tra bản sao khớp bản gốc | `502drive audit MA_BAN_SAO` |
| Tạm dừng hoặc tiếp tục tất cả | `502drive pause` · `502drive resume` |
| Dọn các job cũ đã xong khỏi danh sách | `502drive history clear --older-than 30d` |
| Kiểm tra mọi thứ đã thiết lập chưa | `502drive doctor` |
| Cập nhật lên bản mới nhất | `502drive update --install --restart` |
| Dừng 502Drive | `502drive stop` |

`MA_BAN_SAO` lấy từ `collections list`. Mọi lệnh đều có `--help`, và `--json` cho script.
Danh sách đầy đủ: [tham chiếu CLI](docs/features/cli.md).

**Trong ứng dụng:** phím số `1`–`7` đổi tab, mũi tên di chuyển, `Enter` mở, `←` hoặc `Esc`
quay lại, `q` đóng ứng dụng. Đóng ứng dụng không làm dừng việc copy; 502Drive tiếp tục
chạy nền cho tới khi bạn chạy `502drive stop` hoặc chọn **Dừng 502Drive…** ở khay hệ
thống. Mỗi hàng có một dòng giải thích ngay bên dưới.

## Chuyện gì xảy ra với file

- **Bản gốc thay đổi:** lần làm mới tiếp theo copy file mới và file đã sửa, theo kịp việc
  di chuyển và đổi tên, và giữ bản cũ của file đã sửa trừ khi bạn yêu cầu thay thế.
- **File bị xóa khỏi bản gốc:** bản sao của bạn vẫn giữ. Muốn bản sao xóa theo, bật
  *mirror* cho bản sao đó; file bị xóa sẽ vào thùng rác Drive, không bao giờ xóa vĩnh viễn.
- **Bạn xóa file khỏi bản sao:** 502Drive không tự đưa nó trở lại. `502drive audit
  MA_BAN_SAO` liệt kê những gì đang thiếu, và `audit MA_BAN_SAO --repair` cho xem trước
  những gì sẽ được khôi phục rồi bạn mới áp dụng.
- **Hai file y hệt cùng tên trong một thư mục:** chỉ copy một bản. File cùng tên nhưng
  khác nội dung thì cả hai đều được giữ (`tên (2).đuôi`).
- **Điểm khôi phục** (*snapshot*) giữ các phiên bản trước của bản sao và có thể quay lại.
  Mặc định tắt và tốn dung lượng Drive: [mirror và snapshot](docs/features/protection_VI.md).
- **Gỡ một bản sao:** `502drive collections retire MA_BAN_SAO`, rồi
  `502drive collections cleanup MA_BAN_SAO` cho xem những gì sẽ vào thùng rác và lệnh xác
  nhận chính xác.
- **Dọn danh sách job:** `502drive history clear` (hoặc Cài đặt → *Xóa lịch sử…* trong ứng
  dụng) chỉ ẩn job đã xong khỏi danh sách. Bản sao và file Drive không bị đụng tới.

## Có an toàn không?

- Khóa bạn tạo là của bạn và nằm trên máy bạn. Đăng nhập Google được lưu **mã hóa** ở máy;
  bản sao lưu (`502drive backup`) có chứa nó nên hãy giữ riêng tư.
- 502Drive xin Google quyền dùng Drive của bạn (cần để copy). Nó không chia sẻ công khai
  gì cả và không gửi dữ liệu thống kê. Nó chỉ nói chuyện với Google, với Telegram nếu bạn
  thiết lập, và với GitHub khi kiểm tra bản mới.
- Nó không bao giờ xóa vĩnh viễn gì trên Drive. Dọn dẹp chỉ chuyển bản copy của chính nó
  vào thùng rác, và chỉ sau khi bạn xác nhận.
- Bản sao có ID file mới và bạn là chủ sở hữu. Quyền chia sẻ, bình luận và lịch sử phiên
  bản không được copy; Google quyết định cái gì copy được.
- Hạn mức copy hằng ngày của Google vẫn áp dụng. Khi chạm hạn mức, job chờ rồi tự tiếp tục.

## Có sự cố?

| Bạn thấy | Cách xử lý |
| --- | --- |
| `daemon_unavailable` | Chạy `502drive up`. |
| Cứ khoảng 7 ngày lại bị hỏi đăng nhập | Ứng dụng Google còn ở *Testing*: Publish nó (bước 2.4), rồi chạy `502drive account login`. |
| `account_reconnect_required` | `502drive account login`. |
| Job báo **blocked** | `502drive jobs get JOB_ID` cho biết lý do; thường là hạn mức Google hoặc thiếu quyền. |
| Chưa thiết lập xong | `502drive doctor` liệt kê chỗ thiếu; `502drive onboard` sửa giúp. |

Thêm: [xử lý sự cố](docs/troubleshooting_VI.md) · [gỡ cài đặt và quay lại bản cũ](docs/runbooks.md).

## Đã kiểm thử đến đâu

Đã thử với tài khoản Google thật trên Linux: copy, đồng bộ, phục hồi sau sập máy, kiểm tra
toàn vẹn, mirror và snapshot, và một phiên Telegram ngắn. **Chưa thử thật:** dùng trên
Windows và macOS, Shared Drive, thư mục rất lớn, máy ngủ rồi mở lại và mất mạng, khay hệ
thống trên mọi môi trường desktop, và hướng dẫn tạo khóa từ một project Google còn trống.
Chi tiết: [verification](docs/verification.md).

## Dành cho lập trình viên

Bắt đầu với [mục lục tài liệu](docs/README.md) và [kiến trúc](docs/architecture.md).
Đóng góp: [CONTRIBUTING](CONTRIBUTING.md).

```sh
make check           # định dạng, vet và test race
make docs            # liên kết tài liệu và schema API
make release-local   # build gói phát hành trên máy; không công bố gì
```

[Nhật ký thay đổi](CHANGELOG.md) · [Quyền riêng tư](PRIVACY.md) · [Bảo mật](SECURITY.md) ·
[Thông báo bên thứ ba](THIRD_PARTY_NOTICES.md)

502Drive là dự án độc lập, không do Google tạo ra hay bảo trợ. Google Drive là nhãn hiệu của
Google LLC. Cấp phép theo [GPL-3.0-only](LICENSE).
