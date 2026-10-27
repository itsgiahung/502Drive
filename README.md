<div align="center">

<img src="docs/assets/mascot.gif" alt="The 502Drive cat carrying files between two folders" width="480">

# 502Drive

**Paste a Google Drive link. Get a server-side copy in your own Drive. Keep it in sync.**

[![License: GPL-3.0](https://img.shields.io/badge/license-GPL--3.0-blue.svg)](LICENSE)
![Go](https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white)
![Platforms](https://img.shields.io/badge/platforms-Linux%20%C2%B7%20Windows%20%C2%B7%20macOS%20(CLI)-lightgrey)
![Status](https://img.shields.io/badge/status-tested%20with%20a%20fake%20Drive%2C%20not%20yet%20live--accepted-orange)

English · [Tiếng Việt](README.vi.md)

</div>

One Go binary: a background daemon, a command line with a live terminal UI, a
system-tray launcher and an optional Telegram bot. Type `502drive` and everything
starts.

## Why

- **Server-side copy.** Google copies the files inside Drive (`files.copy`). Your
  machine never downloads or re-uploads a byte, so a 200 GB folder costs no
  bandwidth and no waiting for a local upload.
- **Faithful tree.** Names, folder structure, empty folders, duplicate names and
  shortcuts are preserved; Google-native files are copied when Drive allows and
  export-only types are `skipped` with a stable code. Duplicate names get ` (2)`,
  `skip` or `update` (`--on-conflict`); uncopyable items never fail the whole job.
- **Stays in sync.** Every copy becomes a *collection* with an optional schedule
  (interval, daily, weekly, with a timezone) or realtime sync from the Drive change
  feed. Run now, pause, stop and resume without losing the destination or the
  mapping. Deleting at the source never deletes your copy. A selective filter
  (path/name/size/mime) limits what a later run copies.
- **Batch and queue.** Paste several links at once: one job each, one batch, a
  fair queue with priorities (`move`), and a global pause (`502d pause`).
- **Check and repair.** `502drive audit COLLECTION` compares source and
  destination; `--repair` plans by default (`--apply` writes), and only objects
  502Drive created are ever moved to the Drive trash.
- **Safe after crashes.** Each remote write is recorded first. After a crash or a
  lost response the job *reconciles* instead of writing twice, and reports
  `blocked` when it cannot be sure, never a false "succeeded".
- **Pleasant to use.** Live progress with speed and time left, multi-select bulk
  actions, a tray icon that shows state, desktop notifications, Vietnamese and
  English, and a small animated cat. Scripts get stable JSON and exit codes.

## Quick start

```bash
502drive          # (or just: 502d) asks for your keys on the first run, starts everything, opens the TUI
```

**Each person runs their own copy.** There is no shared server: you use your own
Google account, your own OAuth client, and your data stays on your machine (or in
your own container). Two people copying the same link means two separate copies in
two separate Drives — nothing is relayed through anyone else.

You only need two things from Google once: an OAuth **Desktop** client
([2 minutes, step by step](docs/oauth-setup.md)). Put its id and secret in a `.env`
file next to where you run the command, or hand over the downloaded JSON:

```bash
# .env  (read by `502drive setup`; copied into private 0600 files, never printed)
GOOGLE_OAUTH_CLIENT_ID=1234-abc.apps.googleusercontent.com
GOOGLE_OAUTH_CLIENT_SECRET=...
TELEGRAM_BOT_TOKEN=123456:ABC...        # optional
TELEGRAM_OWNER_ID=123456789             # optional, your numeric id from @userinfobot
```

```bash
502drive setup                              # reads ./.env (or --env-file PATH), or asks in the terminal
502drive setup --oauth-client client_secret.json   # alternative: the JSON Google lets you download
```

Then press **`l`** in the TUI (or run `502drive account login`): it opens the Google
page in your browser and shows the link too, in case you are on another machine.
Approve once and you are connected. Add a destination folder and paste a link:

```bash
502drive destinations add 'https://drive.google.com/drive/folders/FOLDER_ID'
502d 'https://drive.google.com/drive/folders/SOURCE_ID' --wait   # bare link = copy
502drive copy 'https://drive.google.com/drive/folders/SOURCE_ID' --wait
```

The daemon and the tray keep running when you close the terminal or quit the
tray. Only `502drive stop` (or the tray's **Stop 502Drive…**, which asks when
jobs are running) stops the daemon; running jobs resume on the next start.

## Install

One line (verifies the SHA-256 checksum before installing; needs a published release):

```bash
curl -fsSL https://raw.githubusercontent.com/itsgiahung/502Drive/main/scripts/install.sh | sh        # Linux, macOS
irm https://raw.githubusercontent.com/itsgiahung/502Drive/main/scripts/install.ps1 | iex            # Windows (PowerShell)
```

Both install `502drive` and the short alias **`502d`** (d for drive), so after
installing you can just type `502d`. The bare `502` is deliberately left free for other tools.

| Platform | How |
| --- | --- |
| Linux (Fedora/RHEL, Debian/Ubuntu) | `.rpm` / `.deb` from the release page (installs `502drive` and `502d`). [Guide](docs/install-linux.md) |
| Windows 10/11 | `.zip`, or the installer (unsigned: SmartScreen will warn). [Guide](docs/install-windows.md) |
| Docker | [Guide](docs/install-docker.md) |
| From source | `go install github.com/itsgiahung/502Drive/cmd/502drive@latest` (Go 1.27), or `make build` |

Start with the system: `502drive autostart enable` (XDG autostart on Linux, the
Run key on Windows). Check for a new release with `502drive update` (it never
installs anything).

## Everyday use

| I want to… | Command |
| --- | --- |
| Open everything | `502drive` |
| Copy a link (bare form) and wait | `502d LINK --wait` |
| Copy several links as one batch | `502d copy LINK1 LINK2 ...` |
| Preview without writing | `502drive inspect LINK` (add `--deep` for a background full scan) |
| Follow a job | `502drive wait JOB_ID` · `502drive watch JOB_ID` |
| See jobs / pause / resume / cancel / retry | `502drive jobs list`, `jobs pause ID`, … |
| Move a queued job | `502drive jobs move ID --before ID` / `--after ID` / `--priority N` |
| Report failed/blocked/skipped items | `502drive jobs report JOB_ID --format csv` |
| Export job history | `502drive history export --since 30d --format csv` |
| Pause or resume the whole queue | `502drive pause` · `502drive resume` |
| Keep a copy in sync | `502drive sync enable COLLECTION --daily 22:00` · `sync run` · `sync pause` · `sync stop` |
| Audit a copy; repair after review | `502drive audit COLLECTION` · `502drive audit COLLECTION --repair --apply` |
| Retire / resume / forget / trash a copy | `502drive collections retire COLLECTION` · `resume` · `forget --yes` · `cleanup --dry-run=false --yes --confirm-name NAME` |
| Check health (and fix local issues) | `502drive doctor [--online] [--fix]` |
| Read the daemon log | `502drive logs -f` |
| Start/stop in the background | `502drive up` · `502drive stop` |
| Script it | add `--json`, use the documented [exit codes](docs/features/cli.md) |

### In the TUI

`1`–`7` switch screens, `↑↓` move, `enter` opens. **Space** selects rows, **a**
selects all; **p** pause, **u** resume, **c** cancel (asks first), **t** retry,
**n** run now, **x** stop sync apply to the selection. **`7`** opens the Drive
browser: `enter` goes into a folder, `←` goes up, `i` shows metadata, `n` makes a
folder, `d` sets the folder as the default destination and `c` copies the item
(the copy box: `1` keep the name, `2` auto-name it `<name> (copy date)`, `3`
type one, `d` pick another destination). `?` shows help, `q` leaves
the TUI (the daemon keeps running). The cat reacts: it carries files while
copying, hops when a job finishes, turns amber when you need to act and sleeps
when idle. `--no-motion` makes it still; `NO_COLOR` or `--plain` removes it.

### Telegram (optional)

```bash
# put TELEGRAM_BOT_TOKEN and TELEGRAM_OWNER_ID in .env (see above), then:
502drive setup
```

The bot is private-chat only, allow-listed, and uses the same service and
permissions as the HTTP API. Details: [Telegram](docs/features/telegram.md).

## Privacy and security

- The API listens on **loopback only** with a bearer token stored in a private
  file; use SSH forwarding for remote access. The OAuth token is encrypted at rest.
- 502Drive talks to Google (Drive API) and, only if you configure it, Telegram.
  The update check contacts GitHub **only** when you run `502drive update` or opt
  in for the tray. No telemetry.
- It never changes sharing, never makes links public and never bypasses quota.
- Read [PRIVACY.md](PRIVACY.md), [SECURITY.md](SECURITY.md) and the
  [threat model](docs/threat-model.md).

## How it fits together

```
 CLI / TUI ─┐
 Tray ──────┼── HTTP /v1 (+ SSE stream) ──▶ daemon: state (SQLite) · engine · scheduler ──▶ Google Drive
 Telegram ──┘                                            ▲ one service, one writer
```

One daemon owns the database, the workers and the token; every interface is a
client of the same service. Read the [architecture](docs/architecture.md), the
[HTTP contract](docs/control-contracts.md) and the [OpenAPI file](docs/contracts/openapi.json).

## Documentation

Start at the [documentation map](docs/README.md). What is verified, and what is
not, is written down in [verification](docs/verification.md) and
[acceptance](docs/acceptance.md).

## Develop

```bash
make check            # gofmt, go vet, go test -race
make docs             # links, OpenAPI JSON, whitespace
make release-local    # full local release build and verification, publishes nothing
```

See [CONTRIBUTING](CONTRIBUTING.md). CI is optional: every check runs locally.

## What is tested

| Area | Local tests (fake Drive/Telegram) | Real Google/Telegram |
| --- | --- | --- |
| Copy, admission, retry, reconcile, K1/N1/S5/D1 | yes (`go test -race ./...`) | no |
| Batch, fair queue, pause, job move, deep preflight | yes | no |
| Sync schedule, realtime replay, full reconcile, S8, quota ledger | yes | no |
| Audit/repair, lifecycle retire/forget/cleanup (trash only) | yes | no |
| CLI/TUI/report/history JSON and exit codes | yes | — |
| HTTP contract (all routes) and OpenAPI conformance | yes | — |
| Tray, autostart, systemd unit, sleep/resume | logic/tests only | desktop and OS not exercised |
| rpm/deb/zip/installer, Docker image | build + local smoke | install on a real desktop not exercised |

Full command list and results: [verification](docs/verification.md). Fake-provider
tests do not prove live Drive behaviour.

## Limits worth knowing

- "Identical copy" means names, tree, content and type within what Google allows.
  IDs, owners, sharing and revision history are not preserved.
- Not yet accepted against live Google Drive; the tray and TUI are tested without
  a real desktop. See [verification](docs/verification.md).
- The tray needs Linux (StatusNotifier/AppIndicator) or Windows; macOS has the CLI
  and daemon only. Desktop notifications work on Linux (`notify-send`).

## Disclaimer

502Drive is an independent project, not affiliated with or endorsed by Google.
"Google Drive" is a trademark of Google LLC.

Licensed under GPL-3.0-only, see [LICENSE](LICENSE) and
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
