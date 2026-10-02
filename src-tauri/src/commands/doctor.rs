use crate::commands::{get_config_path, get_db_path, get_project_dirs, service};
use rusqlite::{Connection, OpenFlags};
use serde::{Deserialize, Serialize};
use std::fs;
use std::path::Path;
use std::process::Command;
use std::time::Duration;

const RELEASES_API: &str = "https://api.github.com/repos/GiaHung07/502Drive/releases/latest";
const RELEASES_PAGE: &str = "https://github.com/GiaHung07/502Drive/releases";
const DEFAULT_CONFIG_TEMPLATE: &str = include_str!("../../../config.sample.toml");

#[cfg(unix)]
fn enforce_owner_only_file(path: &Path) -> Result<bool, String> {
    use std::os::unix::fs::PermissionsExt;

    let metadata = fs::metadata(path).map_err(|e| e.to_string())?;
    let mode = metadata.permissions().mode() & 0o777;
    if mode == 0o600 {
        return Ok(false);
    }

    fs::set_permissions(path, fs::Permissions::from_mode(0o600)).map_err(|e| e.to_string())?;
    Ok(true)
}

#[cfg(not(unix))]
fn enforce_owner_only_file(_path: &Path) -> Result<bool, String> {
    Ok(false)
}

#[derive(Debug, Serialize, Deserialize, Clone)]
pub struct DoctorCheckItem {
    pub name: String,
    pub passed: bool,
    pub detail: String,
}

#[derive(Debug, Serialize, Deserialize, Clone)]
pub struct DoctorResult {
    pub timestamp_ms: i64,
    pub checks: Vec<DoctorCheckItem>,
    pub raw_output: String,
}

#[derive(Debug, Serialize, Deserialize, Clone)]
pub struct PreflightStep {
    pub id: String,
    pub title: String,
    pub status: String, // "passed" | "warning" | "failed"
    pub message: String,
    pub auto_fixed: bool,
    pub fix_action: Option<String>,
}

#[derive(Debug, Serialize, Deserialize, Clone)]
pub struct RemoteUpdateInfo {
    pub current_version: String,
    pub latest_version: String,
    pub update_available: bool,
    pub changelog: String,
    pub download_url: Option<String>,
}

#[derive(Debug, Serialize, Deserialize, Clone)]
pub struct PreflightReport {
    pub all_passed: bool,
    pub needs_setup: bool,
    pub steps: Vec<PreflightStep>,
    pub remote_update: Option<RemoteUpdateInfo>,
}

fn parse_release_version(value: &str) -> Option<(u64, u64, u64)> {
    let core = value
        .trim()
        .trim_start_matches('v')
        .split(['-', '+'])
        .next()?;
    let mut parts = core.split('.');
    let major = parts.next()?.parse().ok()?;
    let minor = parts.next()?.parse().ok()?;
    let patch = parts.next()?.parse().ok()?;
    if parts.next().is_some() {
        return None;
    }
    Some((major, minor, patch))
}

fn is_newer_release(current: &str, latest: &str) -> bool {
    match (
        parse_release_version(current),
        parse_release_version(latest),
    ) {
        (Some(current), Some(latest)) => latest > current,
        _ => false,
    }
}

