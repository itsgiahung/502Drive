# Thiết kế R1 — Giữ dữ liệu có hạn (Retention)

Tài liệu thiết kế cho slice **R1** theo `docs/handoff/phase-13-spec.md` mục R1 và mục 2.

## 1. Bối cảnh & Mục tiêu

Sau thời gian vận hành dài (nhiều tháng/năm), SQLite database có thể tích tụ số lượng lớn:
- Bản ghi `jobs` đã kết thúc (`done`, `failed`, `cancelled`).
- Bản ghi `events` chi tiết của từng bước copy/sync.
- Bản ghi `telegram_inbox` đã xử lý (`processed`) hoặc hỏng (`failed`).
- Bản ghi `notification_outbox` đã gửi (`sent`) hoặc dead (`dead`).
- Plans hết hạn chưa từng được tiêu thụ (`consumed_job IS NULL`).

Write intent (`items.intent_key`) **không** bị dọn: dòng `items` là mapping còn sống và
intent key là bằng chứng sở hữu mà audit/repair dựa vào, bất kể job đã cũ bao lâu.

**Mục tiêu**:
1. Cung cấp lệnh CLI ngoại tuyến: `502drive maintenance prune [--older-than 90d] [--dry-run]` (và alias `502drive prune`).
2. Tự động dọn dẹp hàng ngày trong daemon (`daemon.go`) khi hệ thống rảnh.
3. Cấu hình `retention_days` (mặc định 90, 0 = giữ mãi) trong `Preferences` (`internal/state/settings.go`), không cần migration SQL mới.
4. **Bất biến an toàn dữ liệu**:
   - KHÔNG xóa collections.
   - KHÔNG xóa mapping (cây quan hệ `source_id` -> `dest_id` trong `items`).
   - KHÔNG xóa job còn dùng làm tham chiếu (`items.job_id`, `items.scan_job`, `items.seen_job`, `plans.consumed_job`, `sync_occurrences.job_id`, hoặc pending `notification_outbox.job_id`).
   - Đảm bảo `PRAGMA foreign_key_check` luôn trả về 0 lỗi.
   - Khớp chính xác số lượng giữa `--dry-run` và khi thực thi thật (`--apply`).

## 2. Thiết kế chi tiết

### 2.1 Cấu hình `retention_days`
- Bổ sung trường `RetentionDays int` vào `state.Preferences` (mặc định 90; 0 = tắt auto-prune / giữ mãi).
- `app.PreferencePatch` và API HTTP `/v1/preferences` cho phép cập nhật `retention_days` (yêu cầu >= 0).
- Cập nhật schema trong `docs/contracts/openapi.json`.

### 2.2 Thuật toán Prune (`internal/state/prune.go`)
1. **Tính mốc thời gian (Cutoff)**:
   `cutoff := time.Now().Add(-olderThan).UTC()`
   Dùng định dạng RFC3339 để so khớp chuỗi thời gian trong JSON SQLite `json_extract(body, '$.updated_at') < cutoffStr`.
   Dùng Unix milliseconds cho bảng Telegram (`received_at`, `updated_at`, `created_at`).

2. **Lọc Job ứng viên**:
   Job thỏa mãn:
   - `state IN ('done', 'failed', 'cancelled')`
   - `json_extract(body, '$.updated_at') < cutoffStr`
   - KHÔNG được tham chiếu bởi bất kỳ bảng nào:
     - `id NOT IN (SELECT DISTINCT job_id FROM items)`
     - `id NOT IN (SELECT DISTINCT scan_job FROM items WHERE scan_job <> '')`
     - `id NOT IN (SELECT DISTINCT seen_job FROM items WHERE seen_job <> '')`
     - `id NOT IN (SELECT DISTINCT consumed_job FROM plans WHERE consumed_job IS NOT NULL AND consumed_job <> '')`
     - `id NOT IN (SELECT DISTINCT job_id FROM sync_occurrences WHERE job_id <> '')`
     - `id NOT IN (SELECT DISTINCT job_id FROM notification_outbox WHERE state = 'pending' AND job_id <> '')`

