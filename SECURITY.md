# Security

## Reporting a vulnerability

**Do not open a public issue for security problems.** Report privately via GitHub
Security Advisories:

<https://github.com/GiaHung07/502Drive/security/advisories/new>

Include a description, affected version/commit, reproduction steps and impact.
You will get a response and a fix or mitigation timeline; credit is optional.

## Supported versions

| Version | Supported |
| --- | --- |
| 0.2.x | yes |
| < 0.2 | no |

Bản phát hành Go hiện tại là `0.2.x` (xem `502drive version` / release tag).

## Never commit

- `config.json`, `api-token`, bot token file
- `.env`, `secrets/`, `master.key`, `google-token.enc`
- `state-v2.db`, `*.db`, `*.db-wal`, `*.db-shm`
- backup (`MANIFEST.json` + DB + key + token)
- logs/reports chứa tên file hoặc token

Repo đã `.gitignore` các mục trên; test/fixtures phải dùng dữ liệu tổng hợp.

## Rotate credentials

1. **Google:** `502drive account revoke` → `502drive account login` →
   `502drive doctor --online`. Hoặc thu hồi tại
   <https://myaccount.google.com/permissions>.
2. **API bearer token:** dừng daemon, ghi token mới vào `token_file` (0600, một
   dòng), khởi động lại; client dùng cùng file (`--token-file`/env).
3. **Telegram bot token:** thu hồi bot token với BotFather, cập nhật file
   `telegram_token_file`, khởi động lại; xoá message/callback cũ nếu cần.
4. **Master key:** chỉ xoay bằng cách login lại (token cũ không đọc được); giữ
   backup cũ nếu còn cần.

Chi tiết mã hoá và ranh giới tin cậy: [docs/threat-model.md](docs/threat-model.md).
Token Google được mã hoá AES-256-GCM trong data dir với `master.key` (0600); API
token so sánh constant-time; HTTP chỉ loopback + bearer.

## Scope

Trong phạm vi: daemon, HTTP API, CLI/TUI/tray, adapter Telegram, backup/restore,
packaging. Ngoài phạm vi: máy đã bị chiếm quyền root/user, thay đổi phía Google,
dependency chưa pin (xem [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)).
