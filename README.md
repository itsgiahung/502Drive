<div align="center">
<img src="docs/assets/mascot.gif" alt="The 502Drive cat carrying files between folders" width="480">

# 502Drive

**Copy Google Drive files server-side. Manage the copy from your terminal.**

[![License: GPL-3.0-only](https://img.shields.io/badge/license-GPL--3.0--only-blue.svg)](LICENSE)
![Go](https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white)
![Platforms](https://img.shields.io/badge/platforms-Linux%20%7C%20Windows%20%7C%20macOS%20CLI-lightgrey)

English · [Vietnamese](README_VI.md)
</div>

502Drive is a self-hosted Google Drive copy and synchronization tool. One Go
daemon owns the database, credentials, queue and workers. The CLI, terminal UI,
system tray and optional Telegram bot control that same service.

File contents stay inside Google Drive: 502Drive requests server-side copies
rather than downloading and uploading files through your machine. Metadata
requests still use your connection, and Google's permissions and quotas apply.

## What you can do

- Copy a file or folder by link, preserving its tree within Google's copy rules.
- Preview work, filter by path/type/size, submit several links and manage a fair queue.
- Keep a collection in sync on a schedule or through Drive Changes polling.
- Audit a copy and preview repairs before applying them.
- Enable **mirror** per collection to remove owned destination copies when their
  sources disappear. It is off by default and never permanently deletes files.
- Capture **snapshots**, pin restore points and preview a restore before applying it.
  Automatic snapshots are off by default. Retained copies consume Drive storage.
- Use Vietnamese or English in the application; scripts receive JSON and stable exit codes.

## Install

Download the package for your platform from [Releases](https://github.com/itsgiahung/502Drive/releases)
and verify its SHA-256 checksum.

| Platform | Packages and instructions |
| --- | --- |
| Linux | RPM, DEB or archive; [Linux guide](docs/install-linux.md) |
| Windows | ZIP or unsigned Inno Setup installer; [Windows guide](docs/install-windows.md) |
| macOS | CLI/daemon archive; no native tray |
| Docker / Podman | Build from this repository; [container guide](docs/install-docker.md) |
| Source | Go 1.27; `go install github.com/itsgiahung/502Drive/cmd/502drive@latest` |

The installer scripts verify release checksums and install the `502d` shortcut:

```sh
# Linux or macOS
curl -fsSL https://raw.githubusercontent.com/itsgiahung/502Drive/main/scripts/install.sh | sh
```

```powershell
# Windows
irm https://raw.githubusercontent.com/itsgiahung/502Drive/main/scripts/install.ps1 | iex
```

## First copy

You need a Google account and your own **Desktop app OAuth client** with the Drive
API enabled. Follow the [OAuth setup guide](docs/oauth-setup.md); credentials stay
on your machine. There is no shared project server or Google account.

```sh
502drive setup --oauth-client /path/to/client_secret.json
502drive up
502drive account login
502drive destinations add 'https://drive.google.com/drive/folders/DESTINATION_ID'
502drive destinations list
502drive destinations default DESTINATION_ID_FROM_LIST
502drive copy 'https://drive.google.com/drive/folders/SOURCE_ID' --wait
```

Use the destination's **502Drive ID** returned by `destinations add/list` for
`destinations default`; it is different from the Google folder ID.

Run `502drive` for the interactive launcher, or `502drive onboard` for a guided
readiness check. The TUI shows setup steps when configuration is incomplete.

## Everyday commands

```sh
502drive jobs list
502drive wait JOB_ID
502drive jobs report JOB_ID --format csv
502drive sync enable COLLECTION_ID --daily 22:00
502drive sync run COLLECTION_ID
502drive audit COLLECTION_ID
502drive pause
502drive resume
502drive doctor
502drive logs -f
```

Inspect before copying with `inspect LINK --deep`. Run `COMMAND --help` for flags;
see the [CLI reference](docs/features/cli.md) for JSON, exit codes and maintenance.

Mirror and snapshots are explicit collection options:

```sh
502drive sync options COLLECTION_ID --snapshots=true --keep-days 30 --keep-count 10
502drive snapshots create COLLECTION_ID
502drive sync options COLLECTION_ID --mirror=true
502drive snapshots restore SNAPSHOT_ID             # preview only
502drive snapshots restore SNAPSHOT_ID --apply     # applies and pauses sync
```

Read [protection options](docs/features/protection.md) before enabling mirror or
restoring. A snapshot retains physical copies; it cannot recover a file someone
permanently deletes or guarantee recovery after an external content edit.

## Terminal UI and background service

`1`–`7` select tabs. Use `↑↓` to select rows, `Enter` to open and `←` or `Esc` to
return from detail views. On a collection, select **Sync** and press `Enter` to
choose a schedule or action. Letter shortcuts remain available. In text fields,
arrows edit the text; in the Drive browser, `←` opens the parent folder.

`q` exits the TUI while the daemon keeps running. Only `502drive stop` or the
tray's **Stop 502Drive…** action stops the detached daemon. Tray startup and
system autostart are optional; foreground `serve` ends with its process.

Telegram is optional. It supports private chats, an allowlist, owner/operator
permissions and expiring one-shot callbacks. See the [Telegram guide](docs/features/telegram.md).

## Safety and limitations

- Copies have different IDs and may have different owners and permissions.
  Sharing settings and revision history are not replicated.
- Source removals preserve destination files by default; mirror changes that
  policy only for the selected collection's owned copies.
- Remote writes record durable intent. Unknown outcomes are reconciled; the
  daemon blocks when it cannot establish a safe result.
- The API uses loopback HTTP with a private bearer token. Use SSH forwarding for
  remote access. Google tokens are encrypted at rest; backups contain secrets.
- No automatic public links, quota bypass, telemetry or automatic software updates.

Core copy/sync/recovery and Telegram flows have a recorded live Linux smoke test.
Mirror/snapshots currently have local fixture coverage, not live Google acceptance.
Windows installer, Shared Drives and large-tree acceptance remain outstanding.
See [verification](docs/verification.md) for the precise scope.

## Documentation and development

Start with the [documentation index](docs/README.md). Maintainers should read
[architecture](docs/architecture.md), [HTTP/OpenAPI](docs/control-contracts.md),
[data contracts](docs/data-contracts.md) and [CONTRIBUTING](CONTRIBUTING.md).

```sh
make check           # formatting, vet, race tests
make docs            # documentation links, OpenAPI JSON, whitespace
make release-local   # local release build; publishes nothing
```

[Changelog](CHANGELOG.md) · [Privacy](PRIVACY.md) · [Security](SECURITY.md) ·
[Third-party notices](THIRD_PARTY_NOTICES.md)

502Drive is independent of Google and is not endorsed by Google. Google Drive is
a trademark of Google LLC. Licensed under [GPL-3.0-only](LICENSE).
