# HTTP control contract

[OpenAPI](contracts/openapi.json) is the machine-readable contract. Routes live in
[internal/httpapi](../internal/httpapi); conformance tests validate responses.
There are 70 operations, including two health routes.

## Authentication and scope

The daemon normally binds to loopback. Every route except `/health/live` requires
`Authorization: Bearer TOKEN` from the private token file. The token maps to the
owner on the server; callers cannot supply their own principal in a request.
Telegram uses the same service with owner/operator scope.

Owner-only operations include shutdown/SSE, Drive browsing/search/mkdir,
destination administration, operators, Doctor/fixes, global queue controls and
`all=true` reads. Operators can only access their own collections/jobs/audits.
Foreign objects return not-found rather than exposing ownership metadata.

## Common protocol

Success is `{"data":...,"request_id":"req_..."}`. Errors contain
`error:{code,message,retryable,retry_after_seconds}` and `request_id`.

Mutations that declare `Idempotency-Key` require 1–128 printable characters.
Same actor/path/key and equivalent body replay the original admission with
`Idempotency-Replayed: true`; a different body returns `409 idempotency_mismatch`.
Queue pause/resume are state-idempotent; job moves are revision-guarded; shutdown
has its own explicit lifecycle semantics.

Use `expected_revision` and the resource-specific subscription revision fields.
A mismatch is `409 stale_revision`. `202` means durable admission, not successful
completion. HTTP request lifetime does not own a background job.

Opaque list cursors bind principal and filters. Reusing them with another filter
returns 400. Parameters named `{id}` in Go correspond positionally to the more
specific OpenAPI parameter names.

`503 maintenance` refuses operational mutations during maintenance. `503 suspending`
refuses admission during machine sleep transition. Rate-limit responses carry
Retry-After where appropriate.

## Routes

The table is derived from OpenAPI. Read each operation there for its exact request,
revision requirements, pagination bounds and response schema. A dash means that
no Idempotency-Key parameter is declared for that operation.

| Route | Success | Idempotency-Key |
| --- | --- | --- |
| `GET /health/live` | 200 | — |
| `GET /health/ready` | 200 | — |
| `GET /v1/status` | 200 | — |
| `GET /v1/plans/{plan_id}` | 200 | — |
| `POST /v1/plans/{plan_id}/preflight/cancel` | 200 | — |
| `POST /v1/plans` | 201 | — |
| `GET /v1/jobs` | 200 | — |
| `POST /v1/jobs` | 202 | — |
| `GET /v1/jobs/{job_id}` | 200 | — |
| `GET /v1/jobs/{job_id}/events` | 200 | — |
| `GET /v1/jobs/{job_id}/items` | 200 | — |
| `GET /v1/destinations` | 200 | — |
| `POST /v1/destinations` | 201 | — |
| `PUT /v1/destinations/default` | 200 | — |
| `GET /v1/account` | 200 | — |
| `POST /v1/account/auth-sessions` | 201 | — |
| `GET /v1/account/auth-sessions/{session_id}` | 200 | — |
| `POST /v1/account/revoke` | 202 | — |
| `GET /v1/preferences` | 200 | — |
| `PATCH /v1/preferences` | 200 | — |
| `GET /v1/operators` | 200 | — |
| `POST /v1/operators` | 201 | — |
| `DELETE /v1/operators/{principal_id}` | 204 | — |
| `GET /v1/doctor` | 200 | — |
| `POST /v1/doctor/fix` | 200 | — |
| `POST /v1/jobs/{job_id}/move` | 200 | — |
| `GET /v1/batches/{batch_id}` | 200 | — |
| `POST /v1/queue/pause` | 200 | — |
| `POST /v1/queue/resume` | 200 | — |
| `POST /v1/jobs/{job_id}/pause` | 202 | — |
| `POST /v1/jobs/{job_id}/resume` | 202 | — |
| `POST /v1/jobs/{job_id}/cancel` | 202 | — |
| `POST /v1/jobs/{job_id}/retry` | 202 | — |
| `GET /v1/collections` | 200 | — |
| `GET /v1/collections/{collection_id}` | 200 | — |
| `PUT /v1/collections/{collection_id}/sync` | 200 | — |
| `POST /v1/collections/{collection_id}/sync/pause` | 202 | — |
| `POST /v1/collections/{collection_id}/sync/resume` | 202 | — |
| `POST /v1/collections/{collection_id}/sync/stop` | 202 | — |
| `POST /v1/collections/{collection_id}/runs` | 202 | — |
| `GET /v1/stream` | 200 | — |
| `POST /v1/shutdown` | 202 | — |
| `GET /v1/drive/folders/{id}` | 200 | — |
| `GET /v1/drive/files/{id}` | 200 | — |
| `GET /v1/drive/search` | 200 | — |
| `POST /v1/drive/folders` | 201 | — |
| `POST /v1/collections/{collection_id}/audits` | 202 | — |
| `GET /v1/collections/{collection_id}/audits/latest` | 200 | — |
| `GET /v1/audits/{audit_id}` | 200 | — |
| `POST /v1/audits/{audit_id}/repair` | 202 | — |
| `GET /v1/audits/{audit_id}/findings` | 200 | — |
| `GET /v1/collections/{collection_id}/filter` | 200 | — |
| `PUT /v1/collections/{collection_id}/filter` | 200 | — |
| `POST /v1/collections/{collection_id}/retire` | 200 | — |
| `POST /v1/collections/{collection_id}/resume` | 200 | — |
| `POST /v1/collections/{collection_id}/forget` | 200 | — |
| `POST /v1/collections/{collection_id}/cleanup` | 202 | — |
| `GET /v1/history` | 200 | — |
| `GET /v1/setup` | 200 | — |
| `POST /v1/batches/{batch_id}/seal` | 200 | — |
| `GET /v1/collections/{collection_id}/protection` | 200 | — |
| `PUT /v1/collections/{collection_id}/protection` | 200 | Required |
| `GET /v1/collections/{collection_id}/snapshots` | 200 | — |
| `POST /v1/collections/{collection_id}/snapshots` | 202 | Required |
| `GET /v1/snapshots/{snapshot_id}` | 200 | — |
| `GET /v1/snapshots/{snapshot_id}/items` | 200 | — |
| `POST /v1/snapshots/{snapshot_id}/pin` | 200 | Required |
| `DELETE /v1/snapshots/{snapshot_id}/pin` | 200 | Required |
| `POST /v1/snapshots/{snapshot_id}/restore` | 202 | Required |
| `GET /v1/jobs/{job_id}/restore-plan` | 200 | — |

