-- S1: integrity audit & repair. An audit is a durable read-only job; its
-- findings are the report. Repairs reference the audit they fix.
CREATE TABLE audits(
  id TEXT PRIMARY KEY,
  collection_id TEXT NOT NULL REFERENCES collections(id),
  job_id TEXT NOT NULL REFERENCES jobs(id),
  state TEXT NOT NULL,
  counts TEXT NOT NULL DEFAULT '{}',
  plan TEXT NOT NULL DEFAULT '',
  scanned INTEGER NOT NULL DEFAULT 0,
  revision INTEGER NOT NULL DEFAULT 1,
  started_at INTEGER NOT NULL,
  finished_at INTEGER
);
CREATE INDEX audits_collection ON audits(collection_id, started_at);

CREATE TABLE audit_findings(
  audit_id TEXT NOT NULL REFERENCES audits(id) ON DELETE CASCADE,
  kind TEXT NOT NULL,
  source_id TEXT NOT NULL DEFAULT '',
  dest_id TEXT NOT NULL DEFAULT '',
  repairable INTEGER NOT NULL DEFAULT 0,
  repaired INTEGER NOT NULL DEFAULT 0,
  detail TEXT NOT NULL DEFAULT '',
  PRIMARY KEY(audit_id, kind, source_id, dest_id)
);
CREATE INDEX audit_findings_kind ON audit_findings(audit_id, kind, repairable);
