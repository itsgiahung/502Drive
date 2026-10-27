# Q1 — Theo dõi byte 24h và chặn khi hết quota

Thiết kế cho §Q1 (phase-13). Gốc: S2/S3/F1 trong working tree.

## Vấn đề

`dailyLimitExceeded` và `storageQuotaExceeded` hiện là `KindQuota`: engine coi
như lỗi item, đốt `attempts`, retry mù tới `retry_exhausted`. Không có số byte
đã copy trong 24h để giải thích vì sao chạm trần.

## Dữ liệu (migration `008_copy_ledger.sql`)

```sql
CREATE TABLE copy_ledger(
  principal TEXT NOT NULL,
  hour INTEGER NOT NULL,   -- epoch giờ (Unix/3600)
  bytes INTEGER NOT NULL,
  PRIMARY KEY(principal, hour)
);
```

- Ghi **trong cùng transaction** với `CompleteItem`, chỉ khi item chuyển sang
  `done` thật (`RowsAffected>0`) và là file có `size>0` → hoàn thành lặp lại
  (crash/reconcile) không thổi phồng sổ.
- `CopiedBytes24h(principal, now)`: một `SUM` có index trên 24 bucket gần nhất,
  không quét job/item.
- Byte là **ước lượng theo metadata** (`size`); file Google native/shortcut
  không có size nên không cộng. Sổ dùng để giải thích/ước lượng, không phải
  billing.

## Phân loại quota

`drive.QuotaScope(err)`:

| Reason | Scope | resume_after |
| --- | --- | --- |
| `dailyLimitExceeded` | daily | **midnight America/Los_Angeles** kế tiếp (đúng DST) |
| `storageQuotaExceeded` | storage | `now + 24h` (hết dung lượng cần người dọn; vẫn tự kiểm tra lại) |
| khác (`quotaExceeded`…) | quota | `Retry-After` nếu có, không thì `now + 1h` |

## Luồng

Khi write (copy/folder/shortcut/align) trả `KindQuota`:

1. Item về `pending`, `attempts-1`, `error_code=quota_exceeded`,
   `next_attempt_at=resume_after` (`ReleaseItemThrottled`) — **không đốt lượt**,
   **không gửi lại mù**.
2. Lần đầu: job → `blocked`, `last_error.code=quota_exceeded`,
   body có `resume_after`; cờ `run.quota` làm mọi worker dừng claim, `settle`
   không finalize.
3. `GET /v1/jobs/{id}` trả `resume_after` (null khi không có). Job vẫn chiếm
   `one_active_run`, không sinh job mới.
4. Engine mỗi vòng poll gọi `ResumeDueBlocked(now)`: job `blocked`,
   `desired_state=run`, có `resume_after<=now` → `queued` (cùng id, cùng
   revision tăng), xoá `resume_after`/`last_error`/`phase`.
5. `ClaimQueued` chạy lại chính job đó: quét lại (delta) và copy các item
   `pending` đã tới hạn. Không có admission/idempotency mới.

Quota khi đang **list**: `blockQuota(nil, err)` — job blocked, folder giữ
`list_state` dở dang; resume quét tiếp từ checkpoint (hoặc quét lại batch).

## Cửa sổ lỗi

| Cửa sổ | Ứng xử |
| --- | --- |
| Quota giữa copy | item pending không đốt lượt, job blocked + `resume_after` |
| Quota giữa list | job blocked; checkpoint giữ; resume quét tiếp |
| Nhiều worker cùng gặp quota | chỉ lần đầu ghi job; các lần sau chỉ trả item |
| Chết khi đang blocked | DB giữ `resume_after`; khởi động lại engine tự resume khi tới hạn |
| Resume nhưng vẫn hết quota | block lại với mốc mới, không tăng attempts, không job mới |
| User cancel/pause khi blocked | `cancel` thắng (state cancelled); pause trên blocked là transition không hợp lệ |
| Hết dung lượng thật | kiểm tra lại mỗi 24h, không spam; cần người dọn |
| Đồng hồ đổi múi giờ/DST | `nextDailyQuotaReset` tính theo America/Los_Angeles |

## Test

- `state/quota_test.go`: migration 007→008 có dữ liệu + `foreign_key_check`;
  `ResumeDueBlocked` chỉ resume khi qua mốc và `desired_state=run`.
- `engine/quota_test.go`: daily → blocked + đúng midnight Pacific, 0 file đích,
  item pending `attempts=0`, resume cùng job id rồi succeeded; storage → +24h,
  đúng 1 call copy; ledger 10+20+5=35, chạy lại no-op không tăng, ngoài 24h = 0;
  `quotaResume` qua DST.
- `httpapi` conformance: job DTO luôn có `resume_after` (null/date-time).
- Chưa chạy: quota thật từ Google, reset thật, Shared Drive hết storage.
