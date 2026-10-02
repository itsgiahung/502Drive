const DOCKERFILE: &str = include_str!("../Dockerfile");

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