3. **Dọn dẹp phụ thuộc trong Transaction**:
   - Xóa `events` của các job ứng viên (và các events cũ hơn cutoff của các job đã kết thúc).
   - Xóa `jobs` ứng viên.
   - Xóa `telegram_inbox` có `state IN ('processed', 'failed') AND received_at < cutoffMs`.
   - Xóa `notification_outbox` có `state IN ('sent', 'dead') AND updated_at < cutoffMs`.
   - Xóa `callback_states` đã tiêu thụ hoặc hết hạn: `(consumed = 1 OR expires_at < nowMs) AND created_at < cutoffMs`.
   - Xóa `plans` hết hạn không được tiêu thụ: `consumed_job IS NULL AND id NOT IN (SELECT DISTINCT plan_id FROM collections) AND json_extract(body, '$.expires_at') < cutoffStr`.
   - Không đụng `items.intent_key`: mapping còn dùng, xóa intent sẽ làm audit/repair mất quyền chứng minh sở hữu (xem [clone-jobs](../features/clone-jobs.md)).

4. **Báo cáo kết quả (`PruneReport`)**:
   Trả về:
   - `OlderThan`: chuỗi thời gian (vd: "90d", "2160h0m0s")
   - `Cutoff`: thời điểm mốc
   - `DryRun`: true/false
   - `JobsPruned`: số job
   - `EventsPruned`: số event
   - `IntentsPruned`: luôn 0 (giữ trường cho tương thích JSON/CLI; intent của mapping sống không bị dọn)
   - `TelegramInboxPruned`: số inbox telegram
   - `TelegramOutboxPruned`: số notification outbox
   - `PlansPruned`: số unconsumed plans

### 2.3 Lớp ngoại tuyến (`internal/maintenance/prune.go`)
- Sử dụng `maintenance.LockDataDir(dataDir)` để chiếm `instance.lock`. Nếu daemon đang chạy, từ chối với `ErrWriterRunning` (tương tự `vacuum`, `backup`).
- Mở cơ sở dữ liệu với `state.Open`, thực hiện `store.Prune`, kiểm tra `PRAGMA foreign_key_check`, đóng store và mở khóa data directory.

### 2.4 CLI (`internal/cli/maintenance.go`)
- Lệnh: `502drive maintenance prune [--older-than 90d] [--dry-run]`
- Hỗ trợ cú pháp ngày `d` / `D` (vd: `90d` -> `90 * 24 * time.Hour`).
- Hỗ trợ cờ `--dry-run` (mặc định false).
- Hỗ trợ xuất `--json` theo chuẩn phong bì của 502Drive.

### 2.5 Tự động hóa trong Daemon (`internal/daemon/daemon.go`)
- Tích hợp vào vòng lặp bảo trì nhàn rỗi (idle maintenance loop).
- Mỗi 24 giờ kiểm tra preferences của principal mặc định. Nếu `RetentionDays > 0`, thực hiện `store.Prune` tự động với `OlderThan = RetentionDays * 24h` trong nền.

## 3. Kế hoạch kiểm chứng (Acceptance)
1. `R1-1`: Ngưỡng thời gian: job/event/inbox mới hơn ngưỡng không bị xóa; cũ hơn bị xóa.
2. `R1-2`: `--dry-run` khớp số lượng 100% với lần thực thi thật sau đó (`--apply`).
3. `R1-3`: Khóa ngoại toàn vẹn (`PRAGMA foreign_key_check` = 0 lỗi).
4. `R1-4`: Job còn được tham chiếu (`items.job_id`, `plans.consumed_job`, v.v.) không bị xóa.
5. `R1-5`: Cấu hình `retention_days = 0` giữ lại toàn bộ dữ liệu (không xóa gì).
