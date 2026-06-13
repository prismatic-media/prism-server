# transcode-bulk-enqueue Specification

## Purpose

TBD - created by archiving change transcoding-enhancements. Update Purpose after archive.

## Requirements

### Requirement: Admin can bulk-enqueue all untranscoded media items

The system SHALL provide an endpoint `POST /api/v1/jobs/bulk-enqueue` that, when called with `{ "filter": "untranscoded" }`, creates transcode jobs for all media items that have never been transcoded (items where no prior transcode job exists). All created jobs SHALL have `priority = 0`.

#### Scenario: All untranscoded items enqueued

- **WHEN** an authenticated admin sends `POST /api/v1/jobs/bulk-enqueue` with body `{ "filter": "untranscoded" }`
- **AND** N media items have no existing transcode job
- **THEN** N new transcode jobs are created with `status = 'pending'` and `priority = 0`
- **AND** the response includes the count of jobs created

#### Scenario: Already-transcoded items are skipped

- **WHEN** the bulk-enqueue request with `"filter": "untranscoded"` is processed
- **AND** some media items have an existing transcode job (in any status)
- **THEN** no new jobs are created for those items

#### Scenario: No untranscoded items returns zero count

- **WHEN** the bulk-enqueue request with `"filter": "untranscoded"` is processed
- **AND** all media items already have a transcode job
- **THEN** the response returns a count of 0 and no jobs are created

---

### Requirement: Admin can bulk-enqueue all failed transcode jobs

The system SHALL allow `POST /api/v1/jobs/bulk-enqueue` with `{ "filter": "failed" }` to create new transcode jobs for all media items whose most recent transcode job has `status = 'failed'`. All created jobs SHALL have `priority = 0`.

#### Scenario: All failed items re-enqueued

- **WHEN** an authenticated admin sends `POST /api/v1/jobs/bulk-enqueue` with body `{ "filter": "failed" }`
- **AND** N media items have a most-recent transcode job with `status = 'failed'`
- **THEN** N new transcode jobs are created with `status = 'pending'` and `priority = 0`
- **AND** the response includes the count of jobs created

#### Scenario: Non-failed items are skipped in failed filter

- **WHEN** the bulk-enqueue request with `"filter": "failed"` is processed
- **AND** a media item's most recent transcode job has `status = 'done'`, `'pending'`, or `'processing'`
- **THEN** no new job is created for that item

---

### Requirement: Bulk enqueue requires admin authentication

The `POST /api/v1/jobs/bulk-enqueue` endpoint SHALL require admin authentication. Unauthenticated or non-admin requests SHALL be rejected.

#### Scenario: Unauthenticated request rejected

- **WHEN** an unauthenticated request is sent to `POST /api/v1/jobs/bulk-enqueue`
- **THEN** the response is 401 Unauthorized

#### Scenario: Non-admin request rejected

- **WHEN** an authenticated non-admin user sends `POST /api/v1/jobs/bulk-enqueue`
- **THEN** the response is 403 Forbidden

---

### Requirement: Invalid filter value is rejected

The system SHALL reject bulk-enqueue requests with an unrecognized `filter` value.

#### Scenario: Unknown filter returns 400

- **WHEN** an authenticated admin sends `POST /api/v1/jobs/bulk-enqueue` with an unrecognized `filter` value
- **THEN** the response is 400 Bad Request
