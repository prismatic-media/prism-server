-- +goose Up
CREATE TABLE media_subtitles (
    id             TEXT PRIMARY KEY,
    media_item_id  TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    language       TEXT NOT NULL,
    label          TEXT NOT NULL,
    vtt_content    TEXT NOT NULL,
    created_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE INDEX idx_media_subtitles_media_item_id ON media_subtitles (media_item_id);

-- +goose Down
DROP INDEX IF EXISTS idx_media_subtitles_media_item_id;
DROP TABLE IF EXISTS media_subtitles;
