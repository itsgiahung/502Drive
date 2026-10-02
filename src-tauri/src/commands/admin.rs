use crate::commands::{get_config_path, get_db_path, get_log_path, get_project_dirs};
use gdclone_bot::state::{db::Database, repo};
use serde::{Deserialize, Serialize};

const BACKUP_NAME_PREFIX: &str = "backup_";

fn validate_backup_name(name: &str) -> Result<&str, String> {
    let bytes = name.as_bytes();
    let valid = bytes.len() == 22
        && name.starts_with(BACKUP_NAME_PREFIX)
        && bytes[7..15].iter().all(u8::is_ascii_digit)
        && bytes[15] == b'_'
        && bytes[16..22].iter().all(u8::is_ascii_digit);
    if !valid {
        return Err("invalid backup name".to_string());
    }
    Ok(name)
}

fn backup_dir() -> Result<std::path::PathBuf, String> {
    let dirs = get_project_dirs().ok_or_else(|| "cannot locate project dirs".to_string())?;
    Ok(dirs.data_dir().join("backups"))
}

// ── Default Destination ─────────────────────────────────────────────────────

#[tauri::command]
pub async fn set_default_destination(
    folder_id: String,
    folder_name: String,
    drive_id: Option<String>,
) -> Result<(), String> {
    let db = Database::open(&get_db_path())
        .await
        .map_err(|e| format!("cannot open state database: {e:#}"))?;
    repo::upsert_destination_profile(
        &db,
        repo::NewDestinationProfile {
            google_account_id: "default".to_string(),
            label: folder_name,
            destination_parent_id: folder_id,
            destination_drive_id: drive_id,
            destination_resource_key: None,
            is_default: true,
        },
    )
    .await
    .map_err(|e| format!("failed to save default destination: {e:#}"))?;
    Ok(())
}

// ── Multi-User Management ───────────────────────────────────────────────────

#[derive(Debug, Serialize, Deserialize)]
pub struct AuthorizedUserDto {
    pub telegram_user_id: i64,
    pub role: String,
    pub enabled: bool,
}

#[tauri::command]
pub async fn list_authorized_users() -> Result<Vec<AuthorizedUserDto>, String> {
    let db = Database::open(&get_db_path())
        .await
        .map_err(|e| format!("cannot open state database: {e:#}"))?;
    let users = repo::list_authorized_users(&db)
        .await
        .map_err(|e| format!("failed to list users: {e:#}"))?;
    Ok(users
        .into_iter()
        .map(|u| AuthorizedUserDto {
            telegram_user_id: u.telegram_user_id,
            role: u.role,
            enabled: u.enabled,
        })
        .collect())
}

#[tauri::command]
pub async fn add_authorized_user(telegram_user_id: i64, role: String) -> Result<(), String> {
    if telegram_user_id <= 0 {
        return Err("Telegram User ID phải là số nguyên dương".to_string());
    }
    let db = Database::open(&get_db_path())
        .await
        .map_err(|e| format!("cannot open state database: {e:#}"))?;
    repo::upsert_authorized_user(&db, telegram_user_id, &role, true)
        .await
        .map_err(|e| format!("failed to add user: {e:#}"))?;
    Ok(())
}

#[tauri::command]
pub async fn batch_add_authorized_users(
    user_ids_text: String,
    role: String,
) -> Result<usize, String> {
    let db = Database::open(&get_db_path())
        .await
        .map_err(|e| format!("cannot open state database: {e:#}"))?;
    let mut count = 0;
    for token in
        user_ids_text.split(|c: char| c == ',' || c == '\n' || c == ' ' || c == ';' || c == '\t')
    {
        let trimmed = token.trim();
        if trimmed.is_empty() {
            continue;
        }
        if let Ok(id) = trimmed.parse::<i64>() {
            if id > 0 {
                if repo::upsert_authorized_user(&db, id, &role, true)
                    .await
                    .is_ok()
                {
                    count += 1;
                }
            }
        }
    }
    Ok(count)
}

