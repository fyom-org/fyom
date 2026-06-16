-- Migration: Remove password_change_required from users table (rollback)

ALTER TABLE users
DROP COLUMN password_change_required;
