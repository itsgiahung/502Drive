# S6 — Batch admission and fair queue

A batch groups independent copy jobs; it does not merge sources into one engine.
Clients derive stable batch/per-link keys for replay, skip duplicate/blank inputs
and report each failed admission without rolling back accepted links.

Migration 021 exposes generated batch/priority/order/fair-group fields from job
JSON. Indexes support batch lookup, queue candidates and active groups. One
transaction selects the next queued job by priority, least active fair group,
then queue order/rowid. Priority is -5..5. Moves are revision-guarded and maintain
relative order; generated columns remain read-only.

Global queue pause is durable. Running work cooperatively pauses at a safe point;
queued work remains unclaimed. Resume restarts only queue-paused work, preserving
individual user pauses. The safe-point transaction rechecks the global flag so a
concurrent resume cannot strand an unscheduled paused job.

Admission capacity, job slots and shared API workers are distinct controls.
Queue positions simulate fair claims but remain changing estimates. Batch completion
notifications require an admission seal; see [U10](u10-batch-notices.md).

See [queue state](../../internal/state/queue.go),
[CLI](../features/cli.md), [data contracts](../data-contracts.md).
