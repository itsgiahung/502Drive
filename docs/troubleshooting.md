# Troubleshooting

English · [Tiếng Việt](troubleshooting_VI.md)

Start with `502drive status`, `502drive doctor` and daemon logs. `doctor --online`
contacts Google; ordinary Doctor checks are local.

| Symptom | Check and recovery |
| --- | --- |
| `daemon_unavailable` | Start `502drive up`; check endpoint and private token-file configuration |
| `unauthenticated` | Use the daemon's current token file, not a copied obsolete token |
| `not_ready` | Check data-directory writer lock, permissions and binary/schema compatibility |
| `account_reconnect_required` | Run `account login`; restore consent if revoked. If it returns about every 7 days, the Google app is still in Testing: publish it (see [OAuth setup](oauth-setup.md)) and sign in again |
| `destination_required` | Add a folder, then choose its 502Drive destination ID |
| Destination inside source / unverifiable | Choose another destination; ancestry validation fails closed |
| `destination_not_writable` | Obtain add-child permission or choose a writable folder |
| `copy_not_permitted` | Respect source restrictions; no bypass exists |
| `capacity_exceeded` | Wait for capacity; do not submit duplicate jobs with new keys |
| Queue paused | `502drive resume`; individually paused jobs stay paused |
| `suspending` | Wait for wake/recovery; this is not a configuration error |
| `collection_busy` | Finish or cancel the active writer; paused/blocked work may still own the collection |
| Retired/not-retired conflict | Resume before updating; forget requires retirement |
| `already_copied` | Choose `copy --update` or `copy --new` explicitly |
| Interrupted deep plan | Create a new deep preview; partial discovery is not a total |
| Audit reports an extra | Inspect manually; unrelated destination additions are report-only |
| Repair did not write | CLI repair defaults to preview; inspect the plan before `--apply` |
| Quiet notifications | Check quiet hours, timezone and notification preferences |
| `plan_expired` / `plan_changed` | Inspect again; do not reuse a stale plan |
| `stale_revision` | Refresh and review current state before another action |
| `unknown_remote_outcome` | Let reconciliation run; inspect intent/events, never blindly submit another copy |
| Quota blocked | Read `resume_after`; daily and storage limits differ from rate limiting |
| Sync `needs_reconcile` | The syncer replaces an unusable cursor and requests a full scan |
| `waiting_for_baseline` | Complete/retry the initial copy before expecting sync |
| Bot error | Check private bot-token file and connectivity; core jobs remain independent |
| No Linux tray | Check desktop D-Bus and a StatusNotifier/AppIndicator host; use TUI headlessly |
| Offline maintenance refused | Stop the daemon/service/container; do not remove its lock |
| Doctor permission error | `doctor --fix` handles safe local fixes; Google credentials need reauthorization |
| Database growth or a long job list | `history clear --older-than 30d` works while the daemon runs; for deep compaction back up, preview `prune` offline, then `vacuum` |
| Restore target exists | Choose a fresh empty directory; maintenance restore does not overwrite |
| Backup integrity/key error | Use a verified complete backup; a missing key cannot decrypt its token |
| OAuth callback waits | Verify exact callback port/namespace and SSH/container forwarding |
| Mirror blocked | Check intent, source scan, mass-removal guard and nonempty destination folders |
| Snapshot unavailable | A saved physical copy may have been edited, removed or become inaccessible |
| Sync paused after restore | Expected; review the result and resume manually when ready to follow source changes |
| Detail arrow changes the wrong object on an old binary | Upgrade/rebuild; fixed navigation keeps detail identity and uses Left to return |

Never delete database/WAL/master-key files to unstick a job, remove a volume as a
cleanup shortcut, or change idempotency keys to bypass an unknown result. Check job
state/events, not merely process existence. See the [runbook](runbooks.md).
