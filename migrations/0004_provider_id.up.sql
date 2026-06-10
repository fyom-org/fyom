-- Every media item belongs to exactly one provider.
-- 'local' is the default for items imported from the local filesystem.
ALTER TABLE media_items ADD COLUMN provider_id TEXT NOT NULL DEFAULT 'local';

CREATE INDEX IF NOT EXISTS idx_media_provider ON media_items(provider_id);
