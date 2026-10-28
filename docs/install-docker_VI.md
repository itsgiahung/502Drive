# Docker và Podman

[English](install-docker.md) · Tiếng Việt

[Dockerfile](../Dockerfile) build một file Go chạy độc lập rồi đặt vào image
"distroless" chạy bằng người dùng thường. Image không có shell hay curl; lệnh
`502drive healthcheck` đảm nhiệm việc kiểm tra sức khỏe.

## Cấu hình và khởi động

```sh
cp .env.example .env
502drive setup --config ./config/config.json --container
sudo chown -R 65532:65532 ./config
docker compose up -d --build
```

Với Podman không cần quyền root, dùng `podman unshare chown -R 65532:65532 ./config`
thay cho `sudo chown`. Người dùng trong container phải đọc được file cấu hình và token
riêng tư. Trên máy bật SELinux, dùng nhãn volume trong file compose hoặc thêm `:z` /
`:Z` ở các điểm gắn tương đương.

Chế độ container mở API bên trong container. Các cổng công bố ra ngoài phải luôn nằm
trên **loopback của máy chủ**, kể cả cổng callback OAuth:

```sh
docker run -d --name 502drive --restart unless-stopped --stop-timeout 30 \
  -v "$PWD/config:/config:z" -v 502drive-data:/data \
  -p 127.0.0.1:5020:5020 -p 127.0.0.1:50200:50200 \
  -e TZ=UTC 502drive:latest
```

Chế độ container là ngoại lệ có chủ ý của quy tắc "chỉ nghe trên loopback". Tuyệt đối
không mở API ra mọi địa chỉ của máy chủ.

## Sức khỏe và đăng nhập

```sh
docker compose ps
docker compose exec 502drive 502drive healthcheck
docker compose logs -f 502drive
docker compose exec -it 502drive 502drive account login
```

Mở link đăng nhập in ra trên máy của bạn; đường quay về phải tới được cổng loopback đã
công bố. Xem [hướng dẫn OAuth](oauth-setup_VI.md). Token nằm trong `/data`, không nằm
trong image. Từ máy chủ có thể dùng CLI với `--endpoint http://127.0.0.1:5020` và
`--token-file ./config/api-token`.

## Sao lưu và nâng cấp

Dừng dịch vụ trước khi bảo trì ngoại tuyến; giữ nguyên thư mục cấu hình và volume dữ
liệu khi thay container.

```sh
docker compose stop
docker compose run --rm 502drive backup /data/backups/backup-new
docker compose up -d
```

Hãy chép bản sao lưu ra ngoài volume. Chúng chứa thông tin bí mật. Muốn thử khôi phục,
dừng dịch vụ rồi khôi phục vào một thư mục **mới và trống**:

```sh
docker compose run --rm 502drive restore /data/backups/backup-new --to /data/restored
```

Kiểm tra kỹ rồi mới trỏ cấu hình hoặc volume vào thư mục đã khôi phục. Khi nâng cấp:
dừng, sao lưu, build hoặc kéo phiên bản mới, tạo lại container, rồi kiểm tra sức khỏe và
`doctor`. Cơ sở dữ liệu chỉ nâng cấp tiến lên; bản cũ có thể không mở được dữ liệu đã
nâng cấp. Khôi phục dữ liệu cục bộ không hoàn tác những gì đã làm trên Google Drive.

## Giới hạn

- Podman cần một trình hỗ trợ compose cho `podman compose`; nếu không thì dùng build/run.
- Định dạng OCI của Podman có thể bỏ qua `HEALTHCHECK` của Docker. Dùng định dạng Docker
  hoặc chạy trực tiếp `podman exec 502drive /usr/local/bin/502drive healthcheck`.
- Telegram dùng long polling nên không cần mở cổng vào. Bot lỗi không làm dịch vụ chính
  báo "chưa sẵn sàng".
- Ưu tiên cổng trên mạng bridge như trong tài liệu, thay vì dùng host networking.
- Mới có một lần kiểm thử Podman (image, health, API) vào ngày 27/10/2026; chưa kiểm
  Docker Compose, đăng nhập OAuth thật hay mọi kiểu volume. Xem
  [verification](verification.md).
- Chưa có manifest Kubernetes và chữ ký cho image.
