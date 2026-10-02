use std::{
    env, fs,
    path::{Path, PathBuf},
};

use anyhow::Context;

use crate::config::AppConfig;

use super::Platform;

pub struct LinuxPlatform;

impl Platform for LinuxPlatform {
    fn machine_key_material(&self) -> anyhow::Result<Vec<u8>> {
        let machine_id = fs::read_to_string("/etc/machine-id").context("read /etc/machine-id")?;
        Ok(machine_id.trim().as_bytes().to_vec())
    }

    fn install_service(&self, config_path: &Path) -> anyhow::Result<()> {
        let unit_dir = user_unit_dir()?;
        fs::create_dir_all(&unit_dir)?;

        let config = AppConfig::load(config_path)?;
        let writable_paths = service_writable_paths(&config)?;
        for path in &writable_paths {
            fs::create_dir_all(path)
                .with_context(|| format!("create service writable path {}", path.display()))?;
        }

        let exe = env::current_exe()?;
        let unit = build_systemd_unit(&exe, config_path, &writable_paths);
        fs::write(unit_dir.join("gdclone-bot.service"), unit)?;
        println!(
            "Installed user unit at {}",
            unit_dir.join("gdclone-bot.service").display()
        );
        println!(
            "Run: systemctl --user daemon-reload && systemctl --user enable --now gdclone-bot.service"
        );
        Ok(())
    }

    fn uninstall_service(&self) -> anyhow::Result<()> {
        let path = user_unit_dir()?.join("gdclone-bot.service");
        if path.exists() {
            fs::remove_file(&path)?;
        }
        println!("Removed {}", path.display());
        println!("Run: systemctl --user daemon-reload");
        Ok(())
    }
}

fn service_writable_paths(config: &AppConfig) -> anyhow::Result<Vec<PathBuf>> {
    let db_parent = config
        .storage
        .db_path
        .parent()
        .context("storage.db_path must have a parent directory")?;

    let mut paths = vec![
        config.storage.config_dir.clone(),
        db_parent.to_path_buf(),
        config.storage.log_dir.clone(),
        config.storage.report_dir.clone(),
    ];
    paths.sort();
    paths.dedup();
    Ok(paths)
}

fn build_systemd_unit(exe: &Path, config_path: &Path, writable_paths: &[PathBuf]) -> String {
    let writable = writable_paths
        .iter()
        .map(|path| format!("ReadWritePaths={}\n", systemd_quote(path)))
        .collect::<String>();

    format!(
        "[Unit]\n\
         Description=502Drive — Local-first Google Drive Telegram Bot\n\
         Wants=network-online.target\n\
         After=network-online.target\n\
         \n\
         [Service]\n\
         Type=simple\n\
         ExecStart={} --config {} run\n\
         Restart=on-failure\n\
         RestartSec=5\n\
         TimeoutStopSec=30\n\
         StandardOutput=journal\n\
         StandardError=journal\n\
         SyslogIdentifier=gdclone-bot\n\
         Environment=RUST_LOG=gdclone_bot=info,warn\n\
         NoNewPrivileges=yes\n\
         PrivateTmp=yes\n\
         ProtectSystem=strict\n\
         ProtectHome=read-only\n\
         ProtectKernelTunables=yes\n\
         ProtectControlGroups=yes\n\
         RestrictSUIDSGID=yes\n\
         LockPersonality=yes\n\
         MemoryDenyWriteExecute=yes\n\
         RestrictRealtime=yes\n\
         SystemCallFilter=@system-service\n\
         SystemCallErrorNumber=EPERM\n\
         {writable}\
         \n\
         [Install]\n\
         WantedBy=default.target\n",
        systemd_quote(exe),
        systemd_quote(config_path),
    )
}

fn systemd_quote(path: &Path) -> String {
    format!(
        "\"{}\"",
        path.to_string_lossy()
            .replace('\\', "\\\\")
            .replace('"', "\\\"")
            .replace('%', "%%")
    )
}

fn user_unit_dir() -> anyhow::Result<PathBuf> {
    let base = directories::BaseDirs::new().context("resolve home directory")?;
    Ok(base.home_dir().join(".config/systemd/user"))
}

#[cfg(test)]
mod tests {
    use super::{build_systemd_unit, service_writable_paths, systemd_quote};
    use crate::config::AppConfig;
    use std::path::{Path, PathBuf};

    #[test]
    fn service_unit_uses_effective_storage_paths() {
        let mut config =
            AppConfig::load(Path::new("config.sample.toml")).expect("load sample config");
        config.storage.config_dir = PathBuf::from("/tmp/502drive config");
        config.storage.db_path = PathBuf::from("/srv/502drive/state.db");
        config.storage.log_dir = PathBuf::from("/var/tmp/502drive/logs");
        config.storage.report_dir = PathBuf::from("/var/tmp/502drive/reports");

        let writable = service_writable_paths(&config).expect("writable paths");
        let unit = build_systemd_unit(
            Path::new("/opt/502drive/bin/502drive"),
            Path::new("/tmp/502drive config/config.toml"),
            &writable,
        );

        for expected in [
            "/tmp/502drive config",
            "/srv/502drive",
            "/var/tmp/502drive/logs",
            "/var/tmp/502drive/reports",
        ] {
            assert!(
                unit.contains(&format!(
                    "ReadWritePaths={}",
                    systemd_quote(Path::new(expected))
                )),
                "missing writable path {expected}"
            );
        }
        assert!(unit.contains(
            "ExecStart=\"/opt/502drive/bin/502drive\" --config \"/tmp/502drive config/config.toml\" run"
        ));
    }

    #[test]
    fn systemd_quote_escapes_specifier_percent() {
        assert_eq!(
            systemd_quote(Path::new("/tmp/100%/config.toml")),
            "\"/tmp/100%%/config.toml\""
        );
    }
}
