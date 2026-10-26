# Nghiên cứu và nguồn tham khảo

Rút gọn từ giai đoạn thiết kế; chỉ giữ nguồn còn dùng để kiểm chứng quyết định
trong code Go hiện tại. Không có benchmark riêng của dự án.

## Google Drive

- Drive API v3 — files.list/get/create/copy, `supportsAllDrives`,
  `includeItemsFromAllDrives`, appProperties (intent key):
  <https://developers.google.com/workspace/drive/api/reference/rest/v3>
- Changes API (start page token, list, `newStartPageToken`):
  <https://developers.google.com/workspace/drive/api/guides/manage-changes>
- OAuth Desktop app + PKCE loopback:
  <https://developers.google.com/identity/protocols/oauth2/native-app>

Code: [internal/drive](../internal/drive). Test: [client_test.go](../internal/drive/client_test.go),
[reference_test.go](../internal/drive/reference_test.go).

## Telegram

- Bot API (getUpdates/offset, sendMessage, editMessageText, callback_query,
  429 `retry_after`): <https://core.telegram.org/bots/api>
- FAQ giới hạn bot: <https://core.telegram.org/bots/faq>

Code: [internal/telegram](../internal/telegram); SDK `fyne.io/systray` không liên
quan; bot dùng `github.com/go-telegram/bot` v1.27.0 cho phần gửi.

## Terminal UI / tray

- Bubble Tea: <https://github.com/charmbracelet/bubbletea>
- Lip Gloss: <https://github.com/charmbracelet/lipgloss>
- systray (Linux StatusNotifier qua D-Bus, Windows): <https://github.com/fyne-io/systray>
- go-winres: <https://github.com/tc-hib/go-winres>

## SQLite và Go

- `modernc.org/sqlite` (thuần Go, WAL, `VACUUM INTO`): <https://pkg.go.dev/modernc.org/sqlite>
- `time/tzdata` để zone IANA không phụ thuộc hệ điều hành:
  <https://pkg.go.dev/time/tzdata>

## Đóng gói

- GoReleaser v2: <https://goreleaser.com/customization/>
- nfpm (rpm/deb): <https://nfpm.goreleaser.com/>
- Inno Setup 6: <https://jrsoftware.org/isinfo.php>
- distroless: <https://github.com/GoogleContainerTools/distroless>

## Quyết định đã chốt và lý do (tóm tắt)

| Quyết định | Lý do |
| --- | --- |
| Một binary Go + HTTP-first | adapter dùng chung service; không engine thứ hai |
| SQLite một writer | serialize transaction, idempotency/key rõ ràng |
| Server-side copy, không relay | tránh tải bytes, giữ identity nguồn bằng ID |
| Realtime = Changes polling | không cần endpoint public; cursor + run cùng transaction |
| Callback Telegram one-shot + TTL | chống double click, không đoán ID |
| Không tự enable systemd/autostart | chỉ owner bật, không side effect khi cài |
