# Mirror, snapshots and restore

English · [Tiếng Việt](protection_VI.md)

**Mirror and automatic snapshots are off by default.** Configure each collection
explicitly from the CLI, the collection detail screen of the TUI or the Telegram
collection card.

## Configure

After the initial copy, obtain a collection ID from `collections list`:

```sh
502drive sync options col_example
502drive sync options col_example --snapshots=true --keep-days 30 --keep-count 10
502drive snapshots create col_example
502drive sync options col_example --mirror=true
```

Option changes require no active collection job, including paused/blocked jobs.
Unspecified CLI flags preserve current values; `--dry-run` only previews settings.
Mirror applies to subsequent manual, scheduled and realtime runs. Manual capture
works even when automatic capture is disabled.

## Desktop and chat controls

The collection detail screen shows a **Protection** row (below Filter/Integrity)
and the Telegram collection card has a **Protection** button. Both offer the same
read-only menu: mirror on/off, automatic snapshots on/off, retention days (7, 30
or 90) and unpinned points (5, 10 or 20) picked from fixed lists, capture now,
the snapshot list with pin/unpin, and restore. No retention value is typed.

Enabling mirror always asks first and names the consequence: source deletions
move the copies to the Drive trash. Enabling it in chat is a separate
confirmation card. Restore is always admitted as a **dry run** first; the screen
or card shows the action count from that job and only then offers apply, which
has its own confirmation saying that sync pauses. TUI/TG control only calls the
existing HTTP/`app` service; TUI writes go through `PUT
/v1/collections/{id}/protection` with the revision the screen showed. The desktop
screen lists the newest 100 points and chat the newest 8; use the CLI to page
through older ones.

## Mirror behavior

Drive answers "not found" both for a deleted file and for one you can no longer
see. If the owner of a shared source removes your access to some files, mirror
treats them as deleted and moves their copies to the trash (recoverable for about
30 days). The mass-removal guard still stops a run when a large share vanishes.

- Act only on mapped source IDs after a complete source scan and successful copy.
  Listing/copy/reconciliation failures and the mass-removal guard prevent mirror.
- Trash missing-source copies only with the exact ownership intent/generation.
  No operation permanently deletes Drive files.
- Process children before parents; only move/trash empty folders. Destination
  additions are preserved, and a nonempty folder can block the job.
- Start scans at the source root. Detached folders are no longer reachable source
  content. Filter exclusions are not deletion requests; their existing copies stay.
- Move snapshot-referenced copies into the sibling archive
  `.502drive-versions-<collection-id>` instead of trashing them. Archive creation
  records intent first; a lost response is reconciled, never blindly re-created.
- A file whose byte-identical twin (same folder and name, skipped as a duplicate) is
  still at the source keeps its copy; mirror waits until both are gone.
- A returning source uses a new generation, rather than reusing a finished intent.

## Capture and restore

```sh
502drive snapshots list col_example
502drive snapshots show snap_example
502drive snapshots pin snap_example
502drive snapshots unpin snap_example
502drive snapshots restore snap_example                 # preview plan
502drive snapshots plan job_example                     # inspect that plan
502drive snapshots restore snap_example --apply         # apply; pauses sync
502drive sync resume col_example                        # follow the source again
```

List/plan pages contain up to 100 rows; pass `--cursor` for the next page. Snapshot
mappings are paginated through `GET /v1/snapshots/{id}/items`. Create/restore wait
by default; `--wait=false` returns after admission. `--wait-timeout` stops waiting
without cancelling the job.

A snapshot stores source-to-**physical destination ID** mappings, intent,
name/parent, generation and content identity. It is not an independent byte backup
and does not preserve Google revisions, owners or permissions. Binary files use
MD5 when available; modified time/version provide conservative fallback checks.
Missing or edited content blocks restore. Permanently deleted files cannot be recovered.

Automatic capture runs after successful copy/mirror while holding the same
collection writer. SQL freezes mappings; paged metadata reads checkpoint progress.
Snapshot readiness and job success commit together. Restart resumes capture;
pause/cancel/retry use the normal job controls.

Restore validates all target copies before writes, restores parents before children,
then removes owned extras outside the selected restore point. Each action has a
durable pending/executing/done checkpoint. After an uncertain result, continuation
reads current remote state. Trashed targets can be untrashed if still accessible.

Apply pauses the subscription **in the admission transaction**. Sync resume or
reconfiguration is refused while restore is active, and the subscription stays
paused afterwards. Resuming later reapplies the source's current changes. Preview
neither writes Google nor changes the subscription.

## Retention and storage

Keep 30 days / 10 unpinned points by default; pinned points are outside both limits.
Metadata pruning does not delete Drive files. `replace_copy` and repair preserve
referenced versions; root cleanup is refused when a snapshot protects the root.
`forget` removes collection/snapshot metadata while retaining Drive files.

Archives may keep consuming storage after points expire. Automatic physical
archive garbage collection is not implemented. External edits/deletes can make a
point unavailable even when its metadata remains.

## Evidence

See protection tests under [engine](../../internal/engine/protection_test.go),
[state](../../internal/state/protection_test.go),
[HTTP](../../internal/httpapi/protection_test.go),
[CLI](../../internal/cli/protection_test.go) and
[Drive](../../internal/drive/restore_test.go). Local fixtures do not replace live
Google/Shared Drive acceptance. Existing collections are not enabled implicitly.
