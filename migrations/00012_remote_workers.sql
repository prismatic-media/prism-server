-- +goose Up
CREATE TABLE transcode_workers (
    id             TEXT PRIMARY KEY,
    name           TEXT NOT NULL,
    api_key        TEXT NOT NULL UNIQUE,
    threads        INTEGER NOT NULL DEFAULT 2,
    hwaccel        TEXT NOT NULL DEFAULT 'none',
    status         TEXT NOT NULL DEFAULT 'offline',
    last_heartbeat TEXT,
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL
);

ALTER TABLE transcode_jobs ADD COLUMN worker_id TEXT REFERENCES transcode_workers(id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE transcode_jobs DROP COLUMN worker_id;
DROP TABLE IF EXISTS transcode_workers;
