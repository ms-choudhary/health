-- 0001_rename_food_to_ingredient.sql
--
-- Transforms a pre-rename database into the current schema:
--   table  foods                       -> ingredients
--   column log_entries.food_id         -> ingredient_id
--   column log_entries.food_name       -> ingredient_name
--   column log_entries.food_unit       -> ingredient_unit
--   column recipe_ingredients.food_id  -> ingredient_id
--
-- Run once against an existing database, e.g.:
--   sqlite3 health.db < backend/sql/migrations/0001_rename_food_to_ingredient.sql
--
-- Requires SQLite >= 3.25.0 (ALTER TABLE ... RENAME COLUMN). Running it on a
-- fresh or already-migrated database fails on the first statement and the
-- transaction rolls back, leaving the database unchanged.

-- Make table renames rewrite the foreign-key references in child tables
-- (the default; set explicitly in case the connection enabled legacy mode).
PRAGMA legacy_alter_table = OFF;

-- foreign_keys can only be toggled outside a transaction; disable enforcement
-- so the in-place schema edits below are not second-guessed mid-migration.
PRAGMA foreign_keys = OFF;

BEGIN TRANSACTION;

-- Renaming the table also rewrites the "REFERENCES foods(id)" clauses in
-- log_entries and recipe_ingredients to "REFERENCES ingredients(id)".
ALTER TABLE foods RENAME TO ingredients;

ALTER TABLE log_entries RENAME COLUMN food_id   TO ingredient_id;
ALTER TABLE log_entries RENAME COLUMN food_name TO ingredient_name;
ALTER TABLE log_entries RENAME COLUMN food_unit TO ingredient_unit;

ALTER TABLE recipe_ingredients RENAME COLUMN food_id TO ingredient_id;

COMMIT;

-- Re-enable enforcement and confirm no references were left dangling
-- (this returns zero rows when integrity is intact).
PRAGMA foreign_keys = ON;
PRAGMA foreign_key_check;
