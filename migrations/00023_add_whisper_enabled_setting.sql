-- +goose Up
INSERT OR IGNORE INTO settings (key, value) VALUES ('whisper_enabled', 'false');

-- +goose Down
DELETE FROM settings WHERE key = 'whisper_enabled';
