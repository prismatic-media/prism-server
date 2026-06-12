-- +goose Up
PRAGMA foreign_keys = OFF;

-- delete thumbnails entries
DELETE FROM storage_areas WHERE kind = 'thumbnails';

-- recreate the table with check constraint restricted to segments
CREATE TABLE storage_areas_new (
    id         TEXT PRIMARY KEY,
    kind       TEXT NOT NULL CHECK (kind = 'segments'),
    path       TEXT NOT NULL UNIQUE,
    enabled    INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

INSERT INTO storage_areas_new (id, kind, path, enabled, created_at, updated_at)
SELECT id, kind, path, enabled, created_at, updated_at
FROM storage_areas;

DROP TABLE storage_areas;
ALTER TABLE storage_areas_new RENAME TO storage_areas;
CREATE INDEX idx_storage_areas_kind_enabled ON storage_areas (kind, enabled);

PRAGMA foreign_keys = ON;

-- +goose Down
PRAGMA foreign_keys = OFF;

CREATE TABLE storage_areas_old (
    id         TEXT PRIMARY KEY,
    kind       TEXT NOT NULL CHECK (kind IN ('segments', 'thumbnails')),
    path       TEXT NOT NULL UNIQUE,
    enabled    INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

INSERT INTO storage_areas_old (id, kind, path, enabled, created_at, updated_at)
SELECT id, kind, path, enabled, created_at, updated_at
FROM storage_areas;

DROP TABLE storage_areas;
ALTER TABLE storage_areas_old RENAME TO storage_areas;
CREATE INDEX idx_storage_areas_kind_enabled ON storage_areas (kind, enabled);

PRAGMA foreign_keys = ON;
