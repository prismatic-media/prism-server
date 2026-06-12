## 1. Database and Bootstrap

- [x] 1.1 Add a migration that creates the `storage_areas` table with `kind`, `path`, `enabled`, and timestamps
- [x] 1.2 Add defaults/bootstrap logic for `storage_min_free_bytes` with default `21474836480`
- [x] 1.3 Add bootstrap seeding for default storage areas (`segments:/data/segments`, `thumbnails:/data/thumbs`) when missing
- [x] 1.4 Add sqlite store methods for listing, creating, and updating storage areas

## 2. Backend Storage Management APIs

- [x] 2.1 Add admin storage handler types for list/create/update/config operations
- [x] 2.2 Add admin storage routes and enforce admin auth on all storage endpoints
- [x] 2.3 Add backend disk-telemetry utilities to compute total/used/free/utilization and path health
- [x] 2.4 Return thumbnail utilization in storage API responses

## 3. Transcode Placement and Streaming

- [x] 3.1 Implement storage area selection logic that skips disabled/unavailable/unwritable/below-reserve segment areas
- [x] 3.2 Update transcode worker output placement to choose the eligible area with highest free raw bytes
- [x] 3.3 Ensure job failure includes actionable error when no eligible segment area exists
- [x] 3.4 Update stream segment serving to resolve segment paths from persisted media `mpd_path` location

## 4. Admin UI and Navigation

- [x] 4.1 Add `/admin/storage` route protected by admin guard
- [x] 4.2 Add a Storage link under Admin in sidebar sub-navigation with active-state and mobile close behavior
- [x] 4.3 Build Admin Storage page to list storage areas with utilization/free-space/health status
- [x] 4.4 Add UI actions to create storage areas and enable/disable existing areas
- [x] 4.5 Add UI control for configuring reserve headroom (`storage_min_free_bytes`)

## 5. Validation and Tests

- [x] 5.1 Add migration and bootstrap tests for `storage_areas` and `storage_min_free_bytes`
- [x] 5.2 Add sqlite store tests for storage area CRUD and enabled-state behavior
- [x] 5.3 Add handler/router tests for admin storage API auth and payload behavior
- [x] 5.4 Add transcoder tests for max-free-byte placement, skip rules, reserve filtering, and no-eligible-area failure
- [x] 5.5 Add stream handler tests confirming segment resolution from `mpd_path`-derived directory
- [x] 5.6 Add frontend route/component tests for Admin->Storage navigation and storage page behaviors
