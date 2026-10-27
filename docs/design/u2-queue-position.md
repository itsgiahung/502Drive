# U2 — Queue positions

One scheduler helper reads queued candidates through jobs_queue and active fair
groups through jobs_fair, then simulates successive admissions (increments the
winning group's active share). ClaimQueued uses that same helper for its next
pick. No COUNT query per poll; two indexed range reads, no client estimation.
Positions are a snapshot of successive claims, not a promise about finish times.
Priority wins, then least active fair group, then queue_order/rowid. Pause keeps
positions internally but all clients show the durable queue pause instead.
Read-only derived fields are never persisted. No migration or engine edits.

EXPLAIN QUERY PLAN (local synthetic DB):
```
SEARCH jobs USING INDEX jobs_fair (state=?)
SEARCH jobs USING INDEX jobs_queue (state=?)
USE TEMP B-TREE FOR LAST 2 TERMS OF ORDER BY
```
Only queued candidates and active groups are read. In-memory selection is quadratic
in the pending queue (admission defaults to 100), with no item/tree loading.
