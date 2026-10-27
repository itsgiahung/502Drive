# H1 — API còn thiếu cho TUI/Telegram

Ngày: 2026-10-27. Phạm vi: chỉ thêm khóa/đường dẫn, không đổi hành vi cũ.

## Quyết định

- `GET /v1/collections?include_retired=true` tách khỏi `all`. `all` vẫn là
  "owner xem mọi principal" và tiếp tục bao gồm collection đã ngừng (hành vi cũ,
  test `TestRetireAndResumeCollection`). Cursor gắn thêm cờ này để đổi bộ lọc
  giữa hai trang là lỗi 400, không trả trang sai.
- `GET /v1/collections/{id}/audits/latest` trả audit mới nhất (404 nếu chưa có).
  `repairable_findings` đếm finding `repairable=1 AND repaired=0` bằng
  `COUNT(*)` có chỉ mục riêng, không nạp danh sách finding.
- `GET /v1/drive/search?q=` tìm trong Drive của tài khoản đã kết nối. Chỉ owner
  (như `ls`/`browse`), tối đa 20, cắt q ở 100 rune, `trashed=false`; Google tự
  lọc theo quyền xem. Tên được escape `'`/`\` trước khi vào cú pháp `name contains`.
- Lọc job phía server: `collection_id` cộng với `state`, `limit`, `cursor`.
  Truy vấn dựng theo bộ lọc thay vì `(?='' OR col=?)` để SQLite dùng chỉ mục;
  chỉ mục thêm `jobs(collection_id)` và `jobs(state)` (rowid là khóa phụ cuối)
  tránh `TEMP B-TREE`. Cursor gắn `state` và `collection_id`.
- Preferences có `daily_report` + `daily_report_at` (HH:MM). Telegram dùng
  chính hai trường này thay vì khóa `tg_report:` riêng, nên API và bot không
  lệch nhau; `tg_report_sent:` vẫn là dấu "đã gửi hôm nay".
- `max_active_jobs` trong config (mặc định 2, 1..32) thay cho hằng số
  `Engine.Jobs=2`.

## Cửa sổ lỗi đã kiểm

| Tình huống | Kỳ vọng | Test |
| --- | --- | --- |
| Đổi `collection_id`/`state` nhưng dùng cursor cũ | 400 | `TestJobListFiltersAndCursorBinding` |
| `include_retired` với principal khác | không lộ collection người khác | `TestCollectionPageIncludeRetiredKeepsPrincipalScope` |
| Audit chưa có | 404, không trả audit rỗng | `TestLatestAuditWithRepairableCount` |
| Finding đã sửa | không còn trong `repairable_findings` | `TestLastAuditCountsUnrepairedRepairableFindings` |
| Tìm Drive: quá 20, file trong thùng rác, q rỗng | 20, không có rác, 400 | `TestDriveSearchIsBoundedAndExcludesTrash` |
| Lọc job trên bảng lớn | dùng chỉ mục, không sort cả bảng | `TestJobListFiltersByCollectionAndPagesNewestFirst` (`EXPLAIN QUERY PLAN`) |

## Chưa làm

- UI (TUI/Telegram) chưa dùng search/last-audit; ghi móc ở `PROGRESS.md`.
- Chưa chạy với Drive thật; `Search` mới kiểm bằng `drivetest.Fake`.
