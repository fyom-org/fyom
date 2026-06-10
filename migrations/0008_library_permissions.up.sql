CREATE TABLE IF NOT EXISTS library_permissions (
    user_id    TEXT NOT NULL,
    library_id TEXT NOT NULL,
    can_view   INTEGER NOT NULL DEFAULT 1,
    PRIMARY KEY (user_id, library_id),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (library_id) REFERENCES libraries(id) ON DELETE CASCADE
);

INSERT INTO library_permissions (user_id, library_id, can_view)
SELECT u.id, l.id, 1
FROM users u
CROSS JOIN libraries l
WHERE NOT EXISTS (
    SELECT 1 FROM library_permissions lp
    WHERE lp.user_id = u.id AND lp.library_id = l.id
);
