-- Batch H: the per-job problem report reads the failed/skipped/blocked items of
-- one job newest first. The rowid suffix keeps the cursor stable.
CREATE INDEX IF NOT EXISTS items_job_state ON items(job_id, state);
