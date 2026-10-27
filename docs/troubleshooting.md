# Troubleshooting

Triệu chứng thật gặp với code Go. Kiểm tra trước: `502drive status`,
`502drive doctor` (thêm `--online` để gọi Google), log của `serve`.

| Triệu chứng | Kiểm tra | Cách xử lý |
| --- | --- | --- |
| `daemon_unavailable` | `serve` còn chạy? endpoint/token đúng? | chạy `502drive serve`; kiểm `--endpoint`, `--token-file`/`DRIVE502_TOKEN_FILE` |
| `unauthenticated` (401) | token file có bị thay? | dùng token hiện tại trong `config.json.token_file`; đừng copy token cũ |
| `not_ready` (409) | schema, lock, config | xem log; nếu "data directory already has a writer" → tắt daemon khác; schema mới hơn binary → nâng binary |
| `account_reconnect_required` | `account status`, `doctor --online` | `502drive account login`; nếu quyền bị thu hồi, cấp lại trên Google |
| `destination_required` | `destinations list` | `destinations add FOLDER_LINK` + `destinations default ID` |
| `destination_unverifiable`/`destination_inside_source` | cây đích | chọn folder đích khác; đích không được nằm trong nguồn |
| `destination_not_writable` (409) | đích Shared Drive/read-only | chọn đích khác hoặc xin quyền `canAddChildren` |
| `copy_not_permitted` (403) | nguồn chặn copy | bỏ mục đó; cả cây thì nó thành `skipped/cannot_copy` |
| `capacity_exceeded` (429) | `status` running/queued | chờ job xong (retry cùng key/idempotency); không mở thêm job song song |
| `queue.paused=true` | ai bấm tạm dừng? | `502drive resume` (hoặc tray *Tiếp tục tất cả*); job tự pause vẫn pause |
| `suspending` (503) | máy sắp ngủ | chờ vài giây rồi thử lại; không phải lỗi cấu hình |
| `collection_busy` (409) | collection còn job chạy | chờ/pause job rồi retire/cleanup lại |
| `collection_retired` / `collection_not_retired` | vòng đời | `collections resume ID` trước, hoặc `forget` chỉ sau retire |
| `already_copied` (409) | đã có bản sao nguồn→đích | `copy --update` (làm mới) hoặc `--new` (bản mới) |
| Plan `deep.state=interrupted` | daemon restart giữa quét sâu | chạy lại `inspect --deep`; kết quả cũ không phải tổng |
| Audit `extra_at_destination` | object không có dấu của ta | chỉ báo, không tự sửa; kiểm tay trước khi xóa |
| Repair không đổi gì | mặc định dry-run | xem `audit show ID`, rồi `audit COLLECTION --repair --apply` |
| Không thấy thông báo Telegram ban đêm | giờ yên tĩnh | thông báo chờ tới hết cửa sổ; trả lời lệnh của bạn vẫn gửi ngay |
| `plan_expired` / `plan_changed` | plan TTL 10 phút, destination revision | chạy lại lệnh (inspect mới); không dùng lại plan cũ |
| `stale_revision` | thao tác từ màn cũ | refresh rồi thao tác lại; CLI `sync` tự retry một lần |
| `needs_action` | collection/item blocked | `jobs get ID`, xem lỗi; `jobs retry ID` chỉ khi an toàn |
| Sync `needs_reconcile` | cursor hết hạn/quyền | syncer tự lấy token mới + reconcile; xem `collections get ID` |
| Sync `waiting_for_baseline` | copy gốc chưa xong/partial | hoàn tất copy gốc hoặc `jobs retry` item lỗi |
| `bot_state=error` | token file, mạng | log "bot token rejected" → cập nhật token; core vẫn chạy |
| Tray không hiện (Linux) | `DBUS_SESSION_BUS_ADDRESS` | chạy trong phiên desktop; GNOME cần extension AppIndicator; tạm dùng `502drive menu` |
| Tray exit 1 "no desktop session" | headless/SSH | dùng TUI/CLI |
| `writer_running` khi backup/cleanup | daemon còn lock | `systemctl --user stop 502drive` (hoặc dừng container) rồi chạy lại |
| `doctor` báo quyền/thư mục | secret 0644, data dir thiếu | `502drive doctor --fix` (chỉ owner; không đụng Drive/token) |
| DB phình sau nhiều job | event/intent cũ | dừng daemon → `prune --older-than 90d` (dry-run trước) → `vacuum` |
| `target_exists` khi restore | đích không trống | restore vào dir mới/trống, không ghi đè dir đang dùng |
| `backup_integrity`/`backup_secret` | manifest/key/token | dùng backup khác; thiếu `master.key` thì không giải mã được token |
| Login treo | callback port/namespace | dùng `ssh -L <port>:127.0.0.1:<port>` đúng port CLI in ra; hoặc đặt `oauth_callback_port` cố định |
| Login xong vẫn disconnected | callback không tới daemon | kiểm port publish (Docker) / firewall / state param hết hạn |
| Copy chậm/không tiến | `jobs events ID`, `doctor --online` | 429/quota → chờ cooldown; provider lỗi → reconnect; không xóa DB/lock |
| DB đầy đĩa | dung lượng data dir | dừng daemon, dọn log/backup cũ, `cleanup` file tạm; không xóa DB/WAL/mapping |

## Không nên làm

- Không xóa `instance.lock`/DB/WAL/`master.key` để "chữa treo".
- Không dùng `git clean -fdx` hay xóa volume để dọn dữ liệu người dùng.
- Không retry bằng idempotency key khác khi chưa chắc request trước đã fail.
- Không đánh giá "đã sửa" chỉ vì process còn sống; xem `status`/events.
