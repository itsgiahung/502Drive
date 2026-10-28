# Tạo khóa Google (OAuth)

[English](oauth-setup.md) · Tiếng Việt

Mỗi người dùng 502Drive tự tạo một **khóa Google riêng** (loại *Desktop app*). Khóa
nằm trên máy bạn; dịch vụ nền giữ nó, còn các giao diện khác (TUI, tray, Telegram)
không bao giờ nhận được khóa. Lý do phải tự tạo: file của bạn không đi qua dự án
Google của người khác. Việc này làm một lần, mất khoảng 5–10 phút.

## Tạo khóa (5 bước)

Google đổi tên khu vực này thành **Google Auth Platform**. Các link dưới đây mở đúng
trang hiện tại. Lệnh `502drive onboard` dẫn qua đúng 5 bước này ngay trong terminal:
nó tự mở từng trang rồi chờ bạn làm xong (Enter = đã xong, `s` = bỏ qua bước này,
`q` = thoát; dán đường dẫn file JSON ở bất kỳ bước nào để kết thúc sớm).

1. **Dự án**: tạo mới, hoặc chọn dự án có sẵn —
   <https://console.cloud.google.com/projectcreate>
2. **Drive API**: bật cho dự án đó —
   <https://console.cloud.google.com/apis/library/drive.googleapis.com>
3. **Màn hình đồng ý**: trong **Google Auth Platform → Branding** chọn
   **External**; trong **Audience** thêm địa chỉ Gmail bạn sẽ đăng nhập vào mục
   **Test user**, rồi bấm **Publish app** để trạng thái là **In production** —
   <https://console.cloud.google.com/auth/branding>
4. **Client**: **Google Auth Platform → Clients → Create client**, loại
   **Desktop app** — <https://console.cloud.google.com/auth/clients/create>
5. **Tải về**: bấm **Download JSON** trên client vừa tạo. File có tên dạng
   `client_secret….json`.

Nếu bạn tải nhầm loại **Web application**, 502Drive từ chối và nói rõ phải tạo loại
**Desktop app**. Nếu trong thư mục `~/Downloads` có file JSON Desktop hợp lệ vừa tải
trong vòng một ngày, `onboard` chỉ hỏi một câu "dùng file này không?". Nếu khóa đã được
cấu hình (trong cấu hình, biến môi trường hoặc `.env`) thì `onboard` không hỏi gì.

**Vì sao phải Publish?** Khi ứng dụng còn ở trạng thái **Testing**, Google cho phép
đăng nhập chỉ trong 7 ngày rồi hết hạn, nên mỗi tuần bạn phải đăng nhập lại
(`account_reconnect_required`). Publish ("In production") dùng cho cá nhân thì ổn: khóa là
của bạn, chỉ bạn dùng, Google không duyệt cho dùng cá nhân. Lúc đăng nhập, Google hiện
màn hình "ứng dụng chưa được xác minh": chọn **Advanced → Go to … (unsafe)**. Nếu bạn
đã đăng nhập rồi mới Publish, hãy chạy lại `502drive account login` để token mới không
bị giới hạn 7 ngày.

## Nạp khóa vào 502Drive

Chọn một cách:

```sh
502drive onboard   # có hướng dẫn như trên, rồi kiểm tra mọi điều kiện
502drive setup --oauth-client /duong/dan/client_secret.json
```

Hoặc đặt `GOOGLE_OAUTH_CLIENT_ID` và `GOOGLE_OAUTH_CLIENT_SECRET` trong file `.env`
riêng của bạn rồi chạy `502drive setup`. Lệnh `setup` đọc `./.env`, `.env` trong thư
mục cấu hình, hoặc `--env-file ĐƯỜNG_DẪN`; cờ dòng lệnh được ưu tiên hơn.

`setup` chép khóa vào một file riêng tư trong thư mục cấu hình và không in giá trị ra
màn hình. Đừng đưa file JSON gốc, `.env`, file cấu hình hay file token lên Git.

## Đăng nhập tài khoản Google

```sh
502drive up
502drive account login
502drive doctor --online
```

Phím `l` trong ứng dụng cũng chạy đúng luồng đăng nhập này. Nó mở trình duyệt và hiện
một đường link có thể sao chép. Trang đăng nhập sẽ quay về dịch vụ nền qua đúng địa
chỉ và cổng ghi trong `redirect_uri`. Phiên đăng nhập hết hạn sau 10 phút và mỗi mã
chỉ dùng được một lần.

Token Google được mã hóa AES-GCM trong `secrets/google-token.enc`, dùng khóa riêng
`secrets/master.key`. Mã đăng nhập và token không bao giờ được ghi vào log.

## Dịch vụ nền chạy trên máy khác (SSH)

Chuyển tiếp cổng callback từ máy bạn sang máy chạy dịch vụ **trước khi** mở link đăng
nhập. Dùng đúng cổng mà lệnh đăng nhập in ra:

```sh
ssh -L 50200:127.0.0.1:50200 USER@MAY_CHAY_DICH_VU
```

Số `50200` ở trên chỉ là ví dụ. Có thể đặt `oauth_callback_port` trong cấu hình nếu
muốn cổng cố định; đừng đoán một cổng khác.

## Container

Mở cổng callback ra loopback của máy chủ, gắn thư mục cấu hình và dữ liệu, rồi chạy
`account login` bên trong container. Xem [cài đặt bằng container](install-docker.md).
Không bao giờ nhét khóa vào image.

## Thu hồi hoặc kết nối lại

`502drive account revoke` xóa token trên máy và yêu cầu Google thu hồi quyền. Các job
có thể cần kết nối lại. Chạy lại `account login` khi gặp `account_reconnect_required`;
502Drive không bao giờ tự đổi sang tài khoản khác.

| Lỗi | Cần kiểm tra |
| --- | --- |
| `oauth_not_configured` | Nạp khóa loại Desktop qua `setup` hoặc `onboard` |
| Đăng nhập xong mà vẫn ngắt kết nối | Cổng callback, và quyền ghi vào file token |
| `account_reconnect_required` | Quyền đã bị thu hồi/hết hạn, hoặc khóa OAuth đã đổi |
| Đăng nhập trong container chờ mãi | Mở cổng callback, chuyển tiếp SSH |

Không hỗ trợ đăng nhập qua Telegram, tài khoản dịch vụ (service account), hay nhiều
tài khoản Google cùng lúc.
