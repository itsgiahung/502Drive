CREATE TABLE settings(key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE destinations(
  id TEXT PRIMARY KEY,
  folder_id TEXT NOT NULL,
  resource_key TEXT NOT NULL DEFAULT '',
  drive_id TEXT NOT NULL DEFAULT '',
  name TEXT NOT NULL,
  revision INTEGER NOT NULL DEFAULT 1,
  created_at INTEGER NOT NULL
);
-- items double as write intents: one row per (collection, source object).
CREATE TABLE items(
  collection_id TEXT NOT NULL REFERENCES collections(id),
  source_id TEXT NOT NULL,
  job_id TEXT NOT NULL REFERENCES jobs(id),
  parent_source_id TEXT NOT NULL,
  kind TEXT NOT NULL CHECK(kind IN ('wrapper','folder','file','shortcut')),
  name TEXT NOT NULL,
  mime_type TEXT NOT NULL DEFAULT '',
  resource_key TEXT NOT NULL DEFAULT '',
  size INTEGER,
  shortcut_target TEXT NOT NULL DEFAULT '',
  shortcut_target_mime TEXT NOT NULL DEFAULT '',
  fingerprint TEXT NOT NULL DEFAULT '',
  generation INTEGER NOT NULL DEFAULT 1,
  intent_key TEXT NOT NULL DEFAULT '',
  state TEXT NOT NULL CHECK(state IN ('pending','executing','reconciling','done','retry_wait','blocked','failed','skipped','cancelled')),
  dest_parent_id TEXT NOT NULL DEFAULT '',
  dest_id TEXT NOT NULL DEFAULT '',
  attempts INTEGER NOT NULL DEFAULT 0,
  next_attempt_at INTEGER NOT NULL DEFAULT 0,
  error_code TEXT NOT NULL DEFAULT '',
  list_state TEXT NOT NULL DEFAULT 'none' CHECK(list_state IN ('none','partial','done','failed')),
  page_token TEXT NOT NULL DEFAULT '',
  op TEXT NOT NULL DEFAULT 'create' CHECK(op IN ('create','align')),
  seen_job TEXT NOT NULL DEFAULT '',
  scan_job TEXT NOT NULL DEFAULT '',
  old_dest_id TEXT NOT NULL DEFAULT '',
  PRIMARY KEY(collection_id, source_id)
);
CREATE INDEX items_job_state ON items(job_id, state);
CREATE INDEX items_parent ON items(collection_id, parent_source_id);
CREATE TABLE sync_subscriptions(
  collection_id TEXT PRIMARY KEY REFERENCES collections(id),
  id TEXT NOT NULL UNIQUE,
  mode TEXT NOT NULL CHECK(mode IN ('scheduled','realtime')),
  state TEXT NOT NULL CHECK(state IN ('waiting_for_baseline','bootstrapping','active','paused','stopping','stopped','needs_reconcile','error')),
  schedule TEXT NOT NULL DEFAULT '',
  schedule_revision INTEGER NOT NULL DEFAULT 1,
  content_policy TEXT NOT NULL DEFAULT 'versioned_copy' CHECK(content_policy IN ('versioned_copy','replace_copy')),
  revision INTEGER NOT NULL DEFAULT 1,
  next_due INTEGER,
  next_key TEXT NOT NULL DEFAULT '',
  last_attempt_at INTEGER,
  last_success_at INTEGER,
  last_error TEXT NOT NULL DEFAULT '',
  cursor TEXT NOT NULL DEFAULT '',
  rerun_requested INTEGER NOT NULL DEFAULT 0,
  next_poll_at INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE item_versions(
  collection_id TEXT NOT NULL,
  source_id TEXT NOT NULL,
  generation INTEGER NOT NULL,
  dest_id TEXT NOT NULL,
  fingerprint TEXT NOT NULL,
  state TEXT NOT NULL CHECK(state IN ('current','superseded','trashed')),
  PRIMARY KEY(collection_id, source_id, generation)
);
CREATE TABLE sync_occurrences(
  collection_id TEXT NOT NULL,
  schedule_revision INTEGER NOT NULL,
  occurrence_key TEXT NOT NULL,
  job_id TEXT NOT NULL DEFAULT '',
  PRIMARY KEY(collection_id, schedule_revision, occurrence_key)
);
