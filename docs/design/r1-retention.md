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

The daemon's ten-minute maintenance tick prunes at most once per 24 hours. It
reads the owner's preferences (`prefs:owner`, what the app and Telegram save;
90 days when unset, 0 disables it), because history belongs to the whole daemon,
and stores the last run under `retention:last_run`, so a laptop that restarts
every day still prunes and never prunes twice in a day.
Snapshot metadata uses separate per-collection retention and never automatically
deletes physical archives.

See [state prune](../../internal/state/prune.go),
[maintenance](../../internal/maintenance) and [runbook](../runbooks.md).
