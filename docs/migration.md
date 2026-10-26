# Nâng cấp, backup và rollback

Rust/Tauri/React đã bị gỡ ngày **2026-10-26**; bản nén chỉ để tra cứu ngoài git
(`legacy-backup/`). Không có runtime legacy, không có importer: 502Drive Go dùng
data dir riêng (`state-v2.db`, `application_id=502002`), không đọc DB Rust.

## Cài mới

```bash
502drive setup                       # config.json + api-token (0600)
502drive setup --oauth-client client_secret.json
502drive setup --telegram-token-file bot-token --telegram-owner-id 123456789
502drive serve                       # foreground (systemd/container quản lý)
502drive account login               # link + chờ callback loopback
502drive destinations add FOLDER_LINK && 502drive destinations default ID
```

Data dir mặc định nằm cạnh config (`data-v2`) hoặc do bạn chọn; `serve` giữ
`instance.lock` và mở SQLite.

## Nâng cấp Go → Go

- Migration forward-only trong [internal/state/migrations](../internal/state/migrations)
  (hiện `001`–`005`), áp trong transaction khi `state.Open`; `user_version` =
  bản cao nhất. **Không sửa migration đã áp** — thêm file mới.
- Binary cũ gặp DB mới hơn sẽ từ chối mở (`unsupported database version`);
  nâng cấp một chiều.
- Quy trình: stop daemon → [backup](#backup-và-restore) → thay binary → `serve`
  → `502drive status` / `doctor` → quan sát job/sync.
- `serve --maintenance` đọc/status/doctor được, nhưng từ chối job mới, sync config
  và run now (`503 maintenance`), engine + syncer không chạy.

## Backup và restore

Source: [internal/maintenance](../internal/maintenance). Test:
[maintenance_test.go](../internal/maintenance/maintenance_test.go).

```bash
502drive backup /backups/502drive-$(date +%F)     # đích phải mới
502drive backup ... --help                        # (không có flag khác)
502drive restore /backups/502drive-2026-10-26 --to /srv/502drive-new
502drive cleanup [--dry-run|--apply] [--older-than 24h]
```

- `backup`: từ chối nếu daemon giữ `instance.lock`; `VACUUM INTO` DB + `secrets/master.key`
  + `secrets/google-token.enc` (nếu có) + `config.json`; thư mục 0700, file 0600,
  `MANIFEST.json` (schema_version, sha256 từng file, thời điểm, version binary).
- `restore`: chỉ vào data dir **mới/trống**; kiểm sha256, `integrity_check`,
  `foreign_key_check`, schema ≤ binary, giải mã token bằng key trong backup; lỗi
  → không để lại thư mục dở dang; `config.json` được sửa `data_dir` sang đích.
- `cleanup`: chỉ xóa `*.tmp` do chương trình tạo, cũ hơn ngưỡng, không theo
  symlink, không ra ngoài data dir, từ chối khi có writer. Không xóa DB/WAL/key/
  backup/mapping.

## Rollback

- **Binary + DB phải khớp.** Schema không tương thích: restore **cả bộ** backup
  vào data dir mới rồi trỏ config sang; không chép DB cũ đè lên DB mới.
- **Không rollback tác động trên Drive.** Restore DB cũ không xóa file đã tạo
  trên Drive kể từ backup. Sau rollback: freeze schedule/realtime, reconcile
  output bằng operation identity, chỉ resume khi trạng thái rõ. Không tự trash
  output để "undo".
- Nếu không thể reconcile an toàn: để job/sync `blocked`/`needs_reconcile` và xử
  lý tiến (forward), không hứa rollback một click.

## Docker và packaging

- Image: xem [install-docker](install-docker.md); dữ liệu ở volume `/data`
  (`state-v2.db`, `secrets/`, `instance.lock`).
- rpm/deb: xem [install-linux](install-linux.md); Windows: [install-windows](install-windows.md).
- Không chạy `backup/restore/cleanup` trên data thật khi daemon còn chạy.
