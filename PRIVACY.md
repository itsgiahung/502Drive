# Privacy Policy — 502Drive

**Last updated:** 2026-10-26. Tài liệu này mô tả runtime Go hiện tại; đối chiếu
[HTTP contract](docs/control-contracts.md), [identity](docs/features/identity-control.md)
và [threat model](docs/threat-model.md).

## 1. Tóm tắt

502Drive là daemon cục bộ: bạn chạy `502drive serve` trên máy của mình, dữ liệu
nằm trong data dir bạn chọn. Không có dịch vụ hosted, không analytics/telemetry,
không có web frontend tải font/asset từ bên thứ ba trong code hiện tại.

## 2. Dữ liệu lưu trên máy bạn

| Dữ liệu | Vị trí | Ghi chú |
| --- | --- | --- |
| Config JSON | `config.json` (mặc định `~/.config/502drive-next/config.json`; Windows `%AppData%`) | Đường dẫn listen/data/token; **không** chứa secret |
| API bearer token | `token_file`, ví dụ `api-token` (0600) | Client CLI/tray đọc file này |
| Google OAuth client JSON | đường dẫn bạn chỉ định khi `setup --oauth-client` | Giữ tại chỗ, không copy vào DB |
| Google token | `data/secrets/google-token.enc` | AES-256-GCM; key `data/secrets/master.key` (0600, không lưu keyring) |
| Telegram bot token | file bạn chỉ định (`--telegram-token-file`) | Chỉ daemon đọc |
| Job/metadata | `data/state-v2.db` (SQLite, 0600) | Tên/ID/size/kết quả job, mapping, events, idempotency, subscription, inbox/outbox Telegram |
| Backup | thư mục bạn tạo bằng `502drive backup PATH` | Chứa cả key + token mã hoá ⇒ bảo vệ như secret |

Không lưu mật khẩu Google, không lưu nội dung file Drive.

## 3. Dữ liệu đi đâu

- **Google Drive API:** request metadata và thao tác copy server-side; OAuth
  consent qua browser của bạn. Token chỉ dùng để gọi Drive.
- **Telegram (nếu bạn cấu hình bot):** lệnh, nút bấm, tên/ID file hiển thị trong
  menu/card, thông báo job. Không gửi token/resource key.
- **Máy bạn:** CLI/TUI/tray nói chuyện với daemon qua loopback HTTP.
- Không gửi dữ liệu tới dịch vụ của dự án; không có telemetry ngầm. Log daemon
  redact token/secret; tên file có thể xuất hiện trong log tuỳ mức log bạn cấu hình.

## 4. Lưu giữ và xoá

- Job/event/idempotency/mapping được giữ mặc định (phục vụ replay/đối chiếu);
  chưa có auto-prune. `502drive cleanup` chỉ xoá file tạm.
- Xoá dữ liệu: dừng daemon rồi xoá data dir (và backup liên quan). Xoá
  `master.key` làm token đã mã hoá không đọc được.
- Thu hồi quyền Google: `502drive account revoke` (xoá cục bộ + gọi Google revoke)
  hoặc tại <https://myaccount.google.com/permissions>.

## 5. Trẻ em và liên hệ

Sản phẩm tự host, không thu thập dữ liệu người dùng qua Internet. Câu hỏi riêng
tư: mở issue trên repository (không kèm secret) hoặc dùng kênh bảo mật ở
[SECURITY.md](SECURITY.md).
