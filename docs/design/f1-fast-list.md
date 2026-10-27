# F1 — Batched folder listing

Discovery batches up to **40 parents** per `files.list` query to reduce requests
on trees with many small folders. `BatchLister` is optional; providers without it
keep the single-parent path. Batching changes reads, not parent-before-child creation.

## Query and checkpoint rules

Use a parent OR query with `trashed=false`, correct quote/backslash escaping,
page size 1000 and required mapping metadata. Resource-key headers combine the
parent ID/key pairs without exposing keys to presentation/logging.

A page token belongs to its whole query. Resume partial single-folder listings
individually. Drain a new batch before committing per-parent children; never
attach a batch token to one parent. Already-committed folders remain done after a
crash; others can be relisted with idempotent inserts.

Bound accumulation to 20,000 files per batch, then fall back to single-folder
listing. Batch errors can fall back to isolate an inaccessible parent. Retry
throttles with shared backoff/Retry-After without spending item attempts.
Pause/cancel returns cancellation, never a successful empty final page.

Workers still require a completed parent before child creation. Deep trees,
duplicate names, shortcuts, trash filtering and later pages retain the normal
mapping semantics. Delta reachability begins at the actual source root.

See [discovery](../../internal/engine), [Drive tests](../../internal/drive/client_test.go)
and the explicitly fake benchmark scope in [verification](../verification.md).
