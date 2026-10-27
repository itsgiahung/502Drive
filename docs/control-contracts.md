# HTTP contract v1

Router: [internal/httpapi](../internal/httpapi) (`server.go` + `sync.go`,
`collection_lifecycle.go`, `audits.go`, `drive.go`, `filter.go`, `history.go`).
Nguồn máy đọc: [docs/contracts/openapi.json](contracts/openapi.json); conformance
test [openapi_test.go](../internal/httpapi/openapi_test.go) so khớp mọi response
test với schema. Danh sách route dưới đây lấy từ `rg 'route\("' internal/httpapi`.

## Nguyên tắc chung

- Bind loopback (config `listen`), **luôn** cần `Authorization: Bearer <token>`
  trừ `/health/live`. Token đọc từ `token_file` (0600), map sang `owner` server-side.
- Response: `{"data": ..., "request_id": "req_..."}`; lỗi:
  `{"error":{"code","message","retryable","retry_after_seconds"},"request_id":...}`.
- Mutation có `Idempotency-Key` (1–128 ký tự in được): plans, jobs, job actions,
  destinations, login/revoke, preferences, operators, mkdir, sync, audits/repair,
  lifecycle. Cùng key + cùng body → replay (`Idempotency-Replayed: true`); khác body
  → `409 idempotency_mismatch`.
- **Không** dùng idempotency: `POST /v1/queue/pause|resume` (idempotent theo trạng
  thái), `POST /v1/jobs/{id}/move` (revision-guarded), shutdown.
- Revision: `expected_revision` (job/collection/destination) và
  `expected_subscription_revision` (sync). Lệch → `409 stale_revision`.
- `202` = đã admit, **không** phải xong. `503 maintenance` khi daemon chạy
  `--maintenance` (từ chối jobs, sync, runs, audits/repair, filter, mkdir);
  `503 suspending` trong ~2 giây trước khi máy ngủ.
- Cursor list là opaque, gắn principal + filter; dùng lại sai filter → `400`.

## Route