#[tauri::command]
pub async fn toggle_authorized_user(telegram_user_id: i64, enabled: bool) -> Result<(), String> {
    let db = Database::open(&get_db_path())
        .await
        .map_err(|e| format!("cannot open state database: {e:#}"))?;
    if let Some(user) = repo::authorized_user(&db, telegram_user_id)
        .await
        .map_err(|e| e.to_string())?
    {
        repo::upsert_authorized_user(&db, telegram_user_id, &user.role, enabled)
            .await
            .map_err(|e| format!("failed to toggle user: {e:#}"))?;
    }
    Ok(())
}

#[tauri::command]
pub async fn remove_authorized_user(telegram_user_id: i64) -> Result<bool, String> {
    let db = Database::open(&get_db_path())
        .await
        .map_err(|e| format!("cannot open state database: {e:#}"))?;
    repo::delete_authorized_user(&db, telegram_user_id)
        .await
        .map_err(|e| format!("failed to delete user: {e:#}"))
}

// ── Database & Backup Management ────────────────────────────────────────────

#[derive(Debug, Serialize, Deserialize)]
pub struct BackupInfoDto {
    pub name: String,
    pub timestamp_ms: i64,
    pub size_bytes: u64,
    pub files_count: usize,
}

#[tauri::command]
pub async fn backup_database() -> Result<BackupInfoDto, String> {
    let backup_dir = backup_dir()?;
    std::fs::create_dir_all(&backup_dir).map_err(|e| e.to_string())?;
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        std::fs::set_permissions(&backup_dir, std::fs::Permissions::from_mode(0o700))
            .map_err(|e| e.to_string())?;
    }

    let now = chrono::Local::now();
    let name = format!("backup_{}", now.format("%Y%m%d_%H%M%S"));
    let target_dir = backup_dir.join(&name);
    std::fs::create_dir_all(&target_dir).map_err(|e| e.to_string())?;
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        std::fs::set_permissions(&target_dir, std::fs::Permissions::from_mode(0o700))
            .map_err(|e| e.to_string())?;
    }

    let db_path = get_db_path();
    let cfg_path = get_config_path();

    let mut total_size = 0u64;
    let mut files_count = 0;

    if db_path.exists() {
        let dest_db = target_dir.join("state.db");
        std::fs::copy(&db_path, &dest_db).map_err(|e| e.to_string())?;
        total_size += std::fs::metadata(&dest_db).map(|m| m.len()).unwrap_or(0);
        files_count += 1;
    }

    if cfg_path.exists() {
        let dest_cfg = target_dir.join("config.toml");
        std::fs::copy(&cfg_path, &dest_cfg).map_err(|e| e.to_string())?;
        total_size += std::fs::metadata(&dest_cfg).map(|m| m.len()).unwrap_or(0);
        files_count += 1;
    }

    Ok(BackupInfoDto {
        name,
        timestamp_ms: now.timestamp_millis(),
        size_bytes: total_size,
        files_count,
    })
}

#[tauri::command]
pub async fn list_backups() -> Result<Vec<BackupInfoDto>, String> {
    let backup_dir = match backup_dir() {
        Ok(path) => path,
        Err(_) => return Ok(Vec::new()),
    };
    if !backup_dir.exists() {
        return Ok(Vec::new());
    }

    let mut list = Vec::new();
    if let Ok(entries) = std::fs::read_dir(&backup_dir) {
        for entry in entries.flatten() {
            if entry.file_type().map(|t| t.is_dir()).unwrap_or(false) {
                let path = entry.path();
                let name = entry.file_name().to_string_lossy().to_string();
                if validate_backup_name(&name).is_err() {
                    continue;
                }
                let mut size = 0u64;
                let mut count = 0;
                if let Ok(files) = std::fs::read_dir(&path) {
                    for f in files.flatten() {
                        if let Ok(m) = f.metadata() {
                            size += m.len();
                            count += 1;
                        }
                    }
                }
                let timestamp_ms = entry
                    .metadata()
                    .and_then(|m| m.modified())
                    .map(|t| {
                        t.duration_since(std::time::UNIX_EPOCH)
                            .unwrap_or_default()
                            .as_millis() as i64
                    })
                    .unwrap_or(0);
                list.push(BackupInfoDto {
                    name,
                    timestamp_ms,
                    size_bytes: size,
                    files_count: count,
                });
            }
        }
    }
    list.sort_by(|a, b| b.timestamp_ms.cmp(&a.timestamp_ms));
    Ok(list)
}

