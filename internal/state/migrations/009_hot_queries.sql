-- Hot-path indexes found while recording EXPLAIN QUERY PLAN for A2. The claim
-- queries filter folders by job/scan job and list_state; sync ticks filter
-- subscriptions by mode/state and the next due time. Without these, a 10k-item
-- tree scans `items` on every discovery batch.
CREATE INDEX items_job_list ON items(job_id, kind, list_state);
CREATE INDEX items_scan_list ON items(scan_job, kind, list_state);
CREATE INDEX sync_due_poll ON sync_subscriptions(mode, state, next_poll_at);
CREATE INDEX sync_due_full ON sync_subscriptions(mode, state, next_full_reconcile_at);

-- Replay-buffer counters: the cap check must not COUNT(*) the buffer on every
-- poll. The migration recomputes them once for any buffer left by 007.
ALTER TABLE sync_subscriptions ADD COLUMN replay_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE sync_subscriptions ADD COLUMN replay_bytes INTEGER NOT NULL DEFAULT 0;
UPDATE sync_subscriptions SET
  replay_count=(SELECT count(*) FROM sync_pending_changes p WHERE p.collection_id=sync_subscriptions.collection_id),
  replay_bytes=(SELECT coalesce(sum(length(change_json)),0) FROM sync_pending_changes p WHERE p.collection_id=sync_subscriptions.collection_id);
