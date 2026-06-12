-- +goose Up

CREATE TABLE storage_areas (
    id         TEXT PRIMARY KEY,
    kind       TEXT NOT NULL CHECK (kind IN ('segments', 'thumbnails')),
    path       TEXT NOT NULL UNIQUE,
    enabled    INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE INDEX idx_storage_areas_kind_enabled ON storage_areas (kind, enabled);

-- +goose Down

DROP TABLE IF EXISTS storage_areas;
