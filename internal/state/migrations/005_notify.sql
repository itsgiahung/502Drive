-- Delivery bookkeeping for Telegram notifications about work the user did not
-- start in a chat (CLI copy, scheduled/realtime sync). One row per principal
-- and subject records the last error code that was already announced, so a
-- repeating failure is not repeated until it changes or clears.
CREATE TABLE notify_markers(
  principal TEXT NOT NULL,
  subject TEXT NOT NULL,
  code TEXT NOT NULL,
  updated_at INTEGER NOT NULL,
  PRIMARY KEY(principal, subject)
);
-- A final notification carries a short machine note (job state or error code)
-- so the adapter can render it without another query.
ALTER TABLE notification_outbox ADD COLUMN note TEXT NOT NULL DEFAULT '';