#[tauri::command]
pub async fn run_preflight_check() -> Result<PreflightReport, String> {
    let mut steps = Vec::new();
    let mut needs_setup = false;

    // 1. Check & Auto-provision Local Directories
    let mut fs_auto_fixed = false;
    let mut fs_ok = true;
    if let Some(dirs) = get_project_dirs() {
        let data_dir = dirs.data_dir();
        let logs_dir = data_dir.join("logs");
        let reports_dir = data_dir.join("reports");

        if !data_dir.exists() || !logs_dir.exists() || !reports_dir.exists() {
            if let Err(e) =
                fs::create_dir_all(&logs_dir).and_then(|_| fs::create_dir_all(&reports_dir))
            {
                fs_ok = false;
                steps.push(PreflightStep {
                    id: "storage".to_string(),
                    title: "Hệ thống tệp & thư mục lưu trữ".to_string(),
                    status: "failed".to_string(),
                    message: format!("Không thể tạo thư mục lưu trữ: {e}"),
                    auto_fixed: false,
                    fix_action: None,
                });
            } else {
                fs_auto_fixed = true;
            }
        }
    }

    if fs_ok {
        steps.push(PreflightStep {
            id: "storage".to_string(),
            title: "Hệ thống tệp & thư mục lưu trữ".to_string(),
            status: "passed".to_string(),
            message: if fs_auto_fixed {
                "Đã tự động khởi tạo thư mục dữ liệu và nhật ký".to_string()
            } else {
                "Các thư mục lưu trữ cục bộ sẵn sàng".to_string()
            },
            auto_fixed: fs_auto_fixed,
            fix_action: None,
        });
    }

    // 2. Check Database integrity and WAL mode.
    let db_path = get_db_path();
    let db_was_missing = !db_path.exists();
    if db_was_missing {
        if let Some(parent) = db_path.parent() {
            let _ = fs::create_dir_all(parent);
        }
    }

    let (db_status, db_message, db_auto_fixed) = match Connection::open(&db_path) {
        Ok(conn) => {
            let _ = conn.busy_timeout(Duration::from_secs(2));
            match conn.query_row("PRAGMA quick_check;", [], |row| row.get::<_, String>(0)) {
                Ok(check) if check == "ok" => {
                    let current_mode = conn
                        .query_row("PRAGMA journal_mode;", [], |row| row.get::<_, String>(0))
                        .unwrap_or_else(|_| "unknown".to_string());
                    if current_mode.eq_ignore_ascii_case("wal") {
                        (
                            "passed",
                            if db_was_missing {
                                "Đã tạo SQLite, quick_check=ok và bật WAL mode".to_string()
                            } else {
                                "SQLite quick_check=ok, journal_mode=wal".to_string()
                            },
                            db_was_missing,
                        )
                    } else {
                        let switched = conn
                            .query_row("PRAGMA journal_mode=WAL;", [], |row| row.get::<_, String>(0))
                            .ok()
                            .is_some_and(|mode| mode.eq_ignore_ascii_case("wal"));
                        if switched {
                            (
                                "passed",
                                format!(
                                    "SQLite quick_check=ok; đã chuyển journal_mode từ {current_mode} sang wal"
                                ),
                                true,
                            )
                        } else {
                            (
                                "warning",
                                format!(
                                    "SQLite quick_check=ok nhưng journal_mode={current_mode}; chưa thể chuyển sang WAL"
                                ),
                                false,
                            )
                        }
                    }
                }
                Ok(check) => (
                    "failed",
                    format!("SQLite quick_check báo lỗi: {check}"),
                    false,
                ),
                Err(err) => (
                    "failed",
                    format!("Không thể chạy SQLite quick_check: {err}"),
                    false,
                ),
            }
        }
        Err(err) => (
            "failed",
            format!("Không thể mở cơ sở dữ liệu SQLite: {err}"),
            false,
        ),
    };

    if db_status != "passed" {
        needs_setup = true;
    }
    steps.push(PreflightStep {
        id: "database".to_string(),
        title: "Cơ sở dữ liệu SQLite (WAL)".to_string(),
        status: db_status.to_string(),
        message: db_message,
        auto_fixed: db_auto_fixed,
        fix_action: None,
    });

    // 3. Check & auto-provision a schema-valid config file.
    let cfg_path = get_config_path();
    let mut cfg_auto_fixed = false;
    let mut cfg_create_error = None;
    if !cfg_path.exists() {
        if let Some(parent) = cfg_path.parent() {
            if let Err(err) = fs::create_dir_all(parent) {
                cfg_create_error = Some(format!("Không thể tạo thư mục config: {err}"));
            }
        }
        if cfg_create_error.is_none() {
            match super::config::write_config_file(&cfg_path, DEFAULT_CONFIG_TEMPLATE) {
                Ok(()) => cfg_auto_fixed = true,
                Err(err) => cfg_create_error = Some(format!("Không thể tạo config.toml: {err}")),
            }
        }
    }

    if cfg_create_error.is_none() && cfg_path.exists() {
        match enforce_owner_only_file(&cfg_path) {
            Ok(changed) => cfg_auto_fixed |= changed,
            Err(err) => {
                cfg_create_error = Some(format!("Không thể siết quyền config.toml về 0600: {err}"));
            }
        }
    }

    let (cfg_status, cfg_message, cfg_ready) = if let Some(err) = cfg_create_error {
        ("failed", err, false)
    } else {
        match gdclone_bot::config::AppConfig::load(&cfg_path) {
            Ok(config) => {
                let mut missing = Vec::new();
                if !super::config::is_configured(&config.telegram.bot_token) {
                    missing.push("Telegram Bot Token");
                }
                if config.telegram.owner_telegram_id <= 0 {
                    missing.push("Owner Telegram ID");
                }
                if !super::config::is_configured(&config.google_oauth.client_id) {
                    missing.push("Google OAuth client_id");
                }
                if !super::config::is_configured(&config.google_oauth.client_secret) {
                    missing.push("Google OAuth client_secret");
                }

                if missing.is_empty() {
                    (
                        "passed",
                        "Cấu hình đúng schema và đủ credential bắt buộc".to_string(),
                        true,
                    )
                } else {
                    (
                        "warning",
                        format!(
                            "Config đúng schema nhưng còn thiếu: {}",
                            missing.join(", ")
                        ),
                        false,
                    )
                }
            }
            Err(err) => (
                "failed",
                format!("config.toml không đúng schema: {err}"),
                false,
            ),
        }
    };

    if !cfg_ready {
        needs_setup = true;
    }
    steps.push(PreflightStep {
        id: "config".to_string(),
        title: "Tệp cấu hình ứng dụng".to_string(),
        status: cfg_status.to_string(),
        message: if cfg_auto_fixed && cfg_status == "warning" {
            format!("Đã tạo config.toml mẫu. {cfg_message}")
        } else {
            cfg_message
        },
        auto_fixed: cfg_auto_fixed,
        fix_action: if cfg_ready {
            None
        } else {
            Some("config".to_string())
        },
    });

    // 4. Check Background Service
    #[cfg(any(target_os = "linux", windows))]
    {
        let is_active = service::service_active();
        let service_label = if cfg!(windows) {
            "Scheduled Task 502Drive"
        } else {
            "Service gdclone-bot"
        };

        steps.push(PreflightStep {
            id: "service".to_string(),
            title: "Dịch vụ chạy nền (Daemon)".to_string(),
            status: if is_active {
                "passed".to_string()
            } else {
                "warning".to_string()
            },
            message: if is_active {
                format!("{service_label} đang chạy ổn định")
            } else {
                format!("{service_label} chưa chạy hoặc chưa được cài đặt")
            },
            auto_fixed: false,
            fix_action: if !is_active {
                Some("start_service".to_string())
            } else {
                None
            },
        });

        if !is_active {
            needs_setup = true;
        }
    }

    // 5. Check Google Account Connection
    let mut google_connected = false;
    let mut google_email = None;

    if db_path.exists() {
        if let Ok(conn) = Connection::open_with_flags(&db_path, OpenFlags::SQLITE_OPEN_READ_ONLY) {
            if let Ok(mut stmt) = conn.prepare(
                "SELECT email, status FROM google_accounts ORDER BY updated_at_ms DESC LIMIT 1;",
            ) {
                if let Ok(row) = stmt.query_row([], |r| {
                    Ok((r.get::<_, Option<String>>(0)?, r.get::<_, String>(1)?))
                }) {
                    google_connected = row.1 == "connected";
                    google_email = row.0;
                }
            }
        }
    }

    steps.push(PreflightStep {
        id: "google_oauth".to_string(),
        title: "Tài khoản Google Drive".to_string(),
        status: if google_connected {
            "passed".to_string()
        } else {
            "warning".to_string()
        },
        message: if google_connected {
            format!("Đã kết nối tài khoản {}", google_email.unwrap_or_default())
        } else {
            "Chưa đăng nhập Google Drive OAuth".to_string()
        },
        auto_fixed: false,
        fix_action: if !google_connected {
            Some("login".to_string())
        } else {
            None
        },
    });

    if !google_connected {
        needs_setup = true;
    }

    // 6. Remote update information. Network failure must not fabricate a
    // version; the rest of preflight remains useful offline.
    let remote_update = check_remote_update().await.ok();

    let all_passed = steps.iter().all(|s| s.status == "passed");

    Ok(PreflightReport {
        all_passed,
        needs_setup,
        steps,
        remote_update,
    })
}

