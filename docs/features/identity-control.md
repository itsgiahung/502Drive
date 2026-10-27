# Identity và kiểm soát truy cập

Source: [state/principals.go](../../internal/state/principals.go),
[app/account.go](../../internal/app/account.go), [httpapi/server.go](../../internal/httpapi/server.go)
(`authenticate`), [telegram/telegram.go](../../internal/telegram/telegram.go)
(`principal`). Test: [store_test.go](../../internal/state/store_test.go),
[server_test.go](../../internal/httpapi/server_test.go),
[telegram_test.go](../../internal/telegram/telegram_test.go).

## Principal

| Principal | Nguồn | Quyền |
| --- | --- | --- |
| `owner` (ID `owner`) | bearer token trong `token_file` | Toàn bộ: destinations, drive browse/search/mkdir, operators, doctor/`doctor/fix`, queue pause/resume, mọi job/collection/audit (`all=true`) |
| operator | `principals` kind=telegram, role=operator, enabled | Job/collection/sync/audit của chính mình; xem đích hiện có; không admin |

Các thao tác admin chỉ owner: `POST/PUT /v1/destinations*`, `/v1/drive/*`,
`/v1/operators*`, `/v1/doctor*`, `/v1/queue/pause|resume`, `all=true` trên
`/v1/jobs`, `/v1/history`, `/v1/collections`. Retire/resume/forget/cleanup kiểm
chủ sở hữu ở tầng store (owner thấy mọi thứ; operator khác chủ → `404`).

Adapter tự suy principal từ transport (token hash/update đã xác thực); **không**
đọc actor từ body/query. Token so sánh constant-time
(`subtle.ConstantTimeCompare`), raw token chỉ ở file 0600.

## Kiểm tra quyền trong application

- Jobs/collections/sync dùng `scope(p, all)`: owner có thể đọc mọi thứ, operator
  chỉ ID của mình; ghi (pause/cancel/retry/sync) kiểm lại chủ sở hữu.
- Owner điều khiển job của operator nhưng idempotency vẫn tính theo actor
  (`ActionAs`).
- Telegram: chỉ private chat; owner id từ config; operator phải enabled; callback
  kiểm lại principal + chat + TTL + consumed, không đoán ID.

## Credential Google

- OAuth Desktop client (PKCE) + token mã hoá AES-GCM bằng `secrets/master.key`;
  không log token/code/verifier. Trạng thái account: `disconnected`,
  `connected`, `reconnect_required`, `revoking`, `revoked`
  ([auth.go](../../internal/drive/auth.go)).

## Chưa hỗ trợ

- Bảng `api_tokens` (nhiều token, revoke từng token) chưa có: hiện một bearer
  owner từ file.
- Operator chưa có token HTTP riêng (chỉ qua Telegram).
- Không có đăng nhập/SSO trong ứng dụng; không phân quyền theo folder.
- Audit log hành động admin chưa có (chỉ job events).
