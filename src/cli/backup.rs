use std::{
    fs,
    path::{Path, PathBuf},
    time::{SystemTime, UNIX_EPOCH},
};

use anyhow::{Context, bail};

use crate::config::AppConfig;

pub fn run(config_path: &Path, config: &AppConfig, output_dir: &Path) -> anyhow::Result<()> {
    fs::create_dir_all(output_dir)
        .with_context(|| format!("create backup dir {}", output_dir.display()))?;
    harden_private_dir(output_dir)?;

    copy_sensitive_if_exists(config_path, &output_dir.join("config.toml"))?;
    snapshot_sqlite_if_exists(&config.storage.db_path, &output_dir.join("state.db"))?;
    copy_sensitive_if_exists(
        &config.storage.config_dir.join("master.key"),
        &output_dir.join("master.key"),
    )?;

    let restore_path = output_dir.join("RESTORE.vi.txt");
    fs::write(&restore_path, restore_note())?;
    harden_private_file(&restore_path)?;

    println!("Backup đã tạo: {}", output_dir.display());
    println!("Gồm: config.toml, state.db, master.key, RESTORE.vi.txt");
    println!("state.db được chụp nhất quán bằng SQLite VACUUM INTO; không cần dừng bot.");
    Ok(())
}

fn copy_sensitive_if_exists(source: &Path, dest: &Path) -> anyhow::Result<()> {
    if !source.exists() {
        println!("Bỏ qua, chưa có: {}", source.display());
        return Ok(());
    }
    ensure_distinct_paths(source, dest)?;
    fs::copy(source, dest)
        .with_context(|| format!("copy {} -> {}", source.display(), dest.display()))?;
    harden_private_file(dest)?;
    println!("Đã copy: {} -> {}", source.display(), dest.display());
    Ok(())
}

fn snapshot_sqlite_if_exists(source: &Path, dest: &Path) -> anyhow::Result<()> {
    if !source.exists() {
        println!("Bỏ qua, chưa có: {}", source.display());
        return Ok(());
    }
    ensure_distinct_paths(source, dest)?;

    let temp = snapshot_temp_path(dest);
    if temp.exists() {
        fs::remove_file(&temp)
            .with_context(|| format!("remove stale snapshot {}", temp.display()))?;
    }

    let result = (|| -> anyhow::Result<()> {
        let conn = rusqlite::Connection::open(source)
            .with_context(|| format!("open SQLite database {}", source.display()))?;
        let temp_arg = temp.to_string_lossy().into_owned();
        conn.execute("VACUUM INTO ?1", [temp_arg])
            .with_context(|| format!("snapshot SQLite database {}", source.display()))?;

        let snapshot = rusqlite::Connection::open(&temp)
            .with_context(|| format!("open SQLite snapshot {}", temp.display()))?;
        let integrity: String = snapshot.query_row("PRAGMA quick_check", [], |row| row.get(0))?;
        if integrity != "ok" {
            bail!("SQLite backup quick_check failed: {integrity}");
        }
        drop(snapshot);

        harden_private_file(&temp)?;
        if dest.exists() {
            fs::remove_file(dest)
                .with_context(|| format!("replace previous backup {}", dest.display()))?;
        }
        fs::rename(&temp, dest)
            .with_context(|| format!("publish SQLite snapshot {}", dest.display()))?;
        harden_private_file(dest)?;
        println!("Đã snapshot: {} -> {}", source.display(), dest.display());
        Ok(())
    })();

    if result.is_err() && temp.exists() {
        let _ = fs::remove_file(&temp);
    }
    result
}

fn snapshot_temp_path(dest: &Path) -> PathBuf {
    let nonce = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_nanos();
    let file_name = dest
        .file_name()
        .and_then(|name| name.to_str())
        .unwrap_or("state.db");
    dest.with_file_name(format!(".{file_name}.snapshot-{}-{nonce}", std::process::id()))
}

