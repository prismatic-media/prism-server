## Context

Prism Media Server currently treats the source video file and its database row as the source of truth for a media item. Transcoded output (DASH segments + manifest) is stored in a separate storage area, and a `.artifact.json` sidecar records technical metadata (source fingerprint, output paths, rendition profiles). An artifact recovery system exists for disaster recovery: an admin can invoke indexing (scan storage areas for bundles) and relinking (match bundles to media items by fingerprint).

However, the sidecar contains only enough metadata to relink — not enough to reconstruct a media item. If the database is lost, TMDB enrichment data, TV hierarchy, and poster references are gone. If a source file is deleted, the scanner prunes the media item row entirely, even though the bundle is fully playable. And when the same source file appears at a new path (moved/renamed), the system treats it as a brand-new item and retranscodes.

### Current sidecar (v1)
```json
{
  "v": 1,
  "media_item_id": "uuid",
  "source_path": "/path/to/file.mkv",
  "source_fingerprint": "sha256...",
  "output_dir": "/data/segments/uuid/",
  "mpd_path": "manifest.mpd",
  "profiles": [...],
  "duration": 7200.0,
  "written_at": "2026-06-01T..."
}
```

### Current scanner prune (scanner.go:84)
```go
sqlite.DeleteMediaItemsNotIn(ctx, s.db, s.library.ID, paths)
```
This unconditionally deletes any `media_items` row whose file is no longer on disk.

## Goals / Non-Goals

**Goals:**
- Make the transcode bundle self-describing: the sidecar contains all metadata needed to reconstruct a `media_items` row, including TMDB enrichment and TV hierarchy
- Allow media items to survive source file deletion when a bundle exists
- Detect duplicate source files at scan time via fingerprint matching, avoiding redundant transcodes
- Handle file moves/renames without retranscoding
- Enable the indexer to create `media_items` rows from bundle sidecar metadata alone

