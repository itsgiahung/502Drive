# Acceptance and release gates

This matrix separates local automated coverage from live-provider/native-platform
acceptance. Test existence is not a claim that every platform or Google behavior
has been exercised. Current command results belong in [verification](verification.md).

## Core and control plane

| Area | Local evidence | Live/native boundary |
| --- | --- | --- |
| Durable admission, idempotency, plan expiry/revision, scope | [state](../internal/state), [app](../internal/app), [HTTP](../internal/httpapi) tests | Live admission/crash smoke recorded; not a full stress audit |
| Copy tree, names, empty folders, pagination and restart | [engine_test.go](../internal/engine/engine_test.go), [sync_test.go](../internal/engine/sync_test.go) | Small shared-source/personal-account smoke recorded |
| Unknown write result, intent lookup and retry identity | Engine crash-window/reconciliation tests | Several forced daemon crashes recorded without duplicate copies |
| Pure binary rename and native fingerprint handling | Engine sync tests | Small-tree rename/no-op sync smoke recorded |
| Conflicts, skips, filters and shortcut handling | Engine/filter tests; [K1](design/k1-skipped-classification.md), [N1](design/n1-conflict-policy.md) | All provider edge cases not established |
| Shared Drive flags/capabilities | Drive HTTP fixtures and fake Shared Drive roles | Real Shared Drive acceptance outstanding |
| Batched discovery and memory bounds | Engine fast-list/load tests, Drive query tests | Large/deep live trees outstanding |
| Fair queue, priority/moves and durable global pause | State queue tests and [queue-position tests](../internal/tui/queue_position_test.go) | Positions remain estimates; heavy live contention outstanding |
| Deep preflight and incomplete-count labeling | App/HTTP/CLI/deep-preview tests | Large-tree storage/quota estimates not established |
| Scheduled slots, timezone/DST and coalescing | [schedule tests](../internal/schedule/schedule_test.go), daemon syncer tests | Real long-running schedule/DST soak outstanding |
| Changes replay, cursor/admission atomicity and overflow | State replay and daemon syncer tests | Small realtime smoke recorded; overflow/token-expiry live cases outstanding |
| AIMD, read bucket, Retry-After and attempt preservation | [drive throttle tests](../internal/drive/throttle_test.go), engine throttle tests | Real rate-limit storms and daily/storage resets outstanding |
| Byte ledger and quota resume identity | State/engine quota tests | Ledger does not establish Google's actual cap |
| Audit/repair idempotency and ownership | Engine audit/repair tests and HTTP conformance | Small audit/repair smoke recorded; all permission cases outstanding |
| Collection lifecycle and trash-only cleanup | State/app/engine/CLI lifecycle tests | Small cleanup/name-confirmation smoke recorded |
| Terminal bookkeeping and after-copy sync | State/daemon after-copy/bookkeeping tests | Broad crash-window soak outstanding |
| HTTP authorization, SSE bounds and route schema | [OpenAPI tests](../internal/httpapi/openapi_test.go), stream/auth tests | No public HTTP deployment is supported |

## Client behavior

| Area | Local evidence | Remaining checks |
| --- | --- | --- |
| CLI streams, JSON, exit codes, noninteractive operation | [CLI tests](../internal/cli) | Shell completion/native terminal matrix |
| Onboarding readiness and no-stdin script mode | Onboarding, setup and HTTP checklist tests | Native first-run matrix |
| TUI width/height, Unicode/control sanitization and terminal restoration | [tui_test.go](../internal/tui/tui_test.go), [realtime_test.go](../internal/tui/realtime_test.go) | Real terminal/OS combinations |
| Detail focus/identity, Sync selection, nested Left/back and short-terminal visibility | [detail_navigation_test.go](../internal/tui/detail_navigation_test.go) | User's running installed binary must be rebuilt/upgraded |
| State filter, audit browser and full filter editor | [state-picker](../internal/tui/state_picker_test.go), [audit browser](../internal/tui/audit_browser_test.go), [filter editor](../internal/tui/filter_editor_test.go) tests | Native usability walkthrough |
| Shared explanations and honest progress/ETA | [present tests](../internal/present), i18n catalogue tests | Provider conditions beyond fixtures |
| Tray recent-job target and fixed arguments | Tray/CLI tests and [focused-job test](../internal/tui/focus_job_test.go) | Actual Windows/Linux notification clicks |
| Independent daemon/tray lifetimes and explicit stop | Tray instance/shutdown/confirmation tests | OS/session/Explorer restart acceptance |
| Autostart and user-unit behavior | [autostart tests](../internal/autostart), CLI tests | Real service and desktop startup matrix |
| Telegram scope, private chat, callback expiry/revision/one-shot | [Telegram tests](../internal/telegram/telegram_test.go) | Complete real operator/mobile matrix |
| Telegram batches, sealing, summaries and unknown-send fence | Telegram batch/outbox tests | Exactly-once remote delivery is not claimed |
| Operator library-only search | `TestOperatorFindIsPagedScopedAndReadOnly` | Whole-owner-Drive search must remain owner-only |
| Problem CSV, formula protection and bounded truncation | CLI/Telegram report tests | Large real document delivery |
| Daily reports and quiet hours | Notifier/report tests | Real DST/mobile delivery matrix |

