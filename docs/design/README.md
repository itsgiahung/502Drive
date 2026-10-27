# Technical decisions

These notes explain implementation choices and failure handling. The current
feature guides, code and OpenAPI take precedence over old planning terminology.
Private worker assignments, prompts and session reports belong in ignored local
handoffs, not this public reference.

## Copy and state safety

| Note | Decision |
| --- | --- |
| [D1](d1-shared-drives.md) | Shared Drive capability/permission boundaries |
| [F1](f1-fast-list.md) | Bounded multi-parent discovery and page ownership |
| [K1](k1-skipped-classification.md) | Stable skip reasons and shortcut policy |
| [N1](n1-conflict-policy.md) | Durable destination names and conflict limits |
| [L1](l1-lifecycle.md) | Retire, resume, forget and trash-only cleanup |
| [L2](l2-existing-copy.md) | Update an existing collection or create a new copy |
| [S1](s1-integrity.md) | Read-only audit and safe repair |
| [S2](s2-sync-replay.md) | Bootstrap replay and periodic reconciliation |
| [S3](s3-throttle.md) | Shared adaptive worker/read limits |
| [S5](s5-selective-sync.md) | Persisted selective filters |
| [S6](s6-batch-queue.md) | Independent batch jobs and fair queue |
| [S7](s7-deep-preflight.md) | Read-only deep preview and honest totals |
| [Q1](q1-quota.md) | Local ledger and quota resumption |
| [R1](r1-retention.md) | Referentially safe offline history pruning |
| [S9](snapshots.md) | Retained physical copies and preview-first restore |
| [Protection](protection-options.md) | Options, archive intent and installer boundaries |

## Runtime and shared presentation

| Note | Decision |
| --- | --- |
| [T1](t1-power-lifecycle.md) | Power/network gate and in-flight write safety |
| [T2](t2-lifecycle.md) | Separate daemon, tray and TUI lifetimes |
| [T3](t3-autostart.md) | Optional per-user startup |
| [T4](tray.md) | Stable native tray actions and bounded updates |
| [T5](t5-i18n.md) | Application language without protocol changes |
| [S11](s11-linux-reliability.md) | Doctor, WAL and offline maintenance |
| [S12](s12-live.md) | Explicit real-provider test gate |
| [S13](s13-present.md) | Shared client formatting |

## API and client integration

H1–H6 cover [additive APIs](h1-api-gaps.md), [after-copy sync](h2-after-copy-sync.md),
[CLI shortcuts](h3-cli.md), [reports](h4-report-export.md),
[locale/service/prune](h5-locale-systemd-prune.md) and [static checks](h6-staticcheck.md).

U1–U13 cover [onboarding](u1-onboarding.md), [queue hints](u2-queue-position.md),
[explanations](u3-plain-errors.md), [progress](u4-honest-progress.md),
[audit browser](u5-audit-browser.md), [filter editor](u6-filter-editor.md),
[state filters](u7-multi-state.md), [stream preview](u8-stream-preflight.md),
[quota views](u9-quota.md), [batch notices](u10-batch-notices.md),
[operator search](u11-operator-find.md), [notifications](u12-action-notifications.md)
and [job targeting](u13-tray-job.md).