#[tauri::command]
pub async fn check_remote_update() -> Result<RemoteUpdateInfo, String> {
    let current_version = format!("v{}", env!("CARGO_PKG_VERSION"));
    let client = reqwest::Client::builder()
        .timeout(Duration::from_secs(8))
        .build()
        .map_err(|_| "cannot initialize update HTTP client".to_string())?;
    let response = client
        .get(RELEASES_API)
        .header(reqwest::header::USER_AGENT, "502Drive-Desktop")
        .send()
        .await
        .map_err(|_| "cannot reach GitHub Releases".to_string())?;
    if !response.status().is_success() {
        return Err(format!(
            "GitHub Releases returned HTTP {}",
            response.status().as_u16()
        ));
    }

    let payload = response
        .json::<serde_json::Value>()
        .await
        .map_err(|_| "invalid GitHub Releases response".to_string())?;
    let latest_version = payload
        .get("tag_name")
        .and_then(|v| v.as_str())
        .filter(|v| parse_release_version(v).is_some())
        .ok_or_else(|| "latest release has no valid semantic version tag".to_string())?
        .to_string();
    let changelog = payload
        .get("body")
        .and_then(|v| v.as_str())
        .unwrap_or("")
        .trim()
        .to_string();
    let download_url = payload
        .get("html_url")
        .and_then(|v| v.as_str())
        .map(str::to_string)
        .or_else(|| Some(RELEASES_PAGE.to_string()));

    Ok(RemoteUpdateInfo {
        update_available: is_newer_release(&current_version, &latest_version),
        current_version,
        latest_version,
        changelog,
        download_url,
    })
}

