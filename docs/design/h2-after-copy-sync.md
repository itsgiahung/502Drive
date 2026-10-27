# H2 — S8: tự bật đồng bộ sau lần copy đầu

Ngày: 2026-10-27. Migration: `023_after_copy_sync.sql`.

## Hành vi

1. Người dùng chọn `after_copy_sync_mode` (`off|realtime|hourly|six_hourly|daily`)
   trong Preferences, hoặc ghi đè cho một lệnh bằng `copy --sync MODE` /
   `after_copy_sync_mode` trên `POST /v1/jobs`.
2. Job mang mode trong body (bền, có ngay từ admission). Khi job **initial**
   kết thúc `succeeded`, trong **cùng transaction** ghi kết quả (`RunFinished`),
   daemon ghi một dòng `after_copy_sync` nếu:
   - job là `trigger=initial`, kind rỗng (không audit/repair/cleanup),
   - collection chưa có subscription (subscription do người dùng đặt luôn thắng),
   - mode khác `off`.
   Job `partial`/`failed`/`cancelled` không tạo dòng nào.
3. Syncer mỗi tick lấy tối đa 5 dòng đến hạn, đặt `applying` + lease 1 phút, rồi
   gọi `PUT sync` nội bộ với khóa `after-copy:<collection>:<mode>`. Khóa này
   cộng `ON CONFLICT(collection_id)` làm thao tác idempotent: retry sau crash,
   hai lần `RunFinished`, hay hai job cùng collection đều chỉ cho một subscription.
4. Lỗi (Google offline, chưa kết nối, collection chưa ready) ghi `last_error`,
   lùi 1/2/3/4 phút, tối đa 5 lần rồi `failed`. **Không** đụng job copy và không
   đổi kết quả copy.
5. Mode ánh xạ: `realtime` → subscription realtime (cursor lấy lúc áp dụng);
   `hourly`/`six_hourly` → interval 60/360 phút neo ở thời điểm copy;
   `daily` → daily 02:00 theo timezone trong Preferences. `content_policy` là
   `versioned_copy`.

## Cửa sổ lỗi đã kiểm

| Tình huống | Kỳ vọng | Test |
| --- | --- | --- |
| Daemon restart ngay sau copy, trước tick syncer | dòng chờ còn, áp dụng đúng | `TestAfterCopySyncAppliesPreferenceAndSurvivesRestart` |
| Job `partial` | không tạo subscription | `TestAfterCopySyncSkipsPartialAndNonInitialJobs` |
| Run không phải initial (manual/sync) | bỏ qua | cùng test |
| Hai lần `RunFinished` cùng collection | một dòng, một subscription | `TestAfterCopySyncTwiceOnOneCollectionCreatesOneSubscription` |
| Provider lỗi lúc tạo subscription realtime | job vẫn `succeeded`; request `pending` + lỗi; lần sau áp dụng | `TestAfterCopySyncJobFlagOverridesPreferenceAndFailureKeepsJob` |
| Mode trên job ghi đè preference `off` | subscription realtime được tạo | cùng test |

## Chưa làm

- Chưa có UI đặt mode (TUI/Telegram); API + CLI đã có. Móc ở `PROGRESS.md`.
- `daily` cố định 02:00; chưa có preference riêng cho giờ này.
- Chưa chạy Drive thật; `StartPageToken` dùng `drivetest.Fake`.
