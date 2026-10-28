# Telegram bot

Telegram is optional and runs inside the daemon. It uses the same application
service and authorization as HTTP/CLI; bot failure does not stop core copy work.

## Setup and identity

Provide a private `telegram_token_file` and numeric `telegram_owner_id` through
setup. Bot states are `disabled`, `starting`, `polling`, `error`. There is one
poller per bot token. Incoming updates commit to the durable inbox **before**
advancing the poll offset; replay derives idempotency from update identity.

Only private chats are accepted. The configured owner is the owner principal;
other users need an enabled operator record. Unknown users/groups receive no
private job/library data. Operators see their own work, not the owner's Drive.
See [identity](identity-control.md).

## Copy and job cards

`/start`, `/menu` and `/help` expose a persistent keyboard and inline navigation.
Paste a Drive link or use `/copy` to inspect it. The preview retains the source
name and offers destination, rename, filter/conflict/shortcut and after-copy-sync
choices. If no destination exists, the flow can offer My Drive. Destination changes
or stale auto-copy approval return to preview rather than silently switching output.

An existing source/destination pair offers update or new-copy choices. Auto-copy
is opt-in and bound to the chosen destination revision. Restricted/invalid sources
receive actionable errors; sharing restrictions are never bypassed.

Job cards show counts, stable skip reasons, progress and state-appropriate actions.
Panels edit at a bounded interval, honor Telegram Retry-After and avoid unchanged
edits. Quota waits show the saved-timezone resume time and suppress immediate retry.
Job cancellation is explicit; completed files remain.

## Batch and queue

Multiple links create a previewed batch, bounded to 20 links per message. Each link
has its own durable admission key under the batch; one error does not undo others.
Existing copies can be skipped rather than duplicated. New clients seal admission
so completion can produce one batch summary with scoped retry/problem actions.

Owner-only queue pause/resume uses the same durable global flag as other clients.
Operators may move only their own queued jobs. Queue positions are live estimates;
fair admission and other work can change them. Stale revision callbacks refresh
the card rather than forcing a move.

Job management filters server-side and applies bulk actions only to eligible jobs.
Bulk cancel requires confirmation; quota-waiting jobs are excluded from immediate
bulk retry. Results count succeeded, skipped and failed operations separately.

## Collections and search

Library cards expose scheduled/realtime presets, future-slot previews, run now,
pause/stop/resume, collection jobs and read-only integrity audit. Repair previews
before an explicit confirmation. Retire stops tracking but retains output;
forget removes local records, and cleanup confirms the actual root name before
trashing an owned destination. Destructive confirmation cards have a shorter
validity window and preserve the displayed revision.

`/find NAME` has different scopes:

- Owner: search accessible Drive metadata and choose a source for copy.
- Operator: search only that operator's saved collection names, including retired
  entries, with read-only details and paging. It never searches the owner's Drive.

The collection card has a **Protection** button for mirror, automatic snapshots,
fixed retention choices, capture now, pin/unpin and restore. Every control is a
one-shot callback bound to the revision the card showed. Enabling mirror has its
own confirmation card explaining the Drive trash, and a restore is previewed as a
dry run whose action count must be read before a second confirmation applies it
(sync pauses). See the [protection guide](protection.md).

## Deep preview and reports

Deep preview starts a read-only background scan and exposes refresh/cancel. Counts
are labelled partial until complete; cancelled, interrupted or expired scans never
claim a total. Estimates and quota/storage warnings remain advisory.

Problem reports export a scoped CSV with state, stable code, English interchange
reason, relative path, attempts and Drive IDs. Formula prefixes are escaped. Limits
are 5,000 rows / 2 MiB, with explicit truncation. No daemon filesystem paths, tokens
or raw provider messages belong in reports. Captions use shared bilingual explanations.

## Notifications

Preferences control completed/failed/action-required messages and quiet hours.
Unsolicited Telegram notifications are deferred to the end of a quiet window;
direct replies and user-started progress panels can still update. No-op sync runs
do not produce completion noise. Repeated subscription errors are deduplicated
until success clears their marker.

Daily reports are opt-in, timezone-aware, at most once per local day and silent
when there is nothing to report. Durable outbox keys prevent duplicate local
admission. Batch summaries wait for all admitted jobs; paused jobs keep waiting,
while blocked jobs count as needing attention.

Remote delivery is not exactly once. A durable send-attempt fence prevents a blind
resend after an unknown Telegram result; a crash in that window may lose a summary.
Telegram delivery errors do not change job results. `/status` shows the requesting
principal's local 24-hour byte ledger and known resume time without inventing a
Google account cap.

## Callback and presentation safety

Callbacks are opaque, principal/chat/message-bound, revision-bound, expiring and
one-shot. Invalid, consumed or stale callbacks expire instead of repeating a write.
Fresh reads do not authorize applying an old confirmation with a new revision.
Drive names are HTML-escaped; chat text is never executed by a shell. Application
strings support English/Vietnamese; resource keys and credentials stay private.

## Limits and evidence

No Google login through chat, group-chat operation, arbitrary schedule/time editor,
operator HTTP token administration or native desktop notifications through this bot.
Color/button support depends on Telegram clients; functionality must remain usable
without decorative styling.

Sources and local tests are under [internal/telegram](../../internal/telegram).
A small live Linux/Telegram session is recorded in [verification](../verification.md);
that does not establish every bot action, operator flow, quota or large batch.
