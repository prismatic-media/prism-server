-- +goose Up
ALTER TABLE transcode_workers ADD COLUMN is_ephemeral INTEGER NOT NULL DEFAULT 0;

CREATE TABLE ephemeral_worker_tokens (
    id         TEXT PRIMARY KEY,
    token      TEXT NOT NULL UNIQUE,
    name       TEXT NOT NULL,
    created_at TEXT NOT NULL
);

-- +goose Down
DROP TABLE IF EXISTS ephemeral_worker_tokens;
ALTER TABLE transcode_workers DROP COLUMN is_ephemeral;
