use crate::commands::{get_config_path, get_db_path, get_log_path, get_project_dirs};
use gdclone_bot::state::{db::Database, repo};
use serde::{Deserialize, Serialize};
use std::path::{Path, PathBuf};

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

fn backup_dir() -> Result<PathBuf, String> {
    let dirs = get_project_dirs().ok_or_else(|| "cannot locate project dirs".to_string())?;
    Ok(dirs.data_dir().join("backups"))
}

fn managed_backup_source_dir(root: &Path, backup_name: &str) -> Result<PathBuf, String> {
    let source = root.join(backup_name);
    let meta = std::fs::symlink_metadata(&source)
        .map_err(|_| format!("bản sao lưu '{backup_name}' không tồn tại"))?;
    if meta.file_type().is_symlink() || !meta.is_dir() {
        return Err("backup snapshot must be a real directory".to_string());
    }

    let root = std::fs::canonicalize(root).map_err(|e| e.to_string())?;
    let source = std::fs::canonicalize(source).map_err(|e| e.to_string())?;
    if source.parent() != Some(root.as_path()) {
        return Err("backup snapshot escapes the managed backup directory".to_string());
    }
    Ok(source)
}

fn regular_backup_file(source_dir: &Path, name: &str) -> Result<Option<PathBuf>, String> {
    let path = source_dir.join(name);
    let meta = match std::fs::symlink_metadata(&path) {
        Ok(meta) => meta,
        Err(err) if err.kind() == std::io::ErrorKind::NotFound => return Ok(None),
        Err(err) => return Err(err.to_string()),
    };
    if meta.file_type().is_symlink() || !meta.is_file() {
        return Err(format!("backup file '{name}' must be a regular file"));
    }
    Ok(Some(path))
}

fn validate_sqlite_backup(path: &Path) -> Result<(), String> {
    let conn = rusqlite::Connection::open_with_flags(
        path,
        rusqlite::OpenFlags::SQLITE_OPEN_READ_ONLY,
    )
    .map_err(|e| format!("cannot open backup database: {e}"))?;
    let quick_check: String = conn
        .query_row("PRAGMA quick_check", [], |row| row.get(0))
        .map_err(|e| format!("cannot validate backup database: {e}"))?;
    if quick_check != "ok" {
        return Err(format!("backup database quick_check failed: {quick_check}"));
    }
    Ok(())
}

fn read_valid_config_backup(path: &Path) -> Result<String, String> {
    let contents = std::fs::read_to_string(path)
        .map_err(|e| format!("cannot read backup config: {e}"))?;
    contents
        .parse::<toml::Table>()
        .map_err(|e| format!("backup config.toml is invalid: {e}"))?;
    Ok(contents)
}

