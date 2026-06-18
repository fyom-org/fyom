-- Migration 0016: Add media item path semantics and guarantee local provider.
--
-- This migration and ensures
-- the built-in local provider exists before runtime foreign key enforcement
-- depends on provider_id = 'local'.
--
-- Scope:
--   - Seed and normalize the built-in local provider.
--   - Ensure the default library exists and points to the local provider.
--   - Repair orphaned provider/library references on media_items and libraries.
--   - Add root_path, primary_path, and nfo_path to media_items.
--   - Backfill path columns from existing file_path and library source_path.
--
-- Important:
--   Do not touch import_jobs.provider_id here. Historical schemas do not
--   guarantee that import_jobs has a provider_id column at this point.

-- Ensure the built-in local provider exists.
--
-- Use INSERT OR IGNORE instead of UPSERT so this migration stays compatible
-- with older SQLite versions.
INSERT OR IGNORE INTO providers (
    id,
    type,
    display_name,
    config,
    enabled,
    created_at,
    updated_at
)
VALUES (
    'local',
    'local',
    'Local Filesystem',
    '{}',
    1,
    strftime('%Y-%m-%dT%H:%M:%fZ', 'now'),
    strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
);

-- Normalize the local provider row in case it already existed with incomplete
-- or stale values from a development database.
UPDATE providers
SET
    type = 'local',
    display_name = CASE
        WHEN display_name IS NULL OR display_name = '' THEN 'Local Filesystem'
        ELSE display_name
    END,
    config = CASE
        WHEN config IS NULL OR config = '' THEN '{}'
        ELSE config
    END,
    enabled = 1,
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE id = 'local';

-- Normalize the default library so it points at the built-in local provider.
UPDATE libraries
SET
    provider_id = 'local',
    source_path = CASE
        WHEN source_path IS NULL OR source_path = '' THEN '/'
        ELSE source_path
    END,
    metadata_source = CASE
        WHEN metadata_source IS NULL OR metadata_source = '' THEN 'nfo'
        ELSE metadata_source
    END,
    updated_at = datetime('now')
WHERE id = 'default';

-- Repair historical provider references in libraries.
--
-- This is intentionally conservative: only provider_id values that do not map
-- to an existing provider are redirected to the built-in local provider.
UPDATE libraries
SET
    provider_id = 'local',
    updated_at = datetime('now')
WHERE provider_id IS NULL
   OR provider_id = ''
   OR NOT EXISTS (
        SELECT 1
        FROM providers
        WHERE providers.id = libraries.provider_id
   );

-- Repair historical provider references in media_items.
--
-- With runtime foreign key enforcement enabled, media_items.provider_id must
-- reference an existing providers.id. Any orphaned historical provider
-- reference is mapped to the built-in local provider.
UPDATE media_items
SET
    provider_id = 'local',
    updated_at = datetime('now')
WHERE provider_id IS NULL
   OR provider_id = ''
   OR NOT EXISTS (
        SELECT 1
        FROM providers
        WHERE providers.id = media_items.provider_id
   );

-- Ensure every media item has a valid library.
--
-- Migration 0007 added media_items.library_id with a default of 'default'.
-- This repair step protects databases that were created or modified before
-- strict foreign key validation was enabled.
UPDATE media_items
SET
    library_id = 'default',
    updated_at = datetime('now')
WHERE library_id IS NULL
   OR library_id = ''
   OR NOT EXISTS (
        SELECT 1
        FROM libraries
        WHERE libraries.id = media_items.library_id
   );

-- Add new path columns to media_items for explicit path semantics.
--
-- root_path:
--   The logical library or scanner root that owns the item.
--
-- primary_path:
--   The primary playable file path for the item.
--
-- nfo_path:
--   Optional NFO metadata file path. Empty string means no NFO path is known.
--
-- SQLite does not support ADD COLUMN IF NOT EXISTS in older versions, so this
-- migration relies on schema_migrations to run exactly once.
ALTER TABLE media_items
ADD COLUMN root_path TEXT NOT NULL DEFAULT '';

ALTER TABLE media_items
ADD COLUMN primary_path TEXT NOT NULL DEFAULT '';

ALTER TABLE media_items
ADD COLUMN nfo_path TEXT NOT NULL DEFAULT '';

-- Backfill primary_path from the legacy file_path column when available.
UPDATE media_items
SET
    primary_path = file_path,
    updated_at = datetime('now')
WHERE primary_path = ''
  AND file_path IS NOT NULL
  AND file_path <> '';

-- Backfill root_path from the owning library source_path.
UPDATE media_items
SET
    root_path = COALESCE((
        SELECT libraries.source_path
        FROM libraries
        WHERE libraries.id = media_items.library_id
    ), ''),
    updated_at = datetime('now')
WHERE root_path = ''
  AND library_id IS NOT NULL
  AND library_id <> '';

-- If root_path could not be inferred but the item belongs to the default
-- library, use the default filesystem root.
UPDATE media_items
SET
    root_path = '/',
    updated_at = datetime('now')
WHERE root_path = ''
  AND library_id = 'default';

-- Keep NFO path empty for existing rows unless later scanners can infer an
-- exact sidecar metadata path. Guessing here would be unsafe.
UPDATE media_items
SET
    nfo_path = '',
    updated_at = datetime('now')
WHERE nfo_path IS NULL;

-- Indexes for path-based lookups and scanner reconciliation.
CREATE INDEX IF NOT EXISTS idx_media_items_root_path
ON media_items(root_path);

CREATE INDEX IF NOT EXISTS idx_media_items_primary_path
ON media_items(primary_path);

CREATE INDEX IF NOT EXISTS idx_media_items_nfo_path
ON media_items(nfo_path);

CREATE INDEX IF NOT EXISTS idx_media_items_provider_library
ON media_items(provider_id, library_id);
