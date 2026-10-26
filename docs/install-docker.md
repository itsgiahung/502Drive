# Chạy bằng Docker / Podman

Dockerfile: 2 tầng — `golang:1.27-bookworm` build (`CGO_ENABLED=0`) →
`gcr.io/distroless/static-debian12:nonroot`. Image chỉ có binary + CA + tzdata;
không shell, không curl. `HEALTHCHECK` chạy chính binary (`502drive healthcheck`).

## Ba lệnh chạy nhanh

```bash
cp .env.example .env                 # TZ + đường dẫn config; giá trị mẫu là giả
502drive setup --config ./config/config.json --container
docker compose up -d --build
```

- `--container` đặt `"container": true` và `listen: "0.0.0.0:5020"` **chỉ** trong
  config của container; `docker-compose.yml` publish hai cổng **loopback trên
  host**: `127.0.0.1:5020` (API) và `127.0.0.1:50200` (callback OAuth). Không
  publish cổng nào ra ngoài; đừng bỏ tiền tố `127.0.0.1:`.
- Dữ liệu nằm trong volume `502drive-data` (`state-v2.db`, `secrets/`,
  `instance.lock`); config/token/key nằm trong `./config` (0600). Recreate
  container không mất state.
- `container: true` là ngoại lệ duy nhất của luật "chỉ nghe loopback": nếu bạn
  sửa config về `127.0.0.1:5020` trong container thì cổng publish sẽ không tới
  được daemon (Docker DNAT vào địa chỉ mạng của container).

## Kiểm tra

```bash
docker compose ps                    # health: starting -> healthy
docker compose exec -it 502drive 502drive healthcheck   # exit 0 = ready
docker compose logs -f 502drive
docker compose exec -it 502drive 502drive --config /config/config.json status
docker compose exec -it 502drive 502drive --config /config/config.json doctor
```

Từ host: `502drive --endpoint http://127.0.0.1:5020 --token-file ./config/api-token status`
(hoặc `DRIVE502_ENDPOINT=http://127.0.0.1:5020`).

## Đăng nhập Google trong container

```bash
docker compose exec -it 502drive 502drive account login
```

Link hiện ra mở trên máy bạn; callback quay về `127.0.0.1:50200` đã publish sẵn.
`secrets/google-token.enc` nằm trong volume `/data`, không nằm trong image.

## Backup volume

`backup` từ chối chạy khi daemon đang giữ `instance.lock`, nên dừng dịch vụ trước:

```bash
docker compose stop
docker compose run --rm 502drive backup /data/backups/$(date +%F)
docker compose up -d
```

`backup` tạo thư mục mới 0700 (DB `VACUUM INTO`, `master.key`,
`secrets/google-token.enc`, `config.json`, `MANIFEST.json`). Copy thư mục đó ra
khỏi volume (`docker cp`/`rsync`) để có bản ngoài máy. Khôi phục:

```bash
docker compose stop
docker compose run --rm 502drive restore /data/backups/2026-10-27 --to /data/restored
# kiểm xong: đổi volume/config trỏ vào /data/restored, hoặc giữ nguyên và chỉ thử
docker compose up -d
```

## Cập nhật image

```bash
git pull
docker compose build --pull
docker compose up -d                    # recreate; migration tự chạy, chỉ tiến
docker compose exec -it 502drive 502drive healthcheck
```

Rollback: `docker tag 502drive:latest 502drive:previous` trước khi build, hoặc
chạy lại tag cũ. Không hạ schema: bản cũ hơn sẽ từ chối DB mới (`schema_version`).

## Ghi chú vận hành

- Podman: `podman compose up -d --build` (cần `podman-compose` hoặc provider
  docker-compose). Build không cần mạng ngoài sau khi đã có layer `go mod download`.
- Không dùng `network_mode: host` cho deployment mặc định: publish loopback là
  đủ, và healthcheck/port rule ở trên giả định bridge network.
- Telegram long polling không cần mở cổng vào. Bot tắt/sai token không làm
  container unhealthy.
- Chưa hỗ trợ: Kubernetes manifests, ký cosign, buildx multi-arch trong Dockerfile
  (dùng `docker buildx build --platform linux/amd64,linux/arm64` được, nhưng CI
  chỉ build amd64/arm64 bằng GoReleaser).