fn replace_private_file(source: &Path, destination: &Path) -> Result<(), String> {
    let parent = destination
        .parent()
        .filter(|parent| !parent.as_os_str().is_empty())
        .unwrap_or_else(|| Path::new("."));
    std::fs::create_dir_all(parent).map_err(|e| e.to_string())?;

    let file_name = destination
        .file_name()
        .and_then(|name| name.to_str())
        .ok_or_else(|| "restore destination has no valid file name".to_string())?;
    let nonce = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map_err(|e| e.to_string())?
        .as_nanos();
    let temp = parent.join(format!(
        ".{file_name}.restore.{}.{}",
        std::process::id(),
        nonce
    ));

    let result = (|| -> Result<(), String> {
        std::fs::copy(source, &temp).map_err(|e| e.to_string())?;
        #[cfg(unix)]
        {
            use std::os::unix::fs::PermissionsExt;
            std::fs::set_permissions(&temp, std::fs::Permissions::from_mode(0o600))
                .map_err(|e| e.to_string())?;
        }
        std::fs::File::open(&temp)
            .and_then(|file| file.sync_all())
            .map_err(|e| e.to_string())?;
        std::fs::rename(&temp, destination).map_err(|e| e.to_string())?;
        Ok(())
    })();

    if result.is_err() {
        let _ = std::fs::remove_file(&temp);
    }
    result
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
    let backup_root = backup_dir()?;
    let source_dir = managed_backup_source_dir(&backup_root, backup_name)?;

    // Validate every restore input before stopping the daemon or touching live
    // state. Backup entries must be regular files, not symlink indirections.
    let src_db = regular_backup_file(&source_dir, "state.db")?;
    if let Some(path) = src_db.as_deref() {
        validate_sqlite_backup(path)?;
    }
    let src_cfg = regular_backup_file(&source_dir, "config.toml")?;
    let config_contents = src_cfg
        .as_deref()
        .map(read_valid_config_backup)
        .transpose()?;

    let db_path = get_db_path();
    let cfg_path = get_config_path();
    let was_active = stop_daemon_for_restore()?;

    let restore_result = (|| -> Result<(), String> {
        if let Some(src_db) = src_db.as_deref() {
            // Publish the validated single-file snapshot first. If this fails,
            // the old database and its WAL/SHM remain untouched. Only after the
            // replacement succeeds is it safe to discard sidecars that belong
            // to the previous database image.
            replace_private_file(src_db, &db_path)?;
            for sidecar in [
                db_path.with_extension("db-wal"),
                db_path.with_extension("db-shm"),
            ] {
                if sidecar.exists() {
                    std::fs::remove_file(&sidecar).map_err(|e| {
                        format!(
                            "restored database but failed to remove stale SQLite sidecar {}: {e}",
                            sidecar.display()
                        )
                    })?;
                }
            }
        }

        if let Some(contents) = config_contents.as_deref() {
            super::config::write_config_file(&cfg_path, contents)?;
        }
        Ok(())
    })();

    match restore_result {
        Ok(()) => {
            if was_active {
                restart_daemon_after_restore()?;
            }
            Ok(())
        }
        Err(err) => {
            if was_active {
                Err(format!(
                    "{err}; restore aborted and the daemon was left stopped to avoid running on partial state"
                ))
            } else {
                Err(err)
            }
        }
    }
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
    use super::{
        managed_backup_source_dir, read_valid_config_backup, regular_backup_file,
        replace_private_file, validate_backup_name, validate_managed_role, validate_sqlite_backup,
    };
    use std::time::{SystemTime, UNIX_EPOCH};

    fn temp_dir(label: &str) -> std::path::PathBuf {
        let nonce = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_nanos();
        std::env::temp_dir().join(format!(
            "502drive_restore_{label}_{}_{}",
            std::process::id(),
            nonce
        ))
    }

    #[cfg(unix)]
    #[test]
    fn restore_sources_reject_symlink_indirection() {
        use std::os::unix::fs::symlink;

        let root = temp_dir("symlink");
        let outside = temp_dir("outside");
        std::fs::create_dir_all(&root).unwrap();
        std::fs::create_dir_all(&outside).unwrap();

        let name = "backup_20261002_174500";
        symlink(&outside, root.join(name)).unwrap();
        assert!(managed_backup_source_dir(&root, name).is_err());

        std::fs::remove_file(root.join(name)).unwrap();
        let snapshot = root.join(name);
        std::fs::create_dir_all(&snapshot).unwrap();
        let target = outside.join("state.db");
        std::fs::write(&target, b"outside").unwrap();
        symlink(&target, snapshot.join("state.db")).unwrap();
        assert!(regular_backup_file(&snapshot, "state.db").is_err());

        let _ = std::fs::remove_dir_all(root);
        let _ = std::fs::remove_dir_all(outside);
    }

    #[cfg(unix)]
    #[test]
    fn atomic_restore_replaces_destination_symlink_not_target() {
        use std::os::unix::fs::symlink;

        let dir = temp_dir("replace");
        std::fs::create_dir_all(&dir).unwrap();
        let source = dir.join("snapshot.db");
        let target = dir.join("target.db");
        let destination = dir.join("state.db");
        std::fs::write(&source, b"snapshot").unwrap();
        std::fs::write(&target, b"keep").unwrap();
        symlink(&target, &destination).unwrap();

        replace_private_file(&source, &destination).unwrap();

        assert_eq!(std::fs::read(&destination).unwrap(), b"snapshot");
        assert_eq!(std::fs::read(&target).unwrap(), b"keep");
        assert!(!std::fs::symlink_metadata(&destination)
            .unwrap()
            .file_type()
            .is_symlink());

        let _ = std::fs::remove_dir_all(dir);
    }

    #[test]
    fn corrupt_restore_inputs_are_rejected_before_apply() {
        let dir = temp_dir("corrupt");
        std::fs::create_dir_all(&dir).unwrap();

        let db = dir.join("state.db");
        std::fs::write(&db, b"not sqlite").unwrap();
        assert!(validate_sqlite_backup(&db).is_err());

        let cfg = dir.join("config.toml");
        std::fs::write(&cfg, "[telegram\nbot_token='x'").unwrap();
        assert!(read_valid_config_backup(&cfg).is_err());

        let _ = std::fs::remove_dir_all(dir);
    }

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
