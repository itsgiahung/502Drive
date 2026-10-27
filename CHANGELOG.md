# Changelog

All notable changes to 502Drive. Versions follow [Semantic Versioning](https://semver.org/);
before 1.0 a minor version may change behaviour, and every such change is listed under
**Upgrade notes**.

## Unreleased

### Added

- Per-collection opt-in mirror and automatic snapshots, manual capture, retention,
  pinning and durable preview-first restore through the service, HTTP API and CLI.
  Both automatic options default off. Restore apply pauses sync and never deletes
  Drive objects permanently. Schema 027 is forward-only; back up before upgrade.
- Reusable Windows Inno Setup build/checksum and isolated install/uninstall smoke
  scripts; an optional desktop shortcut. Native installer acceptance is pending.

### Changed

- Collection detail arrows select rows in the open collection; Sync opens a
  keyboard-selectable action menu. Job details scroll rather than select another job.
- The terminal cat sleeps curled up, breathing, with a small "z" and a "Z" drawn as
  real letters; idle moments now include playing with a ball of yarn and watching
  a butterfly. Fixed a white square while it washed its cheek and a thin "neck"
  when it stretched.
- The filter editor uses a switch for "keep empty folders", takes sizes like
  `10MB` and dates like `2026-01-31`, and explains mistakes in words instead of
  showing a parse error. Idle toys stay about seven seconds; eyes stay slim.
- `502drive onboard` prints a readable checklist in a terminal (JSON stays for
  scripts and `--json`), and Enter at the destination step means My Drive.
- The README explains the project in plain words, with a four-step quick start.
- Left returns from nested views. Live refresh and returning from reports/filter
  editors preserve object identity instead of reusing a stale list position.
- Public Markdown is English with one Vietnamese introduction, `README_VI.md`.
  Documentation now separates user guides, technical contracts and verification;
  private handoffs and local collaboration settings are ignored.

## 0.3.0 — 2026-10-28

The first release of the Go rewrite: one daemon (copy engine, sync, database and
tokens) with the CLI, a full-screen TUI, a system-tray launcher and a Telegram bot as
its clients. Copies stay **server-side**: 502Drive asks Google Drive to copy, it never
downloads or uploads file contents.

### Highlights

- **Copy a folder or file by link** with its tree, empty folders, duplicate names
  (`name (2)`), shortcuts and Google Docs. Uncopyable items are skipped with a stable
  reason instead of failing the job.
- **Crash-safe by design.** Every Drive write is recorded with an intent before it is
  sent; after a crash, a lost response or a sleep, the daemon looks the write up instead
  of sending it again. A job blocked only by an unknown outcome is re-checked once on
  its own after two minutes.
- **Keep copies in sync** on a schedule or in near real time from the Drive change feed,
  with replay during the first copy, periodic full reconciliation and a guard that stops
  and audits when a large part of the source disappears.
- **Audit and repair**: compare a source with its copy, then fix what is safe. Repair
  and cleanup are dry runs by default and only ever move objects 502Drive created to the
  Drive trash. Nothing is deleted permanently.
- **Batch and queue**: several links in one command, a fair queue with priorities and
  real queue positions, and a durable global pause.
- **Clients**: TUI with first-run checklist, audit browser, filter editor and multi-state
  job filter; tray with status, job shortcuts and desktop notices (Linux); Telegram with
  paste-to-copy, live progress cards, batch summaries and `/find`.

### Added

- Collections: retire, resume, forget and trash-only cleanup (L1); copy again as
  `--update` or `--new` (L2); duplicate-name policy `rename|skip|update` (N1);
  selective filters by path, name, size, type and modification time (S5).
- Shared Drives as source and destination (D1).
- Deep preflight with sizes, Google-native counts and cannot-copy items (S7).
- Adaptive throttling for the whole account (AIMD workers, read token bucket,
  `Retry-After` in seconds or as a date) and a 24-hour quota ledger with automatic resume
  after daily limits (S3, Q1). Batched folder listing, up to 40 parents per call (F1).
- `502drive onboard` and a `/v1/setup` checklist; `doctor --online`, `--fix`.
- Job reports and history export as CSV or JSON; retention (`prune`), `vacuum`, backup
  and restore; systemd user unit and autostart on Linux and Windows.
- Vietnamese and English throughout; quiet hours for notifications.

### Fixed in the first live Google Drive session

Found by running against real Drive and Telegram (see `docs/verification.md`):

- An unchanged Google Doc was copied again on every refresh, because Drive bumps a
  file's `version` for changes nobody made. Files are now compared by md5 (binary) or
  `modifiedTime` (Google files); stored fingerprints are upgraded without re-copying.
- A repair with nothing to fix moved the whole version history to the trash under
  `versioned_copy`. It now trashes only what its own re-copies replaced.
- Audit reported a duplicate renamed to `name (2)` as drift (and a repair would have
  renamed it back into a duplicate); every later `audit` replayed the first report.
- Telegram stayed silent when a blocked job later succeeded after a retry.
- A pure rename of a file now renames its copy instead of copying the same bytes again.
- `cleanup` now confirms the folder name shown in Drive (for example `Photos (2)`), and a
  dry run prints what it would move and the exact command to run it.

Also fixed after an independent code review: ownership is checked before every rename,
move or trash; a write whose lookup failed is never re-sent; repair reconciles first and
does not lose an in-flight intent; a terminal job and its sync bookkeeping are committed
together (migration 026) and replayed after a crash; audits read every page of a folder;
an incomplete Drive search is not taken as a complete listing; the Telegram bot token
never reaches the logs; several races in the queue, tray and checkpoint code.

### Upgrade notes

- **Config folder.** New installs use `~/.config/502drive` (Windows
  `%AppData%\502drive`). An existing setup in `502drive-next` keeps working unchanged;
  `doctor` notes the old name. Do not rename that folder by hand: its `config.json`
  holds absolute paths into it.
- **Database.** Migrations run forward only, up to schema 26. Take a backup first:
  `502drive stop && 502drive maintenance backup PATH`.
- **CLI.** `audit --wait` is now a switch, like `copy --wait`; the time limit moved to
  `--wait-timeout` (the old `--wait 5m` form exits with code 2 and says so).
  JSON output and exit codes are otherwise unchanged; new fields are additive.

### Known limitations

- Verified live on Linux (Fedora, GNOME) with a personal Google account and Telegram.
  Not yet run on real hardware: Windows and macOS, the Windows installer, Shared Drives,
  very large trees (throttling and daily quota), desktop notification clicks, and
  sleep or network loss on a laptop.
- Renaming a Google Doc creates a new generation: Drive moves its `modifiedTime` on a
  rename, so it cannot be told apart from an edit.
- Desktop notices are Linux only; the Windows tray has no per-item menu icons.
- Telegram batch summaries are sent at most once; when a send's outcome is unknown the
  summary is dropped rather than risk a duplicate.
