-- S8: apply the principal's after-copy sync preference to a collection the
-- initial copy just created. The row is written in the same transaction that
-- records the job result, so a daemon restart right after the copy still
-- applies it. collection_id is the key: one collection gets one subscription,
-- and applying the same mode twice is a no-op.
CREATE TABLE after_copy_sync(
  collection_id TEXT PRIMARY KEY REFERENCES collections(id) ON DELETE CASCADE,
  principal TEXT NOT NULL,
  mode TEXT NOT NULL,
  state TEXT NOT NULL DEFAULT 'pending',
  attempts INTEGER NOT NULL DEFAULT 0,
  next_attempt_at INTEGER NOT NULL DEFAULT 0,
  last_error TEXT NOT NULL DEFAULT '',
  warning TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);
CREATE INDEX after_copy_sync_due ON after_copy_sync(state, next_attempt_at);
