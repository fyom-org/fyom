ALTER TABLE media_items ADD COLUMN status TEXT NOT NULL DEFAULT 'available'
  CHECK(status IN ('available', 'missing', 'unknown'));

CREATE INDEX IF NOT EXISTS idx_media_status ON media_items(status);
