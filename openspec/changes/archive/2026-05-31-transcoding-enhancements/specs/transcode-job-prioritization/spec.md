## ADDED Requirements

### Requirement: Admin can prioritize a pending transcode job
The system SHALL provide an endpoint `POST /api/v1/jobs/{id}/prioritize` that sets the target job's `priority` to `MAX(priority) + 1` across all currently pending jobs. This ensures the job will be claimed before all other pending jobs on the next available worker.

#### Scenario: Job is moved ahead of all pending jobs
- **WHEN** an authenticated admin sends `POST /api/v1/jobs/{id}/prioritize` for a `pending` job
- **AND** other pending jobs exist with various priority values
- **THEN** the target job's `priority` is set to `MAX(priority of all pending jobs) + 1`
- **AND** the target job will be the next job claimed by a worker

#### Scenario: Multiple prioritizations form their own FIFO
- **WHEN** two jobs are prioritized in sequence (job A, then job B)
- **THEN** job B has a higher `priority` than job A
- **AND** job B is claimed before job A

#### Scenario: Prioritizing a job with no other pending jobs
- **WHEN** `POST /api/v1/jobs/{id}/prioritize` is called and no other pending jobs exist
- **THEN** the job's `priority` is set to 1 (i.e., `0 + 1`)

---

### Requirement: Only pending jobs can be prioritized
The system SHALL reject prioritize requests for jobs that are not in `pending` status.

#### Scenario: Prioritizing a processing job returns 409
- **WHEN** `POST /api/v1/jobs/{id}/prioritize` is called for a job with `status = 'processing'`
- **THEN** the response is 409 Conflict

#### Scenario: Prioritizing a done job returns 409
- **WHEN** `POST /api/v1/jobs/{id}/prioritize` is called for a job with `status = 'done'`
- **THEN** the response is 409 Conflict

#### Scenario: Prioritizing a failed job returns 409
- **WHEN** `POST /api/v1/jobs/{id}/prioritize` is called for a job with `status = 'failed'`
- **THEN** the response is 409 Conflict

---

### Requirement: Prioritize endpoint requires admin authentication
The `POST /api/v1/jobs/{id}/prioritize` endpoint SHALL require admin authentication.

#### Scenario: Unauthenticated request rejected
- **WHEN** an unauthenticated request is sent to `POST /api/v1/jobs/{id}/prioritize`
- **THEN** the response is 401 Unauthorized

#### Scenario: Non-admin request rejected
- **WHEN** an authenticated non-admin user sends `POST /api/v1/jobs/{id}/prioritize`
- **THEN** the response is 403 Forbidden

---

### Requirement: Prioritizing a non-existent job returns 404
The system SHALL return 404 Not Found when the job ID does not exist.

#### Scenario: Unknown job ID
- **WHEN** `POST /api/v1/jobs/{id}/prioritize` is called with an ID that does not exist in the database
- **THEN** the response is 404 Not Found
