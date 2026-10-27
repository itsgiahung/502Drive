# S2 — Bootstrap replay and full reconciliation

Changes cannot be filtered reliably while the baseline mapping is incomplete.
Capture a cursor before bootstrap, keep polling and durably buffer the raw minimum
change data, then evaluate relevance after mapping completion.

Migration 007 stores ordered pending changes/cursor-after plus overflow and
periodic reconcile fields. Bounds are 5,000 changes or 4 MiB per collection.
Bootstrap reads resume from the last buffered page while the subscription cursor
stays unchanged. Overflow stops extending the buffer and requests full reconciliation.

Replay requires active subscription/expected cursor, then atomically removes the
buffer, advances the cursor and admits/coalesces relevant work. Pause/stop racing
replay preserves pending data instead of losing it. Restart after buffering or
before replay recovers from the same checkpoints.

Periodic full reconciliation defaults to seven days with ±10% jitter; zero disables.
Due slots coalesce with active work and remain due under capacity pressure. Older
subscriptions are scheduled rather than all firing immediately after upgrade.
Expired cursors request a new token/full scan. Root/mapping identity is retained.

The mass-removal guard stops before copy when over 30% of at least ten mapped
children disappear, records a partial result and requests a read-only audit.
Mirror remains gated by complete successful discovery/copy and this guard.

Tests cover migration/FKs, cursor/admission atomicity, replay limits, crash/reopen,
pause races, periodic coalescing and guard thresholds. See
[automation](../features/automation.md), [state replay](../../internal/state/replay.go),
[daemon syncer](../../internal/daemon/syncer.go).
