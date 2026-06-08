-- Create users table
CREATE TABLE IF NOT EXISTS users (
    id         TEXT PRIMARY KEY,
    username   TEXT NOT NULL UNIQUE,
    password   TEXT NOT NULL,
    role       TEXT NOT NULL DEFAULT 'user',
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Create media_items table
CREATE TABLE IF NOT EXISTS media_items (
    id              TEXT PRIMARY KEY,
    type            TEXT NOT NULL CHECK(type IN ('movie', 'episode', 'show')),
    title           TEXT NOT NULL,
    sort_title      TEXT,
    year            INTEGER,
    overview        TEXT,
    rating          REAL,
    duration        INTEGER,
    file_path       TEXT NOT NULL UNIQUE,
    poster_path     TEXT,
    backdrop_path   TEXT,
    parent_id       TEXT,
    season          INTEGER,
    episode         INTEGER,
    metadata_source TEXT,
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at      TEXT NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY (parent_id) REFERENCES media_items(id)
);

-- Create import_jobs table
CREATE TABLE IF NOT EXISTS import_jobs (
    id          TEXT PRIMARY KEY,
    source_path TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending', 'running', 'done', 'error')),
    total_items INTEGER DEFAULT 0,
    done_items  INTEGER DEFAULT 0,
    error_msg   TEXT,
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_media_type ON media_items(type);
CREATE INDEX IF NOT EXISTS idx_media_title ON media_items(title);
CREATE INDEX IF NOT EXISTS idx_media_parent ON media_items(parent_id);
CREATE INDEX IF NOT EXISTS idx_import_status ON import_jobs(status);
