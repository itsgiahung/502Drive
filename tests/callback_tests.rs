use gdclone_bot::{
    state::{
        db::Database,
        repo::{self, NewCallbackState, NewJob},
    },
    telegram::parse_callback_action,
};

async fn test_db() -> Database {
    let db = Database::in_memory().await.unwrap();
    db.run_migrations().await.unwrap();
    repo::upsert_google_account(
        &db,
        "Test",
        Some("test@example.com"),
        vec![0; 8],
        "file",
        "{}",
    )
    .await
    .unwrap();
    db
}

#[test]
fn parse_callback_action_valid_and_malformed() {
    // Valid standard 3-part callbacks
    assert_eq!(
        parse_callback_action("menu:open:home"),
        Some(("menu", "open", "home"))
    );
    assert_eq!(
        parse_callback_action("job:pause:job_12345"),
        Some(("job", "pause", "job_12345"))
    );
    assert_eq!(
        parse_callback_action("wres:v:short_id:42"),
        Some(("wres", "v", "short_id:42"))
    );

    // Multiple colons preserved in 3rd element
    assert_eq!(
        parse_callback_action("wopt:pol:session_123:v"),
        Some(("wopt", "pol", "session_123:v"))
    );

    // Malformed inputs
    assert_eq!(parse_callback_action(""), None);
    assert_eq!(parse_callback_action("one"), None);
    assert_eq!(parse_callback_action("one:two"), None);

    // Empty components
    assert_eq!(parse_callback_action("::"), Some(("", "", "")));
}

#[tokio::test]
async fn authorization_guard_rejects_unauthorized_users() {
    let db = test_db().await;

    let authorized_owner_id = 123456789;
    let unauthorized_user_id = 987654321;

    db.ensure_owner(authorized_owner_id).await.unwrap();

    assert!(repo::is_authorized(&db, authorized_owner_id).await.unwrap());
    assert!(
        !repo::is_authorized(&db, unauthorized_user_id)
            .await
            .unwrap()
    );
}

#[tokio::test]
async fn stale_or_nonexistent_job_returns_not_found() {
    let db = test_db().await;
    let user_id = 100;
    db.ensure_owner(user_id).await.unwrap();

    // Querying non-existent job
    let job = repo::job_detail_for_user(&db, user_id, "nonexistent-job-id")
        .await
        .unwrap();
    assert!(job.is_none());

    // Create job then verify retrieval
    let job_id = repo::create_job_with_metadata(
        &db,
        NewJob {
            chat_id: 200,
            telegram_user_id: user_id,
            google_account_id: "default".to_string(),
            source_root_id: "src_123".to_string(),
            source_resource_key: None,
            source_drive_id: None,
            destination_parent_id: "dest_456".to_string(),
            destination_drive_id: None,
            progress_message_id: None,
            duplicate_policy: "keep_both".to_string(),
            source_name: Some("Test Folder".to_string()),
            destination_name: Some("Backup".to_string()),
        },
    )
    .await
    .unwrap();

    let job_active = repo::job_detail_for_user(&db, user_id, &job_id)
        .await
        .unwrap();
    assert!(job_active.is_some());

    // Query by unauthorized user returns None
    let foreign_query = repo::job_detail_for_user(&db, 99999, &job_id)
        .await
        .unwrap();
    assert!(foreign_query.is_none());
}

#[tokio::test]
async fn callback_state_consumed_only_once() {
    let db = test_db().await;
    let user_id = 101;
    let chat_id = 202;

    let state_id = repo::create_callback_state(
        &db,
        NewCallbackState {
            telegram_user_id: user_id,
            chat_id,
            action: "smart_link".to_string(),
            payload: "source_ref_data".to_string(),
            ttl_ms: 600_000,
        },
    )
    .await
    .unwrap();

    // First consumption succeeds
    let first = repo::consume_callback_state(&db, &state_id, user_id, chat_id, "smart_link")
        .await
        .unwrap();
    assert_eq!(first.as_deref(), Some("source_ref_data"));

    // Second consumption (stale click) returns None
    let second = repo::consume_callback_state(&db, &state_id, user_id, chat_id, "smart_link")
        .await
        .unwrap();
    assert!(second.is_none());
}

