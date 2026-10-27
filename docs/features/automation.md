# Sync theo lịch và realtime

Source: [schedule.go](../../internal/schedule/schedule.go),
[sync.go](../../internal/state/sync.go) (store), [app/sync.go](../../internal/app/sync.go),
[syncer.go](../../internal/daemon/syncer.go) (daemon). Test:
[schedule_test.go](../../internal/schedule/schedule_test.go),
[syncer_test.go](../../internal/daemon/syncer_test.go),
[app/sync_test.go](../../internal/app/sync_test.go),
[httpapi/sync_test.go](../../internal/httpapi/sync_test.go).

## Subscription

Một collection có tối đa một subscription:
`mode` scheduled/realtime, `state` waiting_for_baseline · bootstrapping · active ·
paused · stopping · stopped · needs_reconcile · error, `content_policy`
versioned_copy/replace_copy, `schedule_revision`, `revision`.

- `PUT /v1/collections/{id}/sync` tạo/cập nhật (một subscription/collection, đổi
  mode atomic, không tạo output root mới). Realtime lấy `StartPageToken`
  **trước** khi ghi subscription ([app/sync.go](../../internal/app/sync.go)).
- Collection chưa `ready` → `waiting_for_baseline`; run baseline xong tự bật.

## Schedule

`ScheduleSpec` oneOf:

| Kind | Field | Ghi chú |
| --- | --- | --- |
| interval | `interval_minutes` 5–10080, `anchor_at` UTC | Elapsed UTC, không phụ thuộc host |
| daily | `timezone` IANA, `local_time` HH:MM | Wall-clock theo zone |
| weekly | + `weekdays` ISO 1..7 | Một slot/local calendar day |

- DST: giờ không tồn tại bị bỏ; giờ lặp chỉ chạy **một** slot (key = date+HH:MM+zone).
- Misfire: nhiều slot lỡ → **một** catch-up run, next future.
- `sync_occurrences(collection, schedule_revision, occurrence_key)` chặn chạy lặp.
- Queue đầy: slot giữ nguyên due, tick sau thử lại (không trôi, không spin).
- Timezone mặc định lấy preference (`TZ` hoặc `Asia/Ho_Chi_Minh`), đổi được qua
  `PATCH /v1/preferences`; CLI/Telegram/TUI dùng preference này.
- Preview 3 lần chạy tới: `Spec.Preview` ở CLI `sync enable --dry-run` và thẻ xác
  nhận Telegram trước khi lưu.

## Delta run

- Run sync dùng **cùng collection/root**; `items` giữ mapping. Folder được list
  lại; file thay đổi fingerprint → generation mới; move/rename → `op=align`.
- Item mất khỏi nguồn → `error_code=source_removed`, bản đích giữ nguyên.
- Chống xoá nhầm: nếu một lần quét mất **>30%** mục đã map (sàn 10 mục), run
  dừng **trước khi copy**, thành `partial` với `last_error.code=source_mass_removal`
  và body có `source_removed`; `RunFinished` tạo audit S1 read-only
  (`trigger=reconcile_watch`) để giải thích, không tự sửa/xoá đích. Dưới ngưỡng
  chỉ đánh dấu `source_removed`.
- `versioned_copy`: bản cũ giữ; `replace_copy`: trash bản cũ sau khi map bản mới.
- `name/root` không đổi; không tạo wrapper mới mỗi run.

## Realtime (Drive Changes polling)

- Mỗi subscription có cursor riêng (`sync_subscriptions.cursor`); poll 10s khi
  vừa có thay đổi, 60s thường, 300s khi lâu không đổi; cooldown theo `Retry-After`.
- Mỗi page: lọc bằng `RelevantChange` (item đã biết hoặc nằm trong folder nguồn);
  `AdvancePoll` ghi cursor + admit run **cùng transaction**, nên cursor không vượt
  thay đổi chưa có run.
