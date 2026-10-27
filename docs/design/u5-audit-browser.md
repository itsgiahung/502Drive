# U5 — Audit browser

Collection A opens a dedicated findings screen, leaving the overall tabs intact.
Type filter and next/back page use the existing audit findings cursor API. No new
write routes. `f` requests dry-run; `F` needs a successful dry-run and explicit
confirmation saying only 502Drive objects are touched. Preview shows the existing
persisted audit plan; failed or missing jobs never authorize apply. Path is resolved from the authorized collection mapping via indexed reads;
extra/unmapped objects keep detail/IDs because no relative mapping exists.
Finding JSON adds path and explanation. No engine changes.
