# L1 — Collection lifecycle

Separate stopping tracking, deleting local records and trashing remote output.

| Operation | Preconditions and effect |
| --- | --- |
| Retire | Revision/scope checked; stop sync and eligible pending work; retain files, mappings and history |
| Resume | Return a retired collection to ready |
| Forget | Retired and no active writer; remove local collection/dependent metadata only |
| Cleanup | Retired, valid revision and owned root; durable trash-only job |

Running work returns `collection_busy`; these operations do not create a second
writer. Retired collections are hidden unless explicitly included and cannot be
scheduled or polled as active subscriptions.

CLI cleanup previews first. Apply requires explicit confirmation of the actual
root name shown in Drive, including a conflict suffix. Read current metadata and
verify ownership before trash. Missing/already-trashed roots are idempotent;
unknown write results are read again after restart. A snapshot-protected root is
refused. Forget removes protection metadata but retains physical Drive copies.

There is no permanent delete and no promise about the provider's trash retention.
The HTTP details/defaults are authoritative in [control contracts](../control-contracts.md);
older draft response examples must not be treated as current API schemas.

Migration: [020](../../internal/state/migrations/020_collection_lifecycle.sql).
Sources: [state lifecycle](../../internal/state/lifecycle.go),
[application lifecycle](../../internal/app/lifecycle.go),
[cleanup engine](../../internal/engine/cleanup.go).
