# S9 — Collection snapshots

Snapshots are opt-in and reference retained physical copies, not Google revision
history. They do not make immutable independent byte backups. See
[protection behavior](../features/protection.md).

Migration **027** freezes immutable mappings, capture identity and durable action
plans. Older proposals using migration 009 were superseded; never modify an applied
migration. Capture checkpoints pages and commits readiness with job success.

Default metadata retention is 30 days / 10 unpinned points; pin exempts a point.
Retained physical IDs protect versions from replacement/repair. Mirror moves retained
versions to a sibling archive with a separate durable create intent. Unknown archive
creation results reconcile instead of triggering another create.

Restore defaults to preview. Apply pauses sync in admission, validates targets
before writes, restores parents before children and removes owned extras afterwards.
Actions checkpoint pending/executing/done and read current state after uncertainty.
Generations never rewind; align retains the actual physical intent. Only accessible
trashed targets can be untrashed; permanent deletion/external edits can make recovery
unavailable. Metadata pruning never automatically deletes archive files.

Service/HTTP/CLI are available; dedicated desktop/Telegram controls are pending.
See [data contracts](../data-contracts.md) and
[protection tests](../../internal/engine/protection_test.go).
