## Why

After transcoding, the source file has no operational purpose — the transcode bundle contains everything needed to play the media. But the system still treats the source file and database row as the source of truth, meaning a lost database requires expensive re-enrichment and a deleted source file kills the media item entirely, even when a perfectly good bundle exists on disk. The sidecar metadata written at transcode time is too sparse to reconstruct a media item, and there's no mechanism to detect that a newly discovered file already has a transcoded bundle waiting for it.

## What Changes

- **Enriched sidecar metadata**: `.artifact.json` is expanded from technical-only metadata to a comprehensive record including title, year, TMDB enrichment (ID, overview, poster), TV hierarchy (show, season, episode), codecs, duration, and transcode configuration. Poster images are copied into the bundle directory alongside segments.
- **Living sidecar**: The sidecar is updated when metadata changes (e.g., TMDB enrichment completes), not just written once at transcode time.
- **Bundle-backed media items**: Media items gain `source_status` and `bundle_status` tracking. Items with a valid bundle survive source file deletion — the scanner marks the source as missing rather than deleting the row.
- **Indexer as media creator**: The artifact indexer can create `media_items` rows directly from bundle sidecar metadata when no matching media item exists, enabling full database reconstruction from bundles alone.
- **Discovery deduplication**: When the scanner discovers a source file, it computes a fingerprint and checks for existing media items or artifact records with the same fingerprint. If a matching bundle exists, the item is linked to it instead of enqueuing a redundant transcode.
- **Fingerprint on media items**: `media_items` gains a `source_fingerprint` column, computed at discovery time, enabling cross-reference between scanner and indexer without going through the artifact tables.

## Capabilities

### New Capabilities

- `discovery-deduplication`: When a source file is discovered, its fingerprint is checked against existing artifact records and media items. If a matching bundle exists, the media item is linked to the existing bundle, preventing redundant transcodes. Covers the scanner's fingerprint computation, lookup, and linking behavior at discovery time.
- `bundle-sourced-media`: Media items can exist backed solely by a transcode bundle, without a source file on disk. The system tracks source and bundle availability independently, the scanner preserves bundle-backed items during pruning, and the indexer can create full media items from sidecar metadata.

### Modified Capabilities

- `artifact-metadata`: Sidecar metadata is expanded to include full media metadata (title, year, TMDB enrichment, TV hierarchy, poster reference) and is updated when metadata changes occur (TMDB enrichment, metadata refresh). Poster images are stored in the bundle directory.
- `artifact-indexing`: The indexer can create `media_items` rows from bundle sidecar metadata when no matching media item exists in the database, making it a co-equal creator of media items alongside the scanner.
- `auto-transcode-on-discovery`: Before auto-enqueuing a transcode job for a newly discovered item, the system checks whether a bundle with a matching source fingerprint already exists and skips enqueue if so.

## Impact

- **Database schema**: New migration adding `source_fingerprint`, `source_status`, `bundle_status` columns to `media_items`. Index on `source_fingerprint` for fast lookups.
- **Scanner** (`internal/scanner/scanner.go`): Fingerprint computation at discovery, fingerprint-based lookup before upsert, modified prune logic to respect bundle-backed items.
- **Artifact metadata** (`internal/artifact/metadata.go`): Expanded `SidecarMetadata` struct with full media fields, poster path.
- **Transcoder** (`internal/transcoder/pool.go`): Write enriched sidecar with all metadata, copy poster into bundle directory.
- **Enricher** (`internal/metadata/enricher.go`): Update sidecar on disk after TMDB enrichment completes.
- **Indexer** (`internal/scanner/artifact_indexer.go`): Create `media_items` rows from sidecar metadata when no match exists.
- **Store layer** (`internal/store/sqlite/media_items.go`): New queries for fingerprint lookup, source/bundle status updates.
- **Models** (`internal/models/models.go`): New status fields and constants on `MediaItem`.
- **API handlers**: Surface source/bundle status in media item responses.
