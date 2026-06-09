-- Promote the earliest created user to admin
UPDATE users SET role = 'admin' WHERE id = (
    SELECT id FROM users ORDER BY created_at ASC LIMIT 1
);
