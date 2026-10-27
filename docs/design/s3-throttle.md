# S3 — Shared adaptive throttling

One `drive.Throttle` is shared by engine, syncer and audit for the connected account.
Default concurrency begins at four workers, grows to eight and increases by one
after the configured success streak. Rate limiting halves the limit with floor one;
network, permission, quota and server errors do not masquerade as AIMD throttles.

Acquire is context-cancellable. Engine permits cover claim, provider call and
persistence so active item work remains bounded. The paced provider applies the
read token bucket to metadata/list/Changes/lookup calls and observes results;
it does not acquire a second engine permit.

Reads default to eight requests per second with a one-second burst. Writes share
worker concurrency but not the read bucket. Retry-After supports seconds or dates.
429 retries use jitter/backoff and shared cooldown, restore the claim attempt and
keep items pending. Listing/reconciliation throttles also do not spend attempts.

Status exposes worker count, recent throttles and next retry time. Keep server 5xx,
transport/unreachable, authentication, permissions, throttle and quota distinct.
Unknown write results still reconcile before another write.

Tests exercise AIMD floor/growth, cancellation, bucket pacing, cooldown windows,
error classification and item attempt preservation. See
[throttle](../../internal/drive/throttle.go), [automation](../features/automation.md),
[quota](q1-quota.md).