#[tauri::command]
pub async fn run_doctor() -> Result<DoctorResult, String> {
    let now = chrono::Utc::now().timestamp_millis();
    let mut checks = Vec::new();

    let cli_output = Command::new(service::cli_binary_path())
        .arg("doctor")
        .output();

    if let Ok(out) = cli_output {
        let raw = String::from_utf8_lossy(&out.stdout).to_string();
        for line in raw.lines() {
            if line.contains("[OK]") || line.contains("OK -") {
                let parts: Vec<&str> = line.splitn(2, ':').collect();
                if parts.len() == 2 {
                    checks.push(DoctorCheckItem {
                        name: parts[0].replace("[OK]", "").trim().to_string(),
                        passed: true,
                        detail: parts[1].trim().to_string(),
                    });
                }
            } else if line.contains("[FAIL]") || line.contains("ERR -") {
                let parts: Vec<&str> = line.splitn(2, ':').collect();
                if parts.len() == 2 {
                    checks.push(DoctorCheckItem {
                        name: parts[0].replace("[FAIL]", "").trim().to_string(),
                        passed: false,
                        detail: parts[1].trim().to_string(),
                    });
                }
            }
        }

        if !checks.is_empty() {
            return Ok(DoctorResult {
                timestamp_ms: now,
                checks,
                raw_output: raw,
            });
        }
    }

    let cfg = get_config_path();
    let db = get_db_path();

    let cfg_exists = cfg.exists();
    checks.push(DoctorCheckItem {
        name: "Cấu hình ứng dụng".to_string(),
        passed: cfg_exists,
        detail: if cfg_exists {
            format!("Đã tải ({})", cfg.display())
        } else {
            "Chưa tìm thấy".to_string()
        },
    });

    let (db_ok, db_detail, google_connected, google_detail) = if db.exists() {
        match Connection::open_with_flags(&db, OpenFlags::SQLITE_OPEN_READ_ONLY) {
            Ok(conn) => {
                let integrity = conn
                    .query_row("PRAGMA integrity_check;", [], |row| row.get::<_, String>(0))
                    .unwrap_or_else(|e| format!("integrity check failed: {e}"));
                let db_ok = integrity == "ok";
                let google = conn
                    .query_row(
                        "SELECT email, status
                         FROM google_accounts
                         ORDER BY updated_at_ms DESC
                         LIMIT 1",
                        [],
                        |row| Ok((row.get::<_, Option<String>>(0)?, row.get::<_, String>(1)?)),
                    )
                    .ok();
                let google_connected =
                    matches!(google.as_ref(), Some((_, status)) if status == "connected");
                let google_detail = match google {
                    Some((email, status)) => format!(
                        "{} ({status})",
                        email.unwrap_or_else(|| "Google account".to_string())
                    ),
                    None => "Chưa có tài khoản Google trong cơ sở dữ liệu".to_string(),
                };
                (
                    db_ok,
                    if db_ok {
                        format!("integrity_check=ok ({})", db.display())
                    } else {
                        integrity
                    },
                    google_connected,
                    google_detail,
                )
            }
            Err(err) => (
                false,
                format!("Không thể mở DB: {err}"),
                false,
                "Không thể kiểm tra tài khoản Google".to_string(),
            ),
        }
    } else {
        (
            false,
            "Chưa khởi tạo".to_string(),
            false,
            "Cần đăng nhập".to_string(),
        )
    };

    checks.push(DoctorCheckItem {
        name: "Cơ sở dữ liệu".to_string(),
        passed: db_ok,
        detail: db_detail,
    });

    let telegram_check = if cfg_exists {
        fs::read_to_string(&cfg)
            .ok()
            .and_then(|content| content.parse::<toml::Value>().ok())
            .and_then(|value| {
                value
                    .get("telegram")
                    .and_then(|t| t.get("bot_token"))
                    .and_then(|v| v.as_str())
                    .map(str::to_string)
            })
    } else {
        None
    };

    let (telegram_ok, telegram_detail) = match telegram_check {
        Some(token) if super::config::is_configured(&token) => {
            match super::config::verify_telegram_bot_token(&token).await {
                Ok(check) if check.ok => (
                    true,
                    check
                        .username
                        .map(|u| format!("@{u}"))
                        .unwrap_or_else(|| "Bot token hợp lệ".to_string()),
                ),
                Ok(check) => (
                    false,
                    check
                        .error
                        .unwrap_or_else(|| "Telegram token không hợp lệ".to_string()),
                ),
                Err(err) => (false, err),
            }
        }
        _ => (false, "Chưa cấu hình Telegram Bot Token".to_string()),
    };

    checks.push(DoctorCheckItem {
        name: "Telegram Bot".to_string(),
        passed: telegram_ok,
        detail: telegram_detail,
    });

    checks.push(DoctorCheckItem {
        name: "Tài khoản Google".to_string(),
        passed: google_connected,
        detail: google_detail,
    });

    let raw = format!(
        "$ 502drive doctor\n502Drive doctor\n===============\nConfig: {}\nDB: {}\nTelegram: {}\nGoogle: {}\n",
        if cfg_exists { "OK" } else { "MISSING" },
        if db_ok { "OK" } else { "FAIL" },
        if telegram_ok { "OK" } else { "FAIL" },
        if google_connected { "OK" } else { "FAIL" },
    );

    Ok(DoctorResult {
        timestamp_ms: now,
        checks,
        raw_output: raw,
    })
}

