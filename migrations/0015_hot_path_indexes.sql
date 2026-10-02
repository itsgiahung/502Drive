-- Migration 0015: indexes for high-frequency desktop/watch hot paths.

-- Dashboard / Jobs page polls the newest jobs frequently.
CREATE INDEX IF NOT EXISTS idx_jobs_updated
    ON jobs(updated_at_ms DESC, id);

-- Watch summaries are ordered by most recently updated.
CREATE INDEX IF NOT EXISTS idx_watch_subscriptions_updated
    ON watch_subscriptions(updated_at_ms DESC, id);

-- The dispatcher reloads active watches every cycle.
CREATE INDEX IF NOT EXISTS idx_watch_subscriptions_status_id
    ON watch_subscriptions(status, id);

-- Every committed change page checks paused watches sharing the cursor.
CREATE INDEX IF NOT EXISTS idx_watch_subscriptions_cursor_status
    ON watch_subscriptions(cursor_id, status);

-- Traversal claims the next pending folder for a job on every folder page.
CREATE INDEX IF NOT EXISTS idx_traversal_folders_pending
    ON traversal_folders(job_id, scan_state, updated_at_ms, source_folder_id);