- **Trong lúc baseline/reconcile** (`bootstrapping`): feed vẫn được poll nhưng
  page được lưu thô vào `sync_pending_changes` (`file_id`, `removed`, `parents`,
  `trashed`), **cursor chưa tiến**. Sau khi baseline xong, `replayPending` lọc
  `RelevantChange` trên mapping đã đủ rồi `FinishReplay` xoá buffer, tiến cursor
  và admit run **trong một transaction**. Trần 5000 thay đổi hoặc 4 MiB mỗi
  collection; vượt trần → `replay_overflow` → subscription thành
  `needs_reconcile` + full reconcile, không phát lại một tập đã biết thiếu.
  Restart giữa chừng đọc tiếp từ `cursor_after` của dòng cuối, không lặp trang.
- **Full reconcile định kỳ**: `full_reconcile_every` (giây, mặc định 604800 = 7
  ngày, `0` tắt) đặt lúc `PUT /sync` từ config `full_reconcile_every` (Go
  duration, rỗng = 168h, `0`/`off` = tắt). `next_full_reconcile_at` lệch ±10%;
  slot được đẩy trước khi admit nên không chồng run; queue đầy thì giữ slot
  due cho tick sau. Subscription cũ (schema trước 007) được xếp slot ở tick đầu
  tiên, không dồn chạy ngay khi nâng cấp.
- Cursor hết hạn/không dùng được → lấy token mới + `MarkNeedsReconcile` → full
  reconcile run (`bootstrapping`) rồi `active`.
- Notification lỗi lặp của một subscription chỉ gửi một lần (marker
  `notify_markers`), xóa khi có run thành công.

## Quota tài khoản

- `copy_ledger(principal,hour,bytes)`: byte đã copy xong, ghi cùng transaction
  với `CompleteItem`; `CopiedBytes24h` là một `SUM` 24 bucket (xem
  [verification](../verification.md) cho số đo). Chỉ file có `size` mới cộng.
- `dailyLimitExceeded` → job `blocked`, `last_error.code=quota_exceeded`,
  `resume_after` = **midnight America/Los_Angeles** kế tiếp.
  `storageQuotaExceeded` → `resume_after` = +24h (cần người dọn dung lượng, job
  vẫn tự kiểm tra lại). Quota khác: `Retry-After` hoặc +1h.
- Item về `pending` với `attempts-1` và `next_attempt_at=resume_after`; **không
  đốt lượt**, **không gửi lại mù**. Worker dừng claim, job giữ nguyên id.
- Engine mỗi vòng poll gọi `ResumeDueBlocked`; job `blocked` có mốc đã qua và
  `desired_state=run` → `queued` (cùng id), rồi tiếp tục quét/copy phần còn lại.
  `resume_after` hiện trong `GET /v1/jobs/{id}` (null khi không có).

## Pause / stop / resume

- `pause`: dừng lập lịch/poll mới; run đang chạy không bị hủy.
- `stop`: disable subscription, cancel run tự động đang chờ, dừng run đang chạy
  cooperative (`stopping` → `stopped`), **giữ** file/root/history.
- `resume`: validate collection; scheduled coalesce slot lỡ một lần; realtime
  catch-up hoặc `needs_reconcile` nếu cursor mất.
- `POST .../runs`: chạy delta ngay cả khi sync off; nếu đang có run → trả run hiện
  có + `rerun_requested` (coalesce), follow-up admit khi run trước kết thúc.

## Tự bật sau lần copy đầu (S8)

- Preference `after_copy_sync_mode` (`off|realtime|hourly|six_hourly|daily`) và
  cờ `copy --sync MODE` (hoặc `after_copy_sync_mode` trên `POST /v1/jobs`) áp cho
  **collection mới** khi job initial `succeeded`. Mode ghi trên job nên sống qua
  restart; dòng chờ `after_copy_sync` được ghi cùng transaction với kết quả job.
- Subscription do người dùng đặt trước luôn thắng; job `partial`/`failed` không
  tạo sync. Lỗi tạo subscription chỉ ghi vào dòng chờ (retry 5 lần), không đổi
  kết quả copy. Xem [design/h2-after-copy-sync.md](../design/h2-after-copy-sync.md).

## Chưa hỗ trợ

- Chưa có `corpus_key`/cursor theo account; mỗi subscription một cursor.
- Digest/nội dung file và "pause cả run đang chạy" bằng một nút: chưa có.
- Full reconcile định kỳ chỉ đọc lại metadata/cây, không so digest nội dung.
- Filter theo loại/thành viên nguồn (type/member policy) chưa có; chỉ lọc theo cây.
