-- +goose Up
-- SQLite does not support directly modifying CHECK constraints. We must do a table recreation.
ALTER TABLE transcode_sub_jobs RENAME TO transcode_sub_jobs_old;

CREATE TABLE transcode_sub_jobs (
    id               TEXT PRIMARY KEY,
    job_id           TEXT NOT NULL REFERENCES transcode_jobs(id) ON DELETE CASCADE,
    worker_id        TEXT REFERENCES transcode_workers(id) ON DELETE SET NULL,
    type             TEXT NOT NULL CHECK (type IN ('video', 'subtitles', 'whisper')),
    profile_id       TEXT REFERENCES transcode_profiles(id) ON DELETE SET NULL,
    profile_name     TEXT,
    width            INTEGER,
    height           INTEGER,
    video_bitrate_k  INTEGER,
    audio_bitrate_k  INTEGER,
    codec            TEXT,
    status           TEXT NOT NULL DEFAULT 'pending'
                          CHECK (status IN ('pending', 'processing', 'done', 'failed')),
    progress         REAL NOT NULL DEFAULT 0,
    error_msg        TEXT,
    started_at       TEXT,
    finished_at      TEXT,
    created_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

INSERT INTO transcode_sub_jobs (
    id, job_id, worker_id, type, profile_id, profile_name, width, height,
    video_bitrate_k, audio_bitrate_k, codec, status, progress, error_msg,
    started_at, finished_at, created_at
)
SELECT
    id, job_id, worker_id, type, profile_id, profile_name, width, height,
    video_bitrate_k, audio_bitrate_k, codec, status, progress, error_msg,
    started_at, finished_at, created_at
FROM transcode_sub_jobs_old;

DROP TABLE transcode_sub_jobs_old;

CREATE INDEX idx_transcode_sub_jobs_job_id ON transcode_sub_jobs (job_id);
CREATE INDEX idx_transcode_sub_jobs_status ON transcode_sub_jobs (status);

-- Create whisper_transcriptions table
CREATE TABLE whisper_transcriptions (
    id             TEXT PRIMARY KEY,
    media_item_id  TEXT NOT NULL UNIQUE REFERENCES media_items(id) ON DELETE CASCADE,
    language       TEXT NOT NULL,
    vtt_content    TEXT NOT NULL,
    created_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

-- Insert new configurable settings for Whisper
INSERT OR IGNORE INTO settings (key, value) VALUES ('whisper_default_language', 'en');
INSERT OR IGNORE INTO settings (key, value) VALUES ('whisper_model', 'base');

-- +goose Down
DROP TABLE IF EXISTS whisper_transcriptions;
