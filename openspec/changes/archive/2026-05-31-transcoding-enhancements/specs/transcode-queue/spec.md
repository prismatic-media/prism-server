## ADDED Requirements

### Requirement: Jobs are backed by the database, not an in-memory channel
The system SHALL use the `transcode_jobs` SQLite table as the sole backing store for the transcode queue. There SHALL be no in-memory channel of jobs. The queue capacity SHALL be unlimited (bounded only by disk/DB).

#### Scenario: Enqueue succeeds without capacity limit
- **WHEN** any number of transcode jobs are enqueued
- **THEN** all jobs are inserted into the `transcode_jobs` table with `status = 'pending'` and no jobs are dropped

---

### Requirement: Workers claim jobs atomically from the database
Each worker SHALL claim the next job by atomically updating a single `pending` row to `processing` in one SQL statement. The claim SHALL select the job with the highest `priority` value first, breaking ties by `created_at ASC` (oldest first).

#### Scenario: Worker claims highest-priority job
- **WHEN** multiple `pending` jobs exist with different `priority` values
- **THEN** the worker claims the job with the highest `priority`

#### Scenario: Worker breaks priority ties by age
- **WHEN** multiple `pending` jobs share the same `priority` value
- **THEN** the worker claims the job with the earliest `created_at`

#### Scenario: Two workers do not claim the same job
- **WHEN** two workers attempt to claim a job simultaneously
- **THEN** exactly one worker succeeds and the other finds no job to claim

---

### Requirement: Idle workers poll the database at a configurable interval
When no pending jobs are found, a worker SHALL sleep for `transcode_poll_interval` seconds (from settings) before polling again. When a job is found and completed, the worker SHALL immediately poll again without sleeping.

#### Scenario: Worker sleeps when queue is empty
- **WHEN** a worker polls and finds no pending jobs
- **THEN** the worker sleeps for `transcode_poll_interval` seconds before polling again

#### Scenario: Worker immediately picks up next job after completing one
- **WHEN** a worker finishes a job and pending jobs exist
- **THEN** the worker claims the next job without sleeping

---

### Requirement: Stale processing jobs are recovered on startup
On startup, the pool SHALL reset any `transcode_jobs` rows with `status = 'processing'` back to `status = 'pending'`, preserving their `priority` value. These jobs SHALL be picked up on the next poll cycle.

#### Scenario: In-flight jobs recovered after crash
- **WHEN** the server restarts and `transcode_jobs` contains rows with `status = 'processing'`
- **THEN** those rows are reset to `status = 'pending'` with their original `priority` preserved

#### Scenario: No stale jobs on clean startup
- **WHEN** the server restarts and no `transcode_jobs` rows have `status = 'processing'`
- **THEN** no rows are modified during startup recovery
