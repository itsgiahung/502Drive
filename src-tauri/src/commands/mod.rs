pub mod admin;
pub mod config;
pub mod doctor;
pub mod features;
pub mod jobs;
pub mod logs;
pub mod service;
pub mod status;
pub mod watches;

use directories::ProjectDirs;
use rusqlite::{Connection, OptionalExtension};
use std::path::PathBuf;

pub fn get_project_dirs() -> Option<ProjectDirs> {
    ProjectDirs::from("dev", "gdclone", "gdclone-bot")
}

pub fn get_db_path() -> PathBuf {
    if let Some(dirs) = get_project_dirs() {
        dirs.data_dir().join("state.db")
    } else {
        PathBuf::from("state.db")
    }
}

pub fn get_config_path() -> PathBuf {
    if let Some(dirs) = get_project_dirs() {
        dirs.config_dir().join("config.toml")
    } else {
        PathBuf::from("config.toml")
    }
}

pub fn get_log_path() -> PathBuf {
    if let Some(dirs) = get_project_dirs() {
        dirs.data_dir().join("logs").join("502drive.log")
    } else {
        PathBuf::from("502drive.log")
    }
}

/// Open the shared database for the `ui_requests` queue. The table only
/// exists after the daemon has applied migrations at least once.
pub(crate) fn open_queue_conn() -> Result<Connection, String> {
    let db_path = get_db_path();
    if !db_path.exists() {
        return Err("Database does not exist".to_string());
    }
    let conn = Connection::open(&db_path).map_err(|e| e.to_string())?;
    let table_exists: bool = conn
        .query_row(
            "SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE type='table' AND name='ui_requests')",
            [],
            |row| row.get(0),
        )
        .map_err(|e| e.to_string())?;
    if !table_exists {
        return Err(
            "ui_requests table missing — start the 502drive daemon once so it applies migrations"
                .to_string(),
        );
    }
    Ok(conn)
}

/// Resolve the actor for a job row. The shared JobService deliberately scopes
/// every mutation by the row's telegram_user_id, so the desktop control plane
/// must pass the row owner itself (including user 0 for GUI-created jobs).
pub(crate) fn resolve_job_actor(conn: &Connection, job_id: &str) -> Result<i64, String> {
    let owner: Option<i64> = conn
        .query_row(
            "SELECT telegram_user_id FROM jobs WHERE id = ?1",
            [job_id],
            |row| row.get(0),
        )
        .optional()
        .map_err(|e| e.to_string())?;
    owner.ok_or_else(|| format!("job {job_id} not found"))
}

/// [`resolve_job_actor`] for watch rows.
pub(crate) fn resolve_watch_actor(conn: &Connection, watch_id: &str) -> Result<i64, String> {
    let owner: Option<i64> = conn
        .query_row(
            "SELECT telegram_user_id FROM watch_subscriptions WHERE id = ?1",
            [watch_id],
            |row| row.get(0),
        )
        .optional()
        .map_err(|e| e.to_string())?;
    owner.ok_or_else(|| format!("watch {watch_id} not found"))
}

#[cfg(test)]
mod tests {
    use super::{resolve_job_actor, resolve_watch_actor};
    use rusqlite::Connection;

    #[test]
    fn desktop_controls_use_the_rows_real_actor() {
        let conn = Connection::open_in_memory().unwrap();
        conn.execute_batch(
            "CREATE TABLE jobs (id TEXT PRIMARY KEY, telegram_user_id INTEGER NOT NULL);
             CREATE TABLE watch_subscriptions (
                 id TEXT PRIMARY KEY,
                 telegram_user_id INTEGER NOT NULL
             );
             INSERT INTO jobs VALUES ('job-operator', 42), ('job-gui', 0);
             INSERT INTO watch_subscriptions VALUES ('watch-operator', 77), ('watch-gui', 0);",
        )
        .unwrap();

        assert_eq!(resolve_job_actor(&conn, "job-operator").unwrap(), 42);
        assert_eq!(resolve_job_actor(&conn, "job-gui").unwrap(), 0);
        assert_eq!(resolve_watch_actor(&conn, "watch-operator").unwrap(), 77);
        assert_eq!(resolve_watch_actor(&conn, "watch-gui").unwrap(), 0);
    }
}
