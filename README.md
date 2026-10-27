<div align="center">
<img src="docs/assets/mascot.gif" alt="The 502Drive cat carrying files between folders" width="480">

# 502Drive

**Copy and sync Google Drive folders without downloading them.**

[![License: GPL-3.0-only](https://img.shields.io/badge/license-GPL--3.0--only-blue.svg)](LICENSE)
![Go](https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white)
![Platforms](https://img.shields.io/badge/platforms-Linux%20%7C%20Windows%20%7C%20macOS%20CLI-lightgrey)

English · [Tiếng Việt](README_VI.md)
</div>

Paste a Google Drive link and 502Drive copies the whole folder into your Drive,
keeping its folders and file names. The copy happens **inside Google**: nothing is
downloaded to your computer or uploaded again, so a 50 GB folder uses almost none
of your bandwidth. If your laptop sleeps or goes offline, the copy picks up where it
left off. Later 502Drive can keep that copy in sync with the original.

It runs on your own machine with your own Google login. There is no 502Drive
server and no account to create.

## Why use it

- **Big folders, small connection.** Google does the copying; your machine only
  sends instructions.
- **Safe to interrupt.** Close the lid, lose Wi-Fi, restart: jobs resume where they
  stopped, and 502Drive checks Drive before sending a copy again, so an
  interruption does not leave duplicates.
- **Stays in sync.** Re-check a copy on a schedule (daily, every few hours) or
  almost in real time when the original changes.
- **Never deletes your files.** Cleaning up only moves 502Drive's own copies to the
  Drive trash, and only after you confirm.
- **Use it your way:** a terminal app with a cat, plain commands for scripts, a
  tray icon, or a Telegram bot.

## Quick start

**1. Install.** Download your package from
[Releases](https://github.com/itsgiahung/502Drive/releases), or:

```sh
# Linux or macOS
curl -fsSL https://raw.githubusercontent.com/itsgiahung/502Drive/main/scripts/install.sh | sh
```

```powershell
# Windows (PowerShell)
irm https://raw.githubusercontent.com/itsgiahung/502Drive/main/scripts/install.ps1 | iex
```

Both scripts check the download against the published checksums.

**2. Make a Google key (once, about five minutes).** Google requires every app
that copies Drive files to have a key. You create your own in Google Cloud, so
your files are never shared with anyone else's project:
[step-by-step guide](docs/oauth-setup.md). You end up with a small
`client_secret….json` file.

**3. Run the setup guide.**

```sh
502drive onboard
```

It asks for that JSON file (you can drag it into the terminal), starts 502Drive in
the background, opens the Google sign-in page, and asks where copies should go
(press Enter for My Drive). When it prints **Ready**, you are done.

**4. Copy something.**

```sh
502d 'https://drive.google.com/drive/folders/SOURCE_ID' --wait
```

Or just run `502drive` to open the app and paste the link there.

## Everyday use

| I want to… | Command |
| --- | --- |
| Open the app | `502drive` |
| Copy a link and wait for it | `502d LINK --wait` |
| See what a link contains first | `502drive inspect LINK --deep` |
| Copy several links at once | `502d LINK1 LINK2 LINK3` |
| See my jobs | `502drive jobs list` |
| Get a list of files that failed | `502drive jobs report JOB_ID --format csv` |
| See my copies | `502drive collections list` |
| Refresh a copy now | `502drive sync run COPY_ID` |
| Refresh it every night at 22:00 | `502drive sync enable COPY_ID --daily 22:00` |
| Check a copy matches the original | `502drive audit COPY_ID` |
| Pause or resume everything | `502drive pause` · `502drive resume` |
| Check that everything is set up | `502drive doctor` |
| Stop 502Drive | `502drive stop` |
| Update to the latest release | `502drive update --install --restart` |

`COPY_ID` comes from `collections list`. Every command has `--help`, and
`--json` for scripts. The full list is in the [CLI reference](docs/features/cli.md).

### In the app

Number keys `1`–`7` switch tabs, arrows move, `Enter` opens, `←` or `Esc` goes
back, and `q` closes the app. Closing the app does not stop copies: 502Drive keeps
working in the background until you run `502drive stop` or choose **Stop 502Drive…**
in the tray.

### Telegram (optional)

Send a Drive link to your own bot to start a copy and get a message when it
finishes. Only the chats you allow can use it. See the
[Telegram guide](docs/features/telegram.md).

## What happens to files

- **When the original changes**, the next refresh copies new and edited files,
  moves and renames what was moved or renamed, and keeps the older version of an
  edited file unless you ask it to replace them.
- **When a file is deleted from the original**, your copy keeps it. If you want the
  copy to follow deletions too, turn on *mirror* for that copy; deleted files then
  go to the Drive trash, never permanently.
- **When you delete something from your copy yourself**, 502Drive does not put it
  back on its own. `502drive audit COPY_ID` lists what is missing, and
  `audit COPY_ID --repair` shows what it would restore before you apply it.
- **Restore points** (*snapshots*) can keep earlier versions of a copy and roll it
  back. They are off by default and use Drive storage. See
  [mirror and snapshots](docs/features/protection.md).
- To remove a copy entirely: `502drive collections retire COPY_ID`, then
  `502drive collections cleanup COPY_ID` shows what would go to the trash and the
  exact command to confirm it.

## Good to know

- Copies get new file IDs and you become their owner. Sharing settings, comments
  and version history are not copied; Google decides what can be copied.
- Google's daily copy limit still applies. When it is reached, the job waits and
  continues by itself when Google allows it.
- Your Google login is stored encrypted on your machine. Backups made with
  `502drive backup` contain it, so keep them private.
- 502Drive talks to your Google account and, if you set it up, Telegram, plus
  GitHub when you check for updates. No telemetry, no automatic updates, no public
  sharing links.

**Tested so far:** copying, syncing, crash recovery, audit and Telegram on Linux
with a real Google account. **Not yet tested for real:** Windows and macOS
installs, Shared Drives, very large folders, and mirror and snapshots. Details:
[verification](docs/verification.md).

## For developers

Start with the [documentation index](docs/README.md) and
[architecture](docs/architecture.md). Contributions: [CONTRIBUTING](CONTRIBUTING.md).

```sh
make check           # format, vet and race tests
make docs            # documentation links and API schema
make release-local   # build release packages locally; publishes nothing
```

[Changelog](CHANGELOG.md) · [Privacy](PRIVACY.md) · [Security](SECURITY.md) ·
[Third-party notices](THIRD_PARTY_NOTICES.md)

502Drive is an independent project, not made or endorsed by Google. Google Drive is
a trademark of Google LLC. Licensed under [GPL-3.0-only](LICENSE).
