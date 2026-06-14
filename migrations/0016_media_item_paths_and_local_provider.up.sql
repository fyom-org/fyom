-- Migration 0016: Add path semantics columns and seed local provider

-- Add new path columns to media_items for proper path semantics
ALTER TABLE media_items ADD COLUMN root_path TEXT NOT NULL DEFAULT '';
ALTER TABLE media_items ADD COLUMN primary_path TEXT NOT NULL DEFAULT '';
ALTER TABLE media_items ADD COLUMN nfo_path TEXT NOT NULL DEFAULT '';

-- Seed the local provider if it doesn't exist
INSERT INTO providers (id, type, display_name, config, enabled, created_at, updated_at)
SELECT 'local', 'local', 'Local Filesystem', '{}', 1, datetime('now'), datetime('now')
WHERE NOT EXISTS (SELECT 1 FROM providers WHERE id = 'local');
