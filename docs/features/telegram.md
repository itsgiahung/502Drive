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
- Dán link (hoặc `/copy`) → `Inspect` → thẻ preview (Copy / Đổi đích / Hủy).
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

- Duyệt folder đích trong chat, tạo folder; nhập giờ HH:MM tuỳ biến (chỉ preset).
- Đăng nhập Google qua chat (chỉ hướng dẫn chạy CLI trên máy daemon).
- Bật/tắt từng loại notification bằng nút (đổi qua CLI/preferences API).
- Quản lý allowlist operator bằng chat (owner dùng `POST /v1/operators`).
- Ảnh/sticker mascot, nút "copy bản mới" riêng, dedupe UX khi paste cùng link
  đang có job, group chat (mặc định bỏ qua).
- Notification desktop; digest; `/help` đầy đủ mọi lệnh (chỉ menu).
