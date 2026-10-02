use std::{
    path::PathBuf,
    process::{Command, Output},
};

const WINDOWS_TASK_NAME: &str = "502Drive";

pub(crate) fn cli_binary_path() -> PathBuf {
    #[cfg(target_os = "linux")]
    if let Some(home) = std::env::var_os("HOME") {
        let installed = PathBuf::from(home).join(".local/bin/502drive");
        if installed.is_file() {
            return installed;
        }
    }

    #[cfg(windows)]
    {
        if let Ok(current) = std::env::current_exe() {
            if let Some(parent) = current.parent() {
                let sibling = parent.join("502drive.exe");
                if sibling.is_file() {
                    return sibling;
                }
            }
        }
        if let Some(local_app_data) = std::env::var_os("LOCALAPPDATA") {
            let installed = PathBuf::from(local_app_data)
                .join("502drive")
                .join("502drive.exe");
            if installed.is_file() {
                return installed;
            }
        }
    }

    PathBuf::from(if cfg!(windows) {
        "502drive.exe"
    } else {
        "502drive"
    })
}

fn ensure_success(action: &str, output: Output) -> Result<(), String> {
    if output.status.success() {
        return Ok(());
    }
    let stderr = String::from_utf8_lossy(&output.stderr).trim().to_string();
    let stdout = String::from_utf8_lossy(&output.stdout).trim().to_string();
    let detail = if !stderr.is_empty() { stderr } else { stdout };
    Err(if detail.is_empty() {
        format!("{action} failed with status {}", output.status)
    } else {
        format!("{action} failed: {detail}")
    })
}

pub(crate) fn service_active() -> bool {
    #[cfg(target_os = "linux")]
    {
        return Command::new("systemctl")
            .args(["--user", "is-active", "gdclone-bot"])
            .output()
            .map(|out| {
                out.status.success()
                    && String::from_utf8_lossy(&out.stdout).trim() == "active"
            })
            .unwrap_or(false);
    }

    #[cfg(windows)]
    {
        const SCRIPT: &str = "$task = Get-ScheduledTask -TaskName '502Drive' -ErrorAction SilentlyContinue; if ($null -ne $task -and $task.State -eq 'Running') { exit 0 } else { exit 1 }";
        return Command::new("powershell")
            .args(["-NoProfile", "-NonInteractive", "-Command", SCRIPT])
            .status()
            .map(|status| status.success())
            .unwrap_or(false);
    }

    #[cfg(not(any(target_os = "linux", windows)))]
    {
        false
    }
}

#[tauri::command]
pub async fn start_service() -> Result<(), String> {
    #[cfg(target_os = "linux")]
    {
        let output = Command::new("systemctl")
            .args(["--user", "start", "gdclone-bot"])
            .output()
            .map_err(|e| format!("failed to run systemctl: {e}"))?;
        return ensure_success("systemctl start gdclone-bot", output);
    }

    #[cfg(windows)]
    {
        let output = Command::new("schtasks")
            .args(["/Run", "/TN", WINDOWS_TASK_NAME])
            .output()
            .map_err(|e| format!("failed to run schtasks: {e}"))?;
        return ensure_success("start 502Drive scheduled task", output);
    }

    #[cfg(not(any(target_os = "linux", windows)))]
    {
        Err("background service control is unsupported on this platform".to_string())
    }
}

#[tauri::command]
pub async fn restart_service() -> Result<(), String> {
    #[cfg(target_os = "linux")]
    {
        let output = Command::new("systemctl")
            .args(["--user", "restart", "gdclone-bot"])
            .output()
            .map_err(|e| format!("failed to run systemctl: {e}"))?;
        return ensure_success("systemctl restart gdclone-bot", output);
    }

    #[cfg(windows)]
    {
        // /End returns non-zero when the task is registered but currently idle;
        // that is harmless. /Run below is the authoritative restart result.
        let _ = Command::new("schtasks")
            .args(["/End", "/TN", WINDOWS_TASK_NAME])
            .output();
        let output = Command::new("schtasks")
            .args(["/Run", "/TN", WINDOWS_TASK_NAME])
            .output()
            .map_err(|e| format!("failed to run schtasks: {e}"))?;
        return ensure_success("restart 502Drive scheduled task", output);
    }

    #[cfg(not(any(target_os = "linux", windows)))]
    {
        Err("background service control is unsupported on this platform".to_string())
    }
}

#[tauri::command]
pub async fn open_telegram_bot() -> Result<(), String> {
    open::that("https://t.me/Drive502_Bot").map_err(|e| e.to_string())?;
    Ok(())
}

#[tauri::command]
pub async fn trigger_auth_login() -> Result<(), String> {
    let cli = cli_binary_path();

    #[cfg(target_os = "linux")]
    {
        let title = "502Drive - Đăng nhập Google";
        if std::path::Path::new("/usr/bin/ptyxis").exists() {
            Command::new("ptyxis")
                .args(["--title", title, "--"])
                .arg(&cli)
                .args(["auth", "login"])
                .spawn()
                .map_err(|e| format!("failed to spawn ptyxis: {e}"))?;
            return Ok(());
        }
        if std::path::Path::new("/usr/bin/gnome-terminal").exists() {
            Command::new("gnome-terminal")
                .args(["--title", title, "--"])
                .arg(&cli)
                .args(["auth", "login"])
                .spawn()
                .map_err(|e| format!("failed to spawn gnome-terminal: {e}"))?;
            return Ok(());
        }
        if std::path::Path::new("/usr/bin/xterm").exists() {
            Command::new("xterm")
                .args(["-title", title, "-e"])
                .arg(&cli)
                .args(["auth", "login"])
                .spawn()
                .map_err(|e| format!("failed to spawn xterm: {e}"))?;
            return Ok(());
        }
    }

    Command::new(&cli)
        .args(["auth", "login"])
        .spawn()
        .map_err(|e| format!("failed to spawn {} auth login: {e}", cli.display()))?;
    Ok(())
}

#[tauri::command]
pub async fn trigger_auth_revoke() -> Result<(), String> {
    let cli = cli_binary_path();
    let output = Command::new(&cli)
        .args(["auth", "revoke"])
        .output()
        .map_err(|e| format!("failed to run {} auth revoke: {e}", cli.display()))?;
    ensure_success("502drive auth revoke", output)
}
