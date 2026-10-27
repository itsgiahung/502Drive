-- N1: Conflict resolution policy (on_conflict = skip|rename|update).
-- The decision and chosen destination name are stored on the item so
-- reconciliation and restarts never flip the decision. dest_name also
-- reserves a name under dest_parent_id, which is how copies in one folder
-- avoid taking the same name without listing Drive for every item.
ALTER TABLE items ADD COLUMN conflict_action TEXT NOT NULL DEFAULT '';
ALTER TABLE items ADD COLUMN dest_name TEXT NOT NULL DEFAULT '';
-- Objects copied before this migration already hold their name: reserve it.
UPDATE items SET dest_name = name WHERE dest_name = '' AND dest_id <> '';
CREATE INDEX IF NOT EXISTS items_dest_name ON items(dest_parent_id, dest_name);
