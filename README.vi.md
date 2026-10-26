<div align="center">

<img src="docs/assets/mascot.gif" alt="Chú mèo 502Drive khiêng file giữa hai thư mục" width="480">

# 502Drive

**Dán link Google Drive. Có ngay bản copy trong Drive của bạn. Giữ đồng bộ tự động.**

[![License: GPL-3.0](https://img.shields.io/badge/license-GPL--3.0-blue.svg)](LICENSE)
![Go](https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white)
![Nền tảng](https://img.shields.io/badge/n%E1%BB%81n%20t%E1%BA%A3ng-Linux%20%C2%B7%20Windows%20%C2%B7%20macOS%20(CLI)-lightgrey)
![Trạng thái](https://img.shields.io/badge/tr%E1%BA%A1ng%20th%C3%A1i-test%20v%E1%BB%9Bi%20Drive%20gi%E1%BA%A3%2C%20ch%C6%B0a%20nghi%E1%BB%87m%20thu%20th%E1%BA%ADt-orange)

[English](README.md) · Tiếng Việt

</div>

Một binary Go duy nhất: daemon chạy nền, dòng lệnh kèm giao diện terminal chạy
trực tiếp, icon khay hệ thống và bot Telegram tùy chọn. Gõ `502drive` là mọi thứ
tự lên.

## Vì sao dùng

- **Copy phía server.** Google tự copy ngay trong Drive (`files.copy`). Máy bạn không
  tải xuống hay tải lên byte nào, nên thư mục 200 GB không tốn băng thông.
- **Giữ nguyên cây thư mục.** Tên, cấu trúc, thư mục rỗng, tên trùng, shortcut và
  file Google-native đều được giữ. Xong thì trả link kết quả.
- **Luôn đồng bộ.** Mỗi lần copy thành một *bộ sưu tập*, có thể bật lịch (theo
  khoảng, hằng ngày, hằng tuần, có múi giờ) hoặc đồng bộ realtime theo luồng thay
  đổi của Drive. Chạy ngay, tạm dừng, dừng, tiếp tục mà không mất thư mục đích hay
  ánh xạ. Xóa ở nguồn không bao giờ xóa bản copy của bạn.
- **An toàn khi sự cố.** Mỗi thao tác ghi lên Drive được ghi nhận trước. Sau khi
  crash hoặc mất phản hồi, job *đối soát* thay vì ghi hai lần, và báo `blocked`
  khi không chắc chắn, không bao giờ báo "xong" giả.
- **Dễ chịu khi dùng.** Tiến trình trực tiếp kèm tốc độ và thời gian còn lại, chọn
  nhiều mục để thao tác hàng loạt, icon khay hiện trạng thái, thông báo desktop,
  tiếng Việt và tiếng Anh, và một chú mèo hoạt hình nhỏ. Script nhận JSON và mã
  thoát ổn định.

## Bắt đầu nhanh

```bash
502drive          # (hoặc chỉ: 502) lần đầu hỏi khóa của bạn, bật mọi thứ, mở TUI
```

Bạn chỉ cần lấy từ Google một lần: OAuth client loại **Desktop**
([2 phút, từng bước](docs/oauth-setup.md)). Điền id và secret vào file `.env` ở thư
mục bạn chạy lệnh, hoặc đưa file JSON đã tải về:

```bash
# .env  (do `502drive setup` đọc; được chép vào file riêng tư 0600, không bao giờ in ra)
GOOGLE_OAUTH_CLIENT_ID=1234-abc.apps.googleusercontent.com
GOOGLE_OAUTH_CLIENT_SECRET=...
TELEGRAM_BOT_TOKEN=123456:ABC...        # tùy chọn
TELEGRAM_OWNER_ID=123456789             # tùy chọn, số id của bạn lấy từ @userinfobot
```

```bash
502drive setup                              # đọc ./.env (hoặc --env-file PATH), hoặc hỏi ngay trong terminal
502drive setup --oauth-client client_secret.json   # cách khác: file JSON Google cho tải về
```

Sau đó nhấn **`l`** trong TUI (hoặc chạy `502drive account login`): app mở trang Google
trong trình duyệt và hiện cả link, phòng khi bạn đang ở máy khác. Cho phép một lần là
kết nối xong. Thêm thư mục đích rồi dán link:

```bash
502drive destinations add 'https://drive.google.com/drive/folders/FOLDER_ID'
502drive copy 'https://drive.google.com/drive/folders/SOURCE_ID' --wait
```

Daemon và khay vẫn chạy khi bạn đóng terminal. Chỉ **Thoát trên khay** mới dừng
chúng (job đang chạy sẽ tiếp tục ở lần mở sau).

## Cài đặt

Một dòng (kiểm tra checksum SHA-256 trước khi cài; cần đã có bản phát hành):

```bash
curl -fsSL https://raw.githubusercontent.com/itsgiahung/502Drive/main/scripts/install.sh | sh        # Linux, macOS
irm https://raw.githubusercontent.com/itsgiahung/502Drive/main/scripts/install.ps1 | iex            # Windows (PowerShell)
```

Cả hai cài `502drive` và alias ngắn **`502`**, nên cài xong chỉ cần gõ `502`.

| Nền tảng | Cách cài |
| --- | --- |
| Linux (Fedora/RHEL, Debian/Ubuntu) | `.rpm` / `.deb` ở trang release (cài `502drive` và `502`). [Hướng dẫn](docs/install-linux.md) |
| Windows 10/11 | `.zip`, hoặc bộ cài (chưa ký: SmartScreen sẽ cảnh báo). [Hướng dẫn](docs/install-windows.md) |
| Docker | [Hướng dẫn](docs/install-docker.md) |
| Từ mã nguồn | `go install github.com/itsgiahung/502Drive/cmd/502drive@latest` (Go 1.27), hoặc `make build` |

Chạy cùng hệ thống: `502drive autostart enable`. Kiểm tra bản mới bằng
`502drive update` (không bao giờ tự cài).

## Dùng hằng ngày

| Tôi muốn… | Lệnh |
| --- | --- |
| Mở tất cả | `502drive` |
| Copy một link và chờ | `502drive copy LINK --wait` |
| Xem trước, không ghi gì | `502drive inspect LINK` |
| Xem / tạm dừng / tiếp tục / hủy job | `502drive jobs list`, `jobs pause ID`, … |
| Giữ bản copy đồng bộ | `502drive sync enable COLLECTION --daily 22:00` · `sync run` · `sync pause` · `sync stop` |
| Kiểm tra sức khỏe | `502drive doctor` |
| Xem log daemon | `502drive logs -f` |
| Bật/tắt chạy nền | `502drive up` (nút Thoát trên khay sẽ dừng) |
| Viết script | thêm `--json`, dùng [mã thoát](docs/features/cli.md) đã tài liệu hóa |

### Trong TUI

`1`–`6` đổi màn, `↑↓` di chuyển, `enter` mở. **Space** chọn dòng, **a** chọn hết;
**p** tạm dừng, **u** tiếp tục, **c** hủy (có hỏi lại), **t** thử lại, **n** chạy
ngay, **x** dừng sync áp dụng cho các mục đã chọn. **`7`** mở trình duyệt Drive:
`enter` vào thư mục, `←` lên cha, `i` xem metadata, `n` tạo thư mục, `d` đặt thư
mục làm đích mặc định, `c` copy mục đang chọn (hộp copy: `1` giữ tên, `2` tự đặt
tên `<tên> (copy ngày)`, `3` tự nhập, `d` chọn đích khác). `?` là trợ giúp,
`q` thoát TUI (daemon vẫn chạy). Chú mèo phản ứng theo trạng thái: khiêng file khi đang copy,
nhảy khi xong việc, chuyển vàng khi cần bạn xử lý, ngủ khi rảnh. `--no-motion`
làm mèo đứng yên; `NO_COLOR` hoặc `--plain` tắt hẳn.

### Telegram (tùy chọn)

```bash
# điền TELEGRAM_BOT_TOKEN và TELEGRAM_OWNER_ID vào .env (xem trên), rồi:
502drive setup
```

Bot chỉ chạy trong chat riêng, có danh sách cho phép, dùng chung service và quyền
với HTTP API. Chi tiết: [Telegram](docs/features/telegram.md).

## Riêng tư và bảo mật

- API chỉ lắng nghe **loopback** với bearer token lưu trong file riêng tư; truy cập
  từ xa hãy dùng SSH forwarding. Token OAuth được mã hóa khi lưu.
- 502Drive chỉ nói chuyện với Google (Drive API) và Telegram nếu bạn cấu hình.
  Kiểm tra bản mới chỉ gọi GitHub khi bạn chạy `502drive update` hoặc bật cho khay.
  Không có telemetry.
- Không đổi quyền chia sẻ, không tự công khai link, không vượt quota.
- Đọc [PRIVACY.md](PRIVACY.md), [SECURITY.md](SECURITY.md) và
  [mô hình đe dọa](docs/threat-model.md).

## Cách ghép nối

```
 CLI / TUI ─┐
 Khay ──────┼── HTTP /v1 (+ luồng SSE) ──▶ daemon: trạng thái (SQLite) · engine · lịch ──▶ Google Drive
 Telegram ──┘                                         ▲ một service, một người ghi
```

Một daemon sở hữu database, worker và token; mọi giao diện đều là client của cùng
một service. Xem [kiến trúc](docs/architecture.md), [hợp đồng HTTP](docs/control-contracts.md)
và [file OpenAPI](docs/contracts/openapi.json).

## Tài liệu

Bắt đầu từ [bản đồ tài liệu](docs/README.md). Những gì đã kiểm chứng và chưa kiểm
chứng được ghi rõ ở [verification](docs/verification.md) và
[acceptance](docs/acceptance.md).

## Phát triển

```bash
make check            # gofmt, go vet, go test -race
make docs             # link, JSON OpenAPI, khoảng trắng
make release-local    # build và kiểm chứng bản phát hành tại máy, không đăng gì
```

Xem [CONTRIBUTING](CONTRIBUTING.md). CI là tùy chọn: mọi kiểm tra đều chạy được ở máy.

## Giới hạn cần biết

- "Copy giống hệt" nghĩa là tên, cây, nội dung và loại file trong phạm vi Google
  cho phép. ID, chủ sở hữu, chia sẻ và lịch sử phiên bản không được giữ.
- Chưa nghiệm thu với Google Drive thật; khay và TUI được test khi không có desktop
  thật. Xem [verification](docs/verification.md).
- Khay cần Linux (StatusNotifier/AppIndicator) hoặc Windows; macOS chỉ có CLI và
  daemon. Thông báo desktop chạy trên Linux (`notify-send`).

## Lưu ý

502Drive là dự án độc lập, không liên kết với hay được Google bảo trợ. "Google
Drive" là nhãn hiệu của Google LLC.

Giấy phép GPL-3.0-only, xem [LICENSE](LICENSE) và
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