#[tauri::command]
pub async fn restore_backup(backup_name: String) -> Result<(), String> {
    let backup_name = validate_backup_name(&backup_name)?;
    let source_dir = backup_dir()?.join(backup_name);
    if !source_dir.exists() {
        return Err(format!("bản sao lưu '{backup_name}' không tồn tại"));
    }

    let db_path = get_db_path();
    let cfg_path = get_config_path();

    let src_db = source_dir.join("state.db");
    if src_db.exists() {
        std::fs::copy(&src_db, &db_path).map_err(|e| e.to_string())?;
    }

    let src_cfg = source_dir.join("config.toml");
    if src_cfg.exists() {
        std::fs::copy(&src_cfg, &cfg_path).map_err(|e| e.to_string())?;
    }

    #[cfg(target_os = "linux")]
    let _ = std::process::Command::new("systemctl")
        .args(["--user", "restart", "gdclone-bot"])
        .output();

    Ok(())
}

#[tauri::command]
pub async fn delete_backup(backup_name: String) -> Result<(), String> {
    let backup_name = validate_backup_name(&backup_name)?;
    let dir = backup_dir()?.join(backup_name);
    if dir.exists() {
        std::fs::remove_dir_all(&dir).map_err(|e| e.to_string())?;
    }
    Ok(())
}

#[tauri::command]
pub async fn vacuum_database() -> Result<String, String> {
    let db_path = get_db_path();
    if !db_path.exists() {
        return Err("Database does not exist".to_string());
    }
    let conn = rusqlite::Connection::open(&db_path).map_err(|e| e.to_string())?;
    conn.execute_batch("PRAGMA optimize; VACUUM;")
        .map_err(|e| e.to_string())?;
    let new_size = std::fs::metadata(&db_path).map(|m| m.len()).unwrap_or(0);
    Ok(format!("{:.2} MB", (new_size as f64) / (1024.0 * 1024.0)))
}

#[tauri::command]
pub async fn clear_app_logs() -> Result<(), String> {
    let log_path = get_log_path();
    if log_path.exists() {
        std::fs::write(&log_path, "").map_err(|e| e.to_string())?;
    }
    Ok(())
}

// ── Remote Update Execution ─────────────────────────────────────────────────

#[tauri::command]
pub async fn apply_remote_update() -> Result<String, String> {
    // 1. Snapshot safety backup
    let _ = backup_database().await;

    // 2. Trigger packaging install or git pull
    let cmd = "cd /home/admin/Projects/502Drive && (git pull --ff-only 2>&1 || true) && bash packaging/install.sh 2>&1";
    let output = std::process::Command::new("bash")
        .args(["-c", cmd])
        .output()
        .map_err(|e| format!("Lỗi tiến trình cập nhật: {e}"))?;

    let stdout = String::from_utf8_lossy(&output.stdout);
    let stderr = String::from_utf8_lossy(&output.stderr);

    if !output.status.success() && stdout.is_empty() {
        return Err(format!("Cập nhật thất bại: {stderr}"));
    }

    #[cfg(target_os = "linux")]
    let _ = std::process::Command::new("systemctl")
        .args(["--user", "restart", "gdclone-bot", "502drive-tray"])
        .output();

    Ok("Cập nhật thành công! Bản mới nhất đã được cài đặt và khởi động lại dịch vụ.".to_string())
}

#[cfg(test)]
mod tests {
    use super::validate_backup_name;

    #[test]
    fn backup_name_validation_rejects_path_traversal() {
        assert!(validate_backup_name("backup_20261002_174500").is_ok());
        assert!(validate_backup_name("../backup_20261002_174500").is_err());
        assert!(validate_backup_name("../../tmp").is_err());
        assert!(validate_backup_name("backup_20261002_174500/../../tmp").is_err());
        assert!(validate_backup_name("backup_20261002_17450x").is_err());
    }
}
