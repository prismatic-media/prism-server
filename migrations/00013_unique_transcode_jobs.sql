-- +goose Up
-- Clean up duplicate jobs, keeping the most recently created one
DELETE FROM transcode_jobs
WHERE id NOT IN (
    SELECT id
    FROM (
        SELECT id, ROW_NUMBER() OVER (
            PARTITION BY media_item_id
            ORDER BY created_at DESC, id DESC
        ) as rn
        FROM transcode_jobs
    )
    WHERE rn = 1
);

DROP INDEX IF EXISTS idx_transcode_jobs_media_item_id;
CREATE UNIQUE INDEX idx_transcode_jobs_media_item_id ON transcode_jobs(media_item_id);

-- +goose Down
DROP INDEX IF EXISTS idx_transcode_jobs_media_item_id;
CREATE INDEX idx_transcode_jobs_media_item_id ON transcode_jobs(media_item_id);
