# S1 — Integrity Audit & Repair

Thiết kế cho đặc tả §S1. Trạng thái gốc: commit `afea0dc`.

## Mục tiêu

Đối chiếu nguồn ↔ đích của một collection, **báo** mọi sai lệch bằng mã ổn định,
và **sửa** an toàn/idempotent các loại có thể sửa — không bao giờ xóa vĩnh viễn,
không đụng đối tượng không có dấu `502drive_intent`.

## Dữ liệu (migration `006_audit.sql`, forward-only)

```sql
CREATE TABLE audits(
  id TEXT PRIMARY KEY,
  collection_id TEXT NOT NULL REFERENCES collections(id),
  job_id TEXT NOT NULL REFERENCES jobs(id),
  state TEXT NOT NULL,               -- running | completed | failed
  counts TEXT NOT NULL DEFAULT '{}', -- JSON: mã finding → số lượng
  plan TEXT NOT NULL DEFAULT '',     -- JSON: kế hoạch/số liệu lần repair gần nhất
  scanned INTEGER NOT NULL DEFAULT 0,
  revision INTEGER NOT NULL DEFAULT 1,
  started_at INTEGER NOT NULL,
  finished_at INTEGER
);
CREATE INDEX audits_collection ON audits(collection_id, started_at);

CREATE TABLE audit_findings(
  audit_id TEXT NOT NULL REFERENCES audits(id) ON DELETE CASCADE,
  kind TEXT NOT NULL,
  source_id TEXT NOT NULL DEFAULT '',
  dest_id TEXT NOT NULL DEFAULT '',
  repairable INTEGER NOT NULL DEFAULT 0,
  repaired INTEGER NOT NULL DEFAULT 0,
  detail TEXT NOT NULL DEFAULT '',
  PRIMARY KEY(audit_id, kind, source_id, dest_id)
);
CREATE INDEX audit_findings_kind ON audit_findings(audit_id, kind, repairable);
```

`jobs`/`collections` **không đổi cột**: `kind` (`audit`|`repair`), `audit_id`,
`dry_run` là trường mới trong body JSON của job (`state.Job`, `omitempty` — job copy
cũ đọc lên vẫn đúng).

## Hai job bền vững trên engine hiện có

### Audit — kind `audit`, chỉ ĐỌC Drive

`run.execute` branch sớm theo `job.Kind`. Hai lượt BFS **chỉ đọc** (`ListChildren`
tái dùng `listPage` — có backoff/tôn trọng Retry-After, không ghi Drive):

1. **Cây nguồn** từ `collection.SourceID` (collection một file: chỉ `GetFile`) →
   bản đồ nguồn (id → name/size/md5/parent/kind).
2. **Cây đích** từ `collection.RootID` → bản đồ đích + chỉ mục `intent` (mọi object
   có `502drive_intent`).

So với mapping `items` (state `done`, có `dest_id`):

| Mã | Điều kiện | Repairable |
| --- | --- | --- |
| `source_removed` | item mapping nhưng nguồn không còn | không |
| `unreadable_source` | list/get nguồn lỗi quyền (sau retry) | không |
| `missing_at_destination` | nguồn còn, `dest_id` không có trong cây đích | có (copy lại) |
| `name_mismatch` | đích tồn tại, tên ≠ tên nguồn | có nếu object đích có intent của ta |
| `wrong_parent` | đích tồn tại, parent ≠ `dest_parent` đã map | có nếu có intent của ta |
| `size_or_md5_mismatch` | file đích ≠ size/md5 nguồn (khi cả hai biết) | có nếu có intent của ta |
| `duplicate_at_destination` | object đích khác `dest_id` nhưng cùng intent với một item | có (trash bản thừa) |
| `extra_at_destination` | object trong cây đích không có intent và không nằm trong mapping | **không — chỉ báo** |

Tiến độ: `audits.scanned/counts` (đọc qua `GET /v1/audits/{id}`); job không có
items nên `job.counts` không phản ánh audit — repair thì có (items thuộc repair
job). Kết thúc: `audits.state=completed`,
`job=succeeded` (hoặc `partial` nếu có thư mục không đọc được). Findings ghi
`INSERT OR REPLACE` theo PK — **restart giữa audit chỉ quét lại, an toàn/idempotent**
(không checkpoint frontier: đọc-only, quét lại rẻ hơn lưu trạng thái nửa chừng).

### Repair — kind `repair`, tham chiếu `audits/{id}`

Ba pha, dùng lại đường copy hiện có (intent + reconcile — crash-safe):

1. **Plan** (luôn chạy, cả dry-run): đọc findings `repairable=1`, **kiểm lại từng
   finding với trạng thái hiện tại** (finding có thể đã cũ), sinh danh sách hành
   động; ghi JSON vào `audits.plan`, `audits.revision++`. Dry-run dừng ở đây.
2. **Apply**: đẩy việc vào `items` của collection (đúng bảng mapping, đúng
   `intent_key`, `job_id` = repair job) rồi chạy `copyAll` hiện có:
   - `missing_at_destination` → `dest_id='', op=create, state=pending`;
   - `size_or_md5_mismatch` → như trên + `old_dest_id` = bản cũ (để dọn);
   - `name_mismatch`/`wrong_parent` (chỉ khi intent khớp) → `op=align` (rename/move
     idempotent sẵn có);
   - write không rõ kết quả đi `reconcile`/`FindByIntent` như copy thường.
