use gdclone_bot::{
    config::GoogleOAuthConfig,
    drive::auth::{
        choose_redirect_port, decrypt_token, encrypt_token, wait_for_loopback_code,
    },
    secrets::FileSecretStore,
    state::{db::Database, repo},
};
use tokio::io::{AsyncReadExt, AsyncWriteExt};
use uuid::Uuid;

#[tokio::test]
async fn file_secret_store_encrypts_and_decrypts_refresh_token() {
    let dir = std::env::temp_dir().join(format!("gdclone-test-{}", Uuid::new_v4()));
    let store = FileSecretStore::new(&dir);
    let key = store.load_or_create_key().await.unwrap();

    let ciphertext = encrypt_token("refresh-token", &key).unwrap();
    assert_ne!(ciphertext, b"refresh-token");
    assert_eq!(decrypt_token(&ciphertext, &key).unwrap(), "refresh-token");

    let loaded_key = store.load_or_create_key().await.unwrap();
    assert_eq!(loaded_key, key);

    let _ = tokio::fs::remove_dir_all(dir).await;
}

#[tokio::test]
async fn google_account_secret_round_trips_ciphertext_only() {
    let db = Database::in_memory().await.unwrap();
    db.run_migrations().await.unwrap();

    repo::upsert_google_account(
        &db,
        "default",
        Some("user@example.com"),
        vec![9, 8, 7],
        "file",
        r#"["https://www.googleapis.com/auth/drive"]"#,
    )
    .await
    .unwrap();

    let account = repo::google_account_secret(&db, "default")
        .await
        .unwrap()
        .unwrap();
    assert_eq!(account.refresh_token_ciphertext, vec![9, 8, 7]);
    assert_eq!(account.status, "connected");
    assert_eq!(account.email.as_deref(), Some("user@example.com"));

    repo::mark_account_revoked(&db, "default").await.unwrap();
    let account = repo::google_account_secret(&db, "default")
        .await
        .unwrap()
        .unwrap();
    assert_eq!(account.status, "revoked");
}

async fn send_loopback_request(port: u16, path: &str) -> String {
    let mut stream = None;
    for _ in 0..100 {
        match tokio::net::TcpStream::connect(("127.0.0.1", port)).await {
            Ok(connected) => {
                stream = Some(connected);
                break;
            }
            Err(_) => tokio::time::sleep(std::time::Duration::from_millis(10)).await,
        }
    }
    let mut stream = stream.expect("OAuth loopback listener did not start");
    let request = format!(
        "GET {path} HTTP/1.1\r\nHost: 127.0.0.1:{port}\r\nConnection: close\r\n\r\n"
    );
    stream.write_all(request.as_bytes()).await.unwrap();
    let mut response = Vec::new();
    stream.read_to_end(&mut response).await.unwrap();
    String::from_utf8(response).unwrap()
}

#[tokio::test]
async fn oauth_loopback_ignores_wrong_state_before_valid_callback() {
    let listener = tokio::net::TcpListener::bind(("127.0.0.1", 0))
        .await
        .unwrap();
    let port = listener.local_addr().unwrap().port();
    drop(listener);

    let waiter = tokio::spawn(async move {
        wait_for_loopback_code(port, "expected-state").await.unwrap()
    });

    let rejected =
        send_loopback_request(port, "/oauth2/callback?code=evil&state=wrong-state").await;
    assert!(rejected.starts_with("HTTP/1.1 400"));

    let accepted =
        send_loopback_request(port, "/oauth2/callback?code=real-code&state=expected-state").await;
    assert!(accepted.starts_with("HTTP/1.1 200"));
    assert_eq!(waiter.await.unwrap(), "real-code");
}

#[tokio::test]
async fn oauth_redirect_port_chooser_uses_free_port_in_range() {
    let listener = tokio::net::TcpListener::bind(("127.0.0.1", 0))
        .await
        .unwrap();
    let port = listener.local_addr().unwrap().port();
    drop(listener);

    let config = GoogleOAuthConfig {
        client_id: "client.apps.googleusercontent.com".to_string(),
        client_secret: "secret".to_string(),
        redirect_port_start: port,
        redirect_port_end: port,
        scope: "https://www.googleapis.com/auth/drive".to_string(),
    };

    assert_eq!(choose_redirect_port(&config).await.unwrap(), port);
}
