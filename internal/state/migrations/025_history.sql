-- Batch H: the history export lists jobs newest-updated first inside a time
-- window. Ordering on RFC3339 text is wrong when one timestamp has a fractional
-- second and another does not, so the index uses integer milliseconds.
-- rowid is stored implicitly at the end of the index, which makes the keyset
-- (updated_ms, rowid) both ordered and seekable.
CREATE INDEX IF NOT EXISTS jobs_history ON jobs(CAST((julianday(json_extract(body, '$.updated_at')) - 2440587.5) * 86400000 AS INTEGER));
