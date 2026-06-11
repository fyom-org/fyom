-- Migration 0015: Movie NFO Enhancement
-- Adds missing columns for full Jellyfin NFO spec compliance for movies.

ALTER TABLE media_items ADD COLUMN set_overview TEXT NOT NULL DEFAULT '';
ALTER TABLE media_items ADD COLUMN language TEXT NOT NULL DEFAULT '';
ALTER TABLE media_items ADD COLUMN country_code TEXT NOT NULL DEFAULT '';
ALTER TABLE media_items ADD COLUMN custom_rating TEXT NOT NULL DEFAULT '';
ALTER TABLE media_items ADD COLUMN collection_number TEXT NOT NULL DEFAULT '';
ALTER TABLE media_items ADD COLUMN end_date TEXT NOT NULL DEFAULT '';
ALTER TABLE media_items ADD COLUMN release_date TEXT NOT NULL DEFAULT '';
ALTER TABLE media_items ADD COLUMN display_order TEXT NOT NULL DEFAULT '';
ALTER TABLE media_items ADD COLUMN original_title TEXT NOT NULL DEFAULT '';
ALTER TABLE media_items ADD COLUMN user_rating REAL NOT NULL DEFAULT 0;
ALTER TABLE media_items ADD COLUMN date_added TEXT NOT NULL DEFAULT '';
ALTER TABLE media_items ADD COLUMN last_played TEXT NOT NULL DEFAULT '';
ALTER TABLE media_items ADD COLUMN playcount INTEGER NOT NULL DEFAULT 0;