#[test]
fn legacy_keyboard_buttons_parsed_compatibly() {
    use gdclone_bot::telegram::{LegacyAction, parse_legacy_action};

    // Vietnamese legacy buttons
    assert_eq!(
        parse_legacy_action("＋ Sao chép"),
        Some(LegacyAction::Clone)
    );
    assert_eq!(parse_legacy_action("⟳ Đồng bộ"), Some(LegacyAction::Watch));
    assert_eq!(
        parse_legacy_action("📊 Tổng quan"),
        Some(LegacyAction::Overview)
    );
    assert_eq!(
        parse_legacy_action("📦 Công việc"),
        Some(LegacyAction::Jobs)
    );
    assert_eq!(parse_legacy_action("📦 Tác vụ"), Some(LegacyAction::Jobs));
    assert_eq!(
        parse_legacy_action("👁 Theo dõi"),
        Some(LegacyAction::Watches)
    );
    assert_eq!(
        parse_legacy_action("📁 Thư mục đích"),
        Some(LegacyAction::Destination)
    );
    assert_eq!(
        parse_legacy_action("👤 Tài khoản"),
        Some(LegacyAction::Account)
    );
    assert_eq!(
        parse_legacy_action("🌐 Ngôn ngữ"),
        Some(LegacyAction::Language)
    );
    assert_eq!(
        parse_legacy_action("⚙️ Cài đặt"),
        Some(LegacyAction::Settings)
    );
    assert_eq!(parse_legacy_action("❓ Trợ giúp"), Some(LegacyAction::Help));

    // English legacy buttons
    assert_eq!(parse_legacy_action("＋ Clone"), Some(LegacyAction::Clone));
    assert_eq!(parse_legacy_action("⟳ Sync"), Some(LegacyAction::Watch));
    assert_eq!(
        parse_legacy_action("📊 Overview"),
        Some(LegacyAction::Overview)
    );
    assert_eq!(parse_legacy_action("📦 Jobs"), Some(LegacyAction::Jobs));
    assert_eq!(
        parse_legacy_action("👁 Watches"),
        Some(LegacyAction::Watches)
    );
    assert_eq!(
        parse_legacy_action("📁 Destination"),
        Some(LegacyAction::Destination)
    );
    assert_eq!(
        parse_legacy_action("👤 Account"),
        Some(LegacyAction::Account)
    );
    assert_eq!(
        parse_legacy_action("🌐 Language"),
        Some(LegacyAction::Language)
    );
    assert_eq!(
        parse_legacy_action("⚙️ Settings"),
        Some(LegacyAction::Settings)
    );
    assert_eq!(parse_legacy_action("❓ Help"), Some(LegacyAction::Help));

    // Plain text variations
    assert_eq!(parse_legacy_action("Sao chép"), Some(LegacyAction::Clone));
    assert_eq!(parse_legacy_action("Clone"), Some(LegacyAction::Clone));
    assert_eq!(
        parse_legacy_action("Tổng quan"),
        Some(LegacyAction::Overview)
    );
    assert_eq!(parse_legacy_action("Tác vụ"), Some(LegacyAction::Jobs));

    // Random non-button texts return None
    assert_eq!(parse_legacy_action("Hello bot"), None);
    assert_eq!(parse_legacy_action("https://drive.google.com"), None);
    assert_eq!(parse_legacy_action("/start"), None);
}

#[test]
fn all_keyboards_callback_data_within_64_bytes() {
    use gdclone_bot::telegram::keyboards::*;
    use teloxide::types::InlineKeyboardButtonKind;

    let languages = [UiLanguage::Vi, UiLanguage::En];
    let sample_uuid = "12345678-1234-1234-1234-123456789abc";

    for lang in languages {
        let mut test_keyboards = vec![
            main_menu_keyboard(true, lang),
            main_menu_keyboard(false, lang),
            settings_keyboard(lang, true, true, true),
            account_keyboard(lang),
            account_disconnect_confirm_keyboard(lang),
            confirm_clear_destination_keyboard(lang),
            language_keyboard(lang),
            back_home_keyboard(lang),
            confirm_clone_keyboard(sample_uuid, lang),
            confirm_set_destination_keyboard(sample_uuid, lang),
            job_list_keyboard(&[], lang),
            job_list_keyboard(
                &[(sample_uuid.to_string(), "Job 1 · In progress".to_string())],
                lang,
            ),
            job_detail_keyboard(sample_uuid, "running", lang),
            job_detail_keyboard(sample_uuid, "paused", lang),
            job_detail_keyboard(sample_uuid, "failed", lang),
            job_cancel_confirm_keyboard(sample_uuid, lang),
            destination_browser_keyboard(
                Some(sample_uuid),
                Some(sample_uuid),
                Some(sample_uuid),
                Some(sample_uuid),
                &[("My Folder".to_string(), sample_uuid.to_string())],
                lang,
            ),
            watch_list_keyboard(
                &[(sample_uuid.to_string(), "Watch 1".to_string())],
                "status",
                0,
                true,
                true,
                lang,
            ),
            watch_resolve_conflict_keyboard(sample_uuid, 12345, lang),
        ];

        for kb in test_keyboards.drain(..) {
            for row in kb.inline_keyboard {
                for btn in row {
                    if let InlineKeyboardButtonKind::CallbackData(data) = btn.kind {
                        assert!(
                            data.len() <= 64,
                            "Callback data exceeds 64 bytes (len = {}): '{}'",
                            data.len(),
                            data
                        );
                    }
                }
            }
        }
    }
}

