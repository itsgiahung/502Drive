# Copy job: từ link đến kết quả

Source: [reference.go](../../internal/drive/reference.go) (parse),
[service.go](../../internal/app/service.go) (`Inspect`, `Submit`),
[jobs.go](../../internal/state/jobs.go), [run.go](../../internal/engine/run.go). Test:
[engine_test.go](../../internal/engine/engine_test.go),
[sync_test.go](../../internal/engine/sync_test.go),
[server_test.go](../../internal/httpapi/server_test.go).

## 1. Preview (`POST /v1/plans`)

- Nhận **một** link HTTPS `drive.google.com`/`docs.google.com` hoặc ID; resource
  key được giữ ([reference.go](../../internal/drive/reference.go)).
- Từ chối: item trong trash, shortcut làm **root** (phải mở target), file owner
  cấm copy (`canCopy=false`), đích không verify được hoặc nằm trong chính nguồn
  (`checkDestination` đi ngược ancestry, fail-closed).
- Preview đếm một trang children (không phải full scan); plan TTL 10 phút, lưu
  principal, destination revision, root name.

## 2. Admission (`POST /v1/jobs`)

Trong một transaction: authorize (plan thuộc principal, revision khớp, destination
revision còn đúng) → check capacity (`maxActiveTotal=100`, `maxActivePerUser=20`)
→ tạo `collection` + `job` + seed root item + event `admitted` + idempotency →
commit rồi mới trả `202` + `Location`.

- Replay cùng key trả cùng job; plan đã consumed + key mới trả job cũ.
- Adapter không cần chờ; job sống độc lập với HTTP request.

## 3. Engine

| Pha | Việc |
| --- | --- |
| discover | BFS folder, ghi item + `page_token` checkpoint cùng transaction |
| copy | folder → create; file → server-side copy; shortcut → create shortcut; giữ tên/cây |
| finalize | tính `succeeded/partial/blocked/failed` từ item states, set `result_url`, cập nhật collection, gọi `AfterRun` |

- Write có `intent_key` bền vững; timeout → `reconciling`/`blocked`, không gửi
  lặp mù. `blocked` cần người xử lý; `retry` đưa item lỗi về pending.
- Folder rỗng được tạo; item nguồn mất dấu `source_removed`, **không xóa** bản đích.
- `versioned_copy` giữ bản cũ; `replace_copy` chỉ trash bản cũ sau khi bản mới
  được map (`retireSuperseded`, [run.go](../../internal/engine/run.go)).
- Load probe 10 000 file/500 folder: [load_test.go](../../internal/engine/load_test.go).

## 4. Điều khiển và trạng thái

- States: `queued running pausing paused cancelling reconciling blocked
  succeeded partial failed cancelled`. `desired_state` = run/pause/cancel persist.
- `pause` (queued → paused ngay; running → pausing → paused), `resume`, `cancel`
  (giữ output đã tạo), `retry` (chỉ item lỗi, giữ job identity).
- Counts đọc từ item; `total_items` null khi còn folder chưa list xong.
- `result_url` chỉ có khi root đã tạo.

## Kiểm tra toàn vẹn (audit) và sửa (repair)

`502drive audit COLLECTION` đối chiếu cây nguồn với cây đích của một collection
(job kind `audit`, **chỉ đọc** Drive, pause/resume/cancel/restart như job thường):

| Mã finding | Ý nghĩa | Repairable |
| --- | --- | --- |
| `missing_at_destination` | nguồn còn, bản đích đã map không còn ở cây đích | có (copy lại) |
| `extra_at_destination` | object trong cây đích không có dấu của ta | **không — chỉ báo** |
| `name_mismatch` / `wrong_parent` | bản đích sai tên / sai thư mục | có nếu object có `502drive_intent` của ta |
| `size_or_md5_mismatch` | file đích khác size/md5 với nguồn | có nếu có intent của ta |
| `duplicate_at_destination` | bản thừa cùng intent (khác bản đã map) | có (trash bản thừa) |
| `source_removed` / `unreadable_source` | nguồn mất / không đọc được | không |

Kết quả nằm ở `GET /v1/audits/{id}` (counts) + `GET /v1/audits/{id}/findings`
(phân trang). Telegram có nút 🔍 trên thẻ collection (tóm tắt + nút Sửa).

**Repair** (`--repair`) là job riêng trên engine hiện có, tham chiếu audit:

- **Mặc định dry-run**: chỉ lập kế hoạch, ghi vào `audits.plan`, không ghi Drive.
  `--apply` mới thực hiện.
- Copy lại mục thiếu **qua đúng đường copy hiện có** (intent mới, `ClaimItem`
  tự resolve cha, reconcile khi mất phản hồi); đổi tên/di chuyển dùng `align`;
  bản cũ sau khi bản mới được xác nhận bị chuyển vào **thùng rác** (qua
  `item_versions` superseded, kiểm intent trước khi trash). Không bao giờ xoá
  vĩnh viễn, không đụng object thiếu dấu.
- Chạy hai lần: lần hai kiểm lại từng finding với trạng thái hiện tại → kế
  hoạch rỗng, không đụng Drive. Crash giữa repair: restart quét lại, `item_versions`
  giữ bản superseded để dọn tiếp.
- Ràng buộc: collection không được có job khác đang chạy (chung `one_active_run`).

Thiết kế: [design/s1-integrity.md](../design/s1-integrity.md).

## Giới hạn / chưa hỗ trợ

- Không giữ owner/permission/revision/history của bản gốc; không tải/upload bytes.
- Không copy "bản mới" như action riêng: copy cùng nguồn là collection mới (dán
  lại link tạo job mới sau khi job cũ kết thúc).
- Không chọn nhiều nguồn/batch trong một request; không watch/push.
- Unknown outcome chỉ reconcile bằng query (không có tool repair thủ công).
