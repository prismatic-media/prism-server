## Why

A single segments directory is a scaling bottleneck because DASH transcodes can consume large amounts of disk. The system needs first-class multi-disk storage management so new transcodes can be placed where capacity is available and admins can monitor storage health.

## What Changes

- Add a new `storage_areas` data model to manage storage locations by type (segments, thumbnails), including enabled/disabled state.
- Add admin storage APIs to create, update, list, disable, and optionally delete storage areas.
- Add a configurable reserve headroom setting (`storage_min_free_bytes`) with a default of 20 GiB.
- Route new transcode outputs to the enabled segments storage area with the most free raw bytes, excluding areas that are unavailable, unwritable, or below configured reserve headroom.
- Serve segment files based on each media item's persisted `mpd_path` location so playback works regardless of which storage area produced the transcode.
- Add a new Admin -> Storage page in the Angular UI (nested under Admin navigation) to manage storage areas and view per-area utilization/free-space details.
- Show thumbnail storage utilization on the Storage page (thumbnails remain operationally single-area for now).
- **BREAKING** Remove the single-directory segments configuration model in favor of `storage_areas`.

## Capabilities

### New Capabilities

- `storage-areas`: Storage area lifecycle, capacity telemetry, reserve headroom policy, and storage-aware transcode placement.

### Modified Capabilities

- `server-settings`: Replace single `segments_dir` configuration expectations with storage reserve configuration and storage-page-linked management behavior.
- `settings-subnav`: Extend admin nested navigation to include a Storage entry under Admin.
- `transcode-queue`: Add storage-area selection requirements for output placement when jobs are processed.

## Impact

- Database: new migration for `storage_areas`; defaults/bootstrap updates for storage reserve setting and initial storage-area rows.
- Backend: new storage area store/service logic, disk-space probing, path health checks, routing additions, transcoder output-root selection updates, stream path resolution changes.
- API: new admin storage endpoints and response payloads for utilization/status.
- Frontend: new admin Storage route/page, API client additions, sidebar/admin-link updates.
- Tests: migration tests, sqlite store tests, handler tests, transcoder selection tests, and UI routing/component tests for storage navigation and management flows.
