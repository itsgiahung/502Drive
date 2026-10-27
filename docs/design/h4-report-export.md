# H4 — X1: báo cáo lỗi theo job và export lịch sử

Ngày: 2026-10-27. Migration: `025_history.sql`.

## Bề mặt

- `502drive jobs report JOB_ID [--format table|json|csv] [--max-rows N]`
  đọc job rồi trang `GET /v1/jobs/{id}/items?problems=true` (đã có từ H1) và in
  mục `failed`, `blocked`, `skipped`: trạng thái, mã K1, lý do, đường dẫn, số lần thử.
  Mặc định 5000 dòng; cắt thì ghi rõ ở stderr, exit vẫn 0.
- `502drive history export [--since 30d] [--format table|json|csv]`: xuất lịch sử job
  qua `GET /v1/history?since=RFC3339&limit=&cursor=`, mới nhất trước. `--since` nhận
  `30d`, `12h`, `90m`, `2w` hoặc RFC3339; mặc định 30 ngày. Mặc định `--format csv`
  (lệnh `export`), trần 5000 dòng.
- `--format json` và `--json` (global) cùng in **một** envelope JSON; `--json` không
  đè `--format csv` khi người dùng nói rõ `--format`.

## Quyết định

- `GET /v1/history` là truy vấn riêng thay vì thêm `since` vào `GET /v1/jobs`:
  lịch sử sắp theo `updated_at` (job cũ chạy lại vẫn nằm trong cửa sổ), còn
  `/v1/jobs` sắp theo thứ tự nhận việc. Trộn hai thứ tự vào một cursor khó giữ
  đúng và dễ làm hỏng trang cũ.
- Khóa sắp xếp là `(updated_ms, rowid)` với
  `updated_ms = CAST(julianday(json_extract(body,'$.updated_at')) * 86400000 AS INTEGER)`.
  Biểu thức này bỏ được lỗi so sánh chuỗi RFC3339 có/không phần lẻ giây; chỉ mục
  biểu thức `jobs_history` phục vụ cả `since` lẫn phân trang keyset, không
  `TEMP B-TREE`, không nạp cả bảng.
- Cursor của `/v1/history` là chuỗi mờ `base64(updated_ms:rowid)`; đổi `since`,
  `all` hay principal là 400, giống cursor job.
- `all=true` chỉ owner (như `/v1/jobs`); mặc định chỉ job của chính principal.
- CSV: dùng `encoding/csv` (tự quote dấu phẩy, nháy kép, xuống dòng) rồi chặn công
  thức: ô bắt đầu bằng `=`, `+`, `-`, `@`, TAB hoặc CR được thêm `'` phía trước.
  Tên nguồn/đường dẫn vẫn qua `Sanitize` trước khi ghi.
- Lý do (reason) lấy từ `present.Reason(code)` — một bảng mã K1 dùng chung cho CLI
  và các bề mặt sau; mã lạ in nguyên văn, không đoán.

## Cửa sổ lỗi

| Tình huống | Kỳ vọng | Test |
| --- | --- | --- |
| Job chưa có mục lỗi | báo cáo rỗng, exit 0 | `TestJobsReportEmptyAndFormats` |
| Job không tồn tại / không thuộc principal | 404, không lộ job người khác | `TestHistoryAndReportScopedToPrincipal` |
| Ô CSV bắt đầu `=`, `+`, `-`, `@` | thêm `'`, không thành công thức | `TestCSVFormulaGuard` |
| Tên có phẩy/nháy/xuống dòng | một ô đúng chuẩn CSV | `TestCSVFormulaGuard` |
| Quá trần 5000 | cắt, nói rõ, exit 0 | `TestReportTruncatesWithNotice` |
| `--since` sai | exit 2, không gọi daemon | `TestParseSince` |
| Cursor với `since`/principal khác | 400 | `TestHistoryCursorBoundToFilters` |
| Nhiều job cùng giây | thứ tự ổn định theo rowid, không lặp/mất | `TestHistoryPagesWithoutDuplicates` |
| Lịch sử lớn | dùng chỉ mục `jobs_history`, không sort cả bảng | `TestHistoryQueryPlanUsesIndex` |

## Chưa làm

- Nút "📄 Báo cáo lỗi" của Telegram (file CSV chỉ cho owner/operator trong chat
  riêng) — ghi móc UI, không sửa `flow.go` trong đợt này.
- Chưa chạy Drive thật; báo cáo chỉ đọc DB.
