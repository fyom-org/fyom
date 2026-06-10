CREATE TABLE IF NOT EXISTS providers (
    id           TEXT PRIMARY KEY,
    type         TEXT NOT NULL,
    display_name TEXT NOT NULL,
    config       TEXT NOT NULL DEFAULT '{}',
    enabled      INTEGER NOT NULL DEFAULT 1,
    created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
