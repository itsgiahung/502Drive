# S2 — Replay khi bootstrapping + full reconciliation định kỳ

Thiết kế cho đặc tả §S2 (phase-12). Trạng thái gốc: commit `41c7c37` (S1).

## Vấn đề

Realtime lấy cursor **trước** reconcile run. Trong lúc baseline/reconcile đang
quét cây (có thể vài giờ), feed thay đổi nhưng `sync_subscriptions.state` là
`bootstrapping` nên `DuePolls` không trả subscription → thay đổi nằm chờ; nếu
cursor cũ bị đẩy lên sau baseline thì mất. Đặc tả yêu cầu: **đọc feed, lưu thô,
phát lại sau baseline, trong cùng transaction với cursor**; vượt trần thì
`needs_reconcile` (quét lại, không sync im lặng). Thêm full reconciliation định
kỳ và ngưỡng chống xoá nhầm.

## Dữ liệu (migration `007_sync_replay.sql`, forward-only)

```sql
CREATE TABLE sync_pending_changes(
  collection_id TEXT NOT NULL REFERENCES collections(id),
  seq INTEGER NOT NULL,             -- 1,2,3… theo collection, thứ tự feed
  cursor_after TEXT NOT NULL,       -- cursor sau trang chứa thay đổi này
  change_json TEXT NOT NULL,        -- change tối thiểu để lọc relevant
  PRIMARY KEY(collection_id, seq)
);

ALTER TABLE sync_subscriptions ADD COLUMN full_reconcile_every INTEGER NOT NULL DEFAULT 604800;
ALTER TABLE sync_subscriptions ADD COLUMN next_full_reconcile_at INTEGER NOT NULL DEFAULT 0;
ALTER TABLE sync_subscriptions ADD COLUMN replay_overflow INTEGER NOT NULL DEFAULT 0;
```

- `change_json` chỉ chứa `{file_id, removed, parents[], trashed}` — đủ cho
  `RelevantChange`, không lưu metadata đầy đủ (bộ nhớ/DB nhỏ).
- Trần replay (hằng trong `internal/state/replay.go`): **5000 thay đổi** hoặc
  **4 MiB** mỗi collection, tuỳ cái tới trước. Vượt → `replay_overflow=1`,
  **không** lưu thêm; cursor subscription **không** tiến.
- `full_reconcile_every` giây (mặc định 604800 = 7 ngày, `0` = tắt).
  `next_full_reconcile_at` tính bằng `now + every × [0.9, 1.1]` (jitter ±10%).

## Luồng

### 1. Bootstrapping: lưu thô

`DuePolls` trả cả subscription `bootstrapping`. `syncer.poll`:

1. Điểm đọc = `max(subscription.cursor, cursor_after của dòng pending cuối)`
   (restart không đọc lặp, không mất trang đã lưu).
2. Đọc feed như cũ (`maxFeedPages` một tick).
3. `state.AppendPendingChanges(...)` trong **một transaction**: cấp `seq`
   tiếp theo, ghi từng change + `cursor_after`, kiểm trần; đặt
   `next_poll_at = now + warm`.
4. `RelevantChange` **không** chạy lúc lưu: lúc này `items` còn đang được
   baseline điền; lọc sau baseline mới đúng.
5. Lỗi feed: như cũ (`deferPoll`, cooldown, không tiến cursor).

Cursor subscription **không** đổi trong lúc bootstrapping. Vì vậy nếu tiến
trình chết, feed vẫn đọc lại được từ cursor cũ (dòng đã lưu chặn đọc lặp).

### 2. Sau baseline: phát lại

`tick` gọi `replayPending` **trước** `pollFeeds`, cho mọi collection có dòng
pending và subscription `active`:

1. Đọc dòng theo `seq` tăng dần (giới hạn 5000/lần).
2. `replay_overflow=1` → xoá dòng, `MarkNeedsReconcile` với token mới (full
   reconcile run). Không phát lại một tập đã mất.
3. Ngược lại: tính `RelevantChange` cho từng change (theo thứ tự), rồi
   `state.FinishReplay` trong **một transaction**: `DELETE sync_pending_changes`,
   `cursor = cursor_after` của dòng cuối, `admitRun(trigger="realtime")` nếu có
   change liên quan (coalesce nếu run đang chạy), `next_poll_at` sửa theo cadence.
4. Transaction khoá `state='active'` và `cursor` cũ: nếu subscription bị
   pause/stop giữa chừng → `ErrTransition`, dòng pending giữ nguyên; lần active
   sau phát lại tiếp (cursor vẫn cũ → không mất).

