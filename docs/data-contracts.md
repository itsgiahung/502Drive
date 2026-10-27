# SQLite data contracts

The authoritative schema is [internal/state/migrations](../internal/state/migrations).
`state.Open` applies migrations forward-only to `state-v2.db`. `user_version` is
the latest migration and `application_id=502002` identifies this database format.
Never modify an applied migration; add a new one.

There are 19 migration files: **001–011 and 020–027**. Numbers 012–019 are absent
and must not be reused casually as though they were new migrations.

## Storage conventions

- Opaque string IDs; database timestamps normally epoch milliseconds. JSON times
  use protocol timestamps and the copy ledger uses epoch hours.
- Complex resource bodies are JSON. Generated virtual columns are read-only views
  of that body, not a second source of truth.
- One write connection, WAL, foreign keys enabled and 5-second busy timeout.
- Revisions protect concurrent edits. Unknown values remain NULL, not invented zeroes.
- Only the daemon writes runtime state. Offline maintenance enforces the writer lock.

## Migration inventory

### 001_core.sql

`plans` stores principal, preview body and consumed job; plan TTL is ten minutes.
`collections` persists copied sources across runs. `jobs` stores principal,
collection, state and body; `one_active_run` excludes concurrent writers per
collection. `events` is append-only by sequence. `idempotency` is keyed by
principal/path/key and stores request hash and replay body.

### 002_engine.sql

| Table | Purpose and key fields |
| --- | --- |
| `settings` | Preferences, default destination, durable queue pause and notification markers |
| `destinations` | Folder/resource/drive IDs, name, revision and creation time |
| `items` | PK collection/source; mapping, generation, intent, destination/parent, attempts, next attempt, error, list/page checkpoints, create/align operation and seen/scan jobs |
| `sync_subscriptions` | One per collection; mode/state/schedule, schedule revision, due slot, cursor, rerun request, poll time and last outcome |
| `item_versions` | PK collection/source/generation; destination, fingerprint and current/superseded/trashed state |
| `sync_occurrences` | Unique collection/schedule revision/slot admission |

Item states include pending, executing, reconciling, done, blocked, failed, skipped
and cancelled; the historical CHECK also admits retry_wait. Runtime success is
`done` for items and `succeeded` for jobs. Indexes support job state and parent lookup.

### 003_principals.sql

`principals` stores kind/external ID, role owner/operator, enabled flag and creation
time. Kind/external ID is unique. HTTP owner is synthetic; Telegram operators use
numeric external user IDs.

### 004_telegram.sql

`telegram_meta` persists bot instance and next update offset. `telegram_inbox`
records updates before the offset advances. `callback_states` binds random IDs to
principal/chat/message/action/object/revision/payload, expiry and consumed state.
`notification_outbox` stores durable/coalesced delivery state, attempts, send time,
last hash/state and error. Callback mutation lifetime is bounded and one-shot.

### 005_notify.sql

`notify_markers(principal,subject)` suppresses repeated subscription errors until
code change or success. Adds the outbox notification note field.

### 006_audit.sql

`audits` stores collection/job, running/completed/failed state, counts/plan JSON,
revision, scan count and timestamps. `audit_findings` upserts on
`(audit_id,kind,source_id,dest_id)` with repairable/repaired/detail fields.
Job kind, audit ID and dry-run flags are additive JSON body fields.

### 007_sync_replay.sql

`sync_pending_changes(collection_id,seq)` buffers raw Changes pages and cursor-after
checkpoints during bootstrap. Subscription fields add periodic full reconciliation
(default 604800 seconds), next reconcile time and replay overflow.

### 008_copy_ledger.sql

`copy_ledger(principal,hour)` records confirmed logical bytes per hour. Completion
and ledger increment commit together; reads sum a rolling 24-hour window. It does
not measure Google's authoritative quota cap.

### 009_hot_queries.sql

Adds indexes `items_job_list`, `items_scan_list`, `sync_due_poll`, `sync_due_full`.
Subscription `replay_count`/`replay_bytes` are backfilled and maintained rather than
counting the buffer on every poll.

### 010_conflict_policy.sql

Adds `items.conflict_action` and `dest_name`, with `items_dest_name` lookup/reservation.
Existing mapped items retain their resolved names across retries.

### 011_selective_sync.sql

Adds `items.rel_path`, backfilled once through an indexed temporary table and
recursive traversal. `items_collection_kind_state` supports filtered empty-folder work.

### 020_collection_lifecycle.sql

Adds nullable collection `retired_at`, retirement index and indexed source/destination
lookup scoped by principal. Retirement is distinct from deleting remote files.

### 021_queue.sql

Generated job fields: `batch_id`, `queue_priority`, `queue_order`, `fair_group`.
`saveJob` remains the body source of truth. Indexes `jobs_batch`, `jobs_queue` and
`jobs_fair` support batch queries and fair priority admission.

### 022_h_api.sql

Adds repairable-finding and collection/state/job-list indexes. Newest-first keyset
pagination uses index row order rather than temporary sort trees.

### 023_after_copy_sync.sql

`after_copy_sync` is keyed by collection and stores principal, requested mode,
pending/retry state, attempts, next attempt, error/warning and timestamps. Initial
success and its pending request commit together; failures retry without changing
the completed copy's result.

### 024_report.sql

Ensures the job/state item index supports failed/blocked/skipped report paging.

### 025_history.sql

`jobs_history` indexes JSON `updated_at` converted to epoch milliseconds. History
keysets use `(updated_ms,rowid)`, avoiding lexical RFC3339 fractional-time issues.

### 026_job_bookkeeping.sql

`job_bookkeeping(job_id,created_at)` references jobs with cascade. Terminal job and
collection state commit with this marker. `RunFinished` clears it after subscription/
after-copy bookkeeping; startup replays unfinished markers. Non-initial audit/repair/
cleanup jobs do not overwrite the collection's baseline state.

### 027_protection.sql

| Table | Purpose |
| --- | --- |
| `snapshots` | Collection/job, capturing/ready, pin/revision/time/expiry; one per capture job |
| `snapshot_items` | PK snapshot/source; immutable item JSON, destination ID and captured content identity |
| `destination_actions` | PK job/ordinal; frozen restore/remove plan with pending/executing/done checkpoint |
| `collection_archives` | Durable parent/create intent, attempted flag and destination ID |

Destination lookup indexes protect referenced physical versions. Capture readiness
commits with job success; cancellation removes unfinished capture metadata.
Collection JSON adds `protection:{mirror,snapshots,keep_days,keep_count}` with defaults
false/false/30/10. Runs freeze options at admission; restore jobs carry snapshot ID.
Restored mapping generations never decrease and align retains the physical intent.
Future creates use a fresh higher generation rather than reusing an old operation.

## Notification markers and retention

Batch notices use existing settings keys for open/sealed/final markers and a unique
outbox coalesce key. `delivery_unknown` fences uncertain first-summary sends; no
exactly-once remote guarantee is claimed.

Prune preserves current mappings, collections and referenced jobs, including
snapshot capture jobs. **CLI prune applies unless `--dry-run` is specified**;
preview before changing history. Snapshot retention removes metadata only and
pinned points are exempt. Forget cascades collection metadata while keeping Drive
files. Offline cleanup only removes app temporary files. See
[operations](runbooks.md) and [maintenance implementation](../internal/maintenance).
