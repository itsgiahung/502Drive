CREATE TABLE snapshots (
 id TEXT PRIMARY KEY,
 collection_id TEXT NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
 job_id TEXT NOT NULL REFERENCES jobs(id),
 state TEXT NOT NULL CHECK(state IN ('capturing','ready')),
 pinned INTEGER NOT NULL DEFAULT 0,
 revision INTEGER NOT NULL DEFAULT 1,
 created_at INTEGER NOT NULL,
 expires_at INTEGER NOT NULL,
 UNIQUE(job_id)
);
CREATE INDEX snapshots_collection ON snapshots(collection_id,created_at);
CREATE TABLE snapshot_items (
 snapshot_id TEXT NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
 source_id TEXT NOT NULL,
 dest_id TEXT NOT NULL,
 body TEXT NOT NULL,
 identity TEXT NOT NULL DEFAULT '',
 PRIMARY KEY(snapshot_id,source_id)
);
CREATE INDEX snapshot_items_dest ON snapshot_items(dest_id,snapshot_id);
CREATE TABLE destination_actions (
 job_id TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
 ordinal INTEGER NOT NULL,
 action TEXT NOT NULL CHECK(action IN ('restore','remove')),
 body TEXT NOT NULL,
 state TEXT NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','executing','done')),
 PRIMARY KEY(job_id,ordinal)
);
CREATE TABLE collection_archives (
 collection_id TEXT PRIMARY KEY REFERENCES collections(id) ON DELETE CASCADE,
 intent_key TEXT NOT NULL,
 parent_id TEXT NOT NULL,
 dest_id TEXT NOT NULL DEFAULT '',
 attempted INTEGER NOT NULL DEFAULT 0
);