#[cfg(test)]
mod tests {
    use super::{
        DEFAULT_CONFIG_TEMPLATE, enforce_owner_only_file, is_newer_release, parse_release_version,
    };

    #[test]
    fn preflight_template_matches_runtime_config_schema() {
        let parsed = toml::from_str::<gdclone_bot::config::AppConfig>(DEFAULT_CONFIG_TEMPLATE);
        assert!(
            parsed.is_ok(),
            "preflight template must deserialize as AppConfig"
        );
    }

    #[cfg(unix)]
    #[test]
    fn preflight_hardens_existing_config_permissions() {
        use std::os::unix::fs::PermissionsExt;

        let path = std::env::temp_dir().join(format!(
            "502drive-doctor-config-{}.toml",
            std::process::id()
        ));
        std::fs::write(&path, "[telegram]\n").unwrap();
        std::fs::set_permissions(&path, std::fs::Permissions::from_mode(0o644)).unwrap();

        assert!(enforce_owner_only_file(&path).unwrap());
        let mode = std::fs::metadata(&path).unwrap().permissions().mode() & 0o777;
        assert_eq!(mode, 0o600);
        assert!(!enforce_owner_only_file(&path).unwrap());

        let _ = std::fs::remove_file(path);
    }

    #[test]
    fn release_version_parser_handles_v_prefix_and_suffixes() {
        assert_eq!(parse_release_version("v0.2.0"), Some((0, 2, 0)));
        assert_eq!(parse_release_version("1.4.7"), Some((1, 4, 7)));
        assert_eq!(parse_release_version("v1.4.7-beta.1"), Some((1, 4, 7)));
        assert_eq!(parse_release_version("v1.4"), None);
        assert_eq!(parse_release_version("latest"), None);
    }

    #[test]
    fn update_comparison_only_accepts_strictly_newer_versions() {
        assert!(is_newer_release("v0.2.0", "v0.2.1"));
        assert!(is_newer_release("v0.2.9", "v0.3.0"));
        assert!(!is_newer_release("v0.2.0", "v0.2.0"));
        assert!(!is_newer_release("v0.3.0", "v0.2.9"));
        assert!(!is_newer_release("invalid", "v1.0.0"));
    }
}
