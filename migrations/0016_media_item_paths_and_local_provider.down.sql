-- Roll back migration 0016.
--
-- SQLite versions before 3.35 do not support ALTER TABLE DROP COLUMN. To avoid
-- rebuilding media_items and accidentally dropping columns introduced by later
-- migrations, this down migration only removes indexes created by this
-- migration.
--
-- The root_path, primary_path, and nfo_path columns are intentionally left in
-- place.

DROP INDEX IF EXISTS idx_media_items_provider_library;
DROP INDEX IF EXISTS idx_media_items_nfo_path;
DROP INDEX IF EXISTS idx_media_items_primary_path;
DROP INDEX IF EXISTS idx_media_items_root_path;

-- Do not delete the built-in local provider here. Other rows may now depend on
-- providers.id = 'local'.
