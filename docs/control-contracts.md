# HTTP contract v1

Router: [internal/httpapi/server.go](../internal/httpapi/server.go) và
[internal/httpapi/sync.go](../internal/httpapi/sync.go). Nguồn máy đọc:
[docs/contracts/openapi.json](contracts/openapi.json); conformance test
[openapi_test.go](../internal/httpapi/openapi_test.go) so khớp mọi response test
với schema.

## Nguyên tắc chung

- Bind loopback (config `listen`), **luôn** cần `Authorization: Bearer <token>`
  trừ `/health/live`. Token đọc từ `token_file` (0600), map sang `owner` server-side.
- Response: `{"data": ..., "request_id": "req_..."}`; lỗi:
  `{"error":{"code","message","retryable","retry_after_seconds"},"request_id":...}`.
- Mutation cần `Idempotency-Key` (1–128 ký tự in được). Cùng key + cùng body →
  replay (`Idempotency-Replayed: true`); khác body → `409 idempotency_mismatch`.
- Revision: `expected_revision` (job/collection) và `expected_subscription_revision`
  (sync). Lệch → `409 stale_revision`.
- `202` = đã admit, **không** phải xong. `503 maintenance` khi daemon chạy
  `--maintenance` (từ chối `POST /v1/jobs`, `PUT .../sync`, `POST .../runs`).
- Cursor list là opaque, gắn principal + filter; dùng lại sai filter → `400`.

## Route

| Route | Trạng thái | Nguồn |
| --- | --- | --- |
| `GET /health/live` | Có (không auth) | server.go |
| `GET /health/ready` | Có (auth, schema + worker) | server.go |
| `GET /v1/status` | Có | server.go, `app.Status` |
| `GET /v1/stream` | Có (SSE, owner) | [stream.go](../internal/httpapi/stream.go) |
| `POST /v1/shutdown` | Có (202, owner) | [stream.go](../internal/httpapi/stream.go) |
| `POST /v1/plans` | Có (201) | server.go, `app.Inspect` |
| `POST /v1/jobs` | Có (202 + Location) | server.go, `app.Submit` |
| `GET /v1/jobs?state&limit&cursor&all` | Có | server.go, `state.ListJobs` |
| `GET /v1/jobs/{id}` | Có | server.go |
| `GET /v1/jobs/{id}/events?after_sequence&limit` | Có | server.go |
| `POST /v1/jobs/{id}/pause\|resume\|cancel\|retry` | Có (202) | server.go, `app.JobAction` |
| `GET /v1/drive/folders/{id}?page_token` | Có (owner; `root` = My Drive; breadcrumb ở trang đầu) | [drive.go](../internal/httpapi/drive.go), `app.Browse` |
| `GET /v1/drive/files/{id}` | Có (owner; metadata một mục) | drive.go, `app.FileInfo` |
| `POST /v1/drive/folders` | Có (201, owner, idempotent theo key+parent+tên) | drive.go, `app.Mkdir` |
| `GET /v1/destinations` | Có | server.go |
| `POST /v1/destinations` | Có (201) | server.go, `app.AddDestination` |
| `PUT /v1/destinations/default` | Có (200) | server.go |
| `GET /v1/account` | Có | server.go, `app.Account` |
| `POST /v1/account/auth-sessions` | Có | server.go, `app.StartLogin` |
| `GET /v1/account/auth-sessions/{id}` | Có | server.go, `app.LoginStatus` |
| `POST /v1/account/revoke` | Có | server.go, `app.Revoke` |
| `GET /v1/preferences` | Có (language, timezone, notify, auto_copy…) | server.go, `app.Preferences` |
| `PATCH /v1/preferences` | Có | server.go, `app.PatchPreferences` |
| `GET /v1/operators`, `POST /v1/operators`, `DELETE /v1/operators/{id}` | Có (owner) | server.go, `app.Operators` |
| `GET /v1/doctor?online` | Có (owner) | server.go, `app.Doctor` |
| `GET /v1/collections?limit&cursor&query&sync_mode&all` | Có, phân trang thật | server.go, `state.CollectionPage` |
| `GET /v1/collections/{id}` | Có (`sync` thật) | server.go, `dto.go` |
| `PUT /v1/collections/{id}/sync` | Có (200) | sync.go, `app.ConfigureSync` |
| `POST /v1/collections/{id}/sync/pause\|resume\|stop` | Có (202) | sync.go, `app.SyncAction` |
| `POST /v1/collections/{id}/runs` | Có (202 + Location) | sync.go, `app.RunNow` |
| `POST /v1/collections/{id}/audits` | Có (202 + `Location: /v1/audits/{id}`) | [audits.go](../internal/httpapi/audits.go), `app.RequestAudit` |
| `GET /v1/audits/{id}` | Có | audits.go, `app.Audit` |
| `GET /v1/audits/{id}/findings?kind&limit&cursor` | Có (phân trang) | audits.go, `app.AuditFindings` |
| `POST /v1/audits/{id}/repair` | Có (202 + Location, cần `expected_revision`, `dry_run`) | audits.go, `app.RequestRepair` |

