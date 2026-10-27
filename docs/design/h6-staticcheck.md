# H6 — Dọn staticcheck

Ngày: 2026-10-27. Không đổi hành vi.

- `staticcheck ./...` sạch. Đã xóa hàm chết thật sự: `cli.terminalWidth`, `state.rootSource`,
  `telegram.showBare`, `tui.plural`, `fakeAPI.pollCount`, `lifecycleFixture`.
- Sửa `SA4023` ở menu job: `menuJob` trả `nil` khi người dùng quay lại thay vì luôn trả lỗi.
- `ST1005` viết lại thông điệp lỗi credential; `S1039` bỏ `fmt.Sprintf` không tham số;
  `S1011` gộp vòng append trong TUI.
- Test: thay literal U+202E bằng `\u202e`, dùng `http.StatusBadGateway`, bỏ field/hàm test
  không dùng, dọn `SA4006` (giá trị model cuối vòng không đọc) bằng cách bỏ gán hoặc `_ =`.
- Không sửa `internal/mascot`, `packaging/assets/logo-*`, `.env`.
