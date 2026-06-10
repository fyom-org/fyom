DROP INDEX IF EXISTS idx_media_library;
-- SQLite cannot DROP COLUMN before 3.35.0; columns are left harmlessly.
DROP TABLE IF EXISTS libraries;