## Important mutation defaults

- CLI audit repair previews by default; raw HTTP repair with omitted `dry_run`
  applies. API clients must explicitly send their intended mode.
- CLI collection cleanup previews by default; raw cleanup omitting `dry_run`
  requires valid apply/name confirmation. `confirm_name` uses the actual Drive
  root name, including a conflict suffix; compatibility accepts the original name
  for older collections.
- Snapshot restore defaults to `dry_run:true` in both HTTP and CLI. Apply pauses
  sync atomically and does not resume it automatically.
- Option changes and snapshot mutations refuse an active collection writer,
  including paused/blocked work. Pinning uses snapshot revision.

Cleanup and repair only trash owned objects. No permanent Drive delete route exists.

## Read models

- `/v1/setup` returns a local prerequisite checklist without Google calls.
- Job reads include nullable queue position/ahead fields. They describe a changing
  fair-admission snapshot, not a promised start time.
- Job state filters accept trimmed, deduplicated comma-separated sets; invalid
  members return `invalid_request`. Cursor identity includes that set.
- Status includes the principal's local 24-hour copy ledger, `limit_known:false`
  when Google's limit is unknown, and nullable quota resume time.
- Audit findings expose only paths inside the authorized collection and shared
  explanations. An unmapped extra can have no relative path.
- Batch sealing marks the notification admission pass complete; it does not cancel
  jobs or prohibit later retry. Old daemons/clients can keep per-job notifications.
- `SyncSubscription.deletion_policy` defaults to `preserve_destination` and becomes
  `mirror_owned` when collection mirror is enabled.

Destination/operator lists currently return complete lists rather than paginated
backends. There are no separate operator HTTP tokens, public webhooks or schedule
preview endpoint; clients can calculate schedule previews locally.

## Server-Sent Events

`GET /v1/stream` carries only a **change counter**, never row data:

- `hello` on connect, then `changed` with `{"version":N}` after durable state writes.
- Writes coalesce over 150 ms. Clients re-read their current views; no replay log.
- A ping comment every 15 seconds; clients treat prolonged silence as disconnect.
- At most 16 concurrent streams; excess returns `429 too_many_streams`.
- The ordinary write timeout is disabled for this route. Daemon shutdown closes it.

The CLI reconnects with bounded backoff and falls back to polling when disconnected.
An SSE disconnect never stops a copy or the daemon.

## Shutdown

`POST /v1/shutdown` is owner-only, returns 202 then schedules graceful shutdown.
Without a shutdown hook it returns 501. Admission stops, workers reach safe points,
and unfinished work remains durable for startup. TUI exit/tray exit are separate
from the tray's explicit **Stop 502Drive…** action.

## Stable errors

| Status | Representative codes |
| --- | --- |
| 400 | invalid_request, invalid_filter, ambiguous_source, unsupported_source, destination_inside_source, destination_not_folder, not_a_folder, name_confirmation_mismatch |
| 401 | unauthenticated |
| 403 | forbidden, permission_denied, copy_not_permitted, account_reconnect_required |
| 404 | not_found, source_not_found |
| 409 | stale_revision, invalid_transition, plan_changed, plan_expired, idempotency_mismatch, needs_action, already_copied, collection_retired, collection_not_retired, collection_busy, destination_required, destination_unverifiable, destination_not_writable, callback_unavailable, oauth_not_configured, not_ready |
| 429 | capacity_exceeded, rate_limited, too_many_streams |
| 500/502/503 | internal_error, doctor_fix_failed, revoke_failed, provider_unavailable, maintenance, suspending |

Protection jobs can report `snapshot_protected`, `snapshot_unavailable`,
`mirror_incomplete` or `restore_incomplete`. Restore counts actions rather than
copy items. See [protection](features/protection.md) and
[error mapping](../internal/app/errors.go).
