# Docker and Podman

English · [Tiếng Việt](install-docker_VI.md)

The [Dockerfile](../Dockerfile) builds a static Go binary and copies it into a
non-root distroless image. There is no shell or curl in the runtime image;
`502drive healthcheck` supplies the health check.

## Configure and start

```sh
cp .env.example .env
502drive setup --config ./config/config.json --container
sudo chown -R 65532:65532 ./config
docker compose up -d --build
```

For rootless Podman, use `podman unshare chown -R 65532:65532 ./config` instead
of host `sudo chown`. The non-root container user must be able to read its private
configuration and token files. On SELinux hosts use the volume labels in the
compose file, or `:z` / `:Z` on equivalent mounts.

Container mode binds the API inside the container. Published ports must remain on
**host loopback**, including the OAuth callback:

```sh
docker run -d --name 502drive --restart unless-stopped --stop-timeout 30 \
  -v "$PWD/config:/config:z" -v 502drive-data:/data \
  -p 127.0.0.1:5020:5020 -p 127.0.0.1:50200:50200 \
  -e TZ=UTC 502drive:latest
```

Container mode is the explicit exception to the daemon's ordinary loopback-only
bind. Binding to container loopback prevents bridge-network published ports from
reaching it. Do not publish the API to all host interfaces.

## Health and authorization

```sh
docker compose ps
docker compose exec 502drive 502drive healthcheck
docker compose logs -f 502drive
docker compose exec -it 502drive 502drive account login
```

Open the printed OAuth link on your machine; its callback must reach the published
loopback port. See [OAuth setup](oauth-setup.md). The token is stored in `/data`,
not an image layer. Host CLI access can use `--endpoint http://127.0.0.1:5020`
and `--token-file ./config/api-token`.

## Backup and upgrade

Stop the service before offline maintenance; preserve the configuration mount
and data volume across container replacement.

```sh
docker compose stop
docker compose run --rm 502drive backup /data/backups/backup-new
docker compose up -d
```

Copy backups outside the volume as well. They contain secrets. To validate a
restore, stop the daemon and restore to a **new empty** path:

```sh
docker compose run --rm 502drive restore /data/backups/backup-new --to /data/restored
```

Inspect it before pointing configuration/volumes at the restored directory.
For upgrades: stop, back up, build/pull the desired version, recreate, then check
health and Doctor. Migrations are forward-only; a previous binary may not open
an upgraded database. Restoring local state does not undo Google Drive operations.

## Operational limits

- Podman needs a compose provider for `podman compose`; otherwise use build/run.
- Podman's OCI image format may ignore Docker `HEALTHCHECK`. Use Docker format or
  invoke `podman exec 502drive /usr/local/bin/502drive healthcheck` explicitly.
- Telegram long polling needs no inbound port. A bot error does not make core
  readiness fail.
- Prefer the documented bridge-network ports over host networking.
- A Podman image/health/API smoke test was recorded on 2026-10-27. That record does
  not establish Docker Compose, live OAuth or every volume configuration.
- No bundled Kubernetes manifests, signed-image attestation or complete native
  multi-platform deployment acceptance. See [verification](verification.md).
