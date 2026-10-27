# S7 — Read-only deep preflight

`inspect --deep` / `deep:true` returns a durable plan while a daemon-owned read-only
scan discovers the full tree. It uses paginated BFS with a directory frontier,
not an in-memory copy of every file. Progress writes are coalesced.

Only completed scans have complete totals. Cancel, expiry, failure or daemon restart
retain partial counts with cancelled/expired/failed/interrupted status. Deep scans
expire after an hour; request cancellation does not turn an unfinished page into
an empty successful result. No copy job or provider write is needed to preview.

Counts distinguish files, folders, native types, shortcuts, uncopyable items and
unreadable folders. Estimates derive from measured successful logical-byte progress;
unknown basis stays unknown. Storage/daily-limit warnings are advisory and depend
on available account/configuration data, not a promise of Google's actual cap.

Permission/not-found can count an unreadable folder; other scan failure does not
claim a total. SSE refreshes the active preview while connected; polling resumes
when disconnected. See [U8](u8-stream-preflight.md),
[copy guide](../features/clone-jobs.md), [preflight code](../../internal/app).
