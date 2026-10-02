-- Stable birth timestamp for adaptive polling.
-- Existing cursors start their idle-age clock from the best persisted
-- timestamp available at migration time; new cursors always set this field.
ALTER TABLE change_cursors
    ADD COLUMN created_at_ms INTEGER NOT NULL DEFAULT 0;

UPDATE change_cursors
SET created_at_ms = COALESCE(
    last_event_at_ms,
    last_success_at_ms,
    NULLIF(next_poll_at_ms, 0),
    CAST(strftime('%s', 'now') AS INTEGER) * 1000
)
WHERE created_at_ms = 0;
