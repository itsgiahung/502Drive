# L2 — Copying an existing source again

Identify existing copies by `(principal, source_id, destination_id)`. A different
destination is a different pair; a requested display/root name does not change
this identity. If several copies exist, update selects the latest eligible one.

- Missing mode: return `already_copied` when a copy exists; otherwise admit normally.
- `update`: request a delta against the existing collection/root; create normally
  when none exists. A retired copy returns `collection_retired`.
- `new`: create a new collection even when the source/destination pair already exists.

Consumed-plan/idempotency replay returns its original job before a new duplicate
check. Scripts default to update; interactive CLI/TUI/Telegram can choose update,
new or cancel rather than silently duplicate output.

Lookup uses `collections_source_dest` from migration 020. Verify indexed lookup,
retired handling, no new root on update and stable admission replay.
See [copy jobs](../features/clone-jobs.md).
