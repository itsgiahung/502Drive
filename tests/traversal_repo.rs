use gdclone_bot::state::{db::Database, repo};

async fn test_db() -> (Database, String) {
    let db = Database::in_memory().await.unwrap();
    db.run_migrations().await.unwrap();
    repo::upsert_google_account(
        &db,
        "default",
        Some("user@example.com"),
        vec![1, 2, 3],
        "test",
        r#"["https://www.googleapis.com/auth/drive"]"#,
    )
    .await
    .unwrap();
    let job_id = repo::create_job(&db, 1, 2, "default", "source-root", "dest-parent")
        .await
        .unwrap();
    (db, job_id)
}

#[tokio::test]
async fn child_traversal_completion_counts_once_and_reuses_existing_row() {
    let (db, job_id) = test_db().await;

    assert!(
        repo::record_child_traversal_completion(
            &db,
            &job_id,
            "child-folder",
            Some("rk-1"),
            "dst-child",
            Some("src-parent"),
        )
        .await
        .unwrap()
    );

    let counts = repo::job_counts(&db, &job_id).await.unwrap();
    assert_eq!(counts.total_discovered, 1);
    assert_eq!(counts.completed_items, 1);

    assert!(
        !repo::record_child_traversal_completion(
            &db,
            &job_id,
            "child-folder",
            Some("rk-2"),
            "dst-child-recovered",
            Some("src-parent"),
        )
        .await
        .unwrap()
    );

    let counts = repo::job_counts(&db, &job_id).await.unwrap();
    assert_eq!(counts.total_discovered, 1);
    assert_eq!(counts.completed_items, 1);

    let claimed = repo::next_pending_traversal_folder(&db, &job_id)
        .await
        .unwrap()
        .unwrap();
    assert_eq!(claimed.source_folder_id, "child-folder");
    assert_eq!(claimed.source_resource_key.as_deref(), Some("rk-2"));
    assert_eq!(claimed.destination_folder_id, "dst-child-recovered");
}

#[tokio::test]
async fn traversal_folder_claim_requeue_and_finish() {
    let (db, job_id) = test_db().await;

    repo::record_traversal_folder(&db, &job_id, "src-folder", Some("rk"), "dst-folder", None)
        .await
        .unwrap();

    let claimed = repo::next_pending_traversal_folder(&db, &job_id)
        .await
        .unwrap()
        .unwrap();
    assert_eq!(claimed.source_folder_id, "src-folder");
    assert_eq!(claimed.source_resource_key.as_deref(), Some("rk"));
    assert_eq!(claimed.destination_folder_id, "dst-folder");

    assert!(
        repo::next_pending_traversal_folder(&db, &job_id)
            .await
            .unwrap()
            .is_none()
    );

    repo::requeue_traversal_folder_page(&db, &job_id, "src-folder", "page-2")
        .await
        .unwrap();
    let claimed = repo::next_pending_traversal_folder(&db, &job_id)
        .await
        .unwrap()
        .unwrap();
    assert_eq!(claimed.next_page_token.as_deref(), Some("page-2"));

    repo::finish_traversal_folder(&db, &job_id, "src-folder")
        .await
        .unwrap();
    assert!(!repo::has_pending_traversal(&db, &job_id).await.unwrap());
}
