-- +goose Up
ALTER TABLE media_subtitles ADD COLUMN similarity_score REAL;
ALTER TABLE media_subtitles ADD COLUMN sync_offset REAL NOT NULL DEFAULT 0.0;
ALTER TABLE media_subtitles ADD COLUMN alignment_status TEXT NOT NULL DEFAULT 'pending';

-- +goose Down
-- SQLite does not easily support dropping columns in older versions, but for the down migration, we can leave them or reconstruct if needed.
-- Since Goose migrations are usually additive and SQLite doesn't support ALTER TABLE DROP COLUMN before v3.35.0, we can use simple comment or empty block.
