# Telegram

Source: [internal/telegram](../../internal/telegram) — [telegram.go](../../internal/telegram/telegram.go)
(poll/inbox/offset), [handle.go](../../internal/telegram/handle.go),
[cards.go](../../internal/telegram/cards.go), [notify.go](../../internal/telegram/notify.go),
[notifier.go](../../internal/telegram/notifier.go), [render.go](../../internal/telegram/render.go).
Store: [state/telegram.go](../../internal/state/telegram.go), migration
[004](../../internal/state/migrations/004_telegram.sql),
[005](../../internal/state/migrations/005_notify.sql). Test:
[telegram_test.go](../../internal/telegram/telegram_test.go),
[notifier_test.go](../../internal/telegram/notifier_test.go) (fake Bot API qua httptest).

## Kết nối và định danh

- Một poller / bot token (`telegram_token_file`). Update được ghi vào
  `telegram_inbox` **trước** khi offset tiến (`IngestUpdates` một transaction);
  restart xử lý lại update pending/sẽ nhận lại; job không tạo hai lần vì key
  idempotency suy từ `update_id`.
- Chỉ private chat. `telegram_owner_id` → `app.Owner`; id khác phải là operator
  enabled (`principals.kind='telegram'`) → `app.Principal{owner}`; còn lại im lặng.
- Bot không chạy được (token sai, mất mạng) không làm daemon unready; `BotState`
  trả `disabled|starting|polling|error`.

## Menu và hành động

- Home: trạng thái Google/queue + nút Copy · Jobs · Library & sync · Destination ·
  Settings · Account. Không cần gõ lệnh.
- Dán link (hoặc `/copy`) → `Inspect` → thẻ xem trước đã có mặc định hợp lý:
  **giữ tên gốc** của nguồn và **lưu vào My Drive** khi chưa có thư mục đích nào;
  không cần setup trước lần copy đầu. Nút: Copy ngay · Đổi tên · Chọn nơi lưu ·
  Huỷ. Trong ngăn chọn nơi lưu: Lên / Trang sau / **Lưu vào đây** / **Thư mục mới**
  (tạo xong mở luôn). Lỗi hiện bằng ngôn ngữ đang chọn, kèm hướng xử lý.
  Test: [flow_test.go](../../internal/telegram/flow_test.go)
  `TestFirstLinkNeedsNoSetupAndKeepsTheSourceName`, `TestRenameAsksThenUsesTheTypedName`,
  `TestPickingAFolderChangesWhereTheCopyIsSaved`, `TestNewFolderFromTheChooserIsCreatedAndOpened`,
  `TestErrorsAreShownInTheUsersLanguage`.
- `auto_copy` bật và destination revision còn khớp → tự `Submit`; revision đổi
  → quay về preview (không tự đổi đích).
- Job card: counts, lỗi, nút Pause/Resume/Cancel/Retry theo state; một panel/job,
  edit tối đa mỗi 3s, 429 tôn trọng `retry_after`, "message not modified" là no-op,
  panel bị xóa → gửi lại một lần.
- Panel job đang chạy (`queued|running|pausing|cancelling|reconciling`) có
  hiệu ứng file đi từ 📂 sang 📁 (5 frame) + thanh tiến trình `▰▱` từ counts thật.
  Frame đổi theo mốc 3 giây của đồng hồ; số lần edit vẫn bị giới hạn bởi khoảng
  edit của outbox và không edit khi text không đổi. Job kết thúc dừng hiệu ứng,
  hiện emoji trạng thái và không còn thay đổi giữa các frame.
  Test: `TestJobPanelShowsABarAndAnAnimationFrameWhileRunning`.
- Library & sync: chọn collection → bật preset (30m/1h/6h/22:00/T2-4-6/realtime)
  → thẻ xác nhận hiện 3 lần chạy tới → Lưu; chạy ngay, tạm dừng, dừng.
- Callback: bảng `callback_states` (id ngẫu nhiên, principal/chat/message/action/
  object/revision/payload, TTL 15 phút, `consumed`). Nút ghi là one-shot; nút cũ/
  đã dùng/sai chat → toast "Nút đã hết hạn".
- Huỷ hàng loạt: `/cancelall` hoặc nút đỏ **Huỷ tất cả** ở trang Công việc →
  thẻ xác nhận (nút đỏ, không tự huỷ) → chỉ huỷ job mà principal đang chat sở hữu,
  job đã xong không bị đụng; kết quả "Đã huỷ N, bỏ qua M, lỗi K". Callback
  one-shot nên bấm hai lần chỉ hủy một lần. Test:
  [cancelall_test.go](../../internal/telegram/cancelall_test.go).

## Tuỳ chọn khi copy (đợt G)

Thẻ xem trước có thêm ba nhóm nút; mọi lựa chọn đi theo payload của nút (không lưu ở nơi khác), nên đổi tên hay
đổi nơi lưu không làm mất chúng. Mã: [options.go](../../internal/telegram/options.go).

