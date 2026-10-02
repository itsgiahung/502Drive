const DOCKERFILE: &str = include_str!("../Dockerfile");
const DOCKER_COMPOSE: &str = include_str!("../docker-compose.yml");
const LINUX_INSTALLER: &str = include_str!("../packaging/install.sh");
const LINUX_PACKAGER: &str = include_str!("../packaging/package.sh");
const SYSTEMD_INSTALLER: &str = include_str!("../scripts/install-systemd-user.sh");
const BACKUP_SCRIPT: &str = include_str!("../scripts/backup.sh");
const RESTORE_SCRIPT: &str = include_str!("../scripts/restore.sh");

#[test]
fn docker_uses_canonical_binary_and_creates_legacy_alias() {
    assert!(
        DOCKERFILE.contains("RUN cargo build --release --bin 502drive"),
        "Docker builder must compile the canonical 502drive binary"
    );
    assert!(
        DOCKERFILE
            .contains("COPY --from=builder /app/target/release/502drive /usr/local/bin/502drive"),
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

#[test]
fn linux_installer_generates_daemon_unit_from_effective_config() {
    assert!(LINUX_INSTALLER.contains(
        "\"$HOME/.local/bin/502drive\" --config \"$CONFIG_DIR/config.toml\" service-install"
    ));
    assert!(!LINUX_INSTALLER.contains(
        "cp \"$SCRIPT_DIR/gdclone-bot.service\" \"$SYSTEMD_USER_DIR/gdclone-bot.service\""
    ));
    assert!(!LINUX_PACKAGER.contains("cp packaging/gdclone-bot.service \"$TAR_DIR/packaging/\""));
    assert!(
        SYSTEMD_INSTALLER.contains("\"$BINARY_PATH\" --config \"$CONFIG_PATH\" service-install")
    );
    assert!(!SYSTEMD_INSTALLER.contains("ReadWritePaths=$DATA_DIR"));
}

#[test]
fn shell_backup_uses_consistent_cli_snapshot() {
    assert!(BACKUP_SCRIPT.contains("\"$BIN\" --config \"$CONFIG_PATH\" backup \"$SNAPSHOT_DIR\""));
    assert!(!BACKUP_SCRIPT.contains("FILES+=(\"$DATA_DIR/\")"));
}

#[test]
fn shell_restore_never_extracts_archive_into_filesystem_root() {
    assert!(!RESTORE_SCRIPT.contains("tar -xzf \"$ARCHIVE\" -C /"));
    assert!(RESTORE_SCRIPT.contains("--no-same-owner --no-same-permissions"));
    assert!(RESTORE_SCRIPT.contains("Unexpected archive entry"));
    assert!(RESTORE_SCRIPT.contains("service remains stopped for verification"));
}
