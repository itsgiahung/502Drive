-- Migration 0017: optimize the desktop job list hot path.
--
-- The GUI polls user-visible one-shot jobs every few seconds using:
--   WHERE kind = 'one_shot'
--   ORDER BY updated_at_ms DESC, id
-- This composite index satisfies both the filter and ordering without a
-- temporary sort.
CREATE INDEX IF NOT EXISTS idx_jobs_kind_updated
    ON jobs(kind, updated_at_ms DESC, id);
