# R1 — Local history retention

Prune is offline and lock-checked. It removes eligible old terminal history,
events, processed notification state and expired unconsumed plans without removing
collections, active mappings or ownership intents.

A candidate job must not remain referenced by items, plans, sync occurrences,
audits, pending outbox/bookkeeping or snapshots. Delete dependents in a transaction
and preserve foreign-key integrity. A mapping's old job age is not permission to
delete its intent: audit and repair still need that ownership evidence.

```sh
502drive prune --older-than 90d --dry-run
```

**CLI prune applies when `--dry-run` is omitted.** Preview first. Reports retain
compatibility fields such as `intents_pruned`, which stays zero for live mappings.
Vacuum is a separate offline operation after pruning.

The daemon maintenance loop checks for daily pruning after 24 hours of process
uptime, on its ten-minute maintenance tick. It reads `Preferences("default")`,
not the authenticated user's preference slot; unset values use 90 days. This
namespace mismatch means a per-user retention setting is not a reliable global
prune control. Keep that limitation explicit until the settings wiring is aligned.
Snapshot metadata uses separate per-collection retention and never automatically
deletes physical archives.

See [state prune](../../internal/state/prune.go),
[maintenance](../../internal/maintenance) and [runbook](../runbooks.md).