**Non-Goals:**
- Shared bundles across libraries (each library's copy gets independent bundles)
- Automatic startup indexing (indexing remains admin-invoked)
- Heuristic/fuzzy matching for discovery deduplication (fingerprint-exact only)
- UI changes for bundle/source status display (deferred to a follow-up)
- Automatic source file cleanup after transcoding

## Decisions

### 1. Sidecar schema v2 with full media metadata

**Decision**: Bump the sidecar to version 2, adding title, year, overview, TMDB ID, media type, poster filename, codecs, resolution, file size, and TV episode fields (show name, season/episode numbers).

**Rationale**: The sidecar must contain everything needed to create a `media_items` row (and parent TV show/season rows) without any external data source. This makes database reconstruction from bundles alone possible.

**Rejected alternative**: Store metadata in a separate companion file (e.g., `.metadata.json`) — adds complexity with no clear benefit over enriching the existing sidecar.

```json
{
  "v": 2,
  "media_item_id": "uuid",
  "source_path": "/movies/Inception.mkv",
  "source_fingerprint": "sha256...",
  "output_dir": "/data/segments/uuid/",
  "mpd_path": "manifest.mpd",
  "profiles": [...],
  "duration": 7200.0,
  "written_at": "2026-06-01T...",

  "media_type": "movie",
  "title": "Inception",
  "year": 2010,
  "overview": "A thief who steals...",
  "tmdb_id": 27205,
  "poster_file": "poster.jpg",
  "video_codec": "h264",
  "audio_codec": "aac",
  "width": 1920,
  "height": 800,
  "file_size": 4294967296,

  "tv_show_name": null,
  "season_number": null,
  "episode_number": null,

  "metadata_updated_at": "2026-06-01T..."
}
```

**Backward compatibility**: The indexer and relinking code already read v1 sidecars. They will continue to work — v1 sidecars simply lack the enriched fields. Items created from v1 sidecars will have incomplete metadata (no TMDB data, no poster) and will need manual enrichment or a rescan to pick up source metadata.

### 2. Poster images stored in the bundle directory

**Decision**: Copy the poster/still image into the bundle directory as `poster.jpg` (or `poster.png`) at transcode time (if already enriched) and at enrichment time (if bundle exists). The sidecar records the poster filename in `poster_file`.

**Rationale**: Makes the bundle fully self-contained — the poster travels with the segments and can be served without the thumbnails storage area. During DB reconstruction, the indexer can serve posters directly from bundles.

**Rejected alternative**: Store only the TMDB poster URL in the sidecar and re-download on recovery — requires network access during recovery, fragile if TMDB URLs change.

### 3. Living sidecar updated on enrichment

**Decision**: The enricher updates the on-disk sidecar after successfully persisting TMDB metadata to the DB. The enricher derives the bundle output directory from `mpd_path` (its parent directory). The sidecar's `metadata_updated_at` timestamp is set on each update.

**Rationale**: Ensures the sidecar stays in sync with the DB for the metadata that matters most (TMDB enrichment). Without this, a sidecar written at transcode time before enrichment completes would have empty TMDB fields.

**Update flow**:
```
Enricher.EnrichItem()
  ├── Search TMDB → get metadata
  ├── Download poster → thumbs dir
  ├── sqlite.UpdateMediaMetadata()  (existing)
  ├── If item.MPDPath is set:
  │   ├── Read sidecar from bundle dir
  │   ├── Update title, year, overview, tmdb_id, poster_file
  │   ├── Copy poster to bundle dir
  │   └── Write sidecar back
  └── Done
```

**Race condition safety**: The enricher runs asynchronously. The sidecar is a single file written atomically (write to temp + rename). No concurrent writers — enrichment runs once per item, and transcoding has already finished when enrichment updates the sidecar.

### 4. Source fingerprint as a first-class column on media_items

**Decision**: Add `source_fingerprint TEXT` to `media_items`, computed at scan time when the file is first discovered (or on the next scan if upgrading from an existing DB). Add an index for fast lookups.

**Rationale**: The fingerprint is the bridge between scanner and indexer worlds. The scanner needs to check "does an item with this fingerprint already exist in this library?" before creating a new row. Storing it on `media_items` avoids a join through `artifact_records` for this common-path check.

**Rejected alternative**: Use `artifact_records.source_fingerprint` exclusively — requires a multi-table join on every scan upsert, and doesn't help when no artifact exists yet (pre-transcode items).

### 5. Source and bundle status tracking on media_items

**Decision**: Add two columns to `media_items`:
- `source_status TEXT CHECK(source_status IN ('available', 'missing')) DEFAULT 'available'`
- `bundle_status TEXT CHECK(bundle_status IN ('none', 'available', 'missing')) DEFAULT 'none'`

**Rationale**: These status fields allow the system to make lifecycle decisions: "can I delete this row?" (only if both are gone), "can I retranscode?" (only if source is available), "is this playable?" (yes if bundle is available).

**Rejected alternative**: A single `lifecycle_state` enum (pending/complete/bundle-only/ghost) — less flexible, harder to extend, and conflates two independent dimensions.

### 6. Scanner prune logic: respect bundle-backed items

**Decision**: Replace `DeleteMediaItemsNotIn()` with a two-phase prune:
1. For items NOT in the found set AND `bundle_status = 'none'`: delete the row (current behavior)
2. For items NOT in the found set AND `bundle_status IN ('available', 'missing')`: set `source_status = 'missing'` instead

Similarly, the fsnotify Remove/Rename handler changes from `DeleteMediaItem()` to: check bundle_status, delete if none, set source_status=missing if bundle exists.

**Rationale**: This is the core behavior change that enables bundle-only items. A bundle-backed item survives source file deletion.

### 7. Scanner fingerprint-based deduplication at discovery time

**Decision**: At scan time, after FFprobe, compute the source fingerprint. Before upserting, check:
1. **Same library, same fingerprint, different path**: This is a file move/rename. Update the existing row's `file_path` and `source_status = 'available'`. Do not fire `EventMediaCreated` or enqueue a transcode.
2. **Same library, same fingerprint, existing bundle**: The source file matches an existing bundle. Link the item, set `bundle_status = 'available'`, `transcode_status = 'done'`, assign `mpd_path`. Do not enqueue a transcode.
3. **No fingerprint match**: Create a new row as today.

**Rationale**: Handles the common case of library reorganization (file moves) without expensive retranscoding. Also catches re-added files that already have bundles.

**Note**: Cross-library duplicates are explicitly ignored — each library is independent.

### 8. Indexer as media item creator

**Decision**: When the indexer discovers a v2 sidecar with no matching `media_items` row (by fingerprint), it creates a new `media_items` row from the sidecar metadata:
- `source_status = 'missing'` (no source file verified yet)
- `bundle_status = 'available'`
- `transcode_status = 'done'`
- `mpd_path` from sidecar
- Title, year, TMDB ID, overview, poster from sidecar
- For episodes: upsert parent TVShow and TVSeason from `tv_show_name` and `season_number`

When the scanner next runs, it may find the source file, match by fingerprint, and set `source_status = 'available'`.

**Rationale**: Enables full database reconstruction from bundles. The indexer + scanner working together can rebuild the entire media catalog from disk state alone.

### 9. Auto-transcode skips bundle-matched items

**Decision**: The auto-enqueue listener already checks `item.TranscodeStatus != models.TranscodeStatusPending`. When discovery deduplication links an item to an existing bundle, it sets `transcode_status = 'done'`, so auto-enqueue naturally skips it. No additional check is needed in the listener itself.

However, as a safety belt, the `Enqueue()` method adds a guard: if `bundle_status = 'available'` and `transcode_status = 'done'`, reject the enqueue with an error.

**Rationale**: Defense in depth. The primary deduplication happens at scan time, but the enqueue guard catches edge cases where an item was linked to a bundle by the indexer between scan and enqueue.

## Risks / Trade-offs

**Sidecar write failures during enrichment** → The enricher update is best-effort (logged, not propagated), consistent with the existing enrichment pattern. A failed sidecar update means the sidecar has stale metadata but is still valid for relinking. The next enrichment or manual metadata update can retry.

**Fingerprint collision risk** → SHA-256 of first 64KB is deterministic but not collision-proof. Two different files could theoretically produce the same fingerprint. Given the 64KB window and SHA-256, this is astronomically unlikely for real media files. The fingerprint match is always scoped to a single library, further reducing risk.

**Migration of existing items without fingerprints** → Existing `media_items` rows won't have `source_fingerprint` after migration. The scanner will compute and store fingerprints on the next scan. Until then, deduplication won't catch existing items. This is acceptable — the next full scan populates all fingerprints.

**V1 sidecars can't reconstruct items** → Existing bundles with v1 sidecars lack TMDB metadata, poster, etc. The indexer can still create items from them (with partial data), and the scanner/enricher will fill in the gaps when sources are available. A future "rewrite sidecars" admin operation could upgrade v1 → v2 for all bundles.

**Sidecar size growth** → Adding full metadata roughly doubles sidecar size (from ~500 bytes to ~1-2KB). This is negligible compared to the transcode output (gigabytes).

## Migration Plan

1. **Migration 00007**: Add `source_fingerprint`, `source_status`, `bundle_status` columns to `media_items`. Default values ensure backward compatibility (`source_status='available'`, `bundle_status='none'`). Add index on `source_fingerprint`.
2. **Deploy code changes**: Sidecar v2, enricher sidecar updates, scanner fingerprint computation, modified prune logic, indexer media creation.
3. **First scan after deploy**: Scanner computes fingerprints for all existing items and stores them. Existing bundles keep v1 sidecars until their items are re-enriched or retranscoded.
4. **Admin index operation**: Optionally run indexing to create items from orphaned bundles (bundles without matching media items).

**Rollback**: The new columns have safe defaults. Reverting the code restores the old scanner prune behavior (deleting all missing-source items). Bundle-only items created during the new code's operation would be pruned on the next scan after rollback, but their bundles remain on disk and can be re-linked later. No data loss.
