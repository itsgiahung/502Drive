# Verification evidence

Updated 2026-10-28. Local test coverage, recorded live smoke tests and outstanding
acceptance are separate. This page summarizes reproducible evidence; private logs,
worker handoffs, real object IDs and session transcripts are not public documentation.

## Current local checks

Use Go 1.27 and the repository's existing tools:

```sh
gofmt -l cmd internal
go vet ./...
staticcheck ./...
go test -race -count=1 -timeout 900s ./...
python3 scripts/check-docs-links.py .
python3 -c "import json; json.load(open('docs/contracts/openapi.json'))"
git diff --check
```

The protection implementation passed formatting, vet, staticcheck and the full
race suite on Linux. Protection failure-window tests also passed repeated race
runs. CGO-disabled Windows amd64 CLI/tray and Linux CLI cross-builds passed;
PowerShell AST parsing accepted the installer scripts. These do not establish
native installer execution or live protection behavior.

Navigation regression tests reproduce and cover collection/job retargeting,
collection refresh reordering, missing targets, Sync menu selection, nested Left
navigation and focused rows in short terminals. Their focused race suite passed
three repeated runs. The final full-repository result for the documentation/navigation
change is recorded after that check completes.

Normal `go test ./...` includes the package named `internal/live` but does not
activate the build-tagged live scenarios. No provider credentials are required
for ordinary local checks.

## Earlier live smoke record — 2026-10-28

The existing project record describes a Fedora/GNOME development daemon, one
personal account, a small source shared by a different account with viewer access,
and isolated test destinations. Those operations were not rerun for the current
navigation/documentation change.

| Scenario | Recorded outcome |
| --- | --- |
| Online Doctor and readiness checklist | Successful |
| Inspect/deep inspect and small folder copy | Names/tree, native document and empty folders preserved; duplicate names resolved |
| No-op update/sync | Native fingerprint regression found and fixed; unchanged content no longer recopied |
| Source additions, moves, folder changes and removal | Delta behavior exercised; default removal preserved destination |
| Audit and repair | Name-conflict/replayed-audit/history-cleanup regressions found and fixed |
| Realtime on shared source | A new item reached the destination after polling delay |
| Pure binary rename | Existing destination physical ID renamed in place |
| Forced daemon termination during copy | Reconciliation/delayed recheck completed several small runs without duplicate copies |
| Cleanup | Preview/name confirmation and trash-only application exercised |
| Telegram start/status, paste-to-copy and progress/final cards | Successful small flow; retry-final-notification regression found and fixed |

This is a smoke record, not exhaustive acceptance. It does not cover the newly
the mirror/snapshot paths (recorded separately below), every file type, operator
action or Google error reason.

## Live mirror, snapshot and restore — 2026-10-28

Daemon `e3e7ee4`, same personal account; source shared by another account with
viewer access (`keep.txt`, `gone.txt`, `sub/nested-gone.txt`); isolated destination.

| Scenario | Recorded outcome |
| --- | --- |
| Enable snapshots, capture one | Ready snapshot with 5 items |
| Enable mirror; delete `gone.txt` and `sub/` at the source; sync | Both left the copy; snapshot-referenced copies moved to `.502drive-versions-<collection>` (children before the emptied folder); a new source file was copied |
| Restore preview | Plan of 5 restores and 1 removal; nothing changed |
| Restore apply | 6/6; tree restored exactly, nested file back in its folder; a newer file moved to the archive because a later automatic snapshot referenced it |
| Mirror of a file no snapshot references | Its copy went to the Drive trash |
| Cleanup with retained snapshots | Real run blocked `snapshot_protected`; the dry run had promised the trash move — fixed |

## Other recorded local evidence

- A Podman distroless-image/health/host-loopback API smoke was recorded on
  2026-10-27. Docker Compose was not exercised in that record.
- Fake 10,000-file tree/load probes measured bounded discovery and indexed state
  queries. Their timings are fixture/hardware-specific, not predicted Google speed.
- Query-plan tests check queue/fair-group, history and mapping hot paths; retained
  mappings/intents are not disposable merely because a job is old.
- Standard streams/JSON, locale, callback scope and fault injection have local
  regression coverage. See [acceptance](acceptance.md) for test locations.

## Not established

- Large/deep trees and Shared Drives (a personal account cannot create one).
- Actual daily quota reset, storage exhaustion or sustained rate-limit storms.
- Windows 10/11 runtime, Inno installer install/uninstall, SmartScreen and macOS runtime.
- Complete KDE/GNOME/session-restart and native notification-action behavior.
- Physical laptop suspend/hibernate/VPN changes during real provider writes.
- Complete mobile/operator Telegram, large document delivery and uncertain-send recovery.
- Long soak, disk-full/DB-busy recovery and a production-scale history database.
- Signed installers/images or a newly published release.

## Running additional acceptance

Use the [runbook](runbooks.md), [acceptance matrix](acceptance.md) and
[explicit live harness](design/s12-live.md). Live tests perform remote writes and
need separate authorization and isolated data. Never paste credentials, resource
keys, real database files or private reports into Git. State the exact command,
fixture/provider, platform and outcome before marking a gate complete.
