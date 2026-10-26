CREATE TABLE principals(
  id TEXT PRIMARY KEY,
  kind TEXT NOT NULL,
  external_id TEXT NOT NULL,
  role TEXT NOT NULL CHECK(role IN ('owner','operator')),
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at INTEGER NOT NULL,
  UNIQUE(kind, external_id)
);