3. **Cleanup + finalize**:
   - bản cũ sau khi copy lại (`old_dest_id` ≠ `dest_id` mới, item `done`) →
     `GetFile` xác nhận **có intent của ta** rồi `TrashFile` (404 = coi như xong);
   - `duplicate_at_destination` → xác nhận bản được map vẫn đúng (tên+size) rồi
     trash bản thừa (cùng intent);
   - bản cũ (`item_versions` superseded): chỉ trash bản do chính lần repair này
     thay thế; dưới `versioned_copy` các generation cũ là lịch sử của người dùng
     và được giữ. `replace_copy` trash mọi bản cũ như sync;
   - đánh dấu `repaired=1` trên finding đã xử lý xong; job `succeeded`/`partial` (hoặc `blocked`
     `unknown_remote_outcome` khi còn mục chưa đối soát được, `partial` `findings_unverified`
     khi không đọc được Drive để kiểm chứng; retry sẽ kiểm lại);
     `extra_at_destination`/`source_removed`/`unreadable_source` không bao giờ đụng.

Restart repair: `ActiveJobs` nhặt lại job → plan chạy lại (kiểm lại từng finding,
những cái đã sửa bị loại) → apply/cleanup idempotent. **Hai lần repair không tạo
thêm gì.** Crash giữa copy và trash: hai object cùng intent tạm thời → lần repair
kế trash đúng bản cũ; audit kế báo `duplicate_at_destination` nếu chưa dọn.

## Admission và khoá

- Cả hai đi qua `s.idempotent` (admission commit trước "accepted", replay trả cùng
  job); audit cần `Idempotency-Key`, repair cần thêm `expected_revision` của audit
  (lệch → 409 `stale_revision`).
- **Cùng ràng buộc `one_active_run` theo collection**: không audit/repair khi
  collection đang có job active (copy/sync/audit/repair) và ngược lại → 409
  `invalid_transition` kèm thông điệp rõ. Đây là hàng rào chống hai worker cùng
  mutations một cây.
- Repair chỉ nhận khi `audits.state=completed`. Repair của audit cũ sau khi cây đã
  đổi: plan kiểm lại từng finding nên chỉ sửa cái còn sai.

## Bề mặt

| Bề mặt | Hình thức |
| --- | --- |
| `POST /v1/collections/{id}/audits` | 202 + `Location: /v1/audits/{id}`, cần `Idempotency-Key`; 503 maintenance/suspending như job thường |
| `GET /v1/audits/{id}` | `{id, collection_id, job_id, state, counts{kind→n}, scanned, revision, started_at, finished_at, plan?}` |
| `GET /v1/audits/{id}/findings?kind&limit&cursor` | phân trang `{items, next_cursor}`; cursor opaque offset |
| `POST /v1/audits/{id}/repair` | body `{expected_revision, dry_run}`; 202 + `Location: /v1/jobs/{id}` |
| CLI | `502drive audit COLLECTION [--repair [--dry-run|--apply]] [--json]`; `audit show ID [--kind K] [--limit N] [--all] [--json]` |
| Telegram | nút "🔍 Kiểm tra toàn vẹn" trên thẻ collection → tóm tắt finding + nút Sửa (file mới, không đụng `flow.go`) |

CLI **mặc định dry-run khi `--repair`** (an toàn dữ liệu trước hết); thực sửa cần
`--apply` (tài liệu trong `cli.md`). Không prompt; ngoài TTY không cần `--yes` vì
mặc định không ghi. Audit thường: mặc định **chờ** job xong rồi in báo cáo (đọc
only, an toàn); `--json` in envelope chuẩn.

## Chế độ lỗi

| Cửa sổ | Ứng xử |
| --- | --- |
| Kill giữa audit | job stranded → restart quét lại; findings `INSERT OR REPLACE` |
| Kill giữa copy-lại và trash bản cũ | hai bản cùng intent; repair kế trash bản cũ (có intent); audit kế báo duplicate nếu còn |
| Repair hai lần | lần hai: mọi finding đã hết → plan rỗng, không đụng Drive |
| Finding cũ (cây đã đổi) | plan kiểm lại; cái không còn sai bị bỏ qua |
| Collection đang có run active | 409, không xếp hàng — người dùng thử lại sau |
| `extra_at_destination` | chỉ báo, repair không đụng (kể cả khi trông giống file của ta nhưng thiếu intent) |
| LỗiDrive giữa audit (auth/quota) | như copy: `blocked reconnect_required` / retry có backoff; không tự sửa gì |

## Test (viết trước)

Engine (`audit_test.go`): 8 loại finding; audit read-only (`fake.Calls` không có
write); restart giữa audit; repair dry-run không ghi; repair sửa missing/mismatch/
duplicate/align; repair hai lần = no-op; repair không đụng object lạ; trash lỗi 1
lần → partial → chạy lại xong (không xóa vĩnh viễn); admission chặn khi có run
active; pause/cancel giữa audit.
State (`audit_test.go`): migration 005→006 có dữ liệu + `foreign_key_check` sạch;
findings dedup + pagination; repair revision check.
HTTP (`audits_test.go`): 202+Location+Idempotency, 409 stale, 404 scope, 503
maintenance, findings page, conformance OpenAPI.
CLI (`audit_test.go`): wait + summary, `--repair` mặc định dry-run, `--apply`,
`--json`, `audit show`.
