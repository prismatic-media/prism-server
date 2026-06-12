## 1. Schema & Data Model

- [x] 1.1 Create migration 00007: add `source_fingerprint TEXT`, `source_status TEXT DEFAULT 'available'`, `bundle_status TEXT DEFAULT 'none'` columns to `media_items` with CHECK constraints; add index on `source_fingerprint`
- [x] 1.2 Update `MediaItem` model in `internal/models/models.go`: add `SourceFingerprint`, `SourceStatus`, `BundleStatus` fields with corresponding type constants (`SourceStatusAvailable`, `SourceStatusMissing`, `BundleStatusNone`, `BundleStatusAvailable`, `BundleStatusMissing`)
- [x] 1.3 Update `ArtifactIndexSummary` model to include `MediaItemsCreated int` field

## 2. Sidecar V2 Schema

- [x] 2.1 Expand `SidecarMetadata` struct in `internal/artifact/metadata.go`: add `MediaType`, `Title`, `Year`, `Overview`, `TMDBId`, `PosterFile`, `VideoCodec`, `AudioCodec`, `Width`, `Height`, `FileSize`, `TVShowName`, `SeasonNumber`, `EpisodeNumber`, `MetadataUpdatedAt` fields; set version to 2 in `WriteSidecar`
- [x] 2.2 Ensure `ReadSidecar` handles both v1 and v2 formats: v1 sidecars parse successfully with new fields as zero values; add unit tests for v1/v2 roundtrip
- [x] 2.3 Update `writeArtifactSidecar` in `internal/transcoder/pool.go`: populate all v2 fields from the `MediaItem` (title, year, TMDB ID, overview, poster, codecs, resolution, file size, TV hierarchy); copy poster image into bundle directory as `poster.jpg`

## 3. Living Sidecar (Enrichment Updates)

- [x] 3.1 Add `UpdateSidecarMetadata` function to `internal/artifact/metadata.go`: reads existing sidecar, updates enrichment fields (title, year, TMDB ID, overview, poster_file, metadata_updated_at), writes back atomically
- [x] 3.2 Update `Enricher.EnrichItem` in `internal/metadata/enricher.go`: after `UpdateMediaMetadata` succeeds, if item has `MPDPath` set, call `UpdateSidecarMetadata` and copy poster into bundle directory; log but don't propagate errors
- [x] 3.3 Update `Enricher.EnrichTVEpisode` in `internal/metadata/enricher.go`: same sidecar update logic after episode enrichment completes
- [x] 3.4 Add unit tests for sidecar update: verify fields updated, verify poster copied, verify failure doesn't block enrichment

## 4. Store Layer: Fingerprint & Status Queries

- [x] 4.1 Add `GetMediaItemByFingerprint(ctx, db, libraryID, fingerprint)` to `internal/store/sqlite/media_items.go`: returns media item matching the fingerprint within a library, or nil
- [x] 4.2 Add `UpdateMediaItemFilePath(ctx, db, itemID, newPath)` to update file_path and set source_status to available
- [x] 4.3 Add `SetMediaSourceStatus(ctx, db, itemID, status)` and `SetMediaBundleStatus(ctx, db, itemID, status)` functions
- [x] 4.4 Update `UpsertMediaItem` to include `source_fingerprint` in insert/update
- [x] 4.5 Replace `DeleteMediaItemsNotIn` with `PruneStaleMediaItems(ctx, db, libraryID, foundPaths)`: delete items where source is gone AND bundle_status is 'none'; set source_status='missing' for items where source is gone but bundle exists

## 5. Scanner: Fingerprint Computation & Deduplication

- [x] 5.1 Update `upsertFile` in `internal/scanner/scanner.go`: compute fingerprint via `fingerprint.GenerateDeterministic(path)` after FFprobe; check `GetMediaItemByFingerprint` before upserting
- [x] 5.2 Implement file-move detection in `upsertFile`: if fingerprint matches existing item in same library at different path, update path via `UpdateMediaItemFilePath` and skip EventMediaCreated/enrichment
- [x] 5.3 Implement bundle linking in `upsertFile`: if fingerprint matches an `artifact_records` entry, set `bundle_status=available`, `transcode_status=done`, assign `mpd_path` from artifact record
- [x] 5.4 Update `upsertTVEpisodeFile` with the same fingerprint computation, move detection, and bundle linking logic
- [x] 5.5 Update `ScanAll` to call `PruneStaleMediaItems` instead of `DeleteMediaItemsNotIn`
- [x] 5.6 Update `handleEvent` Remove/Rename case: check bundle_status before deleting — if bundle exists, set source_status=missing instead

## 6. Indexer: Media Item Creation from Bundles

- [x] 6.1 Update `IndexStorageArea` in `internal/scanner/artifact_indexer.go`: after registering an artifact from a v2 sidecar, check if any media item matches the fingerprint; if not, create a media item from sidecar metadata
- [x] 6.2 Implement TV hierarchy creation in indexer: when creating an episode from sidecar, upsert parent TVShow and TVSeason using sidecar's `tv_show_name` and `season_number`
- [x] 6.3 Set `source_status=missing`, `bundle_status=available`, `transcode_status=done`, and assign `mpd_path` on indexer-created items
- [x] 6.4 Skip media creation for v1 sidecars (check sidecar version); skip when fingerprint already matches an existing media item
- [x] 6.5 Update `ArtifactIndexSummary` to track and return `media_items_created` count; update the `/api/v1/admin/artifacts/index` response to include this count

## 7. Auto-Transcode Guard

- [x] 7.1 Update `Enqueue` in `internal/transcoder/pool.go`: reject enqueue with error if item has `bundle_status=available` and `transcode_status=done`
- [x] 7.2 Verify auto-enqueue listener naturally skips bundle-linked items (transcode_status is already done, so existing check suffices); add integration test confirming no enqueue for bundle-matched discoveries

## 8. Testing

- [x] 8.1 Unit tests for sidecar v2: write v2 sidecar, read back, verify all fields; read v1 sidecar, verify backward compat
- [x] 8.2 Unit tests for fingerprint store queries: GetMediaItemByFingerprint, PruneStaleMediaItems with various source/bundle status combinations
- [x] 8.3 Integration test: discovery deduplication — discover file, transcode, remove file, re-add file at new path, verify no retranscode and item linked to existing bundle
- [x] 8.4 Integration test: bundle-only lifecycle — transcode item, delete source file, run scan, verify item preserved with source_status=missing; verify item still playable
- [x] 8.5 Integration test: DB rebuild — clear media_items, run indexer with v2 bundles, verify items created from sidecars with correct metadata; run scanner, verify source_status updated to available
- [x] 8.6 Integration test: enrichment sidecar update — enrich item after transcode, verify sidecar updated with TMDB data and poster copied to bundle dir
- [x] 8.7 Integration test: auto-enqueue guard — discover file that matches existing bundle, verify no transcode job created even with auto_transcode_on_discovery=true
