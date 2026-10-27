# N1 — Destination name conflicts

Drive permits duplicate names. 502Drive reserves a chosen `dest_name` and
`conflict_action` in one SQLite transaction before writing. Migration 010 adds
these fields and the parent/name lookup index. The saved decision survives restart.

## Decisions

1. An available name stays unchanged.
2. A matching file with known equal size/MD5 can be skipped as `conflict`; never
   adopt a foreign object as owned, since later replacement must not trash it.
3. Otherwise `rename` adds a numeric suffix, `skip` records conflict, and `update`
   creates the same name while retaining the previous object.
4. Folders/wrappers rename instead of skipping a whole subtree.

Suffixes preserve ordinary/compound extensions. Root candidates are checked on
Drive because the parent is user-selected. Names inside app-created folders use
local reservations to avoid per-item list requests. Concurrent workers serialize
reservation transactions, not provider names.

## Limits

`update` does not delete old objects and is distinct from `replace_copy`.
Align operations do not perform a complete fresh conflict decision. Destination
additions made outside the application are not comprehensively discovered by the
local child-name reservation path. Ownership remains required for later mutations.

See [copy behavior](../features/clone-jobs.md) and
[migration 010](../../internal/state/migrations/010_conflict_policy.sql).
