# K1 — Stable skip reasons and shortcut policy

Uncopyable children are skipped with a stable code rather than failing the whole
job. Counts aggregate `skipped_by_code`; unknown codes remain visible.

| Code | Meaning |
| --- | --- |
| `shortcut_unresolved` | Missing, inaccessible or intentionally skipped target |
| `shortcut_loop` | Self/cyclic shortcut target |
| `cannot_copy` | Source copy capability or explicit copy restriction |
| `google_native_export_only` | Native type without supported server-side copy |
| `too_large_for_quota` | Explicit synthetic/provider reason only; not a verified Google reason |
| `unreadable` | Definitive unreadable source |
| `trashed_source` | Source is trashed |
| `filtered` | Selective-copy exclusion |
| `conflict` | Conflict-policy skip |

`--shortcuts follow|skip` defaults to follow. Resolve only bounded targets and
check loops, missing IDs, trash and access. A destination shortcut references the
resolved target; it does not copy arbitrary bytes or grant target permissions.

Only definitive target errors become unresolved skips. Network, auth, throttle
and server failures remain retry/reconnect work rather than permanent skips.
Failed skip persistence releases work for recovery. Daily/storage quota failures
block through Q1 instead of masquerading as a skipped large file.

See [copy jobs](../features/clone-jobs.md) and
[state constants](../../internal/state/model.go).
