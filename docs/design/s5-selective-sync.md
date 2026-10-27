# S5 — Lọc khi đồng bộ (selective sync)

Thiết kế cho §S5 (phase-12), bổ sung theo phase-13 mục 0.

## Mô hình

- `internal/filter`: `Spec` (lưu JSON trong plan, job, collection) và `Matcher` đã biên dịch. Thuần Go, không
  phụ thuộc state; `Decide(Entry)` được gọi cho từng mục khi liệt kê.
- Luật: glob (có `/` khớp đường dẫn gốc tương đối, `**` nhiều cấp; không `/` khớp tên), `ext:.pdf`, `class:…`,
  kích thước, `modified_after`. Exclude luôn thắng include. Thư mục chỉ bị loại bởi glob exclude.
- Trạng thái: mục bị loại là `skipped` với mã `filtered`; không bao giờ được ghi.

## Vị trí áp dụng

- Liệt kê khám phá (`scanTree` → `filterPage`) và liệt kê delta: mục mới được đánh dấu trước khi `CommitPage`
  / `CommitDeltaPage`. Thư mục bị loại được lưu với `list_state=done` nên không bị liệt kê.
- Sau khám phá: `PruneEmptyFolders` lặp đến khi không đổi, đánh dấu `filtered` các thư mục không còn con được
  giữ (trừ thư mục gốc của nguồn là thư mục; trừ khi `keep_empty_folders`). Mỗi lượt là một `UPDATE` với
  `items_collection_kind_state` và `items_parent`.
- Đường dẫn: `items.rel_path`, gán lúc liệt kê; đổi tên/di chuyển thư mục cập nhật cả cây con (`moveRelPath`).

## Thay đổi của bộ lọc

- `SetCollectionFilter` tăng `revision` (kiểm tra `expected_revision`, 409 nếu cũ). Lần chạy đang diễn ra giữ luật
  đã nhận khi được admit.
- `applyFilterDecision` (delta): mục đã copy không bị xóa hay copy lại; mục chưa ghi và nay bị loại thì
  `skipped`; mục bị loại trước đó nay được phép thì thành `pending` (thư mục được liệt kê lại); mục đang
  `executing`/`reconciling` không bị đụng.

## Hiệu năng

- Không thêm lệnh API trên đường liệt kê. Đo (10k file / 1k thư mục): `files.list` 26 cả khi không lọc và khi
  lọc (xem `docs/verification.md`).
- Không nạp cây vào RAM: quyết định theo từng trang.

## Giới hạn

- Xem trước chỉ trên một trang cấp cao nhất (một lệnh list, như trước đây).
- Lọc không áp dụng cho nguồn là tệp đơn.
- Đổi tên thư mục do delta: đường dẫn của các mục trong cây con cập nhật, nhưng quyết định của bộ lọc đối với mục
  con mới trong cùng lần chạy dùng đường dẫn thư mục đọc được lúc đó.
- Telegram: đã có preset (Bỏ video / Chỉ tài liệu / Bỏ file > 1 GB) và khối Đạt/Bị loại trên xem trước (đợt G). TUI: chưa (xem "Móc UI cho Claude").
