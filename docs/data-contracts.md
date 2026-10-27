# Data contracts (SQLite v2)

Schema thật nằm ở [internal/state/migrations](../internal/state/migrations), áp
forward-only khi `state.Open` mở `state-v2.db`; `user_version` = migration cao
nhất, `application_id=502002`. Không sửa migration đã áp; thêm file mới.

## Quy ước

- ID là string opaque (`state.ID(prefix)`), thời gian lưu epoch milliseconds,
  body phức tạp lưu JSON trong cột `body`.
- Một write connection (`MaxOpenConns=1`), WAL, `foreign_keys=1`,
  `busy_timeout=5000`.
- Revision tăng khi resource đổi; adapter phải gửi `expected_revision`.
- Unknown nullable, không dùng 0 thay "chưa rõ".

## 001_core.sql

| Bảng | Cột chính | Ghi chú |
| --- | --- | --- |
| `plans` | `id`, `principal`, `body`, `consumed_job` | Plan TTL 10 phút, một job/plan |
| `collections` | `id`, `principal`, `plan_id`, `body` | Bộ nguồn đã copy, sống qua nhiều run |
| `jobs` | `id`, `principal`, `collection_id`, `state`, `body` | Queue là bảng này; partial unique index chặn 2 active run/collection |
| `events` | `sequence`, `job_id`, `body` | Append-only, dùng cho `events`/NDJSON |
| `idempotency` | `(principal,path,key)`, `hash`, `body` | Replay/mismatch theo key |

## 002_engine.sql

| Bảng | Cột chính | Ghi chú |
| --- | --- | --- |
| `settings` | `key`, `value` | Default destination, preferences (`prefs:<principal>`), lịch báo cáo hằng ngày của Telegram (`tg_report:<principal>`, ngày đã gửi `tg_report_sent:<principal>`) |
| `destinations` | `id`, `folder_id`, `resource_key`, `drive_id`, `name`, `revision` | Đích đã validate, không unique theo tên |
| `items` | `(collection_id,source_id)`, `job_id`, `kind`, `state`, `generation`, `intent_key`, `dest_*`, `list_state`, `page_token`, `op`, `conflict_action`, `dest_name`, `rel_path` | Item = mapping + write intent; không unique theo tên. `dest_name` giữ tên dưới `dest_parent_id` (index `items_dest_name`); `rel_path` là đường dẫn tương đối để luật lọc theo đường dẫn (index `items_collection_kind_state`) |
| `sync_subscriptions` | `collection_id` PK, `mode`, `state`, `schedule`, `schedule_revision`, `content_policy`, `next_due`, `next_key`, `cursor`, `rerun_requested`, `next_poll_at`, `full_reconcile_every`, `next_full_reconcile_at`, `replay_overflow` | Một subscription/collection |
| `item_versions` | `(collection_id,source_id,generation)`, `dest_id`, `fingerprint`, `state` | Lịch sử version cho `replace_copy` |
| `sync_occurrences` | `(collection_id,schedule_revision,occurrence_key)` | Claim slot một lần |
| `sync_pending_changes` | `(collection_id,seq)` PK, `cursor_after`, `change_json` | Page feed lưu thô trong lúc bootstrapping; xoá khi replay/overflow |
| `copy_ledger` | `(principal,hour)` PK, `bytes` | Byte copy xong theo giờ, cho cửa sổ 24h và quota Q1 |

## 003_principals.sql

| Bảng | Cột chính | Ghi chú |
| --- | --- | --- |
| `principals` | `id`, `(kind,external_id)` UNIQUE, `role` owner/operator, `enabled` | Owner là principal tổng hợp `owner`; operator Telegram có `external_id` = user id |

## 004_telegram.sql

| Bảng | Cột chính | Ghi chú |
| --- | --- | --- |
| `telegram_meta` | `id=1`, `instance`, `next_update` | Offset chỉ tiến sau khi update đã ghi |
| `telegram_inbox` | `(bot,update_id)`, `body`, `state`, `attempts` | Raw update tối thiểu; replay sau restart |
| `callback_states` | `id`, `principal`, `chat_id`, `message_id`, `action`, `object`, `revision`, `payload`, `expires_at`, `consumed` | Nút ghi là one-shot, TTL 15 phút |
| `notification_outbox` | `id`, `kind` progress/final, `principal`, `job_id`, `chat_id`, `message_id`, `state`, `attempts`, `next_send`, `last_hash`, `last_state`, `note`, `coalesce_key` | Coalesce theo `coalesce_key`; `note` là mã lỗi/state cho thông báo |

## 005_notify.sql

| Bảng | Cột chính | Ghi chú |
| --- | --- | --- |
| `notify_markers` | `(principal,subject)`, `code`, `updated_at` | Lỗi lặp của một subscription chỉ báo một lần tới khi code đổi hoặc run thành công (xóa marker) |

## 006_audit.sql

| Bảng | Cột chính | Ghi chú |
| --- | --- | --- |
| `audits` | `id`, `collection_id`, `job_id`, `state` running/completed/failed, `counts` (JSON: mã finding → số), `plan` (JSON kế hoạch repair), `scanned`, `revision` | Một job audit/repair tương ứng một dòng; revision bảo vệ `POST /repair` |
| `audit_findings` | `(audit_id,kind,source_id,dest_id)` PK, `repairable`, `repaired`, `detail` | Upsert theo PK nên quét lại (restart) không nhân đôi |

`jobs` không thêm cột: `kind` (`audit`/`repair`), `audit_id`, `dry_run` là trường
mới trong body JSON, `omitempty` — job copy cũ đọc lên vẫn đúng. Audit/repair
chung ràng buộc `one_active_run` theo collection với mọi job khác.

## Ownership và vòng đời

- `runtime state` (jobs/items/events/outbox) chỉ daemon ghi; CLI/TUI/tray/
  Telegram không mở DB.
- Retention MVP: giữ job/event/idempotency/mapping. `cleanup` chỉ xóa file tạm
  do chương trình tạo, không xóa DB/WAL/key/backup ([cleanup.go](../internal/maintenance/cleanup.go)).
- Backup dùng `VACUUM INTO` + key + token + config ([maintenance.go](../internal/maintenance/maintenance.go));
  restore kiểm sha256, `integrity_check`, `foreign_key_check`, schema ≤ binary.
