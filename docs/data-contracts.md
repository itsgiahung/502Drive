# Data contracts (SQLite v2)

Schema thật nằm ở [internal/state/migrations](../internal/state/migrations), áp
forward-only khi `state.Open` mở `state-v2.db`; `user_version` = migration cao
nhất, `application_id=502002`. Không sửa migration đã áp; thêm file mới.

Danh sách hiện có: **001–011 và 020–025** (17 file). Dải 012–019 không tồn tại —
số được giữ chỗ cho các đợt trước, đừng tạo lại số cũ.

## Quy ước

- ID là string opaque (`state.ID(prefix)`), thời gian lưu epoch milliseconds
  (trừ `jobs.body.updated_at` dạng RFC3339 và `copy_ledger.hour` epoch giờ).
- Body phức tạp lưu JSON trong cột `body`; cột sinh (`GENERATED ... VIRTUAL`)
  chỉ đọc, không ghi.
- Một write connection (`MaxOpenConns=1`), WAL, `foreign_keys=1`,
  `busy_timeout=5000`.
- Revision tăng khi resource đổi; adapter phải gửi `expected_revision`.
- Unknown là NULL, không dùng 0 thay "chưa rõ".

## 001_core.sql

| Bảng | Cột chính | Ghi chú |
| --- | --- | --- |
| `plans` | `id`, `principal`, `body`, `consumed_job` | Plan TTL 10 phút, một job/plan |
| `collections` | `id`, `principal`, `plan_id`, `body` | Bộ nguồn đã copy, sống qua nhiều run; `retired_at` thêm ở 020 |
| `jobs` | `id`, `principal`, `collection_id`, `state`, `body` | Hàng đợi chính. `jobs_state` cho list; partial unique `one_active_run` chặn hai run active/collection |
| `events` | `sequence`, `job_id`, `body` | Append-only, dùng cho `events`/NDJSON; `events_job(job_id,sequence)` |
| `idempotency` | `(principal,path,key)`, `hash`, `body` | Replay/mismatch theo key |

## 002_engine.sql

| Bảng | Cột chính | Ghi chú |
| --- | --- | --- |
| `settings` | `key`, `value` | Default destination, preferences (`prefs:<principal>`), báo cáo Telegram (`tg_report:*`), `queue_paused` |
| `destinations` | `id`, `folder_id`, `resource_key`, `drive_id`, `name`, `revision`, `created_at` | Đích đã validate, không unique theo tên |
| `items` | PK `(collection_id,source_id)`, `job_id`, `kind` wrapper/folder/file/shortcut, `state`, `generation`, `intent_key`, `dest_parent_id`, `dest_id`, `attempts`, `next_attempt_at`, `error_code`, `list_state`, `page_token`, `op` create/align, `seen_job`, `scan_job`, `old_dest_id` | Item = mapping + write intent. State (CHECK): pending/executing/reconciling/done/retry_wait/blocked/failed/skipped/cancelled; code hiện chỉ ghi pending/executing/reconciling/done/blocked/failed/skipped/cancelled (`internal/state/model.go`). Index `items_job_state`, `items_parent` |
| `sync_subscriptions` | `collection_id` PK, `id`, `mode`, `state`, `schedule`, `schedule_revision`, `content_policy`, `revision`, `next_due`, `next_key`, `cursor`, `rerun_requested`, `next_poll_at`, `last_*` | Một subscription/collection; state waiting_for_baseline…error |
| `item_versions` | `(collection_id,source_id,generation)`, `dest_id`, `fingerprint`, `state` current/superseded/trashed | Lịch sử version cho `replace_copy`/repair |
| `sync_occurrences` | `(collection_id,schedule_revision,occurrence_key)`, `job_id` | Claim slot một lần |

## 003_principals.sql

| Bảng | Cột chính | Ghi chú |
| --- | --- | --- |
| `principals` | `id`, `(kind,external_id)` UNIQUE, `role` owner/operator, `enabled`, `created_at` | Owner là principal tổng hợp `owner`; operator Telegram có `external_id` = user id |

## 004_telegram.sql

| Bảng | Cột chính | Ghi chú |
| --- | --- | --- |
| `telegram_meta` | `id=1`, `instance`, `next_update` | Offset chỉ tiến sau khi update đã ghi |
| `telegram_inbox` | `(bot,update_id)`, `body`, `state`, `attempts`, `error_code`, `received_at` | Raw update tối thiểu; replay sau restart; index `telegram_inbox_state` |
| `callback_states` | `id`, `principal`, `chat_id`, `message_id`, `action`, `object`, `revision`, `payload`, `expires_at`, `consumed` | Nút ghi là one-shot, TTL 15 phút; index hết hạn |
| `notification_outbox` | `id`, `kind` progress/final, `principal`, `job_id`, `chat_id`, `state`, `attempts`, `next_send`, `last_hash`, `last_state`, `error_code`, `coalesce_key`, `note` | Coalesce theo `coalesce_key`; `note` là mã lỗi/state khi gửi |

## 005_notify.sql

| Bảng | Cột chính | Ghi chú |
| --- | --- | --- |
| `notify_markers` | `(principal,subject)`, `code`, `updated_at` | Lỗi lặp của một subscription chỉ báo một lần tới khi code đổi hoặc run thành công (xóa marker) |

`ALTER notification_outbox ADD note` (cùng file).

## 006_audit.sql

