-- +goose Up
CREATE TABLE transcode_profiles (
    id               TEXT PRIMARY KEY,
    name             TEXT NOT NULL UNIQUE,
    width            INTEGER NOT NULL,
    height           INTEGER NOT NULL,
    video_bitrate_k  INTEGER NOT NULL,
    audio_bitrate_k  INTEGER NOT NULL,
    codec            TEXT NOT NULL DEFAULT 'h264',
    is_active        INTEGER NOT NULL DEFAULT 1,
    created_at       TEXT NOT NULL,
    updated_at       TEXT NOT NULL
);

-- Seed with default profiles (corresponding to H.264 standard renditions)
INSERT INTO transcode_profiles (id, name, width, height, video_bitrate_k, audio_bitrate_k, codec, is_active, created_at, updated_at) VALUES
('53a7bcf1-6927-46e3-a212-be007e05ad90', '360p', 640, 360, 400, 64, 'h264', 1, datetime('now'), datetime('now')),
('b2b78dc9-75bd-44a6-b51f-d23be3d414a3', '480p', 854, 480, 800, 96, 'h264', 1, datetime('now'), datetime('now')),
('021946fe-cf98-449b-be0e-a4b5ff4f932e', '720p', 1280, 720, 2500, 128, 'h264', 1, datetime('now'), datetime('now')),
('8df6b490-9c2b-426b-88a2-23c21a4fa0a8', '1080p', 1920, 1080, 8000, 192, 'h264', 1, datetime('now'), datetime('now')),
('f9c5bdf1-b4d4-47b1-9b16-43b9e4a8d0b2', '360p (AV1)', 640, 360, 300, 64, 'av1', 0, datetime('now'), datetime('now')),
('d8c5bde2-d34e-4f1b-ba76-2e11a3d9bc43', '480p (AV1)', 854, 480, 600, 96, 'av1', 0, datetime('now'), datetime('now')),
('c7a5cdf3-c24d-481b-8c46-1e09a2d8bc22', '720p (AV1)', 1280, 720, 1800, 128, 'av1', 0, datetime('now'), datetime('now')),
('b6a4bdf4-b13c-471a-9b36-0d08a1d7bc11', '1080p (AV1)', 1920, 1080, 5000, 192, 'av1', 0, datetime('now'), datetime('now')),
('a5a3cdf5-a02b-461a-8a26-cd07a0d6bc00', '4k (AV1)', 3840, 2160, 15000, 192, 'av1', 0, datetime('now'), datetime('now'));

-- +goose Down
DROP TABLE IF EXISTS transcode_profiles;
