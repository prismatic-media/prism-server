-- +goose Up

CREATE TABLE users (
    id            TEXT PRIMARY KEY,
    username      TEXT NOT NULL UNIQUE,
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    is_admin      INTEGER NOT NULL DEFAULT 0,
    created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE TABLE libraries (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    path        TEXT NOT NULL UNIQUE,
    media_type  TEXT NOT NULL CHECK (media_type IN ('movie', 'tvshow', 'music')),
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE TABLE media_items (
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
    updated_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE INDEX idx_media_items_library_id       ON media_items (library_id);
CREATE INDEX idx_media_items_transcode_status ON media_items (transcode_status);

CREATE TABLE transcode_jobs (
    id            TEXT PRIMARY KEY,
    media_item_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    status        TEXT NOT NULL DEFAULT 'pending'
                       CHECK (status IN ('pending', 'processing', 'done', 'failed')),
    progress      REAL NOT NULL DEFAULT 0,
    error_msg     TEXT,
    started_at    TEXT,
    finished_at   TEXT,
    created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE INDEX idx_transcode_jobs_media_item_id ON transcode_jobs (media_item_id);
CREATE INDEX idx_transcode_jobs_status        ON transcode_jobs (status);

CREATE TABLE watch_history (
    id            TEXT PRIMARY KEY,
    user_id       TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    media_item_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    position      REAL NOT NULL DEFAULT 0,
    completed     INTEGER NOT NULL DEFAULT 0,
    updated_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    UNIQUE (user_id, media_item_id)
);

CREATE INDEX idx_watch_history_user_id ON watch_history (user_id);

-- Replaces Redis for JWT refresh-token revocation.
-- Tokens are identified by a SHA-256 hash of the opaque token value.
CREATE TABLE refresh_tokens (
    id         TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TEXT NOT NULL,
    revoked    INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE INDEX idx_refresh_tokens_user_id    ON refresh_tokens (user_id);
CREATE INDEX idx_refresh_tokens_token_hash ON refresh_tokens (token_hash);

-- +goose Down

DROP TABLE IF EXISTS refresh_tokens;
DROP TABLE IF EXISTS watch_history;
DROP TABLE IF EXISTS transcode_jobs;
DROP TABLE IF EXISTS media_items;
DROP TABLE IF EXISTS libraries;
DROP TABLE IF EXISTS users;
