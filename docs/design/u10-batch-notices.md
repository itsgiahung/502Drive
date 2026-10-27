# U10 — Batch notification

New batch clients seal admission through POST /v1/batches/{id}/seal after attempting
all links. This is necessary: a debounce alone cannot prove a fast first job is
the last admitted job. Seals and done markers use existing indexed settings keys;
no migration. The Telegram outbox sweep checks open seals, waits three seconds,
and requires all admitted jobs terminal before atomically enqueuing one final
summary + marking done. Repeated seals and restart cannot create another summary.
Older servers' missing seal route is tolerated; old clients retain job notices.
Quiet hours and notification preferences apply. Batch callbacks carry a digest
of job IDs/revisions and are one-shot/expiring via existing callback storage.

Telegram has no idempotency key for sendMessage. For a first summary, a durable
send-attempt fence is committed before the request; an uncertain outcome is never
blindly resent. This prevents duplicate summaries across restart but can lose a
summary if the process dies before sending or before receiving its message ID.
Known 429 rejection can safely requeue. This delivery limitation is explicit;
exactly-once external delivery cannot be established by a local outbox alone.
