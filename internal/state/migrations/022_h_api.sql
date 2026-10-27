-- Batch H: server-side library/job queries. The audit card asks for the number
-- of repairable findings of one audit; the job list filters by collection.
CREATE INDEX IF NOT EXISTS audit_findings_repairable ON audit_findings(audit_id, repairable) WHERE repairable = 1;
CREATE INDEX IF NOT EXISTS jobs_collection ON jobs(collection_id, state);
-- A secondary index stores rowid as its last key, so an index on the filtered
-- column alone lets SQLite page newest-first without a temp B-tree.
CREATE INDEX IF NOT EXISTS jobs_collection_rowid ON jobs(collection_id);
CREATE INDEX IF NOT EXISTS jobs_state_rowid ON jobs(state);
