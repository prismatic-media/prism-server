## Why

Transcoding is currently a fully manual process — each media item must be individually triggered via the API. As libraries grow, this becomes impractical. Users need automatic transcoding on discovery, bulk operations for existing libraries, and the ability to prioritize specific items without waiting through a long queue.

## What Changes

- **New setting**: `auto_transcode_on_discovery` — when enabled, newly discovered media items are automatically enqueued for transcoding
- **New setting**: `transcode_poll_interval` — configures how frequently (in seconds) idle workers check the DB for new jobs (default: 15)
- **Bulk enqueue endpoint**: Queue all untranscoded media items (those with no prior transcode job) in one operation
- **Retry failed endpoint**: Re-queue all media items whose last transcode job failed
- **Job prioritization endpoint**: Move a specific pending job to the front of the queue
- **Queue backend overhaul**: Replace the in-memory channel-based queue with a DB-backed polling queue; removes the 64-job capacity ceiling and adds crash recovery
- **Job priority field**: Add `priority` column to `transcode_jobs`; workers always process highest-priority jobs first, then FIFO by creation time

## Capabilities

### New Capabilities

- `transcode-queue`: The transcoding queue — how jobs are stored, ordered, claimed by workers, and recovered after a crash. Replaces the current channel-based queue with a DB-backed, priority-aware polling model.
- `auto-transcode-on-discovery`: Behavior when a new media item is discovered by the scanner. When the setting is enabled, the item is automatically enqueued for transcoding if it has no existing transcode job.
- `transcode-bulk-enqueue`: Bulk operations to enqueue multiple media items at once — either all untranscoded items or all previously-failed items.
- `transcode-job-prioritization`: Ability to elevate a specific pending job so it runs before all other queued jobs.

### Modified Capabilities

- `server-settings`: Two new configurable keys added — `auto_transcode_on_discovery` and `transcode_poll_interval`.

## Impact

- `transcoder/pool.go` — significant rewrite: remove `chan Job`, replace with DB polling loop; add crash recovery on startup; add priority-aware job claiming
- `internal/store/sqlite/jobs.go` — new queries: atomic job claim, `ClaimNextJob`, `BulkEnqueueUntranscoded`, `BulkEnqueueFailed`, `PrioritizeJob`, `RecoverStaleJobs`
- `internal/api/handler/jobs.go` — two new endpoints: `POST /api/v1/jobs/bulk-enqueue`, `POST /api/v1/jobs/{id}/prioritize`
- `internal/api/router.go` — register new routes
- `internal/models/models.go` — add `Priority int` field to `TranscodeJob`
- `migrations/` — new migration adding `priority INTEGER NOT NULL DEFAULT 0` to `transcode_jobs`
- `internal/store/sqlite/settings.go` — register new configurable setting keys
- `internal/scanner/scanner.go` — subscribe to `EventMediaCreated`; conditionally enqueue based on setting
- No new external dependencies
