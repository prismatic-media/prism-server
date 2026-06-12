-- +goose Up
ALTER TABLE media_items ADD COLUMN source_fingerprint TEXT;
ALTER TABLE media_items ADD COLUMN source_status TEXT DEFAULT 'available' CHECK(source_status IN ('available', 'missing'));
ALTER TABLE media_items ADD COLUMN bundle_status TEXT DEFAULT 'none' CHECK(bundle_status IN ('none', 'available', 'missing'));

CREATE INDEX idx_media_items_source_fingerprint ON media_items (source_fingerprint) WHERE source_fingerprint IS NOT NULL;

-- +goose Down
-- In SQLite, dropping columns is supported in newer versions, but to ensure safety and simple rollback, we could recreate the table or just drop the columns/index.
-- To be safe and compliant with SQLite ALTER TABLE DROP COLUMN (supported since 3.35.0):
DROP INDEX IF EXISTS idx_media_items_source_fingerprint;
ALTER TABLE media_items DROP COLUMN source_fingerprint;
ALTER TABLE media_items DROP COLUMN source_status;
ALTER TABLE media_items DROP COLUMN bundle_status;
