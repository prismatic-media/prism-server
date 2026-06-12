-- +goose Up
-- +goose NO TRANSACTION

PRAGMA foreign_keys = OFF;

BEGIN TRANSACTION;

-- Create temporary table with updated transcode_status default and CHECK constraint
CREATE TABLE media_items_new (
    id               TEXT PRIMARY KEY,
    library_id       TEXT NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
    title            TEXT NOT NULL,
    media_type       TEXT NOT NULL,
    file_path        TEXT NOT NULL UNIQUE,
    file_size        INTEGER NOT NULL DEFAULT 0,
    duration         REAL NOT NULL DEFAULT 0,
    width            INTEGER NOT NULL DEFAULT 0,
    height           INTEGER NOT NULL DEFAULT 0,
    video_codec      TEXT NOT NULL DEFAULT '',
    audio_codec      TEXT NOT NULL DEFAULT '',
    tmdb_id          INTEGER,
    year             INTEGER,
    overview         TEXT,
    poster_path      TEXT,
    transcode_status TEXT NOT NULL DEFAULT 'none'
                          CHECK (transcode_status IN ('none', 'pending', 'processing', 'done', 'failed')),
    mpd_path         TEXT,
    created_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    tv_show_id       TEXT,
    tv_season_id     TEXT,
    season_number    INTEGER,
    episode_number   INTEGER,
    source_fingerprint TEXT,
    source_status    TEXT NOT NULL DEFAULT 'available' CHECK(source_status IN ('available', 'missing')),
    bundle_status    TEXT NOT NULL DEFAULT 'none' CHECK(bundle_status IN ('none', 'available', 'missing')),
    director         TEXT,
    cast_members     TEXT,
    backdrop_path    TEXT,
    extra_posters    TEXT
);

-- Copy all existing data
INSERT INTO media_items_new (
    id, library_id, title, media_type, file_path, file_size, duration, width, height,
    video_codec, audio_codec, tmdb_id, year, overview, poster_path, transcode_status,
    mpd_path, created_at, updated_at, tv_show_id, tv_season_id, season_number,
    episode_number, source_fingerprint, source_status, bundle_status, director,
    cast_members, backdrop_path, extra_posters
)
SELECT 
    id, library_id, title, media_type, file_path, file_size, duration, width, height,
    video_codec, audio_codec, tmdb_id, year, overview, poster_path, transcode_status,
    mpd_path, created_at, updated_at, tv_show_id, tv_season_id, season_number,
    episode_number, source_fingerprint, source_status, bundle_status, director,
    cast_members, backdrop_path, extra_posters
FROM media_items;

-- Drop old table
DROP TABLE media_items;

-- Rename new table to original
ALTER TABLE media_items_new RENAME TO media_items;

-- Recreate indexes
CREATE INDEX idx_media_items_library_id       ON media_items (library_id);
CREATE INDEX idx_media_items_transcode_status ON media_items (transcode_status);

-- Update items currently marked 'pending' that do not actually have an enqueued job to 'none'
UPDATE media_items
SET transcode_status = 'none'
WHERE transcode_status = 'pending'
  AND NOT EXISTS (
      SELECT 1 FROM transcode_jobs WHERE media_item_id = media_items.id
  );

COMMIT;

PRAGMA foreign_keys = ON;

-- +goose Down
-- +goose NO TRANSACTION

PRAGMA foreign_keys = OFF;

BEGIN TRANSACTION;

CREATE TABLE media_items_old (
    id               TEXT PRIMARY KEY,
    library_id       TEXT NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
    title            TEXT NOT NULL,
    media_type       TEXT NOT NULL,
    file_path        TEXT NOT NULL UNIQUE,
    file_size        INTEGER NOT NULL DEFAULT 0,
    duration         REAL NOT NULL DEFAULT 0,
    width            INTEGER NOT NULL DEFAULT 0,
    height           INTEGER NOT NULL DEFAULT 0,
    video_codec      TEXT NOT NULL DEFAULT '',
    audio_codec      TEXT NOT NULL DEFAULT '',
    tmdb_id          INTEGER,
    year             INTEGER,
    overview         TEXT,
    poster_path      TEXT,
    transcode_status TEXT NOT NULL DEFAULT 'pending'
                          CHECK (transcode_status IN ('pending', 'processing', 'done', 'failed')),
    mpd_path         TEXT,
    created_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    tv_show_id       TEXT,
    tv_season_id     TEXT,
    season_number    INTEGER,
    episode_number   INTEGER,
    source_fingerprint TEXT,
    source_status    TEXT NOT NULL DEFAULT 'available' CHECK(source_status IN ('available', 'missing')),
    bundle_status    TEXT NOT NULL DEFAULT 'none' CHECK(bundle_status IN ('none', 'available', 'missing')),
    director         TEXT,
    cast_members     TEXT,
    backdrop_path    TEXT,
    extra_posters    TEXT
);

INSERT INTO media_items_old (
    id, library_id, title, media_type, file_path, file_size, duration, width, height,
    video_codec, audio_codec, tmdb_id, year, overview, poster_path, transcode_status,
    mpd_path, created_at, updated_at, tv_show_id, tv_season_id, season_number,
    episode_number, source_fingerprint, source_status, bundle_status, director,
    cast_members, backdrop_path, extra_posters
)
SELECT 
    id, library_id, title, media_type, file_path, file_size, duration, width, height,
    video_codec, audio_codec, tmdb_id, year, overview, poster_path,
    CASE WHEN transcode_status = 'none' THEN 'pending' ELSE transcode_status END,
    mpd_path, created_at, updated_at, tv_show_id, tv_season_id, season_number,
    episode_number, source_fingerprint, source_status, bundle_status, director,
    cast_members, backdrop_path, extra_posters
FROM media_items;

DROP TABLE media_items;

ALTER TABLE media_items_old RENAME TO media_items;

CREATE INDEX idx_media_items_library_id       ON media_items (library_id);
CREATE INDEX idx_media_items_transcode_status ON media_items (transcode_status);

COMMIT;

PRAGMA foreign_keys = ON;
