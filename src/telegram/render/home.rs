//! Home dashboard, account panel and language panel texts.

use crate::state::{db::Database, repo};
use crate::telegram::i18n::TextKey as T;
use crate::telegram::keyboards;

pub(crate) async fn render_home_dashboard(
    config: &crate::config::AppConfig,
    db: &Database,
    telegram_user_id: i64,
    lang: keyboards::UiLanguage,
) -> anyhow::Result<String> {
    let account = repo::account_status(db)
        .await?
        .unwrap_or_else(|| "chưa kết nối".to_string());
    let destination = repo::default_destination_profile(db, "default").await?;
    let running_jobs = repo::active_job_count_for_user(db, telegram_user_id).await?;
    let watches = if config.watch.enabled {
        repo::list_watches_for_user(db, telegram_user_id).await?
    } else {
        Vec::new()
    };
    let watch_active = watches
        .iter()
        .filter(|w| matches!(w.status.as_str(), "active" | "catching_up" | "degraded"))
        .count();
    let failed_jobs = repo::failed_job_count_for_user(db, telegram_user_id).await?;
    let needs_attention = failed_jobs as usize
        + watches
            .iter()
            .filter(|w| w.status == "needs_reconcile")
            .count();

    let drive_status = match account.as_str() {
        "connected" => lang.text(T::HomeStatusConnected),
        "reconnect_required" => lang.text(T::HomeStatusReconnect),
        _ => lang.text(T::HomeStatusNotConnected),
    };
    let destination = match &destination {
        Some(dest) => dest.label.clone(),
        None => lang.text(T::HomeNoDestination).to_string(),
    };

    let (overall_icon, overall_status) = if account == "reconnect_required" {
        (
            "⚠",
            match lang {
                keyboards::UiLanguage::Vi => "Cần kết nối lại",
                keyboards::UiLanguage::En => "Reconnect required",
            },
        )
    } else if account != "connected" {
        (
            "⚠",
            match lang {
                keyboards::UiLanguage::Vi => "Dịch vụ chưa sẵn sàng",
                keyboards::UiLanguage::En => "Service unavailable",
            },
        )
    } else if needs_attention > 0 {
        ("⚠", lang.text(T::HomeNeedsAttention))
    } else {
        ("●", lang.text(T::HomeBotReady))
    };

    let mut lines = vec!["🚀 502Drive".to_string()];
    lines.push(format!("{overall_icon} {overall_status}"));
    lines.push(String::new());

    lines.push(lang.text(T::HomeDriveLabel).to_string());
    lines.push(drive_status.to_string());
    lines.push(String::new());

    lines.push(lang.text(T::HomeDefaultDestination).to_string());
    lines.push(destination);
    lines.push(String::new());

    lines.push(format!(
        "{}  {}",
        lang.text(T::HomeJobsRunning),
        running_jobs
    ));
    if config.watch.enabled {
        lines.push(format!("{}  {}", lang.text(T::HomeWatching), watch_active));
    }
    if needs_attention > 0 {
        lines.push(format!(
            "{}  {}",
            lang.text(T::HomeNeedsAttention),
            needs_attention
        ));
    }
    lines.push(String::new());
    lines.push(lang.text(T::HomeHint).to_string());
    Ok(lines.join("\n"))
}

/// Settings panel with preferences scoped to the current Telegram user.
pub(crate) fn render_settings_panel(
    lang: keyboards::UiLanguage,
    drive_status: &str,
    prefs: &repo::TelegramUserPreferences,
) -> String {
    let lang_value = match lang {
        keyboards::UiLanguage::Vi => "Tiếng Việt",
        keyboards::UiLanguage::En => "English",
    };
    let state = |enabled: bool| {
        if enabled {
            lang.text(T::SettingsEnabled)
        } else {
            lang.text(T::SettingsDisabled)
        }
    };
    let (job_complete, job_error, watch_conflicts) = match lang {
        keyboards::UiLanguage::Vi => ("Job hoàn tất", "Job lỗi", "Xung đột Watch"),
        keyboards::UiLanguage::En => ("Job completed", "Job errors", "Watch conflicts"),
    };
    let lines = vec![
        lang.text(T::SettingsTitle).to_string(),
        String::new(),
        format!("{}  {}", lang.text(T::HomeDriveLabel), drive_status),
        format!("{}:", lang.text(T::SettingsNotifications)),
        format!("• {job_complete}: {}", state(prefs.notify_job_complete)),
        format!("• {job_error}: {}", state(prefs.notify_job_error)),
        format!(
            "• {watch_conflicts}: {}",
            state(prefs.notify_watch_conflicts)
        ),
        format!("{}  {}", lang.text(T::SettingsLanguage), lang_value),
    ];
    lines.join("\n")
}

