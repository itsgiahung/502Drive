# U9 — Ledger quota view

GET status adds quota {used_24h_bytes,limit_known,resume_after}. Byte usage is the
requesting principal's real Q1 hourly ledger (current hour + previous 23), with
an indexed SUM. Resume is the earliest future quota-blocked job of that principal.
No numeric Google limit is known: limit_known=false, even when a local preflight
limit is configured. Wording explicitly says last 24h rather than calendar today,
because the ledger is a rolling window. User timezone formats the resume date/time.
No engine/Drive changes, migration or guessed reset time.