- **Preset lọc (S5)**: `🎬 Bỏ video` (`exclude class:video`), `📄 Chỉ tài liệu` (`include class:doc`),
  `📦 Bỏ file > 1 GB` (`max_size 1073741824`); bấm lần nữa để tắt (nút đang bật có dấu ✓). Thẻ hiện
  `Đạt: N · Bị loại: M` + vài tên bị loại + ghi chú "chỉ tính cấp cao nhất" (xem trước một trang, như API).
- **Chính sách trùng tên (N1)**: một nút xoay vòng `đổi tên → bỏ qua → ghi cùng tên`; giá trị đi vào
  `on_conflict` của plan. `ghi cùng tên` không xoá bản cũ (giới hạn của N1).
- **Copy lại cùng nguồn (L2)**: nút Copy ngay gọi `SubmitMode(mode="")`; khi đã có bản copy tới đúng đích
  (`409 already_copied`) bot hỏi **🔄 Cập nhật bản cũ** (một delta run trên cùng collection) hay **🆕 Tạo bản
  mới**; cả `auto_copy` cũng hỏi thay vì tự nhân đôi. Bản cũ đã ngừng theo dõi → lỗi `collection_retired` kèm
  hướng xử lý.
- **Kết quả**: thẻ/thông báo job đếm mục bị bỏ qua theo mã (`⏭ Bỏ qua: bị bộ lọc loại ×3 · …`, K1); job
  `blocked` do quota hiện `⏳ Chờ quota tới HH:MM (múi giờ)` (múi giờ preferences), **không có nút Thử lại**
  vì job tự tiếp tục (Q1/S3); chỉ còn Huỷ.

## Vòng đời collection (L1)

Thẻ nguồn (Library) có nút **⏹ Ngừng theo dõi**. Mọi bước phá huỷ là thẻ xem trước + một nút đỏ; nút xác nhận là
callback one-shot gắn revision của collection lúc xem trước và **hết hạn sau 5 phút** (trước đó callback vẫn sống
15 phút, nhưng bot từ chối). Mã: [collection_life.go](../../internal/telegram/collection_life.go).

- Ngừng theo dõi → tắt sync, giữ file/lịch sử/ánh xạ (`RetireCollection`); đang có job chạy → lỗi nêu id job kèm
  hướng xử lý. Collection đã ngừng nằm trong **🗄 Đã ngừng (n)** ở Library, với **▶️ Theo dõi lại**,
  **🧹 Dọn đích (vào thùng rác)** và **🗑 Quên**.
- Dọn đích: bước 1 chạy job **dry-run** (không đổi Drive) và hiện id job; bước 2 (nút đỏ) tạo job thật, tên xác nhận
  lấy từ thư mục đích đã hiện trên thẻ. Chỉ đưa vào thùng rác Drive, không xoá vĩnh viễn.
- Quên: chỉ khi đã ngừng theo dõi; xoá bản ghi nội bộ, không đụng Drive.

## Cài đặt thông báo, giờ yên tĩnh, báo cáo hằng ngày (S10)

Trang Cài đặt có: công tắc `🔔 Xong / Lỗi / Cần xử lý` (ghi `notify_completed|failed|action_required` qua
`PatchPreferences`, mỗi người một bộ), nút `🌙 Giờ yên tĩnh 22:00–07:00`, và hàng chọn giờ **📰** (07:00, 12:00, 18:00,
21:00) + nút tắt. Mã: [settings_notify.go](../../internal/telegram/settings_notify.go),
[report.go](../../internal/telegram/report.go), [quiet.go](../../internal/telegram/quiet.go).

- **Báo cáo hằng ngày**: mặc định **tắt** (phải bật). Gửi đúng một lần mỗi ngày dương lịch ở múi giờ preferences
  (qua DST: giờ không tồn tại hay xảy ra hai lần vẫn chỉ một lần), nội dung 24 giờ gần nhất: job xong, lỗi/xong
  một phần, job cần xử lý, số lần đồng bộ có thay đổi, dung lượng và tốc độ trung bình. **Không có gì để nói thì
  không gửi** (và không tính lại cho đến ngày sau). Lịch lưu ở bảng `settings` (`tg_report:<principal>`,
  `tg_report_sent:<principal>`), tin đi qua outbox bền với khoá `report:<principal>:<ngày>` nên restart không gửi
  lặp. Mỗi người chỉ thấy job của mình (operator không thấy job của owner). Chỉ quét 500 job gần nhất.
- **Giờ yên tĩnh**: tin Telegram không do bạn vừa yêu cầu (job xong/lỗi do CLI, lỗi sync, báo cáo) **không bị bỏ**
  mà hoãn tới khi hết khung (`quietEnd`, theo múi giờ preferences, qua nửa đêm). Trả lời thao tác của bạn trong
  chat và panel tiến trình của job bạn tự bắt đầu ở chat không bị chặn. (Tray vẫn bỏ popup như trước.)

## Quản lý job và tìm trong Drive (S10)

