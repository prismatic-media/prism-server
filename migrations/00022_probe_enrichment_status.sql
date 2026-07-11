-- +goose Up
ALTER TABLE media_items ADD COLUMN probe_status TEXT NOT NULL DEFAULT 'pending' CHECK(probe_status IN ('none', 'pending', 'processing', 'done', 'failed'));
ALTER TABLE media_items ADD COLUMN enrichment_status TEXT NOT NULL DEFAULT 'pending' CHECK(enrichment_status IN ('none', 'pending', 'processing', 'done', 'failed'));

-- Migrate existing items so they don't get re-probed or re-enriched
UPDATE media_items SET probe_status = 'done' WHERE video_codec != '' OR duration > 0;
UPDATE media_items SET enrichment_status = 'done' WHERE tmdb_id IS NOT NULL OR poster_path IS NOT NULL;

-- +goose Down
ALTER TABLE media_items DROP COLUMN probe_status;
ALTER TABLE media_items DROP COLUMN enrichment_status;
