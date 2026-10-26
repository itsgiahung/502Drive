CREATE TABLE plans(id TEXT PRIMARY KEY, principal TEXT NOT NULL, body BLOB NOT NULL, consumed_job TEXT);
CREATE TABLE collections(id TEXT PRIMARY KEY, principal TEXT NOT NULL, plan_id TEXT NOT NULL REFERENCES plans(id), body BLOB NOT NULL);
CREATE TABLE jobs(id TEXT PRIMARY KEY, principal TEXT NOT NULL, collection_id TEXT NOT NULL REFERENCES collections(id), state TEXT NOT NULL, body BLOB NOT NULL);
CREATE INDEX jobs_owner ON jobs(principal,id);
CREATE INDEX jobs_state ON jobs(state,id);
CREATE UNIQUE INDEX one_active_run ON jobs(collection_id) WHERE state IN ('queued','running','pausing','paused','cancelling','reconciling','blocked');
CREATE TABLE events(sequence INTEGER PRIMARY KEY AUTOINCREMENT, job_id TEXT NOT NULL REFERENCES jobs(id), body BLOB NOT NULL);
CREATE INDEX events_job ON events(job_id,sequence);
CREATE TABLE idempotency(principal TEXT NOT NULL, path TEXT NOT NULL, key TEXT NOT NULL, hash TEXT NOT NULL, body BLOB NOT NULL, PRIMARY KEY(principal,path,key));
