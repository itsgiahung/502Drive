# Copy jobs

502Drive asks Google Drive to copy server-side. It does not relay file bytes,
change sharing or bypass permissions. A **job** is a durable attempt; a
**collection** retains the root and mappings across later runs.

## Preview and admission

`inspect LINK` creates a plan from one HTTPS Drive/Docs reference or opaque ID.
Resource keys are preserved internally. Root shortcuts must be opened at their
target. Trashed sources, explicitly uncopyable files, unverifiable destinations
and destinations inside the source are rejected.

A shallow preview counts one children page. It is not a whole-tree total.
`inspect --deep` starts a background read-only scan; incomplete/cancelled/
interrupted scans keep totals unknown. Deep scans have a one-hour TTL and report
advisory warnings such as `estimate_unknown`, `destination_storage_short` and
`may_hit_daily_limit`.

Admission validates plan/principal/revisions and capacity, then commits collection,
job, seed item, first event and idempotency before returning `202 + Location`.
Plan TTL is ten minutes. Default admission caps are 100 total / 20 per principal.
Accepted does not mean succeeded. The HTTP request does not own the job lifetime.

## Execution and recovery

1. Discover folders breadth-first, checkpointing each listing page with its items.
2. Create folders, request server-side file copies and handle shortcuts according
   to policy. Preserve names/tree within provider limitations.
3. Derive final state from item states and commit result/bookkeeping durably.

Every write has a recorded intent before the provider call. Unknown results are
looked up and reconciled, never blindly resent. Retry and restart preserve operation
identity. An unresolved result blocks safely. A job blocked solely on an unknown
outcome receives one delayed automatic recheck; persistent uncertainty requires
user attention.

Job states are `queued`, `running`, `pausing`, `paused`, `cancelling`,
`reconciling`, `blocked`, `succeeded`, `partial`, `failed`, `cancelled`.
Pause/cancel are cooperative; cancelling keeps already-created output. Retry
revisits failed/blocked work under the same job identity. Total item count stays
null while discovery is incomplete; a result URL exists only after root creation.

## Copy fidelity

Names, hierarchy, empty folders and supported file types are preserved subject to
Drive's capabilities. IDs, ownership, sharing permissions and revision history are
not cloned. Google-native copy eligibility depends on the file type and account.

Shared Drive calls use the relevant all-drives support/listing flags. Writable
destinations require `canAddChildren`; restricted single files fail inspect,
while uncopyable children are skipped. This is locally tested, not complete live
Shared Drive acceptance.

`--shortcuts follow|skip` defaults to `follow`; unavailable targets and loops are
reported rather than traversed indefinitely. Shortcut traversal does not grant
permissions to its target.

## Name conflicts

Drive allows duplicate names. 502Drive reserves the destination name before writing
and keeps that decision across retries:

| Policy | Behavior |
| --- | --- |
| `rename` (default) | Use `name (2).ext`, then higher suffixes; preserve compound extensions |
| `skip` | Skip the conflicting item |
| `update` | Create the same name while retaining the old object |

A matching size/MD5 copy can be skipped when Drive supplies MD5. Folder conflicts
rename rather than dropping a whole subtree. Root conflicts are checked on Drive;
children under app-created folders use local name reservations. The actual root
name, including a suffix, is recorded for display and cleanup confirmation.

`update` does not delete the old object. It is different from the sync content
policy `replace_copy`. See [name policy](../design/n1-conflict-policy.md).

## Selective filters

Include/exclude rules support paths/names, `ext:` and `class:`, size bounds and
modified-after. Exclude wins. Filtering happens during folder discovery before
writes; excluded subtrees are not listed. Empty filtered folders are omitted
unless `keep_empty_folders` is enabled.

Filters persist from plan to job to collection. Changes affect later runs and
never request deletion of existing copies. A previously excluded item may be
copied when allowed later. **A source that is one file is not filtered.** Shallow
filter previews describe one top-level page, not the whole tree.
See [filter design](../design/s5-selective-sync.md).

## Stable skip reasons

`counts.skipped_by_code` summarizes `shortcut_unresolved`, `shortcut_loop`,
`cannot_copy`, `google_native_export_only`, `conflict`, `unreadable`,
`trashed_source` and `filtered`. The synthetic `too_large_for_quota` code is used
only if returned explicitly; it is not a verified Google reason. Real daily or
storage exhaustion blocks via quota handling rather than silently skipping files.

## Existing copies and lifecycle

For an existing source/destination pair, API `mode=update` performs a delta against
the latest collection; `mode=new` creates another collection. Missing mode returns
`already_copied`. CLI scripts default to update; a TTY can choose update/new/cancel.
Retired copies need resume before update. Idempotent admission replay keeps its
original decision.

- `retire`: stop future sync, cancel eligible queued/paused work and mark retired;
  no Drive writes. Running work returns `collection_busy`.
- `resume`: return the collection to ready.
- `forget`: only after retirement; remove local metadata/history, retain Drive files.
- `collections cleanup`: preview by default in the CLI; apply requires the actual
  root name and ownership tag. It only trashes the app-created output root.
  Snapshot-protected roots cannot be cleaned up.

Offline `cleanup` is a different command that removes local temporary files.

## Audit and repair

Audit is read-only and follows normal job controls. Findings include:

| Finding | Repair behavior |
| --- | --- |
| `missing_at_destination` | Recopy through the normal intent/reconcile path |
| `name_mismatch`, `wrong_parent` | Align only an owned destination object |
| `size_or_md5_mismatch` | Replace only an owned copy after confirmation |
| `duplicate_at_destination` | Trash an owned duplicate |
| `extra_at_destination` | Report only; never automatically remove unrelated objects |
| `source_removed`, `unreadable_source` | Report only |

`audit COLLECTION --repair` previews; `--apply` writes. Repair revalidates current
state, uses the same engine and excludes other collection writers. Superseded
copies are retired only after replacements are mapped, with snapshot references
respected. No permanent delete exists. See [integrity design](../design/s1-integrity.md).

## Batch, queue and rate limits

Multiple links create independent jobs with one batch ID and per-link idempotency.
One rejected link does not roll back the rest. Priority ranges from -5 to 5;
queue admission favors less-active groups before FIFO within equal priority.
`jobs move` is revision-guarded. Queue positions are estimates that can change
with admissions/completions.

Global pause is durable. Resume restarts jobs paused by the queue while retaining
user pauses. `max_active_jobs` controls job slots, not independent API worker pools;
all jobs share AIMD permits and the read token bucket. Rate limits wait without
spending item attempts. See [automation](automation.md), [protection](protection.md)
and [engine tests](../../internal/engine/engine_test.go).
