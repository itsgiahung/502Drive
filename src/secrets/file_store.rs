use std::path::{Path, PathBuf};

use anyhow::{Context, bail};
use rand::RngCore;

#[derive(Debug, Clone)]
pub struct FileSecretStore {
    key_path: PathBuf,
}

impl FileSecretStore {
    pub fn new(config_dir: &Path) -> Self {
        Self {
            key_path: config_dir.join("master.key"),
        }
    }

    pub async fn load_or_create_key(&self) -> anyhow::Result<[u8; 32]> {
        if let Some(parent) = self.key_path.parent() {
            tokio::fs::create_dir_all(parent).await?;
            #[cfg(unix)]
            {
                use std::os::unix::fs::PermissionsExt;
                tokio::fs::set_permissions(parent, std::fs::Permissions::from_mode(0o700))
                    .await
                    .with_context(|| format!("chmod 0700 {}", parent.display()))?;
            }
        }

        if self.key_path.exists() {
            #[cfg(unix)]
            {
                use std::os::unix::fs::PermissionsExt;
                let meta = tokio::fs::symlink_metadata(&self.key_path)
                    .await
                    .with_context(|| format!("stat {}", self.key_path.display()))?;
                if meta.file_type().is_symlink() {
                    bail!("refusing symlinked master key {}", self.key_path.display());
                }
                if !meta.is_file() {
                    bail!("master key path is not a regular file: {}", self.key_path.display());
                }
                tokio::fs::set_permissions(
                    &self.key_path,
                    std::fs::Permissions::from_mode(0o600),
                )
                .await
                .with_context(|| format!("chmod 0600 {}", self.key_path.display()))?;
            }

            let data = tokio::fs::read(&self.key_path)
                .await
                .with_context(|| format!("read {}", self.key_path.display()))?;
            if data.len() != 32 {
                bail!("{} must contain exactly 32 bytes", self.key_path.display());
            }
            let mut key = [0_u8; 32];
            key.copy_from_slice(&data);
            return Ok(key);
        }

        let mut key = [0_u8; 32];
        rand::rng().fill_bytes(&mut key);
        write_new_key(&self.key_path, &key).await?;
        Ok(key)
    }

    pub fn backend_name(&self) -> &'static str {
        "file"
    }
}

#[cfg(unix)]
async fn write_new_key(path: &Path, key: &[u8; 32]) -> anyhow::Result<()> {
    use std::os::unix::fs::OpenOptionsExt;

    let path = path.to_path_buf();
    let key = *key;
    tokio::task::spawn_blocking(move || {
        let mut opts = std::fs::OpenOptions::new();
        opts.write(true).create_new(true).mode(0o600);
        std::io::Write::write_all(&mut opts.open(path)?, &key)?;
        Ok::<_, std::io::Error>(())
    })
    .await??;
    Ok(())
}

#[cfg(not(unix))]
async fn write_new_key(path: &Path, key: &[u8; 32]) -> anyhow::Result<()> {
    tokio::fs::write(path, key).await?;
    Ok(())
}

#[cfg(all(test, unix))]
mod tests {
    use std::{
        os::unix::fs::{PermissionsExt, symlink},
        time::{SystemTime, UNIX_EPOCH},
    };

    use super::FileSecretStore;

    fn temp_dir(label: &str) -> std::path::PathBuf {
        let nonce = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_nanos();
        std::env::temp_dir().join(format!(
            "502drive_secret_{label}_{}_{}",
            std::process::id(),
            nonce
        ))
    }

    #[tokio::test]
    async fn existing_key_permissions_are_hardened_on_load() {
        let dir = temp_dir("permissions");
        std::fs::create_dir_all(&dir).unwrap();
        std::fs::set_permissions(&dir, std::fs::Permissions::from_mode(0o755)).unwrap();
        let key_path = dir.join("master.key");
        std::fs::write(&key_path, [7_u8; 32]).unwrap();
        std::fs::set_permissions(&key_path, std::fs::Permissions::from_mode(0o644)).unwrap();

        let key = FileSecretStore::new(&dir).load_or_create_key().await.unwrap();
        assert_eq!(key, [7_u8; 32]);
        assert_eq!(
            std::fs::metadata(&dir).unwrap().permissions().mode() & 0o777,
            0o700
        );
        assert_eq!(
            std::fs::metadata(&key_path).unwrap().permissions().mode() & 0o777,
            0o600
        );

        let _ = std::fs::remove_dir_all(dir);
    }

    #[tokio::test]
    async fn symlinked_master_key_is_rejected() {
        let dir = temp_dir("symlink");
        std::fs::create_dir_all(&dir).unwrap();
        let target = dir.join("target.key");
        std::fs::write(&target, [9_u8; 32]).unwrap();
        symlink(&target, dir.join("master.key")).unwrap();

        let err = FileSecretStore::new(&dir)
            .load_or_create_key()
            .await
            .unwrap_err();
        assert!(err.to_string().contains("refusing symlinked master key"));

        let _ = std::fs::remove_dir_all(dir);
    }
}
