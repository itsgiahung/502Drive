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
| `capacity_exceeded` (429) | `status` running/queued | chờ job xong (retry cùng key/idempotency); không mở thêm job song song |
| `plan_expired` / `plan_changed` | plan TTL 10 phút, destination revision | chạy lại lệnh (inspect mới); không dùng lại plan cũ |
| `stale_revision` | thao tác từ màn cũ | refresh rồi thao tác lại; CLI `sync` tự retry một lần |
| `needs_action` | collection/item blocked | `jobs get ID`, xem lỗi; `jobs retry ID` chỉ khi an toàn |
| Sync `needs_reconcile` | cursor hết hạn/quyền | syncer tự lấy token mới + reconcile; xem `collections get ID` |
| Sync `waiting_for_baseline` | copy gốc chưa xong/partial | hoàn tất copy gốc hoặc `jobs retry` item lỗi |
| `bot_state=error` | token file, mạng | log "bot token rejected" → cập nhật token; core vẫn chạy |
| Tray không hiện (Linux) | `DBUS_SESSION_BUS_ADDRESS` | chạy trong phiên desktop; GNOME cần extension AppIndicator; tạm dùng `502drive menu` |
| Tray exit 1 "no desktop session" | headless/SSH | dùng TUI/CLI |
| `writer_running` khi backup/cleanup | daemon còn lock | `systemctl --user stop 502drive` (hoặc dừng container) rồi chạy lại |
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
