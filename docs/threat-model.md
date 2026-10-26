# Threat model (hiện tại)

Code Go, một daemon; CLI/TUI/tray/Telegram là client. Không còn webview, không
Rust/Tauri/React trong repo (đã gỡ 2026-10-26).

## Tài sản

| Tài sản | Nơi lưu |
| --- | --- |
| Google refresh/access token | `secrets/google-token.enc`, AES-GCM, key `secrets/master.key` (0600) |
| Bearer token API | `token_file` 0600, chỉ đọc bởi daemon và CLI |
| Bot token Telegram | `telegram_token_file` 0600 |
| Metadata Drive (tên, ID, resource key) | `state-v2.db` (0600), response HTTP |
| Job/state/idempotency/mapping | `state-v2.db` |
| Backup | thư mục 0700 + file 0600, chứa cả key + token mã hoá |

## Ranh giới tin cậy và biện pháp

| Ranh giới | Rủi ro | Biện pháp |
| --- | --- | --- |
| HTTP API | truy cập trái phép, actor giả | bind loopback, bearer bắt buộc, hash + constant-time, principal suy server-side |
| Telegram | người lạ, chat nhóm, callback cũ | chỉ private chat, owner id/operator enabled, callback one-shot + TTL + chat/revision, không shell |
| Drive | tên độc hại, lỗi provider | tên chỉ để hiển thị (Sanitize/escape); lỗi phân loại, không log body |
| CLI/TUI/tray | terminal injection, argv injection | `Sanitize`/`i18n`, argv cố định, clipboard chỉ qua `drive.Parse`, URL chỉ https + host Drive |
| Maintenance | xóa/ghi đè dữ liệu | kiểm `instance.lock`, đích mới/trống, sha256/integrity/FK, không symlink, không ra ngoài data dir |
| Packaging | installer chưa ký | checksum trong release, ghi rõ SmartScreen; không hứa publisher tin cậy |
| Config | secret rò qua log/argv/JSON | private file 0600, không in token, redact endpoint, JSON không chứa secret |

## Ngoài phạm vi (không hứa)

- Kẻ tấn công có quyền root/user trên máy (đọc file 0600, D-Bus session).
- Thay đổi phía Google (share/permission/quota) — provider là authority.
- Website độc hại: sản phẩm không có web UI, không nhận CORS/browser session.
- Supply-chain của dependency: pin version; chưa có SBOM/attestation.
- Bảo vệ output trên Drive: 502Drive không đặt public link, nhưng owner có thể
  chia sẻ sau đó — ngoài tầm kiểm soát.

## Ghi chú vận hành

- Nếu nghi lộ token: `account revoke` (Google), đổi bot token + chạy lại `setup`,
  xoay bearer token bằng `setup` (tạo file mới), backup mới, kiểm log.
- Backup chứa key giải mã token ⇒ bảo vệ như secret; không upload lên nơi công khai.
