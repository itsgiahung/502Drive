# Mirror, snapshot và khôi phục

[English](protection.md) · Tiếng Việt

**Mirror và snapshot tự động đều TẮT mặc định.** Bạn bật riêng cho từng bản copy, bằng
dòng lệnh, màn hình chi tiết của bản copy trong ứng dụng, hoặc thẻ bản copy trong
Telegram.

- **Mirror**: khi một file bị xóa ở nguồn, bản copy tương ứng được đưa vào thùng rác
  Drive (vẫn khôi phục được trong khoảng 30 ngày). Không bao giờ xóa vĩnh viễn.
- **Snapshot** (điểm khôi phục): ghi nhớ bản copy trông thế nào ở một thời điểm để quay
  lại khi cần.

## Bật và cấu hình

Sau khi copy xong, lấy mã bản copy bằng `collections list`:

```sh
502drive sync options col_ví_dụ
502drive sync options col_ví_dụ --snapshots=true --keep-days 30 --keep-count 10
502drive snapshots create col_ví_dụ
502drive sync options col_ví_dụ --mirror=true
```

Chỉ đổi được tùy chọn khi bản copy không có job nào đang chạy (kể cả job bị tạm dừng
hoặc bị chặn). Cờ nào bạn không ghi thì giữ nguyên giá trị cũ; `--dry-run` chỉ xem
trước. Mirror áp dụng cho các lần đồng bộ sau đó. Chụp snapshot thủ công luôn dùng
được, kể cả khi snapshot tự động đang tắt.

## Trong ứng dụng và Telegram

Màn hình chi tiết bản copy có hàng **Bảo vệ** (dưới Bộ lọc và Kiểm tra toàn vẹn);
thẻ bản copy trong Telegram có nút **🛡 Bảo vệ**. Cả hai cho cùng một menu: bật/tắt
mirror, bật/tắt snapshot tự động, số ngày giữ (7, 30 hoặc 90) và số điểm không ghim
(5, 10 hoặc 20) chọn từ danh sách có sẵn (không có ô gõ tự do), chụp ngay, danh sách
snapshot kèm ghim/bỏ ghim, và khôi phục.

- **Bật mirror luôn hỏi trước** và nói rõ hậu quả: file bị xóa ở nguồn sẽ vào thùng
  rác Drive. Trong Telegram, đây là một thẻ xác nhận riêng.
- **Khôi phục luôn xem trước trước.** Màn hình hoặc thẻ hiện số thao tác sẽ làm; chỉ khi
  đó mới có nút "Áp dụng", kèm một xác nhận nói rõ rằng việc đồng bộ sẽ bị tạm dừng.
- Ứng dụng liệt kê 100 điểm mới nhất, Telegram 8 điểm mới nhất; muốn xem cũ hơn dùng
  dòng lệnh.

## Mirror hoạt động thế nào

Drive trả lời "không tìm thấy" như nhau cho file đã bị xóa và file mà bạn không còn
quyền xem. Nếu chủ thư mục nguồn thu hồi quyền của bạn với một số file, mirror coi
chúng là đã xóa và đưa bản copy vào thùng rác (khôi phục được khoảng 30 ngày). Cơ chế
chặn "mất hàng loạt" vẫn dừng lượt chạy khi một phần lớn nguồn biến mất.

- Chỉ chạy sau khi quét nguồn đầy đủ và copy thành công; lỗi khi liệt kê, copy hoặc đối
  soát, hay chạm ngưỡng mất hàng loạt, đều ngăn mirror.
- Chỉ đụng tới bản copy do chính 502Drive tạo (kiểm tra dấu sở hữu). Không có thao tác
  nào xóa vĩnh viễn trên Drive.
- Xử lý file con trước, chỉ đưa đi thư mục đã rỗng. File bạn tự thêm vào bản copy được
  giữ nguyên; thư mục còn file lạ sẽ làm job bị chặn.
- Việc loại trừ bằng bộ lọc **không** phải yêu cầu xóa: bản copy cũ của các file đó ở
  lại.
- Bản copy còn được snapshot tham chiếu thì không vào thùng rác mà được chuyển sang
  thư mục lưu trữ `.502drive-versions-<mã bản copy>` nằm cạnh bản copy.
- File có bản y hệt (cùng thư mục, cùng tên, cùng nội dung, bị bỏ qua vì trùng) còn ở nguồn thì
  bản copy được giữ; mirror chỉ xử lý khi cả hai đã mất.
- Nếu file quay lại ở nguồn, nó được copy lại như file mới.

## Snapshot và khôi phục

```sh
502drive snapshots list col_ví_dụ
502drive snapshots show snap_ví_dụ
502drive snapshots pin snap_ví_dụ
502drive snapshots unpin snap_ví_dụ
502drive snapshots restore snap_ví_dụ                  # xem trước kế hoạch
502drive snapshots plan job_ví_dụ                      # xem lại kế hoạch đó
502drive snapshots restore snap_ví_dụ --apply          # áp dụng; tạm dừng đồng bộ
502drive sync resume col_ví_dụ                         # cho đồng bộ chạy lại
```

Danh sách có tối đa 100 dòng mỗi trang; dùng `--cursor` để xem trang tiếp. Lệnh tạo và
khôi phục mặc định chờ cho đến khi xong; `--wait=false` chỉ gửi yêu cầu rồi thoát,
`--wait-timeout` chỉ dừng việc chờ chứ không hủy job.

**Snapshot không phải bản sao lưu độc lập.** Nó chỉ ghi nhớ file nào của nguồn tương ứng
với bản copy nào (và tên, thư mục cha, nội dung). Nó không giữ lịch sử phiên bản, chủ
sở hữu hay quyền chia sẻ của Google. Nếu file đã bị sửa nội dung hoặc đã biến mất khỏi
Drive thì không khôi phục được, và file bị xóa vĩnh viễn thì mất hẳn.

Khôi phục kiểm tra mọi bản copy trước khi ghi, khôi phục thư mục cha trước thư mục con,
rồi mới đưa đi những file thừa so với điểm khôi phục (chỉ file do 502Drive tạo). Việc
áp dụng **tạm dừng đồng bộ** của bản copy và để nó ở trạng thái tạm dừng cho tới khi bạn
bật lại; khi bật lại, các thay đổi hiện tại của nguồn sẽ được áp dụng. Bản xem trước
không ghi gì lên Google và không đổi trạng thái đồng bộ.

## Giữ lại và dung lượng

Mặc định giữ 30 ngày và 10 điểm không ghim; điểm đã ghim nằm ngoài cả hai giới hạn.
Khi điểm hết hạn, chỉ bản ghi trong máy bị xóa chứ file trên Drive không bị xóa. Không
dọn bản copy cũ được snapshot bảo vệ: lệnh dọn dẹp bị từ chối khi snapshot còn cần nó.
`forget` xóa bản ghi trong máy và giữ nguyên file trên Drive.

**Kho lưu trữ vẫn chiếm dung lượng Drive của bạn** ngay cả khi điểm đã hết hạn; chưa có
tính năng tự dọn kho lưu trữ này. Sửa hoặc xóa ngoài 502Drive có thể làm một điểm khôi
phục không còn dùng được dù bản ghi của nó vẫn còn.

## Đã kiểm thử đến đâu

Mirror, snapshot và khôi phục đã chạy thử với Google Drive thật (xem
[verification](../verification.md)); Shared Drive thì chưa. Các bản copy đã có không
tự bật các tính năng này.
