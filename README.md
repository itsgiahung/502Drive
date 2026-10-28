<div align="center">
<img src="docs/assets/mascot.gif" alt="The 502Drive cat carrying files between folders" width="480">

# 502Drive

**Copy and sync Google Drive folders without downloading them.**

[![License: GPL-3.0-only](https://img.shields.io/badge/license-GPL--3.0--only-blue.svg)](LICENSE)
![Go](https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white)
![Platforms](https://img.shields.io/badge/platforms-Linux%20%7C%20Windows%20%7C%20macOS%20CLI-lightgrey)

English · [Tiếng Việt](README_VI.md)
</div>

## What it does

Paste a Google Drive link. 502Drive makes a copy of that whole folder in **your own
Drive**, keeping the folders and file names. The copy happens **inside Google**:
nothing is downloaded to your computer and uploaded again, so a 50 GB folder uses
almost none of your internet connection.

```
 a Drive link  ──►  502Drive asks Google to copy  ──►  the folder appears in your Drive
 (someone's folder)      (your computer only sends instructions)      (you own the copy)
```

It runs on your own machine with your own Google login. There is no 502Drive server
and no account to create.

**Good for:** copying a big shared folder into your own Drive, keeping that copy up
to date with the original, and doing it from a terminal, a tray icon or a Telegram
bot.

**Not for:** backing up files from your computer, downloading Drive files to disk,
or copying things Google does not allow to be copied.

## Why use it

- **Big folders, small connection.** Google does the copying.
- **Safe to interrupt.** Close the lid, lose Wi-Fi, restart: the job continues where
  it stopped, and 502Drive asks Drive before sending a copy again, so an interruption
  does not leave duplicates.
- **Stays in sync.** Refresh a copy on a schedule, or almost at once when the
  original changes.
- **Never deletes your files.** Cleaning up only moves 502Drive's own copies to the
  Drive trash, and only after you confirm.

## Before you start

| You need | How long |
| --- | --- |
| A Google account (the one that will own the copies) | — |
| About **20 minutes** the first time (about 10 for the Google key, 5 to install, 5 for the first copy) | once |
| Basic comfort with a terminal: you paste a few commands | — |
| *Optional:* a Telegram account to control it from your phone | +5 minutes |

## Set up (4 steps)

### 1. Install (about 5 minutes)

Pick one:

| Your system | Easiest way |
| --- | --- |
| **Windows** | Run `irm https://raw.githubusercontent.com/itsgiahung/502Drive/main/scripts/install.ps1 \| iex` in PowerShell, or download `502drive-setup.exe` from [Releases](https://github.com/itsgiahung/502Drive/releases) (see [Windows guide](docs/install-windows.md)). |
| **Linux** | `curl -fsSL https://raw.githubusercontent.com/itsgiahung/502Drive/main/scripts/install.sh \| sh`, or the `.rpm`/`.deb` package (see [Linux guide](docs/install-linux.md)). |
| **macOS** | The same `install.sh` line as Linux (Apple Silicon only, command line only, [not tested yet](#whats-tested)). |
| **Docker / server** | See the [Docker guide](docs/install-docker.md). |

Both scripts verify the download against the published checksums. Releases from
0.4.5 on also carry a signature of those checksums; the Linux/macOS script checks it
when [`cosign`](https://docs.sigstore.dev/cosign/installation/) is installed. To check
by hand, see [Verify a release](docs/install-linux.md#verify-a-release). The Windows
installer is not code-signed yet, so Windows may show an "unknown publisher" warning.

### 2. Make your Google key (once, about 10 minutes)

Google requires every app that copies Drive files to have a key. You create your own
in Google Cloud, so your files are never shared with anyone else's project. This is
the only step that takes a little care. In short:

1. **Create a project** at <https://console.cloud.google.com/projectcreate>.
2. **Turn on the Google Drive API** for it:
   <https://console.cloud.google.com/apis/library/drive.googleapis.com>.
3. **Consent screen:** at <https://console.cloud.google.com/auth/branding> choose
   **External**, then under **Audience** add your own Gmail as a *test user*.
4. **Publish the app** (button **Publish app**, status **In production**).
   *Do not skip this:* an app left in "Testing" makes Google sign you out every
   7 days. For your own use no review is needed; at sign-in Google shows a
   "this app isn't verified" screen: choose **Advanced → Go to … (unsafe)**. It is
   your own key.
5. **Create the key:** <https://console.cloud.google.com/auth/clients/create>, type
   **Desktop app**, then press **Download JSON**. You get a file named
   `client_secret….json`.

Full screenshots-free walkthrough with every link: [Google key guide](docs/oauth-setup.md).
`502drive onboard` (next step) opens each page for you and waits.

### 3. Run the setup guide (about 3 minutes)

```sh
502drive onboard
```

It asks for the JSON file (you can drag it into the terminal), starts 502Drive in
the background, opens the Google sign-in page, and asks where copies should go
(press Enter for My Drive). When it prints **Ready**, you are done.

### 4. Make your first copy (about 2 minutes)

```sh
502d 'https://drive.google.com/drive/folders/SOURCE_ID' --wait
```

Or run `502drive` to open the app and paste the link there. The copy appears in
your Drive with the same name as the original.

### Optional: control it from Telegram (about 5 minutes)

```sh
502drive onboard --telegram
```

The guide opens **@BotFather** for you: send `/newbot`, choose a name, and paste the
token it gives you. Then open your new bot and press **Start**; 502Drive reads your
numeric id from that message, so you do not need to look it up. After
`502drive stop` and `502drive up` the bot answers only to you. See the
[Telegram guide](docs/features/telegram.md).

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
| Clear old finished jobs from the list | `502drive history clear --older-than 30d` |
| Check that everything is set up | `502drive doctor` |
| Update to the latest release | `502drive update --install --restart` |
| Stop 502Drive | `502drive stop` |

`COPY_ID` comes from `collections list`. Every command has `--help`, and `--json`
for scripts. Full list: [CLI reference](docs/features/cli.md).

**In the app:** number keys `1`–`7` switch tabs, arrows move, `Enter` opens, `←` or
`Esc` goes back, `q` closes the app. Closing the app does not stop copies; 502Drive
keeps working until you run `502drive stop` or pick **Stop 502Drive…** in the tray.
Each row has a one-line explanation under it.

## What happens to files

- **The original changes:** the next refresh copies new and edited files, follows
  moves and renames, and keeps the older version of an edited file unless you ask
  it to replace them.
- **A file is deleted from the original:** your copy keeps it. To make the copy
  follow deletions, turn on *mirror* for that copy; deleted files then go to the
  Drive trash, never permanently.
- **You delete something from your copy:** 502Drive does not put it back by itself.
  `502drive audit COPY_ID` lists what is missing and `audit COPY_ID --repair` shows
  what it would restore before you apply it.
- **Two identical files with the same name in one folder:** one copy is made. Files
  with the same name but different content both stay (`name (2).ext`).
- **Restore points** (*snapshots*) keep earlier versions of a copy and can roll it
  back. They are off by default and use Drive storage:
  [mirror and snapshots](docs/features/protection.md).
- **Remove a copy:** `502drive collections retire COPY_ID`, then
  `502drive collections cleanup COPY_ID` shows what would go to the trash and the
  exact command to confirm it.
- **Tidy the job list:** `502drive history clear` (or Settings → *Clear history…* in
  the app) only hides finished jobs from the lists. Your copies and Drive files are
  untouched.

## Is it safe?

- The key you made belongs to you and stays on your machine. Your Google login is
  stored **encrypted** locally; backups (`502drive backup`) contain it, so keep them
  private.
- 502Drive asks Google for permission to use your Drive (needed to copy). It never
  shares anything publicly and sends no telemetry. It talks only to Google, to
  Telegram if you set it up, and to GitHub when you check for updates.
- It never permanently deletes anything on Drive. Cleanup moves only its own copies
  to the trash, and only after you confirm.
- Copies get new file IDs and you own them. Sharing settings, comments and version
  history are not copied; Google decides what can be copied.
- Google's daily copy limit still applies. When it is reached, the job waits and
  continues by itself.

## Something went wrong?

| What you see | What to do |
| --- | --- |
| `daemon_unavailable` | Run `502drive up`. |
| It asks to sign in again about every 7 days | Your Google app is still in *Testing*: publish it (step 2.4), then run `502drive account login`. |
| `account_reconnect_required` | `502drive account login`. |
| A job says **blocked** | `502drive jobs get JOB_ID` shows why; most often Google's quota or a missing permission. |
| Nothing is set up yet | `502drive doctor` lists what is missing; `502drive onboard` fixes it. |

More: [troubleshooting](docs/troubleshooting.md) · [uninstall and rollback](docs/runbooks.md).

## What's tested

Tested with a real Google account on Linux: copying, syncing, crash recovery,
audit, mirror and snapshots, and a short Telegram session. **Not yet tested for
real:** Windows and macOS use, Shared Drives, very large folders, laptop sleep and
network loss, the tray on every desktop, and the Google key walkthrough from a
blank Google project. Details: [verification](docs/verification.md).

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