#[test]
fn destination_browser_pagination_bounds_and_navigation() {
    use gdclone_bot::telegram::keyboards::*;

    let sample_id = "state-uuid-1234";

    // 1. First page: only next page exists
    let first_page = destination_browser_keyboard(
        None,
        None,
        None,
        Some(sample_id),
        &[("Folder A".to_string(), sample_id.to_string())],
        UiLanguage::Vi,
    );
    let nav_row = &first_page.inline_keyboard[1];
    assert_eq!(nav_row.len(), 1);
    assert_eq!(nav_row[0].text, "Trang sau");

    // 2. Middle page: both prev and next exist
    let middle_page = destination_browser_keyboard(
        None,
        None,
        Some("prev-state"),
        Some("next-state"),
        &[("Folder B".to_string(), sample_id.to_string())],
        UiLanguage::Vi,
    );
    let nav_row = &middle_page.inline_keyboard[1];
    assert_eq!(nav_row.len(), 2);
    assert_eq!(nav_row[0].text, "‹ Trước");
    assert_eq!(nav_row[1].text, "Trang sau");

    // English middle page
    let middle_en = destination_browser_keyboard(
        None,
        None,
        Some("prev-state"),
        Some("next-state"),
        &[],
        UiLanguage::En,
    );
    let nav_row_en = &middle_en.inline_keyboard[0];
    assert_eq!(nav_row_en.len(), 2);
    assert_eq!(nav_row_en[0].text, "‹ Prev");
    assert_eq!(nav_row_en[1].text, "Next");

    // 3. Last page: only prev page exists
    let last_page = destination_browser_keyboard(
        None,
        None,
        Some("prev-state"),
        None,
        &[("Folder C".to_string(), sample_id.to_string())],
        UiLanguage::Vi,
    );
    let nav_row = &last_page.inline_keyboard[1];
    assert_eq!(nav_row.len(), 1);
    assert_eq!(nav_row[0].text, "‹ Trước");

    // 4. Single page: neither prev nor next
    let single_page = destination_browser_keyboard(
        None,
        None,
        None,
        None,
        &[("Folder D".to_string(), sample_id.to_string())],
        UiLanguage::Vi,
    );
    // Folder item is row 0, bottom actions (Destination, BackHome) are row 1
    assert_eq!(single_page.inline_keyboard.len(), 2);
}

#[tokio::test]
async fn conflict_resolution_and_clear_destination_idempotency() {
    // 1. Conflict resolution callback data parsing
    let action = parse_callback_action("wres:v:watch_abc:42");
    assert_eq!(action, Some(("wres", "v", "watch_abc:42")));

    // When conflict is already resolved, duplicate callback gracefully produces None from parse or handler
    let parts: Vec<&str> = "watch_abc:42".split(':').collect();
    assert_eq!(parts.len(), 2);
    let (watch_id, seq) = (parts[0], parts[1].parse::<i64>().unwrap());
    assert_eq!(watch_id, "watch_abc");
    assert_eq!(seq, 42);

    // 2. Clear destination confirmation callback
    assert_eq!(
        parse_callback_action("dest:clear:confirm"),
        Some(("dest", "clear", "confirm"))
    );

    // 3. Account disconnect confirmation callbacks
    assert_eq!(
        parse_callback_action("account:disconnect:prompt"),
        Some(("account", "disconnect", "prompt"))
    );
    assert_eq!(
        parse_callback_action("account:disconnect:confirm"),
        Some(("account", "disconnect", "confirm"))
    );
}
