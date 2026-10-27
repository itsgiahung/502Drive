-- S6: batches and a durable, fair queue. The new columns are generated from the
-- job body, so every existing writer (saveJob, json_set in lifecycle/sync/quota)
-- stays the single source of truth and nothing can drift out of sync.
ALTER TABLE jobs ADD COLUMN batch_id TEXT GENERATED ALWAYS AS (coalesce(json_extract(body,'$.batch_id'),'')) VIRTUAL;
ALTER TABLE jobs ADD COLUMN queue_priority INTEGER GENERATED ALWAYS AS (coalesce(json_extract(body,'$.priority'),0)) VIRTUAL;
ALTER TABLE jobs ADD COLUMN queue_order INTEGER GENERATED ALWAYS AS (coalesce(json_extract(body,'$.queue_order'),0)) VIRTUAL;
-- A batch is one fair-share group; a lone job is its own group (its collection).
ALTER TABLE jobs ADD COLUMN fair_group TEXT GENERATED ALWAYS AS (coalesce(nullif(json_extract(body,'$.batch_id'),''), collection_id)) VIRTUAL;
CREATE INDEX IF NOT EXISTS jobs_batch ON jobs(principal, batch_id) WHERE batch_id <> '';
CREATE INDEX IF NOT EXISTS jobs_queue ON jobs(state, queue_priority, queue_order);
CREATE INDEX IF NOT EXISTS jobs_fair ON jobs(state, fair_group);