pub(crate) fn account_title(lang: keyboards::UiLanguage) -> &'static str {
    match lang {
        keyboards::UiLanguage::Vi => "TÀI KHOẢN GOOGLE",
        keyboards::UiLanguage::En => "GOOGLE ACCOUNT",
    }
}

pub(crate) fn account_status_field(lang: keyboards::UiLanguage) -> &'static str {
    match lang {
        keyboards::UiLanguage::Vi => "Trạng thái",
        keyboards::UiLanguage::En => "Status",
    }
}

pub(crate) fn account_name_field(lang: keyboards::UiLanguage) -> &'static str {
    match lang {
        keyboards::UiLanguage::Vi => "Tên",
        keyboards::UiLanguage::En => "Name",
    }
}

pub(crate) fn account_login_hint(lang: keyboards::UiLanguage) -> &'static str {
    match lang {
        keyboards::UiLanguage::Vi => "Chạy trên máy đang chạy bot:",
        keyboards::UiLanguage::En => "Run this on the machine running the bot:",
    }
}

pub(crate) fn language_panel_text(lang: keyboards::UiLanguage) -> String {
    match lang {
        keyboards::UiLanguage::Vi => {
            "NGÔN NGỮ\n━━━━━━━━━━\nHiện tại: Tiếng Việt\n\nBấm English để đổi ngay cho tài khoản Telegram này.".to_string()
        }
        keyboards::UiLanguage::En => {
            "LANGUAGE\n━━━━━━━━━━\nCurrent: English\n\nTap Tiếng Việt to switch this Telegram account immediately.".to_string()
        }
    }
}

pub(crate) fn language_from_callback(code: &str) -> Option<keyboards::UiLanguage> {
    match code {
        "vi" => Some(keyboards::UiLanguage::Vi),
        "en" => Some(keyboards::UiLanguage::En),
        _ => None,
    }
}

pub(crate) fn language_changed_text(lang: keyboards::UiLanguage) -> String {
    match lang {
        keyboards::UiLanguage::Vi => {
            "Đã đổi sang Tiếng Việt.\n\nCác panel và nút mới sẽ dùng Tiếng Việt ngay.".to_string()
        }
        keyboards::UiLanguage::En => {
            "Switched to English.\n\nNew panels and buttons will use English immediately."
                .to_string()
        }
    }
}

pub(crate) fn language_invalid_text(lang: keyboards::UiLanguage) -> String {
    match lang {
        keyboards::UiLanguage::Vi => {
            "Ngôn ngữ không hợp lệ. Chọn Tiếng Việt hoặc English.".to_string()
        }
        keyboards::UiLanguage::En => "Invalid language. Choose Tiếng Việt or English.".to_string(),
    }
}

pub(crate) fn language_save_error(lang: keyboards::UiLanguage, err: &anyhow::Error) -> String {
    match lang {
        keyboards::UiLanguage::Vi => format!("Lỗi lưu ngôn ngữ: {err}"),
        keyboards::UiLanguage::En => format!("Could not save language: {err}"),
    }
}

pub(crate) fn settings_save_error(lang: keyboards::UiLanguage) -> String {
    match lang {
        keyboards::UiLanguage::Vi => {
            "Không thể lưu cài đặt lúc này. Vui lòng thử lại.".to_string()
        }
        keyboards::UiLanguage::En => {
            "Could not save settings right now. Please try again.".to_string()
        }
    }
}

pub(crate) fn account_destination_title(lang: keyboards::UiLanguage) -> &'static str {
    match lang {
        keyboards::UiLanguage::Vi => "THƯ MỤC ĐÍCH",
        keyboards::UiLanguage::En => "DESTINATION FOLDER",
    }
}

pub(crate) fn account_parent_id_field(lang: keyboards::UiLanguage) -> &'static str {
    match lang {
        keyboards::UiLanguage::Vi => "Parent ID",
        keyboards::UiLanguage::En => "Parent ID",
    }
}

pub(crate) fn account_destination_missing(lang: keyboards::UiLanguage) -> &'static str {
    match lang {
        keyboards::UiLanguage::Vi => "Chưa đặt. Dùng /set_destination <folder_url>.",
        keyboards::UiLanguage::En => "Not set. Use /set_destination <folder_url>.",
    }
}

pub(crate) fn vi_account_status(status: &str) -> &str {
    match status {
        "connected" => "Đã kết nối",
        "reconnect_required" => "Cần đăng nhập lại",
        "revoked" => "Đã thu hồi",
        "disabled" => "Đã tắt",
        other => other,
    }
}

pub(crate) fn account_status_label(lang: keyboards::UiLanguage, status: &str) -> &str {
    if lang == keyboards::UiLanguage::Vi {
        return vi_account_status(status);
    }
    match status {
        "connected" => "Connected",
        "reconnect_required" => "Reconnect required",
        "revoked" => "Revoked",
        "disabled" => "Disabled",
        "chưa kết nối" => "Not connected",
        other => other,
    }
}
