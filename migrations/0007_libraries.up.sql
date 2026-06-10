-- Libraries are the organizational unit that binds storage, metadata rules,
-- and access permissions together. Every media item belongs to exactly one library.
CREATE TABLE IF NOT EXISTS libraries (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    type            TEXT NOT NULL DEFAULT 'mixed' CHECK(type IN ('movie', 'show', 'mixed')),
    provider_id     TEXT NOT NULL DEFAULT 'local',
    source_path     TEXT NOT NULL,
    metadata_source TEXT NOT NULL DEFAULT 'nfo' CHECK(metadata_source IN ('nfo', 'filename')),
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at      TEXT NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY (provider_id) REFERENCES providers(id) ON DELETE RESTRICT
);

-- Default library for all existing media items
INSERT INTO libraries (id, name, type, provider_id, source_path, metadata_source)
VALUES ('default', 'Default Library', 'mixed', 'local', '/', 'nfo');

-- Add library_id to media_items, defaulting all existing items to the default library
ALTER TABLE media_items ADD COLUMN library_id TEXT NOT NULL DEFAULT 'default';
CREATE INDEX IF NOT EXISTS idx_media_library ON media_items(library_id);

-- Also add library_id to import_jobs so we know which library a job was importing into
ALTER TABLE import_jobs ADD COLUMN library_id TEXT NOT NULL DEFAULT 'default';
