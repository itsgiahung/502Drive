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
- `versioned_copy`: bản cũ giữ; `replace_copy`: trash bản cũ sau khi map bản mới.
- `name/root` không đổi; không tạo wrapper mới mỗi run.

## Realtime (Drive Changes polling)

- Mỗi subscription có cursor riêng (`sync_subscriptions.cursor`); poll 10s khi
  vừa có thay đổi, 60s thường, 300s khi lâu không đổi; cooldown theo `Retry-After`.
- Mỗi page: lọc bằng `RelevantChange` (item đã biết hoặc nằm trong folder nguồn);
  `AdvancePoll` ghi cursor + admit run **cùng transaction**, nên cursor không vượt
  thay đổi chưa có run.
- Cursor hết hạn/không dùng được → lấy token mới + `MarkNeedsReconcile` → full
  reconcile run (`bootstrapping`) rồi `active`.
- Notification lỗi lặp của một subscription chỉ gửi một lần (marker
  `notify_markers`), xóa khi có run thành công.

## Pause / stop / resume

- `pause`: dừng lập lịch/poll mới; run đang chạy không bị hủy.
- `stop`: disable subscription, cancel run tự động đang chờ, dừng run đang chạy
  cooperative (`stopping` → `stopped`), **giữ** file/root/history.
- `resume`: validate collection; scheduled coalesce slot lỡ một lần; realtime
  catch-up hoặc `needs_reconcile` nếu cursor mất.
- `POST .../runs`: chạy delta ngay cả khi sync off; nếu đang có run → trả run hiện
  có + `rerun_requested` (coalesce), follow-up admit khi run trước kết thúc.

## Chưa hỗ trợ

- Realtime chưa replay raw change page trong lúc bootstrapping (chỉ reconcile scan).
- Chưa có `corpus_key`/cursor theo account; mỗi subscription một cursor.
- Full metadata reconcile định kỳ, digest, "pause cả run đang chạy" bằng một nút:
  chưa có.
- `after_copy_sync_mode` (preference) chưa tự áp cho collection mới.
- Filter theo loại/thành viên nguồn (type/member policy) chưa có; chỉ lọc theo cây.
