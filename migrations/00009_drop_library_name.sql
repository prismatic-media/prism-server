-- +goose Up
ALTER TABLE libraries DROP COLUMN name;

-- +goose Down
ALTER TABLE libraries ADD COLUMN name TEXT;
