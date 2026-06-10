DROP INDEX IF EXISTS idx_media_provider;

-- Intentionally a no-op for the column itself:
-- ALTER TABLE DROP COLUMN requires SQLite >= 3.35.0 and is not guaranteed
-- across all deployment environments. The column is harmless and will be
-- removed in a future full schema rebuild if needed.
