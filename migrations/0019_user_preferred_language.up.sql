-- Migration: Add preferred_language column to users table + seed default_locale
--
-- Part 1: users.preferred_language
-- Stores the user's explicitly chosen UI locale (e.g. "en", "zh").
-- Empty string means "no preference" — the system falls back to
-- system_settings.default_locale, then navigator.language, then "en".
--
-- Part 2: system_settings.default_locale
-- Seeds the admin-configurable system default locale. Admins update this
-- via PUT /api/v1/admin/settings { "default_locale": "zh" }. Until changed,
-- the value is "en" (the source locale).
--
-- This migration is i18n Phase 1. See docs/i18n.md section 3 (Locale Resolution
-- Chain) for the full priority order.

ALTER TABLE users
ADD COLUMN preferred_language TEXT NOT NULL DEFAULT '';

INSERT OR IGNORE INTO system_settings (key, value) VALUES ('default_locale', 'en');
