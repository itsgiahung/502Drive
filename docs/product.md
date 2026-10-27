# Product scope

502Drive creates server-side Google Drive copies and keeps collections in sync.
One daemon owns workers, SQLite and credentials; every interface uses the same
application service. Google permissions and quotas always apply.

## Interfaces

- CLI: full operational commands, stable JSON and exit codes for scripts.
- TUI: terminal navigation, copy previews, jobs, collections, filters and audits.
- Tray: status and quick actions on Linux/Windows; no second engine.
- Telegram: optional private-chat commands, cards and notifications.

## Implemented capabilities

| Capability | Reference |
| --- | --- |
| Copy, preview, retry, unknown-write reconciliation, conflicts and filters | [Copy jobs](features/clone-jobs.md) |
| Batch admission, fair queue, durable global pause and priorities | [CLI](features/cli.md) |
| Scheduled/realtime synchronization, replay, quota cooldown | [Automation](features/automation.md) |
| Audit/repair and collection retire/resume/forget/cleanup | [Copy jobs](features/clone-jobs.md) |
| Opt-in mirror, snapshots, retention/pin and preview-first restore | [Protection](features/protection.md) |
| Tray, TUI, autostart, systemd and power/network recovery | [Desktop](features/desktop.md) |
| Telegram allowlist, callbacks, notifications and quiet hours | [Telegram](features/telegram.md) |
| Backup/restore, prune, vacuum and doctor | [Runbook](runbooks.md) |

## Non-goals

No file-byte relay, quota bypass, automatic public links, bidirectional sync,
owner/permission/revision cloning, multi-account engine, hosted multi-tenant
service, billing, public HTTP service or web GUI. Source removals preserve copies
by default; mirror must be enabled explicitly.

## Current limits

- Desktop notifications use Linux `notify-send`; Windows/macOS support is incomplete.
- Telegram offers schedule presets rather than an arbitrary schedule editor.
- HTTP bearer authentication represents the owner; operators use Telegram.
- No automatic software installation/update, Windows signing or Drive webhooks.
- Protection options are exposed through service/HTTP/CLI; dedicated TUI/Telegram
  controls are not yet implemented.
- Native Windows/installer, Shared Drive and large-tree acceptance remain incomplete.

See [verification](verification.md) for recorded live evidence and
[acceptance](acceptance.md) for pending checks. Local fixtures do not prove live
provider behavior.
