# Bot Telegram

[English](telegram.md) · Tiếng Việt

Telegram là tùy chọn và chạy ngay trong dịch vụ nền. Nó dùng cùng một lớp dịch vụ và
cùng cách phân quyền với CLI/HTTP; bot lỗi không làm dừng việc copy.

## Thiết lập và danh tính

Khi thiết lập, cung cấp một file token riêng tư (`telegram_token_file`) và ID số của
chủ sở hữu (`telegram_owner_id`). Trạng thái bot: `disabled`, `starting`, `polling`,
`error`. Mỗi token bot chỉ có một tiến trình nhận tin. Tin đến được ghi bền vào hộp thư
**trước khi** đánh dấu đã nhận, nên nhận lại cùng một tin không làm việc hai lần.

Bot chỉ nhận **chat riêng**. Chủ sở hữu là người điều hành chính; người khác cần được
bật làm "operator". Người lạ và nhóm chat không nhận được dữ liệu job hay thư viện nào.
Operator chỉ thấy công việc của chính họ, không thấy Drive của chủ sở hữu. Xem
[identity](identity-control.md).

## Copy và thẻ job

`/start`, `/menu` và `/help` hiện bàn phím cố định và menu điều hướng. Dán link Drive
hoặc dùng `/copy` để xem trước. Bản xem trước giữ nguyên tên nguồn và cho chọn đích, đổi
tên, bộ lọc/xử lý trùng/lối tắt, và có đồng bộ sau khi copy hay không. Nếu chưa có đích,
bot gợi ý Drive của tôi (My Drive). Đổi đích hoặc phê duyệt tự động copy đã cũ sẽ quay
lại bản xem trước, không âm thầm đổi nơi ghi.

Với cặp nguồn/đích đã copy trước đó, bot đưa ra lựa chọn cập nhật hoặc tạo bản mới.
Tự động copy là tùy chọn bật thêm và gắn với đúng đích đã chọn. Nguồn bị hạn chế hoặc
không hợp lệ nhận thông báo lỗi rõ ràng; giới hạn chia sẻ không bao giờ bị vượt qua.

Thẻ job hiện số lượng, lý do bỏ qua, tiến độ và các nút phù hợp trạng thái. Thẻ chỉ
cập nhật theo khoảng thời gian có giới hạn, tuân thủ Retry-After của Telegram và không
sửa khi không có gì đổi. Khi chờ hạn mức, thẻ hiện giờ tiếp tục theo múi giờ đã lưu và
không thử lại ngay. Hủy job luôn do bạn chọn rõ ràng; file đã copy xong vẫn được giữ.

## Nhiều link và hàng đợi

Dán nhiều link sẽ tạo một lô có xem trước, tối đa 20 link mỗi tin nhắn. Mỗi link có khóa
nhận việc riêng nên một link lỗi không làm hỏng các link khác. Bản copy đã có có thể
được bỏ qua thay vì tạo trùng. Khi cả lô xong, bot gửi một tin tóm tắt kèm các nút thử
lại/xem lỗi đúng phạm vi.

Chỉ chủ sở hữu mới tạm dừng/tiếp tục được cả hàng đợi (cùng một cờ bền như các ứng dụng
khác). Operator chỉ đổi thứ tự job của chính mình. Vị trí trong hàng đợi chỉ là ước
tính. Nút đã cũ sẽ làm thẻ tải lại thay vì ép di chuyển.

Danh sách job được lọc ở phía dịch vụ, và thao tác hàng loạt chỉ áp dụng cho job đủ điều
kiện. Hủy hàng loạt cần xác nhận; job đang chờ hạn mức không bị gộp vào thử lại ngay.
Kết quả đếm riêng số thành công, bỏ qua và thất bại.

## Bản copy và tìm kiếm

Thẻ thư viện có chế độ định kỳ/thời gian thực, xem trước các lần chạy tới, chạy ngay,
tạm dừng/dừng/tiếp tục, danh sách job của bản copy và kiểm tra toàn vẹn (chỉ đọc). Sửa
chữa luôn xem trước rồi mới xác nhận. "Ngừng theo dõi" giữ nguyên kết quả; "quên" chỉ xóa
bản ghi trong máy; dọn dẹp hỏi lại đúng tên thư mục gốc trước khi đưa bản copy do 502Drive
tạo vào thùng rác. Thẻ xác nhận cho thao tác nguy hiểm hết hạn nhanh hơn và gắn với
phiên bản bạn đang thấy.

`/find TÊN` có phạm vi khác nhau:

