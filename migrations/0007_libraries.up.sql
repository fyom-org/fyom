-- Libraries are the organizational unit that binds storage, metadata rules,
-- and access permissions together. Every media item belongs to exactly one
-- library.
--
-- Important migration note:
-- This historical migration intentionally does not declare a foreign key from
-- libraries.provider_id to providers.id.
--
-- Reason:
-- The default library uses provider_id = 'local', but the built-in local
-- provider is introduced by a later migration. Enforcing the provider foreign
-- key here makes fresh database creation fail when PRAGMA foreign_keys = ON.
--
-- Runtime code should still treat provider_id as a provider reference. A later
-- migration may safely rebuild this table with a foreign key after the local
-- provider is guaranteed to exist.

CREATE TABLE IF NOT EXISTS libraries (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    type            TEXT NOT NULL DEFAULT 'mixed' CHECK(type IN ('movie', 'show', 'mixed')),
    provider_id     TEXT NOT NULL DEFAULT 'local',
    source_path     TEXT NOT NULL,
    metadata_source TEXT NOT NULL DEFAULT 'nfo' CHECK(metadata_source IN ('nfo', 'filename')),
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Default library for all existing media items.
--
-- INSERT OR IGNORE keeps this migration safe if a development database was
-- manually initialized with the default library before migrations are applied.
INSERT OR IGNORE INTO libraries (
    id,
    name,
    type,
    provider_id,
    source_path,
    metadata_source
)
VALUES (
    'default',
    'Default Library',
    'mixed',
    'local',
    '/',
    'nfo'
);

-- Add library_id to media_items, defaulting all existing and future rows to the
-- default library.
--
-- SQLite does not support ADD COLUMN IF NOT EXISTS in older versions, so this
-- migration relies on schema_migrations to ensure it only runs once.
ALTER TABLE media_items
ADD COLUMN library_id TEXT NOT NULL DEFAULT 'default';

-- Ensure pre-existing rows are explicitly assigned to the default library.
-- This is mostly defensive because the NOT NULL DEFAULT should already backfill
-- existing rows in SQLite.
UPDATE media_items
SET library_id = 'default'
WHERE library_id IS NULL OR library_id = '';

CREATE INDEX IF NOT EXISTS idx_media_library
ON media_items(library_id);

-- Also add library_id to import_jobs so we know which library a job was
-- importing into.
ALTER TABLE import_jobs
ADD COLUMN library_id TEXT NOT NULL DEFAULT 'default';

-- Ensure pre-existing import jobs are explicitly assigned to the default
-- library.
UPDATE import_jobs
SET library_id = 'default'
WHERE library_id IS NULL OR library_id = '';

CREATE INDEX IF NOT EXISTS idx_import_jobs_library
ON import_jobs(library_id);
