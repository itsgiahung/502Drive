# syntax=docker/dockerfile:1

# ------------------------------------------------------------------------------
# Build stage — pure-Go daemon, statically linked (modernc.org/sqlite, so
# CGO_ENABLED=0 is safe and no libc is required at runtime).
# ------------------------------------------------------------------------------
FROM golang:1.27-bookworm AS builder

WORKDIR /src

# Dependency layer first so `go mod download` is reused across source edits.
COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

# Stamp the release version: --build-arg VERSION=$(git describe --tags)
ARG VERSION=docker

RUN CGO_ENABLED=0 GOOS=linux go build \
        -trimpath \
        -ldflags "-s -w -X github.com/itsgiahung/502Drive/internal/cli.Version=${VERSION}" \
        -o /out/502drive ./cmd/502drive \
    && mkdir -p /out/config /out/data

# ------------------------------------------------------------------------------
# Runtime stage — distroless static, non-root, no shell.
# ------------------------------------------------------------------------------
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /out/502drive /usr/local/bin/502drive
# Mount points owned by nonroot (65532) in the image, so anonymous volumes
# inherit the right ownership on first create.
COPY --from=builder --chown=65532:65532 /out/config /config
COPY --from=builder --chown=65532:65532 /out/data /data

USER nonroot
WORKDIR /data
VOLUME ["/data"]

# distroless/static-debian12 already ships CA certificates and tzdata.
ENV TZ=Asia/Ho_Chi_Minh
# Config JSON: /config/config.json on the config volume, or a path under /data.
# Override with `--config PATH` or DRIVE502_CONFIG.
ENV DRIVE502_CONFIG=/config/config.json

# `serve` reads that config JSON, requires data_dir + token_file, and keeps all
# state in the data dir: state-v2.db, secrets/ and instance.lock. Mount /data at
# a persistent location.
#
# Networking: the HTTP API listens on a loopback address only (127.0.0.1 by
# default), so no public inbound port is needed. Google OAuth uses a loopback
# callback (config key oauth_callback_port): bind loopback + publish/forward the
# OAuth callback port when authorizing in-container, and do not expose a public
# inbound port by default.
ENTRYPOINT ["/usr/local/bin/502drive"]
CMD ["serve"]
