# Artifact Recovery Runbook

## Overview

The artifact recovery system provides durable persistence for transcoded media
outputs so they survive database loss, storage path changes, and server
relocations. This runbook covers the operational workflows for indexing and
relinking artifacts.

## Prerequisites

1. Migration 00006 (`artifact_records`) must be applied before any artifact
   operations will work.
2. At least one enabled segment storage area must be configured.
3. Transcoded outputs must have `.artifact.json` sidecar files (written
   automatically by the transcoder since this feature was enabled).

## Checking Artifact Schema Status

```bash
# Check if the artifact schema is ready via the API
curl -s -H "Authorization: Bearer $ADMIN_TOKEN" \
  http://localhost:8080/api/v1/admin/artifacts/status | jq .
```

Response:

```json
{
  "ready": true,
  "areas": [
    {
      "storage_area_id": "uuid...",
      "storage_area_path": "/mnt/segments",
      "enabled": true,
      "by_health": [
        { "health": "healthy", "count": 150 },
        { "health": "stale", "count": 5 },
        { "health": "missing", "count": 2 }
      ]
    }
  ],
  "unmatched": 3,
  "ambiguous": 0
}
```

## Indexing Artifacts

Indexing scans enabled segment storage areas for `.artifact.json` sidecar files
and registers/updates artifact records in the database.

### Running Index

```bash
curl -s -X POST -H "Authorization: Bearer $ADMIN_TOKEN" \
  http://localhost:8080/api/v1/admin/artifacts/index | jq .
```

Response:

```json
{
  "summaries": [
    {
      "storage_area_id": "uuid...",
      "storage_area_path": "/mnt/segments",
      "registered": 42,
      "updated": 108,
      "removed": 3,
      "errors": 0
    }
  ],
  "total": 150
}
```

### Indexing Results

| Field        | Description                                    |
| ------------ | ---------------------------------------------- |
| `registered` | New artifact records created                   |
| `updated`    | Existing records updated (last_seen refreshed) |
| `removed`    | Artifacts marked as missing (sidecar deleted)  |
| `errors`     | Directories that failed to process             |

### Indexing Behavior

1. **New artifacts**: When a sidecar is found that doesn't match any existing
   record, a new `artifact_record` is created with `health=healthy`.

2. **Existing artifacts**: When a sidecar matches an existing record (by
   `storage_area_id + source_path`), the record is updated with a new
   `last_seen_at` timestamp and `health=healthy` is restored.

3. **Missing artifacts**: Artifacts whose sidecars no longer exist are marked
   as `health=missing`.

4. **Stale detection**: Artifacts whose `last_seen_at` is older than 7 days
   (configurable via `staleAfter` in the Indexer) are marked as
   `health=stale`. This can be extended by modifying the Indexer.

5. **Idempotent**: Running index multiple times is safe. It will not create
   duplicate records or break existing links.

## Relinking Artifacts

Relinking compares artifact source fingerprints against media item file paths
and creates `artifact_media_links` for exact matches.

### Running Relink

```bash
curl -s -X POST -H "Authorization: Bearer $ADMIN_TOKEN" \
  http://localhost:8080/api/v1/admin/artifacts/relink | jq .
```

Response:

```json
{
  "linked": 3,
  "unmatched": 1,
  "ambiguous": 0,
  "invalid": 0,
  "skipped": 5
}
```

### Relinking Results

| Field       | Description                                 |
| ----------- | ------------------------------------------- |
| `linked`    | New exact fingerprint matches created       |
| `unmatched` | Artifacts with no matching media item       |
| `ambiguous` | Artifacts matching multiple media items     |
| `invalid`   | Artifacts with invalid/missing fingerprints |
| `skipped`   | Already-linked artifacts (no action needed) |

### Relinking Behavior

1. **Exact fingerprint match**: When an artifact's `source_fingerprint` matches
   a media item's generated fingerprint (from its `file_path`), a link is
   created with `matched_via=fingerprint`.

2. **Unmatched**: Artifacts whose fingerprints don't match any media item are
   left with `status=unmatched` for manual review.

