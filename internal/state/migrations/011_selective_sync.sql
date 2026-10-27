-- S5: selective sync (filter). rel_path keeps the path below the copied root so
-- path rules still apply after a restart. The backfill walks existing trees so
-- collections copied before this migration have correct paths.
ALTER TABLE items ADD COLUMN rel_path TEXT NOT NULL DEFAULT '';
-- Materialize the walk once and index it: a correlated subquery over the CTE
-- would walk the tree again for every item (quadratic on large collections).
CREATE TEMP TABLE _rel_tmp AS
WITH RECURSIVE tree(collection_id, source_id, rel) AS (
  SELECT collection_id, source_id, '' FROM items WHERE parent_source_id = '' AND kind IN ('folder', 'wrapper')
  UNION ALL
  SELECT i.collection_id, i.source_id, CASE WHEN t.rel = '' THEN i.name ELSE t.rel || '/' || i.name END
  FROM items i JOIN tree t ON i.collection_id = t.collection_id AND i.parent_source_id = t.source_id
)
SELECT collection_id, source_id, rel FROM tree;
CREATE INDEX _rel_tmp_key ON _rel_tmp(collection_id, source_id);
UPDATE items SET rel_path = coalesce((SELECT t.rel FROM _rel_tmp t WHERE t.collection_id = items.collection_id AND t.source_id = items.source_id), '');
DROP TABLE _rel_tmp;
-- Pruning empty folders scans one collection's pending folders.
CREATE INDEX IF NOT EXISTS items_collection_kind_state ON items(collection_id, kind, state);
