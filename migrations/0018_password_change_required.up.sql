-- Migration: Add password_change_required to users table
-- This column tracks whether a user must change their password on next login
-- (e.g., after first-run bootstrap creates an admin with a generated password)

ALTER TABLE users
ADD COLUMN password_change_required BOOLEAN NOT NULL DEFAULT FALSE;