3. **Ambiguous**: When an artifact matches multiple media items, the first
   match is linked and the artifact is marked as `status=ambiguous`.

4. **Invalid**: Artifacts without fingerprints are skipped with `invalid` count.

## Database Loss Recovery

### Scenario: Database Corrupted or Lost

1. **Deploy new database**: Set up a fresh database and apply all migrations.

2. **Run indexing**:

   ```bash
   curl -s -X POST -H "Authorization: Bearer $ADMIN_TOKEN" \
     http://localhost:8080/api/v1/admin/artifacts/index | jq .
   ```

   This registers all artifacts found on disk.

3. **Run relinking**:

   ```bash
   curl -s -X POST -H "Authorization: Bearer $ADMIN_TOKEN" \
     http://localhost:8080/api/v1/admin/artifacts/relink | jq .
   ```

   This creates links between artifacts and media items.

4. **Verify**:

   ```bash
   curl -s -H "Authorization: Bearer $ADMIN_TOKEN" \
     http://localhost:8080/api/v1/admin/artifacts/status | jq .
   ```

   Check that `unmatched` count is acceptable and `healthy` count is high.

5. **Manual review**: For unmatched artifacts, check if the source files still
   exist and manually create links if needed.

### Scenario: Storage Path Changed

1. **Update storage area paths** in the admin UI or via the API.

2. **Run indexing**: Artifacts will be re-registered with updated storage area
   references. Existing records are matched by `source_path` so they are
   updated, not duplicated.

3. **Run relinking**: Links may need to be recreated if the path changes
   affect fingerprint resolution.

## Handling Ambiguity

When artifacts are marked as `ambiguous`, manual intervention is needed:

1. **Check the artifacts**:

   ```sql
   SELECT a.id, a.source_path, a.health, l.media_item_id
   FROM artifact_records a
   JOIN artifact_media_links l ON a.id = l.artifact_id
   WHERE l.status = 'ambiguous';
   ```

2. **Review the linked media items** and determine if the link is correct.

3. **Fix incorrect links**: Delete the wrong link and let relinking recreate it:

   ```sql
   DELETE FROM artifact_media_links
   WHERE artifact_id = '<wrong-artifact-id>' AND status = 'ambiguous';
   ```

4. **Re-run relinking** to recreate correct links.

## Troubleshooting

### Indexing Returns 409 Conflict

The artifact schema migration hasn't been applied:

```bash
# Check migration status
goose -dir migrations status

# Apply pending migrations
goose -dir migrations up
```

### Relink Returns No Matches

1. **Check storage area configuration**:

   ```bash
   curl -s -H "Authorization: Bearer $ADMIN_TOKEN" \
     http://localhost:8080/api/v1/admin/artifacts/status | jq .areas
   ```

   Ensure enabled segment storage areas have non-zero artifact counts.

2. **Check for missing fingerprints**:

   ```sql
   SELECT COUNT(*) FROM artifact_records
   WHERE source_fingerprint IS NULL OR source_fingerprint = '';
   ```

3. **Verify source files exist**: The relink process reads source files to
   generate fingerprints. If files are missing or inaccessible, matches will fail.

### Manifest Resolution Fails After Recovery

After database loss recovery, manifest resolution may fail if:

1. The `mpd_path` column is empty on media items
2. The artifact's `output_dir` or `mpd_path` fields are not populated

**Fix**: Run indexing to populate `output_dir` and `mpd_path` from sidecar files,
then run relinking to create links.

### High `stale` Count

Artifacts marked as `stale` haven't been seen in over 7 days. This is normal
for archived content. To adjust the threshold:

- Modify the `staleAfter` field in the `Indexer` struct
- Default is 7 days (7 _ 24 _ time.Hour)

## Rollback

If artifact operations cause issues:

1. **Drop artifact tables**:

   ```sql
   DROP TABLE IF EXISTS artifact_media_links;
   DROP TABLE IF EXISTS artifact_records;
   ```

2. **Revert migration** (optional):

   ```bash
   goose -dir migrations down 1
   ```

3. **Restore from backup** if needed.

Note: Dropping artifact tables does NOT delete any actual media files — only
the database records. Media files on disk are unaffected.
