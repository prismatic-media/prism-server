## Context

The transcoding pipeline currently uses an in-memory buffered Go channel (`chan Job`, cap 64) as its work queue. Workers pull jobs from the channel in strict FIFO order. This design has three significant limitations:

1. **Hard capacity ceiling**: The 64-job buffer silently drops enqueue attempts when full, making bulk operations impossible.
2. **No prioritization**: All jobs are equal; there is no way to move an item to the front of the queue.
3. **No crash recovery**: Jobs sitting in the channel at crash time are lost; they must be manually re-queued.
4. **No auto-trigger**: The scanner emits `EventMediaCreated` but nothing listens to enqueue transcoding automatically.

The existing SQLite `transcode_jobs` table already persists job state (`pending`, `processing`, `done`, `failed`). The DB is the source of truth — the channel is a redundant and lossy approximation of it.

## Goals / Non-Goals

**Goals:**
- Replace the channel queue with DB-backed polling; make the DB the single source of truth for job ordering
- Add numeric priority to jobs; `PrioritizeJob` bumps a job ahead of all others
- Add crash recovery: jobs stuck in `processing` at startup are reset to `pending`
- Add `auto_transcode_on_discovery` setting; wire it to `EventMediaCreated` listener
- Add bulk enqueue for untranscoded items and for failed items
- Add `transcode_poll_interval` setting controlling idle worker sleep (default 15s)

**Non-Goals:**
- Distributed / remote workers (future work; requires Postgres + `LISTEN/NOTIFY`)
- Preempting or cancelling an in-progress transcode job
- Per-library transcode policies
- Worker concurrency changes (still configured via `transcode_workers` setting)

## Decisions

### Decision: DB as queue, polling-based workers

**Chosen**: Remove `chan Job` entirely. Each worker runs a tight loop: claim the next highest-priority pending job via an atomic `UPDATE … RETURNING` statement, process it, then immediately loop back. If no job is found, the worker sleeps for `transcode_poll_interval` seconds before trying again.

**Alternatives considered**:
- `sync.Cond` for instant wakeup: adds complexity without meaningful benefit for a background transcoding queue. Millisecond dispatch latency is irrelevant here.
- Buffered `chan struct{}` as a wake signal alongside polling: slightly better latency but more moving parts. Deferred — can always be added later without changing the queue semantics.

**Rationale**: Polling at 15s means a new job is picked up within 15 seconds of being enqueued when workers are idle. For transcoding (jobs that take minutes), this is imperceptible. The simplicity of a single loop with a single mechanism is worth the minor latency.

### Decision: Atomic job claiming via `UPDATE … RETURNING`

Workers claim jobs using a single SQLite statement:

```sql
UPDATE transcode_jobs
SET status = 'processing', started_at = ?
WHERE id = (
  SELECT id FROM transcode_jobs
  WHERE status = 'pending'
  ORDER BY priority DESC, created_at ASC
  LIMIT 1
)
RETURNING *
```

This is safe under SQLite's serialized write model — two workers cannot claim the same job. No external mutex needed.

### Decision: Priority as relative bump (`MAX + 1`)

`PrioritizeJob` sets the target job's `priority` to `MAX(priority) + 1` across all pending jobs. Multiple prioritizations each leapfrog the previous, forming their own FIFO among the elevated set. Normal jobs have `priority = 0`.

**Alternatives considered**: Binary high/low flag — simpler but means multiple prioritized jobs have undefined ordering between themselves.

### Decision: Worker wake-up is polling only (no early signal)

After `PrioritizeJob`, the job will be claimed at the next poll cycle after the current job finishes — up to `transcode_poll_interval` seconds. This is acceptable; the job is guaranteed to be *next*, just not *immediately* next.

If sub-second dispatch latency becomes important (e.g., interactive use), a buffered `chan struct{}` notify signal can be added without changing queue semantics.

### Decision: Crash recovery on startup

On startup, `Pool.Start()` calls `RecoverStaleJobs()` which resets any `processing` rows back to `pending`, preserving their original `priority`. These jobs rejoin the queue and will be claimed on the next poll cycle.

### Decision: Auto-transcode listener in pool, not scanner

The `EventMediaCreated` subscriber is registered in `pool.Start()` and runs as a goroutine. It checks the `auto_transcode_on_discovery` setting and the item's current `TranscodeStatus` before enqueueing. The scanner remains unmodified.

**Rationale**: The scanner should not know about transcoding. The pool is already the authority on enqueue decisions.

### Decision: Two bulk operations via one endpoint

`POST /api/v1/jobs/bulk-enqueue` accepts `{ "filter": "untranscoded" | "failed" }`.

- `"untranscoded"`: media items with `transcode_status` of `""` (never set) — no prior job exists
- `"failed"`: media items whose last transcode job has `status = 'failed'`

New jobs are created with `priority = 0` (normal). Callers may subsequently `PrioritizeJob` individual jobs if needed.

## Risks / Trade-offs

| Risk | Mitigation |
|---|---|
| SQLite write contention under many workers | SQLite serializes writes; the atomic claim is a single short write. With the default 2 workers, contention is negligible. If worker count grows significantly, this is a signal to migrate to Postgres. |
| Bulk enqueue of very large libraries creates many DB rows at once | Insert in a single transaction for atomicity and performance. The operation is bounded by library size; no streaming needed at current scale. |
| Poll interval means up to 15s latency when queue drains | Acceptable for background transcoding. Configurable down to 1s if needed. |
| Re-scan after restart re-fires `EventMediaCreated` for all files | Listener guards: skip if `TranscodeStatus != ""`. Already-queued, in-progress, done, and failed items are all ignored. |

## Migration Plan

1. Add migration `00004_transcode_job_priority.sql`: `ALTER TABLE transcode_jobs ADD COLUMN priority INTEGER NOT NULL DEFAULT 0`
2. Deploy: existing `pending` jobs get `priority = 0` automatically; no data migration needed
3. Rollback: the column is additive; removing it requires a new migration (SQLite has no `DROP COLUMN` on older versions — use table recreation if needed)

## Open Questions

- None. All decisions resolved during exploration.
