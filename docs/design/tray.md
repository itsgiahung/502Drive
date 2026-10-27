# Tray — thiết kế lại (T4)

## Quyết định thư viện
- **Giữ `fyne.io/systray` v1.12.2.** Lý do: đã có trong dự án, thuần Go trên Linux (SNI/D-Bus) và Windows
  (Shell_NotifyIcon), không cần cgo; đủ menu động (SetTitle/Enable/Disable/Hide) và tooltip; đổi thư viện sẽ đổi
  toàn bộ vòng lặp sự kiện. Chưa tìm được lý do kỹ thuật buộc phải đổi.
- Đã cân nhắc, chưa dùng: `getlantern/systray` (cùng họ, không có callback click trái/đúp rõ ràng hơn),
  `energye/systray` (thêm cgo trên một số nền tảng), SNI thuần qua godbus (tự viết nhiều hơn, phải tự xử lý
  AppIndicator/fallback). Không duyệt web trong phiên này; đánh giá dựa trên mã đã vendor và hành vi đã biết.

## Menu (thứ tự cố định, chỉ đổi nhãn/cờ bật tắt)
```
502Drive · Google: đã kết nối · 2 đang chạy · 0 đang chờ · 0 cần xử lý   (dòng trạng thái, không bấm)
───
Bật 502Drive                       (ẩn khi daemon đang chạy)
Clone từ clipboard                 (mờ kèm lý do: daemon chưa chạy / chưa đăng nhập Google)
Mở 502Drive                        (mở TUI; double-click icon trên Windows cũng mở)
Tác vụ & thư viện ›                (3 job gần nhất · Mở tất cả · Mở thư mục kết quả gần nhất)
───
Tạm dừng tất cả / Tiếp tục tất cả  (mờ khi daemon tắt; API S6 POST /v1/queue/pause|resume)
Cài đặt ›                          (Khởi động cùng hệ thống ✓)
───
Thoát khay                         (chỉ thoát khay; daemon chạy tiếp)
Dừng 502Drive…                     (hai bước khi còn việc; POST /v1/shutdown)
```
Mục trùng đã bỏ (Open TUI xuất hiện một lần). Khi lỗi, dòng trạng thái và tooltip nói cách khắc phục.

## Bảy trạng thái và icon
| Trạng thái | Điều kiện (ưu tiên từ trên xuống) | Icon |
| --- | --- | --- |
| offline | daemon không trả lời, hoặc máy đang ngủ | logo mờ 50% |
| error | `ready=false` | chấm đỏ |
| warning (`needs`) | có job `blocked`, tài khoản cần kết nối lại | chấm vàng |
| reconnecting | vòng đời `resuming` / `reconnecting` | chấm xanh dương |
| paused | hàng đợi tạm dừng (`queue.paused`) | hai vạch trắng ở góc |
| running (`busy`) | có job đang chạy hoặc chờ | vòng sáng quay (4 khung/giây) |
| idle | còn lại | logo |

Logo nền đen (`internal/tray/assets/logo-512.png`) giữ nguyên; các overlay vẽ trên logo. Test kiểm từng trạng thái
khác `idle` ở 16px.

## Tooltip
Tiêu đề, dòng trạng thái (tài khoản, số việc), và một dòng hướng dẫn khi có việc cần xử lý hoặc daemon tắt. Tooltip
không dài hơn một đoạn ngắn để Windows (giới hạn ~128 ký tự cho tooltip) vẫn hiển thị.

## Cập nhật chỉ khi đổi
- Mỗi lần poll dựng `View` như cũ, nhưng tính chữ ký (FNV-64a) của toàn bộ nội dung hiển thị. Chữ ký không đổi →
  không gọi API systray nào. Menu không bao giờ được dựng lại sau khi tạo.
- Icon chỉ đặt lại khi trạng thái đổi (trừ khung hoạt họa busy do animation điều khiển).
- Đo CPU nhàn rỗi: **chưa đo** (không có desktop thật trong môi trường này). Xem `docs/verification.md`.
- Poll vẫn là 3 s khi bận, 10 s khi rảnh (không dùng `/v1/stream` trong slice này; ghi là việc còn lại).

## Clipboard và clone
- Đọc clipboard chỉ khi bấm "Clone từ clipboard"; không đọc nền.
- Phải đúng **một** link Drive qua `drive.Parse` (test `TestReadDriveLinkValidatesTheClipboard`); clone dùng mặc định
  (tên gốc, My Drive). Kết quả hiển thị trong tiêu đề/dòng ghi chú.

## Ma trận click
| Thao tác | Linux (SNI/AppIndicator) | Windows (Shell_NotifyIcon) |
| --- | --- | --- |
| Click trái icon | tùy host: nhiều host chỉ mở menu | mở menu |
| Click phải icon | mở menu (SNI) | mở menu |
| Double-click icon | không có sự kiện chuẩn qua SNI; dùng mục "Mở 502Drive" | mở TUI (hành vi hiện có của `fyne.io/systray`) |
| Mục "Mở 502Drive" | mở terminal chạy `502drive menu` | như Linux |

Ghi chú: trên Linux không có sự kiện click trái đáng tin cậy qua SNI; mọi chức năng đều có trong menu.

## Thiếu sót đã biết
- Chưa chạy trên desktop thật (GNOME/KDE/Windows); evidence "OS thật" trong `docs/acceptance.md` để là "chưa chạy".
- Chưa có mục ngôn ngữ/thông báo trong submenu Cài đặt (đã có ở TUI/CLI).
