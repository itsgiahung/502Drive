-- 020_collection_lifecycle.sql: add retired_at column to collections for fast
-- filtering of retired collections and timestamp tracking.
ALTER TABLE collections ADD COLUMN retired_at INTEGER;
CREATE INDEX IF NOT EXISTS collections_retired ON collections(retired_at);

-- L2: find the existing copy of a source to one destination without a scan.
CREATE INDEX IF NOT EXISTS collections_source_dest ON collections(principal, json_extract(body, '$.source_id'), json_extract(body, '$.destination_id'));
