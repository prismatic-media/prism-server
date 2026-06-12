## 1. Database Migration

- [x] 1.1 Create `migrations/00004_transcode_job_priority.sql` — add `priority INTEGER NOT NULL DEFAULT 0` column to `transcode_jobs`
- [x] 1.2 Update `migrations/embed.go` if needed to pick up the new migration file
- [x] 1.3 Add `Priority int` field to `models.TranscodeJob` struct

## 2. Store Layer — New Queries

- [x] 2.1 Add `ClaimNextJob(ctx) (*TranscodeJob, error)` — atomic `UPDATE … RETURNING` claiming the highest-priority pending job (`ORDER BY priority DESC, created_at ASC`)
- [x] 2.2 Add `RecoverStaleJobs(ctx) error` — reset all `processing` rows to `pending` (preserving `priority`) for crash recovery on startup
- [x] 2.3 Add `PrioritizeJob(ctx, id) error` — set job's `priority` to `MAX(priority)+1` among pending jobs; return error if job is not `pending`
- [x] 2.4 Add `BulkEnqueueUntranscoded(ctx) (int, error)` — create `pending` jobs (`priority=0`) for all media items with no existing transcode job; return count created
- [x] 2.5 Add `BulkEnqueueFailed(ctx) (int, error)` — create `pending` jobs (`priority=0`) for all media items whose most recent transcode job has `status='failed'`; return count created
- [x] 2.6 Register `auto_transcode_on_discovery` and `transcode_poll_interval` in `configurableSettingKeys` in `settings.go`
- [x] 2.7 Add bootstrap defaults for both new settings in `BootstrapSettings()`: `auto_transcode_on_discovery="false"`, `transcode_poll_interval="15"`

## 3. Transcoder Pool — Rewrite

- [x] 3.1 Remove `jobs chan Job` field from `Pool` struct and all channel-based enqueue/worker logic
- [x] 3.2 Add DB-polling worker loop: claim job via `ClaimNextJob`, process it, immediately loop; sleep `transcode_poll_interval` seconds only when no job found
- [x] 3.3 Read `transcode_poll_interval` from settings at startup (parse as int, fall back to 15 on error)
- [x] 3.4 Call `RecoverStaleJobs()` in `Pool.Start()` before launching workers
- [x] 3.5 Update `Pool.Enqueue()` — only inserts the DB row; no channel push
- [x] 3.6 Subscribe to `EventMediaCreated` in `Pool.Start()`; in handler: check `auto_transcode_on_discovery` setting and item's `TranscodeStatus`; enqueue only if status is empty/unset

## 4. API — New Endpoints

- [x] 4.1 Add `BulkEnqueueJobs` handler in `internal/api/handler/jobs.go` — parse `{ "filter": "untranscoded" | "failed" }`, call corresponding store method, return `{ "enqueued": N }`; return 400 for unknown filter
- [x] 4.2 Add `PrioritizeJob` handler in `internal/api/handler/jobs.go` — call store `PrioritizeJob`; return 404 if not found, 409 if not pending, 200 on success
- [x] 4.3 Register routes in `internal/api/router.go`: `POST /api/v1/jobs/bulk-enqueue` and `POST /api/v1/jobs/{id}/prioritize` (both admin-authed)

## 5. Verification

- [x] 5.1 Run existing job store tests (`internal/store/sqlite/jobs_test.go`) — ensure no regressions
- [x] 5.2 Run existing handler tests (`internal/api/handler/jobs_test.go`) — ensure no regressions
- [x] 5.3 Manually verify: start server, add media, confirm `transcode_poll_interval` idle sleep behavior
- [x] 5.4 Manually verify: enable `auto_transcode_on_discovery`, add file, confirm job created
- [x] 5.5 Manually verify: call bulk-enqueue endpoints, confirm correct counts and DB rows
- [x] 5.6 Manually verify: prioritize a pending job, confirm it runs next after current job finishes
- [x] 5.7 Manually verify: crash recovery — simulate stale `processing` rows, restart, confirm reset to `pending`
