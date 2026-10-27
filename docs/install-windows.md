# Windows installation

Windows releases contain `502drive.exe`, the GUI launcher `502drive-tray.exe`,
and the `502d.cmd` shortcut. Choose the ZIP for portable use or
`502drive-setup.exe` for installation.

The installer is **unsigned**. SmartScreen may show an unknown-publisher warning;
verify SHA-256 against the release checksum. Native installer acceptance is still
outstanding; a cross-build is not proof of successful installation.

## Install and configure

The Inno Setup installer offers:

| Option | Default |
| --- | --- |
| Add the installation directory to PATH | On |
| Start on sign-in | Off |
| Create a desktop shortcut | Off |

Start Menu shortcuts open the tray or terminal menu. Uninstall removes program
files and shortcuts, leaving user configuration, tokens and data intact.
The ZIP can be extracted and run directly. A PowerShell installer is also available:

```powershell
irm https://raw.githubusercontent.com/itsgiahung/502Drive/main/scripts/install.ps1 | iex
```

```powershell
502drive setup --oauth-client C:\path\to\client_secret.json
502drive up
502drive account login
502drive doctor --online
```

See [OAuth setup](oauth-setup.md). New configurations use `%AppData%\502drive`;
older `502drive-next` configurations remain supported. Private-file access is
restricted to the user. Never publish these directories.

## Run and stop

`502drive up` starts the detached daemon. The tray offers **Open 502Drive** and
**Stop 502Drive…**. Exiting the TUI or tray normally leaves the daemon running;
the opt-in `tray_quit_stops_daemon` preference changes tray exit behavior.
`502drive serve` runs in the foreground and ends with its process.

Enable sign-in startup with `502drive autostart enable`; inspect it with
`autostart status`. There is no native Windows service installer or automatic
software updater. Daemon shutdown uses HTTP, not process-name termination.

## Build the installer

A Windows host with **Inno Setup 6** is needed to compile `.iss`. The repository's
Windows CI runner can provide that host; a personal Windows computer is optional.
Stage a verified release archive under `dist/windows`, then run:

```powershell
./packaging/windows/build-installer.ps1 -Version 0.3.0
```

Use `-Compiler PATH` for another ISCC location. Output is
`artifacts/windows/502drive-setup.exe` and its `.sha256` file.

`-SmokeTest` is intended for an isolated Windows runner: silently install with
optional tasks disabled, run `version`, uninstall, and check that a synthetic
sentinel outside the application directory survives. It does not start the daemon
or log in. The release workflow invokes this smoke test, but it has not been run
natively in the latest Linux development session.
