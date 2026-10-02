-- Migration 0013: per-user Telegram notification preferences.

ALTER TABLE telegram_user_preferences ADD COLUMN notify_job_complete INTEGER NOT NULL DEFAULT 1;
ALTER TABLE telegram_user_preferences ADD COLUMN notify_job_error INTEGER NOT NULL DEFAULT 1;
ALTER TABLE telegram_user_preferences ADD COLUMN notify_watch_conflicts INTEGER NOT NULL DEFAULT 1;
