-- +goose Up
ALTER TABLE media_items ADD COLUMN director TEXT;
ALTER TABLE media_items ADD COLUMN cast_members TEXT;
ALTER TABLE media_items ADD COLUMN backdrop_path TEXT;
ALTER TABLE media_items ADD COLUMN extra_posters TEXT;

ALTER TABLE tv_shows ADD COLUMN director TEXT;
ALTER TABLE tv_shows ADD COLUMN cast_members TEXT;
ALTER TABLE tv_shows ADD COLUMN backdrop_path TEXT;
ALTER TABLE tv_shows ADD COLUMN extra_posters TEXT;

-- +goose Down
ALTER TABLE tv_shows DROP COLUMN extra_posters;
ALTER TABLE tv_shows DROP COLUMN backdrop_path;
ALTER TABLE tv_shows DROP COLUMN cast_members;
ALTER TABLE tv_shows DROP COLUMN director;

ALTER TABLE media_items DROP COLUMN extra_posters;
ALTER TABLE media_items DROP COLUMN backdrop_path;
ALTER TABLE media_items DROP COLUMN cast_members;
ALTER TABLE media_items DROP COLUMN director;
