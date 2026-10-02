use crate::commands::{get_config_path, get_db_path, get_log_path, get_project_dirs};
use gdclone_bot::state::{db::Database, repo};
use serde::{Deserialize, Serialize};

const BACKUP_NAME_PREFIX: &str = "backup_";

fn validate_managed_role(role: &str) -> Result<(), String> {
    if role == "operator" {
        Ok(())
    } else {
        Err("desktop user management may only create operator accounts".to_string())
    }
}

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

#[cfg(target_os = "linux")]
fn stop_daemon_for_restore() -> Result<bool, String> {
    let was_active = std::process::Command::new("systemctl")
        .args(["--user", "is-active", "gdclone-bot"])
        .output()
        .map(|out| String::from_utf8_lossy(&out.stdout).trim() == "active")
        .unwrap_or(false);
    if was_active {
        let output = std::process::Command::new("systemctl")
            .args(["--user", "stop", "gdclone-bot"])
            .output()
            .map_err(|e| format!("failed to stop daemon before restore: {e}"))?;
        if !output.status.success() {
            return Err(format!(
                "failed to stop daemon before restore: {}",
                String::from_utf8_lossy(&output.stderr).trim()
            ));
        }
    }
    Ok(was_active)
}

#[cfg(windows)]
fn stop_daemon_for_restore() -> Result<bool, String> {
    let output = std::process::Command::new("schtasks")
        .args(["/End", "/TN", "502Drive"])
        .output()
        .map_err(|e| format!("failed to stop scheduled task before restore: {e}"))?;
    Ok(output.status.success())
}

#[cfg(not(any(target_os = "linux", windows)))]
fn stop_daemon_for_restore() -> Result<bool, String> {
    Ok(false)
}

#[cfg(target_os = "linux")]
fn restart_daemon_after_restore() -> Result<(), String> {
    let output = std::process::Command::new("systemctl")
        .args(["--user", "start", "gdclone-bot"])
        .output()
        .map_err(|e| format!("failed to restart daemon after restore: {e}"))?;
    if !output.status.success() {
        return Err(format!(
            "failed to restart daemon after restore: {}",
            String::from_utf8_lossy(&output.stderr).trim()
        ));
    }
    Ok(())
}

#[cfg(windows)]
fn restart_daemon_after_restore() -> Result<(), String> {
    let output = std::process::Command::new("schtasks")
        .args(["/Run", "/TN", "502Drive"])
        .output()
        .map_err(|e| format!("failed to restart scheduled task after restore: {e}"))?;
    if !output.status.success() {
        return Err(format!(
            "failed to restart scheduled task after restore: {}",
            String::from_utf8_lossy(&output.stderr).trim()
        ));
    }
    Ok(())
}

