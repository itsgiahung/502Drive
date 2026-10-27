# S1 — Integrity audit and repair

Audit reports source/destination drift using stable findings. Repair fixes only
revalidated, repairable differences on owned objects. Both use the existing engine
and one-active-writer constraint; there is no permanent delete.

Migration 006 adds audits, counts/plan/revision and idempotent finding keys. Job
kind/audit/dry-run fields are additive JSON. Audit is read-only: traverse every
source/destination page, compare mappings and record missing/name/parent/content/
duplicate/extra/source-unavailable findings. Restart can rescan and upsert without
duplicating findings. Foreign extras and unreadable/removed sources are report-only.

## Repair sequence

1. Revalidate findings against current remote state and persist a plan. CLI defaults
   to dry-run; HTTP clients must explicitly choose their dry-run/apply field.
2. Reuse normal create/align paths and durable intents. Reconcile before applying
   new work so a restart cannot erase an in-flight copy.
3. Trash only verified owned superseded/duplicate objects after the replacement is
   mapped. Under versioned-copy, retain history not replaced by this repair.
   Respect retained snapshot references.
4. Mark findings repaired only after completion. Incomplete verification produces
   a partial/blocked outcome rather than guessed success.

A second repair rechecks current state and can produce an empty plan with no
writes. A crash between replacement and trash leaves recoverable retained versions.
Concurrent copy/sync/audit/repair writers are refused. Audit revision protects the
plan request; current-state checks protect against old findings.

Tests cover read-only audit, foreign objects, paginated trees, repeat repair,
missing/edited copies, lost write responses and restart. See
[engine audit](../../internal/engine/audit.go), [findings API](../control-contracts.md),
[copy guide](../features/clone-jobs.md).
