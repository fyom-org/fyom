CREATE TABLE IF NOT EXISTS user_media_status (
    user_id       TEXT NOT NULL,
    media_item_id TEXT NOT NULL,
    status        TEXT NOT NULL DEFAULT 'none'
      CHECK(status IN ('none', 'want_to_watch', 'watching', 'watched', 'dropped')),
    created_at    TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at    TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (user_id, media_item_id),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (media_item_id) REFERENCES media_items(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_ums_user_status ON user_media_status(user_id, status);