#[cfg(not(any(target_os = "linux", windows)))]
fn restart_daemon_after_restore() -> Result<(), String> {
    Ok(())
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
    validate_managed_role(&role)?;
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
    validate_managed_role(&role)?;
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
        if user.role == "owner" && !enabled {
            return Err("owner account cannot be disabled".to_string());
        }
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
        let conn = rusqlite::Connection::open(&db_path).map_err(|e| e.to_string())?;
        let dest = dest_db.to_string_lossy().into_owned();
        conn.execute("VACUUM INTO ?1", [dest])
            .map_err(|e| format!("failed to create consistent SQLite snapshot: {e}"))?;
        #[cfg(unix)]
        {
            use std::os::unix::fs::PermissionsExt;
            std::fs::set_permissions(&dest_db, std::fs::Permissions::from_mode(0o600))
                .map_err(|e| e.to_string())?;
        }
        total_size += std::fs::metadata(&dest_db).map(|m| m.len()).unwrap_or(0);
        files_count += 1;
    }

    if cfg_path.exists() {
        let dest_cfg = target_dir.join("config.toml");
        std::fs::copy(&cfg_path, &dest_cfg).map_err(|e| e.to_string())?;
        #[cfg(unix)]
        {
            use std::os::unix::fs::PermissionsExt;
            std::fs::set_permissions(&dest_cfg, std::fs::Permissions::from_mode(0o600))
                .map_err(|e| e.to_string())?;
        }
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
    let was_active = stop_daemon_for_restore()?;

    let restore_result = (|| -> Result<(), String> {
        let src_db = source_dir.join("state.db");
        if src_db.exists() {
            // The backup is a single-file SQLite snapshot. Remove sidecar
            // files from the old database before replacing it.
            for sidecar in [db_path.with_extension("db-wal"), db_path.with_extension("db-shm")] {
                if sidecar.exists() {
                    std::fs::remove_file(&sidecar).map_err(|e| e.to_string())?;
                }
            }
            std::fs::copy(&src_db, &db_path).map_err(|e| e.to_string())?;
            #[cfg(unix)]
            {
                use std::os::unix::fs::PermissionsExt;
                std::fs::set_permissions(&db_path, std::fs::Permissions::from_mode(0o600))
                    .map_err(|e| e.to_string())?;
            }
        }

        let src_cfg = source_dir.join("config.toml");
        if src_cfg.exists() {
            std::fs::copy(&src_cfg, &cfg_path).map_err(|e| e.to_string())?;
            #[cfg(unix)]
            {
                use std::os::unix::fs::PermissionsExt;
                std::fs::set_permissions(&cfg_path, std::fs::Permissions::from_mode(0o600))
                    .map_err(|e| e.to_string())?;
            }
        }
        Ok(())
    })();

    if was_active {
        let restart_result = restart_daemon_after_restore();
        if restore_result.is_ok() {
            restart_result?;
        } else {
            let _ = restart_result;
        }
    }

    restore_result
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

// ── Remote Update Handoff ───────────────────────────────────────────────────

#[tauri::command]
pub async fn apply_remote_update() -> Result<String, String> {
    // Do not execute a mutable git checkout or shell installer from the GUI.
    // Resolve the latest release through the same verified GitHub API path as
    // the update checker, then hand installation back to the user.
    let update = super::doctor::check_remote_update().await?;
    if !update.update_available {
        return Ok("502Drive đang ở phiên bản mới nhất.".to_string());
    }

    let url = update
        .download_url
        .ok_or_else(|| "Bản phát hành mới không có URL tải xuống".to_string())?;
    const TRUSTED_RELEASE_PAGE: &str = "https://github.com/GiaHung07/502Drive/releases";
    if url != TRUSTED_RELEASE_PAGE
        && !url.starts_with("https://github.com/GiaHung07/502Drive/releases/")
    {
        return Err("URL bản phát hành không thuộc kho 502Drive chính thức".to_string());
    }

    open::that(&url).map_err(|e| format!("Không thể mở trang tải bản cập nhật: {e}"))?;
    Ok(format!(
        "Đã mở trang GitHub Releases cho {}. Hãy tải gói phù hợp và kiểm tra SHA256 trước khi cài đặt.",
        update.latest_version
    ))
}

#[cfg(test)]
mod tests {
    use super::{validate_backup_name, validate_managed_role};

    #[test]
    fn desktop_user_management_only_creates_operators() {
        assert!(validate_managed_role("operator").is_ok());
        assert!(validate_managed_role("owner").is_err());
        assert!(validate_managed_role("admin").is_err());
    }

    #[test]
    fn backup_name_validation_rejects_path_traversal() {
        assert!(validate_backup_name("backup_20261002_174500").is_ok());
        assert!(validate_backup_name("../backup_20261002_174500").is_err());
        assert!(validate_backup_name("../../tmp").is_err());
        assert!(validate_backup_name("backup_20261002_174500/../../tmp").is_err());
        assert!(validate_backup_name("backup_20261002_17450x").is_err());
    }
}