| Bảng | Cột chính | Ghi chú |
| --- | --- | --- |
| `audits` | `id`, `collection_id`, `job_id`, `state` running/completed/failed, `counts` JSON, `plan` JSON, `scanned`, `revision`, `started_at`, `finished_at` | Một job audit/repair tương ứng một dòng; revision bảo vệ `POST /repair`; index `audits_collection` |
| `audit_findings` | PK `(audit_id,kind,source_id,dest_id)`, `repairable`, `repaired`, `detail` | Upsert theo PK nên quét lại (restart) không nhân đôi; index `audit_findings_kind` |

`jobs` không thêm cột: `kind` (`audit`/`repair`/`cleanup`), `audit_id`, `dry_run`
là trường mới trong body JSON, `omitempty`.

## 007_sync_replay.sql

| Bảng / thay đổi | Ghi chú |
| --- | --- |
| `sync_pending_changes(collection_id,seq)`, `cursor_after`, `change_json` | Page feed lưu thô trong lúc bootstrapping; xoá khi replay/overflow |
| `sync_subscriptions` +`full_reconcile_every` (giây, mặc định 604800), `next_full_reconcile_at`, `replay_overflow` | Slot full reconcile định kỳ |

## 008_copy_ledger.sql

| Bảng | Ghi chú |
| --- | --- |
| `copy_ledger(principal,hour)` PK, `bytes` | Byte copy xong theo giờ, cho cửa sổ 24h và quota Q1 |

## 009_hot_queries.sql

Index cho hot path (đo bằng `EXPLAIN QUERY PLAN`):
`items_job_list(job_id,kind,list_state)`, `items_scan_list(scan_job,kind,list_state)`,
`sync_due_poll(mode,state,next_poll_at)`, `sync_due_full(mode,state,next_full_reconcile_at)`.
Thêm cột `sync_subscriptions.replay_count`, `replay_bytes` (đếm sẵn, không
`COUNT(*)` mỗi poll) và backfill một lần.

## 010_conflict_policy.sql

`items` + `conflict_action` (skip/rename/update), `dest_name`
(index `items_dest_name(dest_parent_id,dest_name)` giữ chỗ tên trong thư mục đích).
Backfill: item đã có `dest_id` được giữ tên hiện tại.

## 011_selective_sync.sql

`items` + `rel_path` (đường dẫn dưới gốc đã copy), backfill bằng CTE đệ quy một
lần qua bảng tạm; index `items_collection_kind_state(collection_id,kind,state)`
cho prune thư mục rỗng sau lọc.

## 020_collection_lifecycle.sql

`collections` + `retired_at` (NULL = chưa retire), index `collections_retired`.
Index `collections_source_dest(principal, json_extract(body,'$.source_id'),
json_extract(body,'$.destination_id'))` cho tra bản sao cũ (L2) không quét bảng.

## 021_queue.sql

`jobs` + cột sinh từ body (một nguồn sự thật là `saveJob`):
`batch_id`, `queue_priority`, `queue_order`, `fair_group`
(`batch_id` hoặc `collection_id`). Index `jobs_batch` (partial), `jobs_queue(state,queue_priority,queue_order)`,
`jobs_fair(state,fair_group)`.

## 022_h_api.sql

Index cho truy vấn API: `audit_findings_repairable(audit_id) WHERE repairable=1`,
`jobs_collection(collection_id,state)`, `jobs_collection_rowid(collection_id)`,
`jobs_state_rowid(state)` (rowid ở cuối index để phân trang newest-first không
cần temp B-tree).

## 023_after_copy_sync.sql

| Bảng | Cột chính | Ghi chú |
| --- | --- | --- |
| `after_copy_sync` | `collection_id` PK, `principal`, `mode`, `state` pending, `attempts`, `next_attempt_at`, `last_error`, `warning`, `created_at`, `updated_at` | Ghi cùng transaction với kết quả job initial (S8); retry 5 lần; index `after_copy_sync_due` |

## 024_report.sql

`items_job_state(job_id,state)` — đọc item failed/blocked/skipped của một job,
rowid giữ cursor ổn định.

## 025_history.sql

`jobs_history(CAST((julianday(json_extract(body,'$.updated_at')) - 2440587.5) * 86400000 AS INTEGER))`
— export lịch sử theo `(updated_ms,rowid)`, tránh so sánh chuỗi RFC3339 có/không
phần lẻ giây.

## Ownership và vòng đời

- Runtime state (jobs/items/events/ledger/outbox) chỉ daemon ghi; CLI/TUI/tray/
  Telegram không mở DB. `maintenance` (backup/restore/cleanup/prune/vacuum) là
  ngoại lệ offline, từ chối khi `instance.lock` đang bị giữ.
- Retention: `maintenance prune --older-than 90d` xóa job đã xong + event +
  intent cũ + inbox/outbox cũ + plan hết hạn, **giữ** collection, mapping đang
  dùng và job còn được tham chiếu (items/plans/sync_occurrences). Mặc định dry-run.
- `cleanup` chỉ xóa file tạm do chương trình tạo, không xóa DB/WAL/key/backup
  ([cleanup.go](../internal/maintenance/cleanup.go)).
- Backup dùng `VACUUM INTO` + key + token + config ([maintenance.go](../internal/maintenance/maintenance.go));
  restore kiểm sha256, `integrity_check`, `foreign_key_check`, schema ≤ binary.
