# Protection options and installer decisions

Mirror and automatic snapshots are explicit per-collection opt-ins, both off by
default. Existing configurations retain behavior. Service/HTTP/CLI expose options
with revision/idempotency checks and one active collection writer.

Migration 027 adds immutable capture mappings, durable destination actions and
archive-create intent. Mirror runs only after complete successful scan/copy,
respects the mass-removal guard and touches exactly owned mapped objects.
Retained snapshot versions move into a sibling archive; other eligible removals
only go to trash. Nonempty folders block rather than carrying user additions away.

Capture freezes mappings in SQL and checkpoints physical content identity by page.
Ready and successful job state commit together. Retention prunes metadata only;
pins are exempt. External changes can invalidate a point.

Restore is preview-first. Apply atomically pauses the subscription, validates all
targets, restores parents first and removes owned extras children first. Restart
reads remote state before repeating an action. Restore never rewinds generation
or reuses completed creation identity. It leaves sync paused for explicit resume.

Windows packaging reuses Inno Setup. PATH/startup/desktop tasks are optional and
an isolated install/version/uninstall smoke test is wired into CI. Native Windows
execution is separate from PowerShell syntax checks and Go cross-builds.

See [protection](../features/protection.md), [Windows guide](../install-windows.md),
[acceptance](../acceptance.md). No provider permission/quota bypass, new byte-relay
engine or automatic permanent deletion is introduced.
