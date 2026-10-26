# Google OAuth setup

Source: [internal/drive/auth.go](../internal/drive/auth.go),
[internal/config/config.go](../internal/config/config.go),
[internal/cli/commands.go](../internal/cli/commands.go) (`account login`). Test:
[auth_test.go](../internal/drive/auth_test.go),
[auth_more_test.go](../internal/drive/auth_more_test.go),
[setup_test.go](../internal/cli/setup_test.go).

## Chuẩn bị

1. Google Cloud Console → *APIs & Services* → bật **Google Drive API** → *Credentials* →
   tạo OAuth client loại **Desktop app**. Màn hình consent để *Testing* và thêm email
   của bạn vào *Test users*.
2. Đưa khóa cho 502Drive bằng **một** trong ba cách (tất cả đều được chép vào
   `<config>/secrets/oauth-client.json` với quyền 0600; file gốc không bị sửa và giá trị
   không bao giờ được in ra):
   - **`.env`**: `GOOGLE_OAUTH_CLIENT_ID=…apps.googleusercontent.com` và
     `GOOGLE_OAUTH_CLIENT_SECRET=…` trong `./.env` (hoặc `<config>/.env`, hoặc
     `--env-file PATH`), rồi `502drive setup`. Tên cũ `GDCLONE__GOOGLE_OAUTH__CLIENT_ID`/
     `…CLIENT_SECRET` cũng được hiểu. Biến môi trường cùng tên cũng được; cờ luôn thắng.
   - **File JSON** tải từ Google: `502drive setup --oauth-client client_secret.json`.
     File dạng *Web* bị từ chối (cần Desktop).
   - **Hỏi trong terminal**: chạy `502drive` hoặc `502drive setup` lần đầu và kéo-thả file JSON.

Client được đọc khi daemon khởi động; secret không nằm trong DB hay response.

## Đăng nhập

```bash
502drive                            # mở TUI, nhấn l (trang Account) — tự mở trình duyệt, hiện link và chờ
502drive account login              # tương đương trên dòng lệnh: in URL, chờ callback loopback
```

Trong TUI, link hiện hai lần: dạng bấm được (OSC 8) và dạng chữ ngắt dòng để chép lại
(ví dụ khi bạn đang ở SSH, xem thêm `ssh -L` bên dưới). `esc` chỉ ẩn link; link còn hiệu
lực tới khi hết 10 phút.

- Luồng PKCE loopback; URL chứa `redirect_uri=http://127.0.0.1:<port>/oauth/callback`.
- Code chỉ dùng một lần; callback sai/không khớp state không consume session hợp lệ;
  session TTL 10 phút.
- Token lưu mã hoá AES-GCM trong `secrets/google-token.enc` bằng
  `secrets/master.key` (0600). Không log token/code/verifier.
- Sau login, kiểm `502drive doctor --online` (account/destination/Drive).

### Daemon ở máy khác / không mở được browser

`account login` in đúng cổng lấy từ `redirect_uri` của session, không đoán:

```text
Daemon on another machine? Run this on your machine first:
ssh -L 50200:127.0.0.1:50200 YOU@DAEMON_HOST
```

`50200` lấy từ URL phiên (nếu bạn cấu hình `oauth_callback_port`). Nếu không
forward được cổng đó, đặt `oauth_callback_port` cố định trong config rồi login lại.
Callback phải đến **cùng** host/port với `redirect_uri`.

## Docker

- Daemon trong container bind loopback trong container; publish/forward cổng
  callback ra host khi login: `docker run -p 127.0.0.1:50200:50200 …` (xem
  [install-docker](install-docker.md)).
- Mount config, token file và data dir; không đưa secret vào image layer.

## Revoke / reconnect

- `502drive account revoke`: xoá token cục bộ + gọi Google revoke; trạng thái
  `revoking`/`revoked`; job đang chạy sẽ `blocked`/cần reconnect.
- `reconnect_required` (refresh token hỏng/quyền bị thu hồi): chạy lại
  `account login`; không tự đổi account.
- Operator không bao giờ thấy token/client secret; chỉ owner cấu hình.

## Lỗi thường gặp

| Triệu chứng | Nguyên nhân thường gặp |
| --- | --- |
| `oauth_not_configured` | Chưa có khóa Google: điền `.env` rồi `502drive setup`, hoặc `setup --oauth-client` |
| Login xong nhưng account `disconnected` | Callback không tới đúng port/namespace, hoặc file token không ghi được |
| `account_reconnect_required` | Refresh token hết hiệu lực, quyền bị thu hồi, client đổi |
| Docker login treo | Chưa publish/forward cổng callback |

Chưa hỗ trợ: login qua Telegram, nhiều account Google, service account.