## Protection options

| ID | Scenario | Evidence | Scope |
| --- | --- | --- | --- |
| PROT-1 | Mirror off by default; preserve destination additions | `TestMirrorIsOptInAndNeverTrashesDestinationAdditions` | Fake Drive |
| PROT-2 | Refuse missing intent, nonempty folders, incomplete scans and mass removal | `TestMirrorRefusesMissingIntentNonemptyFoldersAndIncompleteScans` | Fake Drive |
| PROT-3 | Retain versions, archive mirror removals, restore selected physical IDs and monotonic generations | `TestSnapshotRestoreReusesRetainedCopiesAndDefaultsToDryRun` | Fake Drive |
| PROT-4 | Lost move response does not repeat an applied move; changed content refuses restore | `TestRestoreReconcilesLostMoveResponseAndRejectsChangedContent` | Fake Drive |
| PROT-5 | Resume capture after database reopen and untrash saved copies | `TestSnapshotCaptureContinuesAfterDatabaseReopenAndUntrashes` | Local SQLite/fake Drive |
| PROT-6 | Scope, revision, idempotency, writer exclusion, pins and retention | [state protection tests](../internal/state/protection_test.go) | Local SQLite |
| PROT-7 | Upgrade populated schema 026 to 027 with intact foreign keys/defaults | `TestMigration027Upgrades026WithoutChangingCollections` | Local SQLite |
| PROT-8 | Preview default, pagination, contract and option preservation | [HTTP](../internal/httpapi/protection_test.go), [CLI](../internal/cli/protection_test.go) tests | Loopback fixtures |
| PROT-9 | Untrash explicitly sends false and supports shared drives | [restore_test.go](../internal/drive/restore_test.go) | Fake HTTP |

Mirror/snapshot live Google/Shared Drive acceptance is still required. Dedicated
TUI/Telegram protection controls remain unimplemented; CLI/HTTP are available.

## Operations and packaging

| Area | Local evidence | Native/live boundary |
| --- | --- | --- |
| Backup hashes/integrity/FKs, empty-target restore and writer lock | [maintenance tests](../internal/maintenance/maintenance_test.go) | Test representative private backups separately |
| Referenced-job retention and vacuum | State prune/maintenance tests | Large historical databases and disk-full recovery outstanding |
| Power/network transitions without duplicate writes or attempt burn | Power/daemon/engine fixtures | Actual sleep/hibernate/Wi-Fi/VPN transitions outstanding |
| Windows CLI/tray | CGO-disabled cross-build | Not Windows runtime acceptance |
| Inno build/install/version/uninstall sentinel smoke | [Windows scripts](../packaging/windows), wired into release workflow | Latest Linux session did not execute ISCC/installer |
| RPM/DEB/archive | Local release/package smoke records | Full desktop install/uninstall matrix outstanding |
| Container image, health and loopback API | Earlier Podman smoke recorded | Docker Compose and live container OAuth matrix incomplete |
| Release publishing | Local/dry-run pipeline can verify artifacts | No publication implied by local verification |

## Before claiming a release ready

Run formatting, vet, staticcheck and the full race suite. Check documentation links,
JSON/OpenAPI and whitespace. Record failures by name. For remote-write changes,
include failure-window fixtures and explicit live acceptance where authorized.
Do not equate a fake provider, a cross-build or a compiled installer script with
successful real-provider/native execution. Protect all credentials/logs/test reports
and use isolated authorized data for manual tests.
