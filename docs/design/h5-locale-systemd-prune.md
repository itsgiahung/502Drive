# H5 — Dọn từ đợt B: locale, systemd --user, prune

Ngày: 2026-10-27. Không đổi JSON/exit code.

## Chữ người đọc

- Lệnh nghiệp vụ vẫn in tiếng Anh (hợp đồng CLI đã công bố). Phần **giá trị** dùng
  chung package: `present.HumanBytes` cho dung lượng, `present.Reason` cho mã K1,
  `i18n.FormatTime`/`FormatDate` cho ngày giờ.
- Ngôn ngữ + múi giờ lấy từ `GET /v1/preferences` một lần cho mỗi lệnh (cache trong
  `App`), không đọc DB. Thiếu daemon/preferences thì rơi về locale máy và `time.Local`;
  lỗi định dạng một timestamp không được làm mất dữ liệu (in nguyên văn).
- Ngày giờ áp dụng cho `ls`/`info` (ngày sửa/tạo), `jobs get` (`created`/`updated`),
  `jobs list` (cột `updated`). JSON giữ RFC3339 gốc, không đổi.
- `App.locale` chỉ gọi khi có giá trị ngày giờ cần in (không thêm request cho `status`,
  `doctor`, lệnh JSON).
- Snapshot: `internal/cli/testdata/human/report.txt` chặn đổi bảng báo cáo người đọc;
  snapshot JSON/exit code sẵn có vẫn giữ nguyên.

## systemd `--user`

- `internal/autostart/systemd.go` sinh unit người dùng
  `$XDG_CONFIG_HOME/systemd/user/502drive.service` (mặc định `~/.config/...`), dựa trên
  unit đóng gói: `Restart=on-failure`, giới hạn restart, `Nice`, `MemoryHigh`, `TasksMax`,
  hardening, `[Install] WantedBy=default.target`. Đường dẫn dưới `$HOME` dùng `%h`.
- `502drive autostart systemd status|install|remove`: `install` ghi unit 0600, chạy
  `systemctl --user daemon-reload` rồi `enable` (không tự `start`); `remove` disable + xóa.
  Lệnh XDG (`autostart enable`) vẫn là mặc định để mở tray; unit chỉ dành cho daemon.
- `doctor` thêm check `systemd_user`: không có unit → passed (tray/XDG vẫn chạy);
  có unit và enabled → passed; có nhưng chưa enable → **failed** kèm lệnh sửa.
  Check chỉ xuất hiện khi daemon nối hook (Linux); test tiêm trạng thái, không gọi systemd thật.

## Prune

- `state.Prune` đã bỏ qua job còn được items/plans/sync/audit/pending outbox tham chiếu;
  thêm test `TestPruneKeepsJobsReferencedByAuditOrPendingOutbox` (audit + outbox pending,
  job không tham chiếu vẫn bị xóa, `PRAGMA foreign_key_check` sạch).

## Cửa sổ lỗi

| Tình huống | Kỳ vọng | Test |
| --- | --- | --- |
| Preferences vi + Asia/Ho_Chi_Minh | `ls` in `dd/mm/yyyy`, đổi ngày qua múi giờ; `info --json` vẫn UTC | `TestHumanDatesUsePreferenceLocaleAndTimezone` |
| Không có preferences/daemon | rơi về locale máy, không lỗi | cùng đường dự phòng |
| Unit enabled/disabled/missing | doctor failed chỉ khi installed-but-disabled | `TestDoctorSystemdUserUnitIsActionable` |
| Unit ghi hai lần | idempotent, quyền 0600 | `TestSystemdInstallIsIdempotentAndStatusReportsEnabled` |
| Job còn audit/outbox pending | không xóa, không lỗi khóa ngoại | `TestPruneKeepsJobsReferencedByAuditOrPendingOutbox` |

## Chưa làm

- Chưa dịch toàn bộ câu lệnh CLI sang vi: hợp đồng hiện tại là tiếng Anh; chỉ giá trị
  (ngày giờ, dung lượng, lý do) theo locale.
- Chưa chạy `systemctl --user` thật trong test (dùng ExecCmd giả); `systemd-analyze
  verify` cho unit sinh ra chưa chạy trong CI.
