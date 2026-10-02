-- Migration 0016: persist the owning watch for initial-clone jobs.
--
-- A watch must have at most one initial-clone job for its entire lifetime.
-- This lets startup recovery resume/reuse the exact job instead of creating a
-- second clone with a different job-scoped idempotency key.
ALTER TABLE jobs ADD COLUMN watch_id TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS idx_jobs_watch_initial_clone_watch
    ON jobs(watch_id)
    WHERE kind = 'watch_initial_clone' AND watch_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_jobs_kind_status_watch
    ON jobs(kind, status, watch_id);