- Chủ sở hữu: tìm trong siêu dữ liệu Drive truy cập được rồi chọn nguồn để copy.
- Operator: chỉ tìm trong tên bản copy đã lưu của chính họ (kể cả bản đã ngừng theo
  dõi), xem chi tiết chỉ đọc và có phân trang. Không bao giờ tìm trong Drive của chủ.

Thẻ bản copy có nút **Bảo vệ** cho mirror, snapshot tự động, các mức giữ cố định, chụp
ngay, ghim/bỏ ghim và khôi phục. Mỗi nút chỉ dùng được một lần và gắn với phiên bản thẻ
đang hiện. Bật mirror có thẻ xác nhận riêng, giải thích về thùng rác Drive; khôi phục
được xem trước dưới dạng chạy thử, bạn phải đọc số thao tác rồi mới xác nhận lần hai để
áp dụng (việc đồng bộ sẽ tạm dừng). Xem [hướng dẫn bảo vệ](protection_VI.md).

## Xem trước sâu và báo cáo

Xem trước sâu chạy một lượt quét chỉ đọc ở nền, có nút làm mới/hủy. Con số chỉ được gọi
là "một phần" cho tới khi xong; lượt quét bị hủy, bị gián đoạn hoặc hết hạn không bao giờ
đưa ra tổng số. Ước tính và cảnh báo hạn mức/dung lượng chỉ mang tính tham khảo.

Báo cáo lỗi xuất file CSV theo phạm vi, gồm trạng thái, mã ổn định, lý do (tiếng Anh để
trao đổi), đường dẫn tương đối, số lần thử và ID Drive. Ô bắt đầu bằng ký tự công thức
được thoát an toàn. Giới hạn 5.000 dòng / 2 MiB và có báo rõ khi bị cắt bớt. Báo cáo
không chứa đường dẫn trên máy chủ, token hay thông báo thô của nhà cung cấp. Phần chú
thích dùng lời giải thích song ngữ chung.

## Thông báo

Tùy chọn cho phép bật/tắt thông báo hoàn tất, thất bại, cần xử lý và giờ yên tĩnh. Thông
báo tự phát bị hoãn đến hết giờ yên tĩnh; tin trả lời trực tiếp và bảng tiến độ do bạn
bắt đầu vẫn cập nhật bình thường. Lần đồng bộ không có thay đổi không gây thông báo. Lỗi
đăng ký lặp lại chỉ báo một lần cho tới khi hết lỗi.

Báo cáo hằng ngày là tùy chọn, theo múi giờ của bạn, tối đa một lần mỗi ngày và im lặng
khi không có gì đáng báo. Tóm tắt lô đợi tất cả job đã nhận hoàn tất; job tạm dừng vẫn
được chờ, còn job bị chặn tính là cần chú ý.

Việc gửi tới Telegram không đảm bảo đúng một lần. Hệ thống ghi nhận bền mỗi lần gửi để
không gửi lại mù quáng khi chưa rõ kết quả; nếu máy sập đúng lúc đó, một bản tóm tắt có
thể bị mất. Lỗi gửi tin không làm đổi kết quả của job. `/status` hiện sổ dung lượng
24 giờ của riêng bạn và giờ tiếp tục (nếu có), không bịa ra hạn mức của tài khoản Google.

## An toàn của nút bấm

Mỗi nút là mã không đọc được, gắn với người dùng/chat/tin nhắn và phiên bản, có hạn và
chỉ dùng được một lần. Nút không hợp lệ, đã dùng hoặc đã cũ sẽ hết hiệu lực chứ không lặp
lại thao tác ghi. Đọc dữ liệu mới không cho phép áp dụng xác nhận cũ với phiên bản mới.
Tên trong Drive được thoát HTML; nội dung chat không bao giờ được chạy bằng shell. Chuỗi
của ứng dụng có tiếng Anh và tiếng Việt; khóa tài nguyên và thông tin đăng nhập luôn
được giữ riêng tư.

## Giới hạn và mức kiểm thử

Chưa hỗ trợ: đăng nhập Google qua chat, dùng bot trong nhóm, trình chỉnh lịch/giờ tùy ý,
quản lý token HTTP cho operator, thông báo desktop qua bot này. Màu và kiểu nút phụ
thuộc ứng dụng Telegram; mọi chức năng vẫn phải dùng được khi không có trang trí.

Mã nguồn và test nằm ở [internal/telegram](../../internal/telegram). Một phiên thử ngắn
trên Linux với Telegram thật được ghi ở [verification](../verification.md); điều đó
chưa chứng minh mọi thao tác của bot, luồng operator, hạn mức hay lô lớn.
