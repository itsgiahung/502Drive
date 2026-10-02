use teloxide::utils::command::BotCommands;

#[derive(Debug, Clone, BotCommands)]
#[command(rename_rule = "snake_case", description = "Các lệnh:")]
pub enum Command {
    #[command(description = "khởi động bot và xem hướng dẫn")]
    Start,
    #[command(description = "mở bảng điều khiển chính")]
    Menu,
    #[command(description = "hướng dẫn đăng nhập Google trên máy chạy bot")]
    Connect,
    #[command(description = "xem trạng thái tài khoản Google")]
    Account,
    #[command(description = "xem danh sách lệnh")]
    Help,
    #[command(description = "xem bảng trạng thái realtime trong Telegram")]
    Preview,
    #[command(description = "thu hồi token và ngắt Google Drive")]
    Disconnect,
    #[command(description = "clone Drive URL hoặc ID")]
    Clone(String),
    #[command(description = "clone ngay vào thư mục đích mặc định")]
    CloneHere(String),
    #[command(description = "liệt kê tác vụ đang chạy/tạm dừng")]
    Jobs,
    #[command(description = "xem trạng thái một tác vụ")]
    Status(String),
    #[command(description = "tạm dừng tác vụ")]
    Pause(String),
    #[command(description = "tiếp tục tác vụ đã tạm dừng")]
    Resume(String),
    #[command(description = "hủy tác vụ")]
    Cancel(String),
    #[command(description = "thử lại các mục lỗi")]
    Retry(String),
    #[command(description = "gửi lại báo cáo JSON/CSV của tác vụ gần nhất")]
    LastReport,
    #[command(description = "xem Telegram user id và quyền hiện tại")]
    Whoami,
    #[command(description = "đặt thư mục Drive đích mặc định")]
    SetDestination(String),
    #[command(description = "xem thư mục đích mặc định")]
    Destination,
    #[command(description = "xóa thư mục đích mặc định")]
    ClearDestination,
    #[command(description = "cấp quyền dùng bot cho user id")]
    Grant(String),
    #[command(description = "thu hồi quyền dùng bot của user id")]
    Revoke(String),
    // ── Watch & Sync commands ──────────────────────────────────────────────────
    #[command(description = "đồng bộ realtime từ nguồn sang đích: /sync <nguon> [dich]")]
    Sync(String),
    #[command(description = "theo dõi thư mục nguồn sang thư mục đích")]
    Watch(String),
    #[command(description = "liệt kê thư mục đang theo dõi")]
    Watches,
    #[command(description = "xem chi tiết đồng bộ")]
    WatchStatus(String),
    #[command(description = "tạm dừng đồng bộ")]
    WatchPause(String),
    #[command(description = "tiếp tục đồng bộ")]
    WatchResume(String),
    /// Cú pháp: "<id_watch> <policy>"
    /// policy hợp lệ: versioned_copy | replace_copy | manual_confirmation
    #[command(description = "đổi cách xử lý khi file nguồn thay đổi")]
    WatchPolicy(String),
    /// Cú pháp: "<id_watch> list|add <glob>|remove <glob>|clear"
    /// Ví dụ: "abc123 add *.tmp"
    #[command(
        description = "quản lý glob loại trừ file của watch: /watch_filter <id> list|add <glob>|remove <glob>|clear"
    )]
    WatchFilter(String),
    #[command(description = "dừng theo dõi thư mục")]
    Unwatch(String),
}
