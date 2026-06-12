-- +goose Up

CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

-- Existing installs are considered set up only if they already have at least
-- one admin user. If the users table is empty (e.g. the old terminal first-run
-- was never finished), leave setup_complete=false so the web wizard runs.
INSERT INTO settings (key, value)
SELECT 'setup_complete', CASE WHEN EXISTS (SELECT 1 FROM users WHERE is_admin = 1) THEN 'true' ELSE 'false' END;

-- +goose Down

DROP TABLE IF EXISTS settings;
