# Thiết kế S11 — Linux Reliability

## Bối cảnh & Mục tiêu
Tăng cường độ tin cậy khi vận hành trên Linux và các hệ thống desktop/server:
1. Systemd user service: Cấu hình tài nguyên hợp lý (`Nice=10`, `IOSchedulingClass=best-effort`, `MemoryHigh=512M`, `TasksMax=64`), giữ vững security hardening.
2. `doctor` mở rộng các kiểm tra hệ thống:
   - `disk_space`: Kiểm tra dung lượng trống trên thư mục dữ liệu (cảnh báo nếu < 512MB).
   - `fd_limit`: Kiểm tra giới hạn file descriptors của tiến trình (yêu cầu >= 128).
   - `db_wal`: Kiểm tra kích thước database và file WAL (cảnh báo nếu WAL > 32MB).
   - `checkpoint_age`: Kiểm tra tình trạng checkpoint SQLite.
   - `secret_permissions`: Kiểm tra quyền các file bí mật (token, key, client secrets phải là 0600, không cho phép group/other đọc/ghi).
3. SQLite checkpoint & optimize:
   - `PRAGMA wal_checkpoint(TRUNCATE)` định kỳ khi nhàn rỗi và khi tắt daemon.
   - `PRAGMA optimize` định kỳ và trước khi đóng kết nối.
   - Lệnh offline: `502drive maintenance vacuum` (và alias `vacuum`).
4. Kịch bản soak test `scripts/soak.sh` để kiểm tra độ ổn định tải dài hạn (không chạy trong CI).
5. Đo đạc CPU và RSS nhàn rỗi ghi vào `docs/verification.md`.

## Chi tiết kỹ thuật

### Doctor Checks Interface
Hỗ trợ injection các hàm đọc thông số hệ điều hành (`DiskSpaceFn`, `FDLimitFn`) để unit test mô phỏng các tình huống lỗi: đĩa đầy giả, giới hạn fd thấp, file WAL lớn, quyền file bí mật không an toàn.

### SQLite Maintenance
- `internal/state/checkpoint.go`:
  - `Checkpoint(ctx)`: `PRAGMA wal_checkpoint(PASSIVE)`
  - `CheckpointTruncate(ctx)`: `PRAGMA wal_checkpoint(TRUNCATE)`
  - `Optimize(ctx)`: `PRAGMA optimize`
- `internal/maintenance/vacuum.go`:
  - Khóa `instance.lock` (bảo đảm an toàn offline, từ chối khi daemon đang chạy).
  - Mở SQLite DB, thực hiện `TRUNCATE` checkpoint, `VACUUM`, `PRAGMA optimize`.
  - Trả về báo cáo dung lượng trước/sau.
