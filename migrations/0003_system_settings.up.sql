CREATE TABLE IF NOT EXISTS system_settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
INSERT INTO system_settings (key, value) VALUES ('initialized', 'false');
INSERT INTO system_settings (key, value) VALUES ('allow_registration', 'false');