- Trang Công việc có **🗂 Quản lý**: lọc theo trạng thái (Tất cả, Đang chạy, Chờ, Tạm dừng, Cần xử lý, Lỗi, Xong),
  6 job mỗi trang, và nút theo lô **⏸ Tạm dừng N / ▶️ Tiếp tục N / 🔁 Thử lại N / 🧹 Huỷ N** cho cả nhóm đang lọc.
  Mỗi job đi qua `JobAction` với revision hiện tại của nó; job đã đổi trạng thái thì **bỏ qua**, không ép; kết quả
  là số thật (`xong N, bỏ qua M, lỗi K`). Thử lại theo lô **không** đụng job đang chờ quota. Huỷ theo lô cần thẻ
  xác nhận (nút đỏ, 5 phút). Chỉ thấy/điều khiển job của chính mình. Mã:
  [jobs_manage.go](../../internal/telegram/jobs_manage.go).
- `/find tên`: liệt kê (tối đa 8, thư mục trước) mục ở **cấp cao nhất của My Drive** có tên chứa chuỗi (không phân
  biệt hoa thường), nút chọn mục làm nguồn mở thẻ xem trước copy. Chỉ owner (dùng `Browse` vốn chỉ cho owner);
  operator nhận lỗi "không có quyền", không có lệnh nào gọi tới Drive. Tìm sâu cả cây cần API tìm kiếm (xem báo
  cáo đợt G, "Cần backend"). Mã: [find.go](../../internal/telegram/find.go).
- **Lỗi có hướng xử lý**: thẻ lỗi luôn kèm dòng `💡` (bảng `errHint`, có mặc định), thẻ job lỗi/một phần cũng
  thêm gợi ý theo mã lỗi của job. Chuỗi đợt G nằm ở [texts_g.go](../../internal/telegram/texts_g.go) (vi + en, có
  test so khoá hai ngôn ngữ).

## Thông báo (job không tạo từ chat)

[notifier.go](../../internal/telegram/notifier.go) chạy từ `Engine.AfterRun` +
syncer trong daemon:

- Chỉ job không có panel chat. `owner` → `telegram_owner_id`; operator →
  `principals.external_id`.
- Tôn trọng `notify_completed|failed|action_required`. Job `succeeded` không có
  item mới (trigger ≠ initial, không item nào mang `job_id` của run) → **không báo**.
- Lỗi lặp của một subscription chỉ báo một lần (`notify_markers`); run thành công
  xóa marker. Enqueue vào outbox bền vững; gửi 403/429 không đổi trạng thái job.
- Sync provider lỗi cần người xử lý (auth/permission/not_found/invalid) cũng báo
  một lần cho owner/operator của collection.

## Ngôn ngữ và an toàn

- Chuỗi vi/en theo `Preferences.Language`; tên Drive luôn `html.EscapeString`;
  không gửi token/resource key.
- Không shell-execute nội dung chat; argv cố định ở mọi lệnh ngoài.

## Chưa hỗ trợ

- Nhập giờ HH:MM tuỳ biến (chỉ preset; báo cáo chỉ 4 mốc, giờ yên tĩnh chỉ 22:00–07:00 bằng nút, giờ khác đặt qua
  CLI/preferences API).
- Đăng nhập Google qua chat (chỉ hướng dẫn chạy CLI trên máy daemon).
- Quản lý allowlist operator bằng chat (owner dùng `POST /v1/operators`).
- `/find` tìm sâu cả cây Drive (cần `GET /v1/drive/search`), và cho operator.
- Ảnh/sticker mascot, dedupe UX khi paste cùng link đang có job, group chat (mặc định bỏ qua).
- Notification desktop; `/help` đầy đủ mọi lệnh (chỉ menu).
- Chưa chạy với bot Telegram thật và Drive thật: mọi test dùng bot giả (httptest) và `drivetest.Fake`.

## Menu (màu mè)

- `/start`, `/menu`, `/help` cài **bàn phím cố định** (`is_persistent`, 2 cột, 6 phím có emoji:
  Copy link, Công việc, Đồng bộ, Thư mục đích, Tài khoản, Cài đặt; ô nhập có gợi ý "Dán link
  Google Drive vào đây…") bằng một tin nhắn im lặng, rồi gửi thẻ chào: tiêu đề, lời chào theo
  tên, khối trích dẫn `<blockquote>` liệt kê tính năng, trạng thái Google/đích/job, và các nút
  inline. Bấm phím cố định hoặc nút inline đều mở cùng một trang.
- Nút inline có màu (`style`: `primary` xanh dương cho Copy, `success` xanh lá cho Xác nhận,
  Tiếp tục, Thử lại, `danger` đỏ cho Hủy). Telegram bản trước 2026-02 hiển thị nút không màu.
- Người lạ bấm phím cố định hoặc nhắn gì cũng không được trả lời (kiểm tra allowlist như cũ).
- Test: `TestStartInstallsAPersistentKeyboardThenAColourfulWelcome`,
  `TestKeyboardTapsOpenTheSamePagesAsTheButtons`, `TestKeyboardTapFromAStrangerIsIgnored` trong
  [telegram_test.go](../../internal/telegram/telegram_test.go).
