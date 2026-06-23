-- 0002_rename_source_recipe_id_to_recipe_group_id.sql
--
-- Transforms a pre-rename database into the current schema:
--   column log_entries.source_recipe_id -> recipe_group_id
--   index  idx_log_source_recipe        -> idx_log_recipe_group
--
-- Also normalises legacy custom-recipe group ids, which were stored as negative
-- unix timestamps, to positive values so no negative group ids remain.
--
-- Run once against an existing database, e.g.:
--   sqlite3 health.db < backend/sql/migrations/0002_rename_source_recipe_id_to_recipe_group_id.sql
--
-- Requires SQLite >= 3.25.0 (ALTER TABLE ... RENAME COLUMN). Running it on a
-- fresh or already-migrated database fails on the first statement and the
-- transaction rolls back, leaving the database unchanged.

BEGIN TRANSACTION;

ALTER TABLE log_entries RENAME COLUMN source_recipe_id TO recipe_group_id;

-- Legacy custom recipes used a negative unix timestamp as the group id.
UPDATE log_entries SET recipe_group_id = -recipe_group_id WHERE recipe_group_id < 0;

DROP INDEX IF EXISTS idx_log_source_recipe;
CREATE INDEX IF NOT EXISTS idx_log_recipe_group ON log_entries(recipe_group_id);

COMMIT;
