# Scheduled and realtime synchronization

A collection keeps its destination root and source-to-copy mappings across runs.
Synchronization is one-way: source to destination. Source removal preserves the
copy unless [mirror](protection.md) is explicitly enabled.

## Subscriptions and schedules

Each collection has at most one subscription. Modes are `scheduled` and `realtime`;
states include `waiting_for_baseline`, `bootstrapping`, `active`, `paused`,
`stopping`, `stopped`, `needs_reconcile` and `error`.

`PUT /v1/collections/{id}/sync` creates or updates the subscription atomically.
Realtime captures a start-page token before saving configuration. An incomplete
baseline leaves the subscription waiting until the initial copy succeeds.

| Schedule | Fields | Semantics |
| --- | --- | --- |
| Interval | `interval_minutes` 5–10080, UTC `anchor_at` | Elapsed UTC time |
| Daily | IANA `timezone`, `local_time` HH:MM | Local wall-clock time |
| Weekly | Daily fields plus ISO `weekdays` 1–7 | One slot per selected local date |

Nonexistent DST times are skipped; repeated local times produce one slot.
Missed slots coalesce into one catch-up run. `sync_occurrences` prevents duplicate
admission; a full queue keeps the slot due for a later tick. Defaults follow the
user's timezone preference. `sync enable --dry-run` previews three future runs.

```sh
502drive sync enable COLLECTION --daily 22:00 --timezone UTC
502drive sync enable COLLECTION --every 30m
502drive sync enable COLLECTION --mode realtime
502drive sync status COLLECTION
```

## Delta runs

A run reuses the collection/root. Reachability starts at the source root;
descendants are scanned only when encountered in the current tree. File content
changes create a new generation. Moves and supported pure renames use `align`.

Binary fingerprints prefer MD5; Google-native files prefer modified time, falling
back to version only when necessary. Google can change `version` without a content
edit, so it is not the primary identity. Legacy fingerprints are upgraded without
mass recopying. A binary rename with unchanged MD5 renames its copy in place;
Google-native renames can still look like edits when modified time also changes.

`versioned_copy` retains earlier copies. `replace_copy` trashes a superseded copy
only after the new copy is confirmed, and preserves snapshot-referenced versions.
Missing sources become `source_removed`. Filter changes affect future discovery,
not deletion of existing output.

The mass-removal guard stops **before copying** when more than 30% of at least ten
mapped items disappear. It produces `partial/source_mass_removal` and a read-only
audit. It does not automatically repair or remove destination content.

## Realtime polling and replay

Each subscription has its own Changes cursor. Polling adapts to activity
(10 / 60 / 300 seconds) and honors rate-limit cooldowns. Cursor advancement and run
admission commit together, so a cursor cannot pass changes without durable work.

During baseline/reconcile, raw changes are buffered while mappings are incomplete.
Replay re-evaluates relevance against completed mappings, advances the cursor and
admits work in one transaction. Limits are 5,000 changes or 4 MiB per collection;
overflow requests a full reconcile rather than replaying an incomplete set.
Restart resumes from the persisted page checkpoint.

Full reconciliation defaults to every seven days, with jitter. Configuration
`full_reconcile_every` accepts a Go duration; `0` / `off` disables it. A full queue
keeps its reconcile slot due. Expired cursors request a new token and full scan.
A successful run clears deduplicated subscription-error notification markers.
Full reconcile compares metadata/tree state, not every file's content digest.

## Pause, stop and run now

- `sync pause`: suspend future scheduling/polling; an admitted run is not cancelled.
- `sync stop`: disable the subscription and cooperatively cancel automatic work,
  including scheduled, realtime, coalesced and full-reconcile triggers. Keep files,
  root and history.
- `sync resume`: validate the collection, coalesce a missed schedule or catch up
  Changes; reconcile if the cursor is unusable.
- `sync run`: request a delta even with sync off. An active run is reused and a
  follow-up request coalesces through `rerun_requested`.

Job pause/cancel and global queue pause are separate controls. Restore apply
pauses sync and leaves it paused after completion; see [protection](protection.md).

## After-copy synchronization

`copy --sync off|realtime|hourly|six_hourly|daily` overrides the corresponding
preference for a **new collection**. The choice persists on the initial job and is
applied only after success. An existing user-configured subscription takes priority.
Subscription failures do not change a successful copy result.

Terminal job state, collection state and a bookkeeping marker commit together.
Startup replays unfinished bookkeeping before workers begin, preserving after-copy
configuration across a crash between terminal state and subscription setup.

## Throttle and quota

AIMD worker permits and the read token bucket are shared across jobs. A 429 or
rate-limit reason waits without spending item attempts. Retry delay honors the
larger of backoff/jitter and `Retry-After`. Provider 5xx, network failure,
permissions and quota exhaustion remain distinct error classes.

The per-principal hourly copy ledger records confirmed logical bytes in the same
transaction as item completion. It is a local rolling-24-hour total, not proof of
Google's actual account limit.

| Google reason | Behavior |
| --- | --- |
| `dailyLimitExceeded` | Block until next midnight in America/Los_Angeles |
| `storageQuotaExceeded` | Block for 24 hours; user may need to free storage |
| Other quota | Use Retry-After or a conservative fallback |

Quota-blocked jobs retain their IDs and operation identity. Due jobs with desired
state `run` are requeued; no item attempt is spent. `resume_after` is nullable in
job responses. See [quota design](../design/q1-quota.md).

## Limits and sources

No account-wide shared cursor, webhook/push delivery, bidirectional synchronization
or content-digest full reconcile. See [schedule](../../internal/schedule),
[state sync](../../internal/state/sync.go), [daemon syncer](../../internal/daemon/syncer.go)
and their tests. Local fixtures do not establish live quota/reset behavior.
