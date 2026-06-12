-- +goose Up

CREATE TABLE tv_shows (
    id             TEXT PRIMARY KEY,
    library_id     TEXT NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
    name           TEXT NOT NULL,
    tmdb_id        INTEGER,
    overview       TEXT,
    poster_path    TEXT,
    first_air_year INTEGER,
    created_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    UNIQUE (library_id, name)
);

CREATE INDEX idx_tv_shows_library_id ON tv_shows (library_id);

CREATE TABLE tv_seasons (
    id            TEXT PRIMARY KEY,
    tv_show_id    TEXT NOT NULL REFERENCES tv_shows(id) ON DELETE CASCADE,
    season_number INTEGER NOT NULL,
    tmdb_id       INTEGER,
    overview      TEXT,
    poster_path   TEXT,
    created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    UNIQUE (tv_show_id, season_number)
);

CREATE INDEX idx_tv_seasons_tv_show_id ON tv_seasons (tv_show_id);

ALTER TABLE media_items ADD COLUMN tv_show_id    TEXT;
ALTER TABLE media_items ADD COLUMN tv_season_id  TEXT;
ALTER TABLE media_items ADD COLUMN season_number  INTEGER;
ALTER TABLE media_items ADD COLUMN episode_number INTEGER;

-- +goose Down

ALTER TABLE media_items DROP COLUMN episode_number;
ALTER TABLE media_items DROP COLUMN season_number;
ALTER TABLE media_items DROP COLUMN tv_season_id;
ALTER TABLE media_items DROP COLUMN tv_show_id;
DROP TABLE IF EXISTS tv_seasons;
DROP TABLE IF EXISTS tv_shows;
