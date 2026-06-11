-- Enhanced metadata from NFO parsing
ALTER TABLE media_items ADD COLUMN mpaa TEXT;
ALTER TABLE media_items ADD COLUMN genres TEXT;            -- JSON array: ["Action","Sci-Fi"]
ALTER TABLE media_items ADD COLUMN studios TEXT;           -- JSON array: ["Warner Bros."]
ALTER TABLE media_items ADD COLUMN actors TEXT;            -- JSON array: [{"name":"...","role":"..."}]
ALTER TABLE media_items ADD COLUMN unique_ids TEXT;        -- JSON object: {"imdb":"tt0816692","tmdb":"157336"}
ALTER TABLE media_items ADD COLUMN premiered TEXT;
ALTER TABLE media_items ADD COLUMN outline TEXT;
ALTER TABLE media_items ADD COLUMN tagline TEXT;
ALTER TABLE media_items ADD COLUMN countries TEXT;         -- JSON array
ALTER TABLE media_items ADD COLUMN directors TEXT;         -- JSON array
ALTER TABLE media_items ADD COLUMN credits TEXT;           -- JSON array (writers)
ALTER TABLE media_items ADD COLUMN tags TEXT;              -- JSON array
ALTER TABLE media_items ADD COLUMN set_name TEXT;          -- Collection/set name
ALTER TABLE media_items ADD COLUMN video_codec TEXT;
ALTER TABLE media_items ADD COLUMN video_width INTEGER;
ALTER TABLE media_items ADD COLUMN video_height INTEGER;
ALTER TABLE media_items ADD COLUMN video_duration_seconds INTEGER;
ALTER TABLE media_items ADD COLUMN audio_codec TEXT;
ALTER TABLE media_items ADD COLUMN audio_channels INTEGER;
ALTER TABLE media_items ADD COLUMN subtitle_languages TEXT; -- JSON array: ["eng","spa"]
