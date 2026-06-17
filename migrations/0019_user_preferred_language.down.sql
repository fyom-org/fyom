-- Migration: Remove preferred_language from users + default_locale setting (rollback)

DELETE FROM system_settings WHERE key = 'default_locale';

ALTER TABLE users
DROP COLUMN preferred_language;