`RunFinished` khi `bootstrapping → active`: nếu `replay_overflow=1` thì đặt
`needs_reconcile` (kèm `Failure{code: replay_overflow}`) thay vì `active`; nếu
không thì `active` và giữ dòng pending cho `replayPending` xử lý ngay tick sau
(≤5 s).

### 3. Full reconciliation định kỳ

`syncer.tick` gọi `ClaimDueFullReconciles(now)`: subscription realtime
`state='active'`, `full_reconcile_every>0`, `next_full_reconcile_at<=now`.

- `admitRun(trigger="full_reconcile")` — **coalesce** với run đang chạy (không
  chồng run); slot kế được đẩy trước khi chạy, nên tick sau không tạo thêm.
- Chạy y như delta run (re-arm folder, so khớp mapping, copy thiếu). Không tạo
  root mới.
- `ClaimDueSchedules`/`ClaimDueFullReconciles` dùng tiến trình ở store; đồng hồ
  giả trong test điều khiển được.
- `full_reconcile_every` lấy từ config `full_reconcile_every` (dạng
  `time.ParseDuration`, rỗng = 168h, `0` = tắt) lúc `PUT /sync`; subscription
  giữ giá trị của riêng nó (đổi config không đổi subscription cũ cho tới lần
  cấu hình sau).

### 4. Ngưỡng chống xoá nhầm

Delta scan gọi `FinishScan`, nhận `removed` (mục `done` không còn thấy ở
nguồn). Nếu `known >= 10` và `removed > 30% known` (`known` = số mục `done`
của collection, không tính root):

1. Không chạy `copyAll` (dừng trước khi ghi gì).
2. Job → `partial` với `last_error.code = source_mass_removal`; job body ghi
   `source_removed` (JSON, không đổi cột).
3. `RunFinished` (job đã rời nhóm active) tạo **audit S1** (`kind=audit`,
   `trigger=reconcile_watch`) nếu còn chỗ queue — audit đọc-only, báo sai lệch
   thật; không tự sửa, không xoá đích.
4. Nếu không đủ chỗ: bỏ qua, ghi log; lần full reconcile sau thử lại.

Ngưỡng 30% và sàn 10 mục tránh báo động ở cây nhỏ. Dưới ngưỡng: hành vi cũ
(`source_removed` chỉ đánh dấu, đích giữ nguyên).

## Chế độ lỗi

| Cửa sổ | Ứng xử |
| --- | --- |
| Chết sau khi ghi pending, trước khi phát lại | Dòng còn trong DB; khởi động lại → `replayPending` phát lại, cursor tiến cùng admission |
| Chết giữa trang feed | Trang chỉ được ghi cùng `cursor_after`; `max(...)` nên không lặp, không mất |
| Quá trần replay | `needs_reconcile` + full reconcile, không "sync im lặng" |
| Subscription bị pause/stop khi đang có pending | Dòng giữ; cursor chưa tiến; resume phát lại |
| Hai tick cùng lúc (không xảy ra: syncer một goroutine) | Transaction + kiểm `state/cursor` |
| Full reconcile trùng run đang chạy | `admitRun` coalesce |
| Mất >30% nguồn | job `partial` + audit, không copy |
| Feed token hết hạn khi đang bootstrapping | `reconcileFeed` như cũ (token mới + needs_reconcile) |

## Test (viết trước / cùng)

- `state/replay_test.go`: migration 006→007 có dữ liệu + `foreign_key_check`;
  append theo trần (số dòng và byte); `LastPendingCursor`; `FinishReplay` tiến
  cursor + admit run + xoá dòng trong một transaction; `FinishReplay` từ chối
  khi state không active.
- `daemon/syncer_test.go`: thay đổi **trong** bootstrapping được lưu; sau
  baseline phát lại → đúng 1 run, cursor = cursor_after cuối; thay đổi không
  liên quan → không run nhưng cursor vẫn tiến; vượt trần → needs_reconcile +
  reconcile run; restart giữa chừng (đóng/mở store) → không mất, không lặp;
  pause giữa chừng → giữ dòng; full reconcile due → 1 run + jitter trong ±10%;
  tắt (`0`) → không run; hai tick → không chồng.
- `engine/sync_test.go`: full reconcile chạy delta (copy thiếu), mất 40% nguồn
  → `partial` + `source_mass_removal` + audit được tạo; mất 20% → bình thường.
- Crash matrix có seed (S2): nhiều seed, chết ở các điểm (sau lưu trang, sau
  baseline, sau replay), khởi động lại, khẳng định mapping cuối = nguồn và
  không có run trùng; seed in ra khi fail.
