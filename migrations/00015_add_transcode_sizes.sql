-- +goose Up
ALTER TABLE media_items ADD COLUMN transcode_sizes TEXT;

-- +goose Down
ALTER TABLE media_items DROP COLUMN transcode_sizes;
