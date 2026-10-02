use crate::commands::{config::verify_telegram_bot_token, get_config_path, get_db_path};
use gdclone_bot::{
    config::AppConfig,
    state::{db::Database, repo},
};
use rusqlite::{Connection, OpenFlags};
use serde::Serialize;
use std::{
    path::PathBuf,
    process::Command,
    sync::{Mutex, OnceLock},
    time::{Duration, Instant},
};

const TELEGRAM_STATUS_TTL: Duration = Duration::from_secs(60);
const DB_HEALTH_TTL: Duration = Duration::from_secs(60);

#[derive(Clone)]
struct CachedTelegramStatus {
    token: String,
    checked_at: Instant,
    username: Option<String>,
    status: String,
}

#[derive(Clone)]
struct CachedDbHealth {
    path: PathBuf,
    checked_at: Instant,
    value: String,
}

static TELEGRAM_STATUS_CACHE: OnceLock<Mutex<Option<CachedTelegramStatus>>> = OnceLock::new();
static DB_HEALTH_CACHE: OnceLock<Mutex<Option<CachedDbHealth>>> = OnceLock::new();

#[derive(Debug, Serialize, Clone)]
pub struct SystemStats {
    pub total_jobs: i64,
    pub active_jobs: i64,
    pub completed_jobs: i64,
    pub total_cloned_files: i64,
    pub total_cloned_bytes: i64,
}

#[derive(Debug, Serialize, Clone)]
pub struct SystemStatus {
    pub google_account: Option<String>,
    pub account_status: String,
    pub service_active: bool,
    pub service_name: String,
    pub db_integrity: String,
    pub app_version: String,
    pub bot_username: Option<String>,
    pub telegram_status: Option<String>,
    pub destination_label: Option<String>,
    pub destination_id: Option<String>,
    pub stats: SystemStats,
}

fn check_systemd_service_active(service_name: &str) -> bool {
    #[cfg(target_os = "linux")]
    {
        let output = Command::new("systemctl")
            .args(["--user", "is-active", service_name])
            .output();
        if let Ok(out) = output {
            let status = String::from_utf8_lossy(&out.stdout).trim().to_string();
            return status == "active";
        }
    }
    false
}

fn cached_db_health(conn: &Connection, path: &PathBuf) -> String {
    let cache = DB_HEALTH_CACHE.get_or_init(|| Mutex::new(None));
    if let Ok(guard) = cache.lock() {
        if let Some(cached) = guard.as_ref() {
            if cached.path == *path && cached.checked_at.elapsed() < DB_HEALTH_TTL {
                return cached.value.clone();
            }
        }
    }

    let value = conn
        .query_row("PRAGMA quick_check;", [], |row| row.get::<_, String>(0))
        .unwrap_or_else(|_| "unknown".to_string());

    if let Ok(mut guard) = cache.lock() {
        *guard = Some(CachedDbHealth {
            path: path.clone(),
            checked_at: Instant::now(),
            value: value.clone(),
        });
    }
    value
}

async fn cached_telegram_status(token: &str) -> (Option<String>, String) {
    let cache = TELEGRAM_STATUS_CACHE.get_or_init(|| Mutex::new(None));
    if let Ok(guard) = cache.lock() {
        if let Some(cached) = guard.as_ref() {
            if cached.token == token && cached.checked_at.elapsed() < TELEGRAM_STATUS_TTL {
                return (cached.username.clone(), cached.status.clone());
            }
        }
    }

    let (username, status) = match verify_telegram_bot_token(token).await {
        Ok(check) if check.ok => (check.username, "active".to_string()),
        _ => (None, "error".to_string()),
    };

    if let Ok(mut guard) = cache.lock() {
        *guard = Some(CachedTelegramStatus {
            token: token.to_string(),
            checked_at: Instant::now(),
            username: username.clone(),
            status: status.clone(),
        });
    }
    (username, status)
}

#[tauri::command]
pub async fn get_system_status() -> Result<SystemStatus, String> {
    let db_path = get_db_path();
    let service_active = check_systemd_service_active("gdclone-bot");

    let mut google_account = None;
    let mut account_status = "disconnected".to_string();
    let mut db_integrity = "unknown".to_string();
    let mut destination_label = None;
    let mut destination_id = None;

    let mut total_jobs = 0i64;
    let mut active_jobs = 0i64;
    let mut completed_jobs = 0i64;
    let mut total_cloned_files = 0i64;
    let mut total_cloned_bytes = 0i64;

    if db_path.exists() {
        // Dashboard polling runs every few seconds. Cache the DB health check
        // and use quick_check here; the explicit Doctor flow can do deeper checks.
        if let Ok(conn) = Connection::open_with_flags(&db_path, OpenFlags::SQLITE_OPEN_READ_ONLY) {
            db_integrity = cached_db_health(&conn, &db_path);
            if let Ok(mut stmt) =
                conn.prepare("SELECT COALESCE(SUM(completed_items), 0) FROM jobs;")
            {
                total_cloned_files = stmt.query_row([], |r| r.get(0)).unwrap_or(0);
            }
            if let Ok(mut stmt) = conn.prepare(
                "SELECT COALESCE(SUM(size_bytes), 0) FROM job_items WHERE status = 'done';",
            ) {
                total_cloned_bytes = stmt.query_row([], |r| r.get(0)).unwrap_or(0);
            }
        }

        // Everything else goes through the shared repo layer so the GUI and
        // the daemon always agree on the definitions (active = queued |
        // discovering | running | pausing | paused | cancelling | recovering).
        if let Ok(db) = Database::open(&db_path).await {
            if let Ok(Some(account)) = repo::google_account_secret(&db, "default").await {
                google_account = account.email;
                account_status = account.status;
            }
            // Default destination from destination_profiles; None (empty
            // state) when nothing is configured — no invented fallback label.
            if let Ok(Some(profile)) = repo::default_destination_profile(&db, "default").await {
                destination_label = Some(profile.label);
                destination_id = Some(profile.destination_parent_id);
            }
            if let Ok(counts) = repo::job_status_counts(&db).await {
                for count in counts {
                    total_jobs += count.count;
                    if count.status == "completed" {
                        completed_jobs = count.count;
                    }
                }
            }
            active_jobs = repo::active_job_count(&db).await.unwrap_or(0);
        }
    } else {
        db_integrity = "no_db_yet".to_string();
    }

    let (bot_username, telegram_status) = {
        let config_path = get_config_path();
        if !config_path.exists() {
            (None, Some("no_token".to_string()))
        } else {
            match AppConfig::load(&config_path) {
                Ok(config) => {
                    let token = config.telegram.bot_token.trim();
                    if token.is_empty() {
                        (None, Some("no_token".to_string()))
                    } else {
                        let (username, status) = cached_telegram_status(token).await;
                        (username, Some(status))
                    }
                }
                Err(_) => (None, Some("error".to_string())),
            }
        }
    };

    Ok(SystemStatus {
        google_account,
        account_status,
        service_active,
        service_name: "gdclone-bot.service".to_string(),
        db_integrity,
        app_version: format!("v{}", env!("CARGO_PKG_VERSION")),
        bot_username,
        telegram_status,
        destination_label,
        destination_id,
        stats: SystemStats {
            total_jobs,
            active_jobs,
            completed_jobs,
            total_cloned_files,
            total_cloned_bytes,
        },
    })
}
