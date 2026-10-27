# S13 — UX Thống nhất (`internal/present`)

Thiết kế cho §S13 (phase-12-spec).

## Mục tiêu

1. Tạo package dùng chung `internal/present` chứa các hàm định dạng hiển thị chuẩn hoá cho toàn bộ các client (CLI, TUI, Tray, Telegram):
   - `StateLabel(state, lang string) string`: Chuẩn hoá nhãn trạng thái song ngữ vi/en.
   - `Progress(counts Counts, rate float64, eta time.Duration, lang string) string`: Chuỗi tiến độ gọn gàng (số lượng hoàn thành/tổng số, tốc độ, thời gian còn lại).
   - `HumanBytes(n int64) string`: Định dạng dung lượng (B, KB, MB, GB, TB, PB) chuẩn xác.
   - `HumanDuration(d time.Duration, lang string) string`: Định dạng khoảng thời gian dễ đọc (`1h05m`, `2m30s`, `45s`).
   - `OneLine(job JobSummary, lang string) string`: Một dòng tóm tắt công việc dùng cho danh sách tray, log, TUI, CLI.
2. Kiểm thử bảng (table-driven test) đầy đủ cho cả tiếng Việt (`vi`) và tiếng Anh (`en`), bao gồm các ca biên (số âm, nil pointer, trạng thái lạ, 0 giây, 0 byte).
3. **Ràng buộc quan trọng**: KHÔNG migrate nơi nào trong đợt này do các package `internal/tui`, `internal/tray`, `internal/cli`, `internal/telegram` đang được sửa đổi ở nhánh khác. Liệt kê chi tiết từng vị trí lặp cần migrate (`file:line`) vào báo cáo `BATCH-E-REPORT.md`.

## Chi tiết API

```go
package present

type Counts struct {
    Observed  int
    Total     *int
    Completed int
    Failed    int
    Blocked   int
    Skipped   int
    Bytes     int64
}

type JobSummary struct {
    ID        string
    State     string
    Name      string
    Counts    Counts
    Rate      float64
    ETA       time.Duration
}

func StateLabel(state, lang string) string
func Progress(counts Counts, rate float64, eta time.Duration, lang string) string
func HumanBytes(n int64) string
func HumanDuration(d time.Duration, lang string) string
func OneLine(job JobSummary, lang string) string
```
