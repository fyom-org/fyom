-- Roll back library support.
--
-- SQLite versions before 3.35 do not support ALTER TABLE DROP COLUMN, so this
-- migration rebuilds affected tables without library_id.

DROP INDEX IF EXISTS idx_import_jobs_library;
DROP INDEX IF EXISTS idx_media_library;

-- Rebuild media_items without library_id.
CREATE TABLE media_items_rollback_0007 (
    id                  TEXT PRIMARY KEY,
    provider_id          TEXT NOT NULL,
    external_id          TEXT NOT NULL,
    title               TEXT NOT NULL,
    original_title      TEXT,
    media_type          TEXT NOT NULL,
    year                INTEGER,
    overview            TEXT,
    poster_path         TEXT,
    backdrop_path       TEXT,
    file_path           TEXT,
    duration_seconds    INTEGER,
    created_at          TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at          TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(provider_id, external_id)
);

INSERT INTO media_items_rollback_0007 (
    id,
    provider_id,
    external_id,
    title,
    original_title,
    media_type,
    year,
    overview,
    poster_path,
    backdrop_path,
    file_path,
    duration_seconds,
    created_at,
    updated_at
)
SELECT
    id,
    provider_id,
    external_id,
    title,
    original_title,
    media_type,
    year,
    overview,
    poster_path,
    backdrop_path,
    file_path,
    duration_seconds,
    created_at,
    updated_at
FROM media_items;

DROP TABLE media_items;

ALTER TABLE media_items_rollback_0007
RENAME TO media_items;

CREATE INDEX IF NOT EXISTS idx_media_provider
ON media_items(provider_id, external_id);

CREATE INDEX IF NOT EXISTS idx_media_type
ON media_items(media_type);

CREATE INDEX IF NOT EXISTS idx_media_title
ON media_items(title);

-- Rebuild import_jobs without library_id.
CREATE TABLE import_jobs_rollback_0007 (
    id              TEXT PRIMARY KEY,
    provider_id     TEXT NOT NULL,
    status          TEXT NOT NULL,
    total_items     INTEGER NOT NULL DEFAULT 0,
    processed_items INTEGER NOT NULL DEFAULT 0,
    error_message   TEXT,
    started_at      TEXT NOT NULL DEFAULT (datetime('now')),
    completed_at    TEXT
);

INSERT INTO import_jobs_rollback_0007 (
    id,
    provider_id,
    status,
    total_items,
    processed_items,
    error_message,
    started_at,
    completed_at
)
SELECT
    id,
    provider_id,
    status,
    total_items,
    processed_items,
    error_message,
    started_at,
    completed_at
FROM import_jobs;

DROP TABLE import_jobs;

ALTER TABLE import_jobs_rollback_0007
RENAME TO import_jobs;

CREATE INDEX IF NOT EXISTS idx_import_jobs_provider
ON import_jobs(provider_id);

CREATE INDEX IF NOT EXISTS idx_import_jobs_status
ON import_jobs(status);

DROP TABLE IF EXISTS libraries;
