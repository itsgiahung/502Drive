# Operations runbook

Use one daemon per data directory. Offline maintenance refuses an active writer;
do not delete a lock or database to bypass that check.

## Bootstrap and health

```sh
502drive setup --oauth-client /path/to/client_secret.json
502drive up
502drive account login
502drive destinations add FOLDER_LINK
502drive destinations default DESTINATION_ID
502drive doctor --online
```

Paths in private configuration resolve relative to its directory. Supply secrets
through private files/setup, never public examples or logs.

| Check | Command |
| --- | --- |
| Readiness, lifecycle and queue | `502drive status` |
| Local checks / provider checks | `502drive doctor` / `doctor --online` |
| Account | `502drive account status` |
| Blocked work | `502drive jobs list --state blocked` |
| Subscription | `502drive sync status COLLECTION` |
| Logs | `502drive logs -f` or service/container logs |

A failed optional bot does not make the core daemon unready. For readiness errors,
check configuration/token permissions, an existing writer and binary/schema version.
`doctor --fix` repairs safe local directory/permission problems; it does not change
Drive files or repair invalid Google credentials.

## Back up and restore local state

Stop the daemon or service first:

```sh
502drive stop
502drive backup /backups/502drive-new
502drive restore /backups/502drive-new --to /srv/502drive-restored
```

Backup creates a new private directory with a consistent database copy,
configuration, available encrypted token and master key, and a hash/schema manifest.
**The backup contains secrets.** Store an off-machine copy securely.

Restore requires a new/empty directory. It validates hashes, integrity, foreign
keys, supported schema and token/key compatibility, and rewrites the restored data
path. A failed restore does not leave a partial target. Test a backup before relying
on it; do not overwrite a live data directory.

These commands restore local control state. They do **not** undo files created or
changes made on Google Drive after the backup. After rollback, suspend schedules,
reconcile operation identities and resume only when remote state is understood.

## Retention, temporary files and compaction

```sh
502drive prune --older-than 90d --dry-run
502drive cleanup
502drive cleanup --apply
502drive vacuum
```

Inspect each command's help before applying. **Prune applies unless `--dry-run` is
supplied**; always preview first. It keeps
collections, mappings and referenced jobs. It removes eligible old terminal history,
events, expired plans and notification state. Offline cleanup only removes eligible
old app-created temporary files; it never follows symlinks or removes database/WAL,
master keys or mappings. Vacuum checkpoints and compacts the database offline.

The daemon also prunes history on its own, at most once a day: it uses the
owner's **retention days** setting (Settings in the app; 90 by default, 0 keeps
everything) and records the last run in the database, so restarts neither skip
nor repeat it. Keep verified backups.
Let the service/container runtime rotate logs. Snapshot metadata retention
and physical Drive archive storage are separate; see [protection](features/protection.md).

## Queue and sync controls

Global `pause` persists `settings.queue_paused`; workers reach a safe point before
pausing. Global `resume` retains jobs paused individually. Priorities/moves use the
API/CLI, not direct database edits.

Subscription pause stops future triggers, not an already admitted run. Stop cancels
automatic work cooperatively while keeping output. Restore apply pauses its
subscription atomically and leaves it paused afterwards. Quiet hours delay unsolicited
Telegram/desktop notifications; direct replies can still be sent.

## Linux user service

```sh
502drive autostart systemd install
systemctl --user start 502drive
systemctl --user status 502drive
journalctl --user -u 502drive -f
502drive autostart systemd remove
```

The user unit uses bounded restart-on-failure. Desktop autostart is an alternative,
not a second required mechanism. A custom data path may need a writable-path drop-in.

## Upgrade and rollback

1. Stop the daemon and create a verified backup.
2. Replace the binary/image with the intended version.
3. Start it; forward-only migrations apply on first database open.
4. Check status, Doctor, jobs and subscriptions.

Never edit an applied migration. An older binary refuses a newer schema. Roll back
by restoring the **matching full backup** into a fresh directory; do not copy an old
DB over a newer running one. `serve --maintenance` permits reads while refusing
new operational work. A local-state rollback is not a remote undo.

## Diagnose stalls

429/rate-limit responses wait with shared throttling and Retry-After; quota jobs
expose a resume time. Network loss waits without spending attempts. A full queue
keeps schedule slots due. Inspect events and Doctor before retrying; a new
idempotency key is not a safe way around an uncertain request.

## Build and release checks

```sh
make check
make docs
scripts/release-local.sh
```

Local release builds publish nothing. Optional `--tag` builds an exact clean tag;
`--rpm-smoke` exercises packages in a container. Windows installer compilation needs
a Windows Inno Setup host. Workflow dispatch is a snapshot/dry run; pushing a `v*`
tag enters the separate publishing path. Do not push/release without explicit intent.

## Native laptop checks

Use isolated test folders and record OS/version, lifecycle transitions, job results
and output identity. Keep private logs out of Git.

- Suspend during copy; wake and verify continuation without duplicate copies.
- Sleep past schedule slots; expect one coalesced catch-up run per collection.
- Disconnect/reconnect Wi-Fi or VPN; attempts must not burn while offline.
- Restart during an in-flight write; reconcile before another write.
- Check keep-awake inhibitor/request state before and after jobs.
- Check tray state after OS/Explorer/session restart and after daemon restart.

These require actual OS/provider acceptance. Unit tests alone do not establish them.

## Live provider tests

The harness requires build tag `live` and explicit `DRIVE502_LIVE=1`; it performs
real provider writes in an isolated app-created test folder and trashes that test
root afterwards. It can inject network faults around real HTTP. Use only an account
and destination authorized for testing. Credentials belong in the private mechanism
supported by [internal/live](../internal/live), never in a shell history example,
report or repository. Do not run live tests as part of ordinary local verification.

See [verification](verification.md), [acceptance](acceptance.md) and
[troubleshooting](troubleshooting.md).
