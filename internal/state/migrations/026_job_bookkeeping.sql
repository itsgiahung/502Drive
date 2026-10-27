-- Group B #5: a terminal job commit and its sync bookkeeping used to be
-- separate transactions. A crash in between could leave a collection stale or
-- lose an after-copy sync request. The marker is inserted in the same
-- transaction that records the terminal job/collection state and cleared by the
-- bookkeeping transaction; any row left behind is replayed at startup.
CREATE TABLE job_bookkeeping(
  job_id TEXT PRIMARY KEY REFERENCES jobs(id) ON DELETE CASCADE,
  created_at INTEGER NOT NULL
);
