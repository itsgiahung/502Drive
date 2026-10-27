# H4 / X1 — Problem reports and history export

Problem reports page failed/blocked/skipped items with stable code, shared reason,
relative path and attempts. History is a separate updated-time view rather than a
second sort order hidden behind the admission-ordered job cursor.

Migration 025 indexes numeric epoch milliseconds derived from JSON updated time.
Stable history keys use `(updated_ms,rowid)` so fractional RFC3339 formatting does
not create lexical ordering errors. Cursors bind principal and filters.

Default export bounds are 5,000 rows, with explicit truncation. JSON uses one envelope;
CSV uses encoding/csv and neutralizes formula prefixes. Owner `all` remains separate
from ordinary principal scope. No provider credentials or raw private log paths belong
in exported reports.

Verify empty/missing/foreign jobs, CSV quoting/formulas, invalid dates, same-time
paging, truncation and indexed query plans. See [CLI](../features/cli.md).