| Route | Quyền | Mã / ghi chú |
| --- | --- | --- |
| `GET /health/live` | không auth | `200 {"status":"alive"}` |
| `GET /health/ready` | auth | `200 ready, schema_version, worker_ready`; `409 not_ready` |
| `GET /v1/status` | auth | `version, schema_version, ready, account, bot_state, running_jobs, queued_jobs, blocked_jobs, lifecycle, throttle{workers_now,throttled_last_5m,next_retry_at}, queue{paused}` |
| `GET /v1/stream` | owner | SSE; xem dưới |
| `POST /v1/shutdown` | owner | `202`, chỉ khi daemon có hook; `501` nếu không |
| `POST /v1/plans` | auth | `201`; body `source, destination_id, filter?, deep?`; `409 plan_changed/plan_expired`, `403 copy_not_permitted`, `409 destination_not_writable` |
| `GET /v1/plans/{plan_id}` | auth | `200`; deep scan `preview.deep.state=running/complete/cancelled/interrupted` |
| `POST /v1/plans/{plan_id}/preflight/cancel` | auth | `200`; plan vẫn dùng được như preview nông |
| `POST /v1/jobs` | auth | `202 + Location`; body `plan_id, expected_revision, mode?, batch_id?, after_copy_sync_mode?`; `409 already_copied/collection_retired`, `429 capacity_exceeded` |
| `GET /v1/jobs?state&collection_id&limit&cursor&all` | auth | phân trang; `all=true` owner thấy mọi principal |
| `GET /v1/jobs/{id}` | auth | `200 jobDTO` (counts, result_url, resume_after, revision…) |
| `GET /v1/jobs/{id}/items?problems&limit&cursor` | auth | `problems=true` = failed/blocked/skipped kèm `code` |
| `GET /v1/jobs/{id}/events?after_sequence&limit` | auth | NDJSON qua CLI; `next_sequence` |
| `POST /v1/jobs/{id}/pause\|resume\|cancel\|retry` | auth | `202`; cần `expected_revision`; `409 invalid_transition` |
| `POST /v1/jobs/{id}/move` | auth | `200`; body `expected_revision` + một trong `before/after/priority`; `400` nếu sai tổ hợp |
| `GET /v1/batches/{batch_id}` | auth | `200`; `total, completed_jobs, by_state, items` |
| `POST /v1/queue/pause` / `POST /v1/queue/resume` | owner | `200 {paused, changed, affected_jobs}`; cờ bền `settings.queue_paused` |
| `GET /v1/history?since&limit&cursor&all` | auth | export job; `since` RFC3339/`YYYY-MM-DD`, keyset `(updated_ms,rowid)` |
| `GET /v1/drive/folders/{id}?resource_key&page_token` | owner | `root` = My Drive; breadcrumb ở trang đầu |
| `GET /v1/drive/files/{id}?resource_key` | owner | metadata một mục |
| `GET /v1/drive/search?q` | owner | tối đa 20 kết quả, chỉ đọc |
| `POST /v1/drive/folders` | owner | `201`; body `parent_id,name`; idempotent theo key+parent+tên |
| `GET /v1/destinations` | auth | toàn bộ, `next_cursor:null` |
| `POST /v1/destinations` | auth | `201`; validate quyền ghi đích |
| `PUT /v1/destinations/default` | auth | `200`; body `destination_id,expected_revision` |
| `GET /v1/account` | auth | trạng thái kết nối |
| `POST /v1/account/auth-sessions` | auth | `201`; body `no_browser?` |
| `GET /v1/account/auth-sessions/{id}` | auth | trạng thái phiên login |
| `POST /v1/account/revoke` | auth | `202`; revoke phía Google + local |
| `GET /v1/preferences` | auth | language, timezone, notify, quiet hours, auto_copy, after_copy_sync_mode… |
| `PATCH /v1/preferences` | auth | `200`; validate HH:MM/timezone |
| `GET /v1/operators`, `POST /v1/operators`, `DELETE /v1/operators/{id}` | owner | `201`/`204`; operator Telegram |
| `GET /v1/doctor?online` | owner | checks; `doctor_fix_failed` 500 |
| `POST /v1/doctor/fix` | owner | `200 {fixed, checks}`; chỉ sửa lỗi cục bộ an toàn (quyền secret, thư mục thiếu) |
| `GET /v1/collections?limit&cursor&query&sync_mode&all&include_retired` | auth | `sync_mode=off/scheduled/realtime`; phân trang thật |
| `GET /v1/collections/{id}` | auth | kèm `sync` thật, `filter`, `revision`, `retired_at` |
| `GET /v1/collections/{id}/filter` | auth | `{collection_id, revision, filter}` |
| `PUT /v1/collections/{id}/filter` | auth | `200`; body `filter`, `expected_revision`; `400 invalid_filter` |
| `POST /v1/collections/{id}/retire` | auth | `200`; `409 collection_busy` khi còn job chạy |
| `POST /v1/collections/{id}/resume` | auth | `200`; đưa retired → ready |
| `POST /v1/collections/{id}/forget` | auth | `204`; chỉ khi đã retired (`409 collection_not_retired`) |
| `POST /v1/collections/{id}/cleanup` | auth | `202 + Location`; job `kind=cleanup`; body `expected_revision`, `dry_run`, `confirm_name`; `dry_run` vắng = false nên phải khớp `confirm_name`, CLI mặc định dry-run; `400 name_confirmation_mismatch` |
| `PUT /v1/collections/{id}/sync` | auth | `200`; cần `expected_revision` + `expected_subscription_revision`; `503 maintenance` |
| `POST /v1/collections/{id}/sync/pause\|resume\|stop` | auth | `202`; revision-guarded |
| `POST /v1/collections/{id}/runs` | auth | `202 + Location`; coalesce với run đang chạy |
| `POST /v1/collections/{id}/audits` | auth | `202 + Location`; job audit chỉ đọc |
| `GET /v1/collections/{id}/audits/latest` | auth | audit mới nhất, `404` nếu chưa có |
| `GET /v1/audits/{id}` | auth | counts, `repairable_findings`, `scanned_folders`, `plan` |
| `GET /v1/audits/{id}/findings?kind&limit&cursor` | auth | `limit` 1–500 (mặc định 50); cursor số nguyên |
| `POST /v1/audits/{id}/repair` | auth | `202 + Location`; body `expected_revision`, `dry_run`; vắng = false (apply), CLI `audit --repair` mặc định dry-run |

Path param trong OpenAPI là `{collection_id}`/`{job_id}`, router dùng `{id}`;
matching theo vị trí nên không ảnh hưởng client.

## Mã lỗi ổn định

