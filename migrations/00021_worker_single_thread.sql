-- +goose Up
UPDATE transcode_workers SET threads = 1;

-- +goose Down
-- no-op
