# S12 — Live integration test harness & Fault injector

Thiết kế cho §S12 (phase-12-spec).

## Mục tiêu

1. Cung cấp bộ giả lập lỗi HTTP (`drive.FaultInjector`) bọc `http.RoundTripper` để chèn lỗi 429 (rateLimitExceeded), mất kết nối (connection drop / dial error), mất phản hồi sau khi ghi (lose response / EOF), và độ trễ vào các cuộc gọi Google Drive API mà không cần sửa logic engine.
2. Bộ injector có test đầy đủ ở chế độ không-live (`internal/drive/faultinject_test.go`), chạy tự động trong CI / local test suite thông thường.
3. Cung cấp harness kiểm thử tích hợp trực tiếp với Google Drive thật (`internal/live`), được bảo vệ bằng build tag `live` và biến môi trường `DRIVE502_LIVE=1`.
4. Mọi thao tác ghi của live test được cô lập hoàn toàn bên trong thư mục `502drive-live-test-<unix>` do test tự tạo ở thư mục gốc của tài khoản test, và tự dọn dẹp chuyển vào thùng rác (`TrashFile`) khi test kết thúc. Tuyệt đối không chạm vào bất kỳ file/thư mục nào ngoài thư mục này.

## Thiết kế chi tiết

### 1. `drive.FaultInjector` (`internal/drive/faultinject.go`)

`FaultInjector` bọc một `http.RoundTripper` cơ sở (`http.DefaultTransport` nếu nil):

```go
type FaultAction int

const (
    FaultPass FaultAction = iota
    FaultThrottle429      // Trả 429 rateLimitExceeded kèm Retry-After và JSON chuẩn Google
    FaultDropConnection   // Trả net.OpError / connection reset mô phỏng đứt cáp/mất mạng
    FaultLoseResponse     // Trả io.ErrUnexpectedEOF / network EOF mô phỏng mất phản hồi sau ghi
    FaultServerError      // Trả 500 / 503 Internal Server Error
    FaultCustom           // Thực thi hàm tuỳ biến func(*http.Request) (*http.Response, error)
)

type FaultRule struct {
    Method       string        // "GET", "POST", "PATCH"... ("" = mọi method)
    PathContains string        // Chuỗi con trong URL path/query ("" = mọi URL)
    Action       FaultAction
    StatusCode   int           // Mặc định 429 cho FaultThrottle429, 500 cho FaultServerError
    RetryAfter   time.Duration // Header Retry-After (giây)
    Err          error         // Lỗi trả về cho FaultDropConnection / FaultLoseResponse
    Delay        time.Duration // Trễ trước khi xử lý
    MaxHits      int           // Số lần kích hoạt tối đa (0 = không giới hạn)
    SkipHits     int           // Bỏ qua N lần đầu trước khi kích hoạt
    Match        func(req *http.Request) bool
    Custom       func(req *http.Request) (*http.Response, error)
}
```

#### API thuận tiện:
- `NewFaultInjector(base http.RoundTripper) *FaultInjector`
- `(f *FaultInjector) AddRule(r *FaultRule)`
- `(f *FaultInjector) Inject429(pathContains string, maxHits int, retryAfter time.Duration)`
- `(f *FaultInjector) InjectDrop(pathContains string, maxHits int)`
- `(f *FaultInjector) InjectLoseResponse(pathContains string, maxHits int)`
- `(f *FaultInjector) ClearRules()`
- `(f *FaultInjector) Hits() int`

#### Tích hợp với `drive.Client`:
- Hàm `NewClientWithTransport(ctx, ts, endpoint, transport)` tạo `*Client` sử dụng `http.Client` có Transport là `FaultInjector`.

### 2. Live Test Harness (`internal/live/`)

#### Điều kiện kích hoạt:
- Build tag: `//go:build live` ở các file live test (`live_test.go`).
- Biến môi trường: `DRIVE502_LIVE=1`. Nếu không có, `CheckLive(t)` gọi `t.Skip(...)`.
- Khóa / thông tin xác thực kiểm thử:
  - `DRIVE502_LIVE_CREDENTIALS`: JSON chứa token hoặc credentials, hoặc
  - `DRIVE502_LIVE_CLIENT_SECRET` + `DRIVE502_LIVE_TOKEN_ENC` (hoặc data dir của test account).

#### Quy tắc cô lập và dọn dẹp:
- Mỗi lần chạy test tạo một thư mục duy nhất: `502drive-live-test-<unix_timestamp>`.
- Mọi file và folder con phục vụ kịch bản test (source, dest, intermediate copies) chỉ được tạo bên trong thư mục này.
- Kết thúc test (`t.Cleanup`), harness gọi `TrashFile(liveFolderID)` đưa thư mục vào thùng rác Google Drive.

#### Kịch bản kiểm thử:
1. **OAuth Refresh**: Khởi tạo client từ token nguồn, kiểm tra lấy thông tin tài khoản `Account(ctx)`.
2. **Clone cây nhỏ**: Tạo cây mẫu (`source/` gồm 1 thư mục con và 2 file nhỏ), thực hiện copy sang `dest/`, kiểm tra tên, phân cấp và kích thước ở đích.
3. **Crash / Interrupt & Resume**: Bật FaultInjector để làm mất phản hồi một lượt ghi giữa chừng, engine phát hiện và hồi phục bằng intent (`FindByIntent`), hoàn thành không bị trùng file.
4. **Sync phát hiện thay đổi**: Khởi tạo subscription, thêm file mới vào thư mục nguồn, poll changes, kiểm tra file mới được phát hiện và đồng bộ sang đích.
5. **Telegram Bot thật** (tùy chọn): Nếu có `DRIVE502_LIVE_TELEGRAM_TOKEN`, chạy kiểm tra probe bot getMe.
