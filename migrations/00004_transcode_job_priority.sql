-- +goose Up

ALTER TABLE transcode_jobs ADD COLUMN priority INTEGER NOT NULL DEFAULT 0;

-- +goose Down

-- SQLite does not support DROP COLUMN in all supported versions.
-- No-op rollback; forward migration only.
SELECT 1;
