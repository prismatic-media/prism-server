-- +goose Up
CREATE TABLE transcode_sub_jobs (
    id               TEXT PRIMARY KEY,
    job_id           TEXT NOT NULL REFERENCES transcode_jobs(id) ON DELETE CASCADE,
    worker_id        TEXT REFERENCES transcode_workers(id) ON DELETE SET NULL,
    type             TEXT NOT NULL CHECK (type IN ('video', 'subtitles')),
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

CREATE INDEX idx_transcode_sub_jobs_job_id ON transcode_sub_jobs (job_id);
CREATE INDEX idx_transcode_sub_jobs_status ON transcode_sub_jobs (status);

-- +goose Down
DROP INDEX IF EXISTS idx_transcode_sub_jobs_status;
DROP INDEX IF EXISTS idx_transcode_sub_jobs_job_id;
DROP TABLE IF EXISTS transcode_sub_jobs;
