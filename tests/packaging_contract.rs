const DOCKERFILE: &str = include_str!("../Dockerfile");
const DOCKER_COMPOSE: &str = include_str!("../docker-compose.yml");
const LINUX_INSTALLER: &str = include_str!("../packaging/install.sh");

#[test]
fn docker_uses_canonical_binary_and_creates_legacy_alias() {
    assert!(
        DOCKERFILE.contains("RUN cargo build --release --bin 502drive"),
        "Docker builder must compile the canonical 502drive binary"
    );
    assert!(
        DOCKERFILE.contains(
            "COPY --from=builder /app/target/release/502drive /usr/local/bin/502drive"
        ),
        "Docker runtime must copy the canonical binary"
    );
    assert!(
        !DOCKERFILE.contains(
            "COPY --from=builder /app/target/release/gdclone-bot /usr/local/bin/gdclone-bot"
        ),
        "Docker must not copy the legacy alias as an independently built binary"
    );
    assert!(
        DOCKERFILE.contains("ln -s /usr/local/bin/502drive /usr/local/bin/gdclone-bot"),
        "Docker runtime must preserve the gdclone-bot compatibility alias"
    );
}

#[test]
fn compose_does_not_publish_container_local_oauth_loopback_ports() {
    assert!(
        !DOCKER_COMPOSE.contains("51000-51100:51000-51100"),
        "OAuth listener binds container-local 127.0.0.1; publishing the range is misleading and ineffective"
    );
}

#[test]
fn linux_installer_keeps_secret_config_owner_only() {
    assert!(LINUX_INSTALLER.contains("chmod 700 \"$CONFIG_DIR\""));
    assert!(
        LINUX_INSTALLER.contains(
            "install -m 600 \"$REPO_DIR/config.sample.toml\" \"$CONFIG_DIR/config.toml\""
        )
    );
    assert!(LINUX_INSTALLER.contains("chmod 600 \"$CONFIG_DIR/config.toml\""));
}
