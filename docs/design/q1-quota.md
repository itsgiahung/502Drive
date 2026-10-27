# Q1 — Copy ledger and quota cooldown

Quota exhaustion is job-wide cooldown, not an item retry failure. Confirmed file
completion and the per-principal hourly logical-byte ledger commit together, so
reconciliation/repeated completion does not double-count. Unknown/native sizes
are not invented. A 24-bucket indexed sum describes usage, not a Google quota cap.

| Reason | Resume policy |
| --- | --- |
| `dailyLimitExceeded` | Next midnight America/Los_Angeles, accounting for DST |
| `storageQuotaExceeded` | 24 hours later; the user may need to free storage |
| Other quota | Retry-After when available, otherwise a conservative one-hour delay |

Release the item pending without spending its claim attempt; record job blocked
with `quota_exceeded` and `resume_after`. A shared run flag stops further claims.
Listing quotas retain page checkpoints. Multiple workers only establish the job
block once; their individual work is released safely.

A due blocked job with desired state run is requeued under the **same job ID**.
Restart preserves its resume time; further quota exhaustion establishes another
cooldown rather than another admission. Cancellation wins. Do not blindly resend
an uncertain write while applying quota policy.

Tests cover daily/storage delays, DST, duplicate completion accounting and
same-ID resumption. Live quota/reset and Shared Drive storage exhaustion remain
unverified. See [automation](../features/automation.md),
[migration 008](../../internal/state/migrations/008_copy_ledger.sql).
