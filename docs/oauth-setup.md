# Google OAuth setup

Source: [internal/drive/auth.go](../internal/drive/auth.go),
[internal/config/config.go](../internal/config/config.go),
[internal/cli/commands.go](../internal/cli/commands.go) (`account login`). Test:
[auth_test.go](../internal/drive/auth_test.go),
[auth_more_test.go](../internal/drive/auth_more_test.go),
[setup_test.go](../internal/cli/setup_test.go).

## Chuẩn bị

1. Google Cloud Console → tạo OAuth client **Desktop app**; tải JSON.
2. Đặt file ở chỗ private (0600), ví dụ `~/.config/502drive-next/client_secret.json`.
3. `502drive setup --oauth-client ~/.config/502drive-next/client_secret.json`.

Client JSON được **giữ nguyên tại chỗ** và chỉ đọc khi `serve` khởi động; không
copy secret vào config/DB/response.

## Đăng nhập

```bash
502drive serve                      # terminal/daemon khác
502drive account login              # in URL, chờ callback loopback
```

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
| `oauth_not_configured` | Chưa `setup --oauth-client` hoặc file không đọc được |
| Login xong nhưng account `disconnected` | Callback không tới đúng port/namespace, hoặc file token không ghi được |
| `account_reconnect_required` | Refresh token hết hiệu lực, quyền bị thu hồi, client đổi |
| Docker login treo | Chưa publish/forward cổng callback |

Chưa hỗ trợ: login qua Telegram, nhiều account Google, service account.
