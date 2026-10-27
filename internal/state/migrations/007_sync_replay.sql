-- S2: replay the change feed while a baseline runs, and a periodic full
-- reconcile per realtime subscription. Pending rows stay until they are
-- replayed; the cursor only moves with the replay (or with a clean poll).
CREATE TABLE sync_pending_changes(
  collection_id TEXT NOT NULL REFERENCES collections(id),
  seq INTEGER NOT NULL,
  cursor_after TEXT NOT NULL,
  change_json TEXT NOT NULL,
  PRIMARY KEY(collection_id, seq)
);

ALTER TABLE sync_subscriptions ADD COLUMN full_reconcile_every INTEGER NOT NULL DEFAULT 604800;
ALTER TABLE sync_subscriptions ADD COLUMN next_full_reconcile_at INTEGER NOT NULL DEFAULT 0;
ALTER TABLE sync_subscriptions ADD COLUMN replay_overflow INTEGER NOT NULL DEFAULT 0;
