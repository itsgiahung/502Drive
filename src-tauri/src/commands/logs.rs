use crate::commands::get_log_path;
use std::{
    collections::VecDeque,
    fs::File,
    io::{BufRead, BufReader},
};

const DEFAULT_LOG_LINES: usize = 50;
const MAX_LOG_LINES: usize = 500;

#[tauri::command]
pub async fn get_recent_logs(lines: Option<usize>) -> Result<Vec<String>, String> {
    let log_path = get_log_path();
    let count = lines.unwrap_or(DEFAULT_LOG_LINES).clamp(1, MAX_LOG_LINES);

    if !log_path.exists() {
        return Ok(vec![
            format!("[INFO] Nhật ký lưu tại: {}", log_path.display()),
            "[INFO] Hệ thống sẵn sàng ghi log khi service khởi chạy.".to_string(),
        ]);
    }

    tokio::task::spawn_blocking(move || {
        let file = File::open(&log_path).map_err(|e| e.to_string())?;
        let reader = BufReader::new(file);
        let mut tail = VecDeque::with_capacity(count);

        for line in reader.lines() {
            let line = line.map_err(|e| e.to_string())?;
            if tail.len() == count {
                tail.pop_front();
            }
            tail.push_back(line);
        }

        Ok::<Vec<String>, String>(tail.into_iter().collect())
    })
    .await
    .map_err(|e| format!("log reader task failed: {e}"))?
}

#[cfg(test)]
mod tests {
    use super::{DEFAULT_LOG_LINES, MAX_LOG_LINES};

    #[test]
    fn log_line_limits_are_sane() {
        assert!(DEFAULT_LOG_LINES > 0);
        assert!(DEFAULT_LOG_LINES <= MAX_LOG_LINES);
    }
}