- 400: `invalid_request`, `invalid_filter`, `ambiguous_source`,
  `unsupported_source`, `destination_inside_source`, `destination_not_folder`,
  `not_a_folder`, `name_confirmation_mismatch`.
- 401 `unauthenticated`; 403 `forbidden`, `permission_denied`,
  `copy_not_permitted`, `account_reconnect_required`.
- 404: `not_found`, `source_not_found`.
- 409: `stale_revision`, `invalid_transition`, `plan_changed`, `plan_expired`,
  `idempotency_mismatch`, `needs_action`, `already_copied`, `collection_retired`,
  `collection_not_retired`, `collection_busy`, `destination_required`,
  `destination_unverifiable`, `destination_not_writable`, `callback_unavailable`,
  `oauth_not_configured`, `not_ready`.
- 429: `capacity_exceeded` (retryable), `rate_limited` (retryable, `Retry-After`),
  `too_many_streams`.
- 502 `provider_unavailable`; 503 `maintenance`/`suspending`; 500 `internal_error`,
  `doctor_fix_failed`, `revoke_failed`.
Mapping: [internal/app/errors.go](../internal/app/errors.go).

## Chưa hỗ trợ (không có route)

- Watch/push endpoint (`/v1/watch`, webhook Telegram/Drive): không có.
- Token HTTP riêng cho operator (`api_tokens`): chỉ bearer owner.
- Pagination cho `/v1/destinations`, `/v1/operators` (trả toàn bộ).
- Preview schedule qua API (client tự tính bằng `internal/schedule`).
- Xóa vĩnh viễn trên Drive: không có route nào; cleanup/repair chỉ trash.

## Realtime (`GET /v1/stream`)

Server-Sent Events chỉ mang **bộ đếm thay đổi**, không mang dữ liệu dòng:

- `event: hello` ngay khi nối, rồi `event: changed` mỗi khi daemon ghi trạng thái
  (mọi đường ghi của `internal/state` đều đi qua cùng một tín hiệu,
  [changes.go](../internal/state/changes.go)). Payload `{"version":N}`.
- Một loạt ghi trong 150 ms gộp thành một `changed`. Client đọc lại đúng thứ nó
  đang hiển thị; không có replay sau khi nối lại.
- Comment `: ping` mỗi 15 giây; client coi im lặng >45 giây là đã mất kết nối.
- Tối đa 16 stream (`429 too_many_streams`); stream kết thúc khi daemon dừng.
- Máy chủ bỏ write-timeout 60 giây riêng cho route này.

CLI/TUI dùng [client_stream.go](../internal/cli/client_stream.go) (tự nối lại có
backoff 0,5→5 giây, báo "live"/"reconnecting", rơi về poll 2 giây khi mất stream).
Test: [stream_test.go](../internal/httpapi/stream_test.go).

## Dừng daemon (`POST /v1/shutdown`)

Trả `202` rồi dừng sau ~200 ms: API ngừng nhận request, worker về điểm an toàn,
job đang chạy tiếp tục ở lần khởi động sau. Chỉ owner; `501` nếu daemon không có
hook dừng. Đây là thứ **Quit trên tray** gọi; TUI/CLI không bao giờ dừng daemon.

- U1: `GET /v1/setup` (owner) returns `{ready,checks}` from Doctor's database,
  OAuth client, Google account and default destination checks; no Google calls.

- U2: job reads add nullable `queue_position` (>=1), `queue_ahead` (>=0) and
  `queue_paused`. Queue positions follow successive fair admissions.

- U5: audit findings add `path` (empty if no mapping) and shared `explanation`.
  Paths resolve only inside the authorized audit collection. Repair routes unchanged.

- U7: `GET /v1/jobs?state=a,b,c` accepts multiple states (trimmed, deduplicated,
  canonical order); invalid members return 400 `invalid_request`. Cursor remains
  bound to the state set, principal and collection. Missing query is unchanged.

- U9: status adds `quota:{used_24h_bytes,limit_known,resume_after}` for the
  requesting principal. `limit_known=false`: the ledger does not establish the
  actual Google limit. Resume is nullable, taken from quota-blocked jobs.

- U10: `POST /v1/batches/{batch_id}/seal` marks this principal's admission pass
  complete for notifications; idempotent, `{sealed:true}`. Missing route on old
  daemons is tolerated. It changes no copy job and does not stop later retries.
