-- Migration 0017: Persist import summary on import jobs

ALTER TABLE import_jobs ADD COLUMN scanned_files INTEGER NOT NULL DEFAULT 0;
ALTER TABLE import_jobs ADD COLUMN imported_items INTEGER NOT NULL DEFAULT 0;
ALTER TABLE import_jobs ADD COLUMN updated_items INTEGER NOT NULL DEFAULT 0;
ALTER TABLE import_jobs ADD COLUMN skipped_files INTEGER NOT NULL DEFAULT 0;
ALTER TABLE import_jobs ADD COLUMN parse_warnings TEXT NOT NULL DEFAULT '[]';
ALTER TABLE import_jobs ADD COLUMN duration_ms INTEGER NOT NULL DEFAULT 0;