fn ensure_distinct_paths(source: &Path, dest: &Path) -> anyhow::Result<()> {
    if source.exists() && dest.exists() {
        let source = fs::canonicalize(source)?;
        let dest = fs::canonicalize(dest)?;
        if source == dest {
            bail!("backup destination must not overwrite source {}", source.display());
        }
    }
    Ok(())
}

fn harden_private_dir(path: &Path) -> anyhow::Result<()> {
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        fs::set_permissions(path, fs::Permissions::from_mode(0o700))
            .with_context(|| format!("chmod 0700 {}", path.display()))?;
    }
    Ok(())
}

fn harden_private_file(path: &Path) -> anyhow::Result<()> {
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        fs::set_permissions(path, fs::Permissions::from_mode(0o600))
            .with_context(|| format!("chmod 0600 {}", path.display()))?;
    }
    Ok(())
}

fn restore_note() -> &'static str {
    "502Drive private backup\n\
     =======================\n\
     \n\
     Khôi phục trên PC/laptop khác:\n\
     \n\
     1. Cài binary gdclone-bot cùng phiên bản hoặc mới hơn.\n\
     2. Tạo thư mục config/data mặc định nếu chưa có.\n\
     3. Copy config.toml vào đường dẫn config của máy mới.\n\
     4. Copy state.db vào đúng storage.db_path trong config.toml.\n\
     5. Copy master.key vào cùng thư mục config.toml.\n\
     6. Chạy: gdclone-bot --config <config.toml> doctor\n\
     7. Nếu Google token lỗi, chạy lại: gdclone-bot --config <config.toml> auth login\n\
     \n\
     Không share thư mục backup này công khai. Nó có config, state và key giải mã token.\n"
}

#[cfg(test)]
mod tests {
    use super::*;

    fn temp_dir(label: &str) -> PathBuf {
        let nonce = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_nanos();
        std::env::temp_dir().join(format!(
            "502drive_backup_{label}_{}_{}",
            std::process::id(),
            nonce
        ))
    }

    #[test]
    fn sqlite_snapshot_includes_committed_wal_data() {
        let dir = temp_dir("wal");
        fs::create_dir_all(&dir).unwrap();
        let source = dir.join("source.db");
        let dest = dir.join("backup").join("state.db");
        fs::create_dir_all(dest.parent().unwrap()).unwrap();

        let writer = rusqlite::Connection::open(&source).unwrap();
        writer.pragma_update(None, "journal_mode", "WAL").unwrap();
        writer
            .execute_batch(
                "CREATE TABLE evidence(value TEXT NOT NULL);
                 INSERT INTO evidence(value) VALUES ('committed-in-wal');",
            )
            .unwrap();

        snapshot_sqlite_if_exists(&source, &dest).unwrap();

        let snapshot = rusqlite::Connection::open(&dest).unwrap();
        let value: String = snapshot
            .query_row("SELECT value FROM evidence", [], |row| row.get(0))
            .unwrap();
        assert_eq!(value, "committed-in-wal");
        let integrity: String = snapshot
            .query_row("PRAGMA quick_check", [], |row| row.get(0))
            .unwrap();
        assert_eq!(integrity, "ok");

        #[cfg(unix)]
        {
            use std::os::unix::fs::PermissionsExt;
            assert_eq!(
                fs::metadata(&dest).unwrap().permissions().mode() & 0o777,
                0o600
            );
        }

        drop(snapshot);
        drop(writer);
        let _ = fs::remove_dir_all(dir);
    }

    #[test]
    fn backup_refuses_to_overwrite_its_source() {
        let dir = temp_dir("same-path");
        fs::create_dir_all(&dir).unwrap();
        let source = dir.join("state.db");
        fs::write(&source, b"not-a-db").unwrap();

        let err = ensure_distinct_paths(&source, &source).unwrap_err();
        assert!(err.to_string().contains("must not overwrite source"));

        let _ = fs::remove_dir_all(dir);
    }

    #[test]
    fn restore_note_names_required_files() {
        let note = restore_note();
        assert!(note.contains("config.toml"));
        assert!(note.contains("state.db"));
        assert!(note.contains("master.key"));
        assert!(note.contains("doctor"));
    }
}
