# Cài bằng Docker

[Dockerfile](../Dockerfile): 2 tầng — `golang:1.27-bookworm` build
(`CGO_ENABLED=0 GOOS=linux`) → `gcr.io/distroless/static-debian12:nonroot`.
Image chỉ có binary + CA + tzdata; không shell, không curl. **Chưa** build/chạy
image trong phiên này.

```bash
docker build -t 502drive:latest .
```

## Chuẩn bị config (trên host, không vào image)

```bash
mkdir -p config data && chmod 700 config data
502drive setup --config ./config/config.json     # hoặc viết config JSON thủ công
# config.json cần: listen, data_dir=/data, token_file=/config/api-token,
#                  oauth_client_file=/config/client_secret.json (nếu login),
#                  telegram_token_file=/config/bot-token (nếu bot)
```

`setup` tạo token file 0600; mount cả thư mục `config`. Secret không nằm trong
image layer.

## Chạy

```bash
docker run -d --name 502drive \
  --restart unless-stopped \
  -v "$PWD/config:/config" -v "$PWD/data:/data" \
  -e TZ=Asia/Ho_Chi_Minh \
  502drive:latest
```

hoặc `docker compose up -d` với [docker-compose.yml](../docker-compose.yml).

- HTTP API bind **loopback trong container** ⇒ không publish cổng 5020. Muốn gọi
  từ host: `ssh -L 5020:127.0.0.1:5020 user@docker-host` hoặc
  `docker exec 502drive /usr/local/bin/502drive --config /config/config.json status`
  (binary có trong image, không cần shell).
- `/data` giữ `state-v2.db`, `secrets/` (key + token), `instance.lock`; recreate
  container không mất state nếu volume còn.

## OAuth trong container

Callback là loopback **trong container**; publish đúng cổng callback ra host khi
login:

```bash
docker run --rm -p 127.0.0.1:50200:50200 … 502drive:latest
# rồi trên host: 502drive account login (nếu CLI cùng máy) hoặc mở URL in ra
```

Nếu daemon ở máy khác, dùng `ssh -L 50200:127.0.0.1:50200` như CLI hướng dẫn.
Không publish `5020` ra ngoài; không dùng `network_mode: host` cho deployment
đầu tiên.

## Kiểm tra

```bash
docker logs 502drive
docker exec 502drive /usr/local/bin/502drive --config /config/config.json status
docker exec 502drive /usr/local/bin/502drive --config /config/config.json doctor
```

Không có HEALTHCHECK trong image (distroless không có curl/wget); dùng lệnh
`status`/`doctor` ở trên. Chưa hỗ trợ: Kubernetes manifests, image ký cosign,
multi-arch buildx trong Dockerfile.
