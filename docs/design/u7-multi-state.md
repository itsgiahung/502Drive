# U7 — Multiple job states

state=a,b is canonicalized (trim, deduplicate, sort) by the shared state parser;
unknown or empty members return 400 invalid_request. Missing means all as before.
ListJobs uses parameterized IN over existing jobs_state_rowid (022); no migration
is needed. Cursor binds to the canonical set and collection/principal scope.
CLI --state passes the comma list; TUI f keeps cycling, F opens multi-selection
(space toggles, enter applies, esc cancels). No engine changes.

Local EXPLAIN QUERY PLAN:
```
SEARCH jobs USING INDEX jobs_state_rowid (state=? AND rowid<?)
USE TEMP B-TREE FOR ORDER BY
```
The existing state index covers the bounded candidate search; merging states uses
a temporary sort. No whole-job-table scan or new migration.
