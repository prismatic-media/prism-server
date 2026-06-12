-- +goose Up

CREATE TABLE artifact_records (
    id              TEXT PRIMARY KEY,
    storage_area_id TEXT NOT NULL REFERENCES storage_areas(id) ON DELETE CASCADE,
    source_path     TEXT NOT NULL,
    source_fingerprint TEXT,
    output_dir      TEXT,
    mpd_path        TEXT,
    health          TEXT NOT NULL DEFAULT 'unknown'
                          CHECK (health IN ('unknown', 'healthy', 'stale', 'missing', 'metadata_invalid', 'unavailable')),
    last_seen_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    registered_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    UNIQUE(storage_area_id, source_path)
);

CREATE TABLE artifact_media_links (
    id              TEXT PRIMARY KEY,
    artifact_id     TEXT NOT NULL REFERENCES artifact_records(id) ON DELETE CASCADE,
    media_item_id   TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    matched_via     TEXT NOT NULL CHECK (matched_via IN ('fingerprint', 'heuristic', 'manual')),
    status          TEXT NOT NULL DEFAULT 'linked'
                          CHECK (status IN ('linked', 'unmatched', 'ambiguous')),
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    UNIQUE(artifact_id, media_item_id)
);

CREATE INDEX idx_artifact_records_storage ON artifact_records (storage_area_id, health);
CREATE INDEX idx_artifact_records_fingerprint ON artifact_records (source_fingerprint) WHERE source_fingerprint IS NOT NULL;
CREATE INDEX idx_artifact_media_links_media ON artifact_media_links (media_item_id);
CREATE INDEX idx_artifact_media_links_artifact ON artifact_media_links (artifact_id);
CREATE INDEX idx_artifact_media_links_status ON artifact_media_links (status);

-- +goose Down

DROP TABLE IF EXISTS artifact_media_links;
DROP TABLE IF EXISTS artifact_records;
