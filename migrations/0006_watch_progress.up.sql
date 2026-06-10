CREATE TABLE IF NOT EXISTS watch_progress (
    user_id       TEXT NOT NULL,
    media_item_id TEXT NOT NULL,
    position      INTEGER NOT NULL DEFAULT 0,
    duration      INTEGER NOT NULL DEFAULT 0,
    finished      INTEGER NOT NULL DEFAULT 0,
    updated_at    TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (user_id, media_item_id),
    FOREIGN KEY (user_id) REFERENCES users(id),
    FOREIGN KEY (media_item_id) REFERENCES media_items(id)
);
