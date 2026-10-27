# H2 / S8 — Enable sync after the first copy

Persist `after_copy_sync_mode` on the admitted initial job, overriding the user's
preference only when explicit. Supported modes: off, realtime, hourly, six_hourly,
daily. Success creates a durable pending request only for a new collection without
an existing user subscription; partial/failed/cancelled jobs do not enable sync.

Migration 023 stores requests with attempts/lease/retry outcome. Applying uses a
stable collection/mode key so restart and repeated bookkeeping remain idempotent.
Provider failure retries the pending request without changing the successful copy.
Migration 026's terminal bookkeeping fence closes the crash window before setup.

Realtime captures a token on application; intervals anchor to copy completion.
The after-copy daily preset is 02:00 in the saved timezone, distinct from the
interactive daily schedule preset. Content policy defaults to versioned_copy.

Verify restart before application, duplicate bookkeeping, explicit mode override,
provider failure and existing-subscription precedence. See
[automation](../features/automation.md), [after-copy code](../../internal/state/aftercopy.go).
