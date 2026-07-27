# Interface Contract: Sub-Job Claiming & Bitrate Ordering

## Internal API / Store Interface

### `sqlite.ClaimNextSubJob(ctx context.Context, db *sql.DB, workerID *uuid.UUID) (*models.TranscodeSubJob, error)`

- **Purpose**: Atomically claims the next pending transcode sub-job for processing.
- **Preconditions**:
  - `transcode_sub_jobs` table populated with pending sub-jobs.
  - Optional `workerID` specifies the claiming worker (or `nil` for local pool).
- **Ordering Guarantee**:
  - Sub-jobs belonging to the earliest created parent job (`created_at ASC`) are evaluated first.
  - Video sub-jobs within a parent job are evaluated in ascending order of `video_bitrate_k` (`COALESCE(video_bitrate_k, 0) ASC`).
- **Return Value**:
  - Returns the claimed `*models.TranscodeSubJob` updated to `status = 'processing'`.
  - Returns `nil, nil` if no eligible pending sub-jobs exist.

---

## HTTP Worker API Endpoint

### `POST /api/v1/worker/claim`

- **Headers**:
  - `X-Worker-API-Key`: Secret key identifying remote worker.
- **Request Body**:
  ```json
  {
    "worker_id": "00000000-0000-0000-0000-000000000000"
  }
  ```
- **Response** (`200 OK`):
  ```json
  {
    "sub_job": {
      "id": "11111111-1111-1111-1111-111111111111",
      "job_id": "22222222-2222-2222-2222-222222222222",
      "type": "video",
      "profile_name": "360p",
      "width": 640,
      "height": 360,
      "video_bitrate_k": 800,
      "status": "processing"
    }
  }
  ```
- **Behavior**: Calls `sqlite.ClaimNextSubJob` under the hood. Ensures the lowest bitrate video sub-job available for the active parent job is returned.