Path param trong OpenAPI là `{collection_id}`/`{job_id}`, router dùng `{id}`;
matching theo vị trí nên không ảnh hưởng client.

## Mã lỗi ổn định

`invalid_request` (400), `unauthenticated` (401), `forbidden` (403),
`privacy`-free `not_found` (404), `stale_revision`/`plan_changed`/`plan_expired`/
`idempotency_mismatch`/`needs_action` (409), `capacity_exceeded`/`rate_limited`/
`provider_unavailable` (429/502), `maintenance` (503), `internal_error` (500).
Mapping: [internal/app/errors.go](../internal/app/errors.go).

## Chưa hỗ trợ (không có route)

- Watch/push endpoint (`/v1/watch`, webhook Telegram/Drive): không có.
- `/v1/status` chưa có due/backlog indicator chi tiết.
- Token HTTP riêng cho operator (`api_tokens`): chỉ bearer owner.
- Pagination cho `/v1/destinations`, `/v1/operators` (trả toàn bộ).
- Preview schedule qua API (client tự tính bằng `internal/schedule`).

## Realtime (`GET /v1/stream`)

Server-Sent Events chỉ mang **bộ đếm thay đổi**, không mang dữ liệu dòng:

- `event: hello` ngay khi nối, rồi `event: changed` mỗi khi daemon ghi trạng thái
  (mọi đường ghi của `internal/state` đều đi qua cùng một tín hiệu,
  [changes.go](../internal/state/changes.go)). Payload `{"version":N}`.
- Một loạt ghi trong 150 ms gộp thành một `changed`. Client đọc lại đúng thứ nó
  đang hiển thị; không có replay sau khi nối lại, nên mất một sự kiện chỉ làm
  chậm một lần làm mới.
- Comment `: ping` mỗi 15 giây; client coi im lặng >45 giây là đã mất kết nối.
- Tối đa 16 stream (`429 too_many_streams`); stream kết thúc khi daemon dừng.
- Máy chủ bỏ write-timeout 60 giây riêng cho route này.

CLI/TUI dùng [client_stream.go](../internal/cli/client_stream.go) (tự nối lại có
backoff 0,5→5 giây, báo trạng thái "live"/"reconnecting", rơi về poll 2 giây khi
mất stream). Test: [stream_test.go](../internal/httpapi/stream_test.go).

## Dừng daemon (`POST /v1/shutdown`)

Trả `202` rồi dừng sau ~200 ms: API ngừng nhận request, worker về điểm an toàn,
job đang chạy tiếp tục ở lần khởi động sau. Chỉ owner; `501` nếu daemon không có
hook dừng. Đây là thứ **Quit trên tray** gọi; TUI/CLI không bao giờ dừng daemon.
