# storage-areas Specification

## Purpose

TBD - created by archiving change multi-storage-areas. Update Purpose after archive.

## Requirements

### Requirement: Storage areas are persisted as typed, toggleable resources

The system SHALL persist storage areas in a dedicated `storage_areas` data model. Each storage area SHALL include a storage kind (`segments` or `thumbnails`), a filesystem path, and an enabled/disabled state that controls whether the area participates in runtime selection.

#### Scenario: Default storage areas exist after bootstrap

- **WHEN** the server starts against a database with no storage areas
- **THEN** one enabled `segments` area at `/data/segments` and one enabled `thumbnails` area at `/data/thumbs` exist

#### Scenario: Admin disables a storage area without deleting it

- **WHEN** an authenticated admin marks a storage area as disabled
- **THEN** the storage area remains persisted and is excluded from placement selection until re-enabled

### Requirement: Admin can manage storage areas via admin API

The system SHALL provide admin-authenticated API endpoints to list, create, and update storage areas, including enabled state.

#### Scenario: Admin lists storage areas

- **WHEN** an authenticated admin requests storage area listing
- **THEN** the response includes all persisted storage areas with id, kind, path, and enabled state

#### Scenario: Non-admin cannot manage storage areas

- **WHEN** a non-admin user requests a storage area management endpoint
- **THEN** the response is 403 Forbidden

### Requirement: Storage page shows utilization and health for each area

The system SHALL provide an admin Storage page that displays utilization and free-space telemetry for each storage area and includes thumbnail storage utilization.

#### Scenario: Storage page shows per-area utilization

- **WHEN** an authenticated admin opens the Storage page
- **THEN** each listed storage area shows total bytes, used bytes, free bytes, and utilization percent

#### Scenario: Storage page shows path health status

- **WHEN** a configured storage area path is missing or not writable
- **THEN** the Storage page indicates that area as unavailable and includes an actionable status indicator

#### Scenario: Storage page includes thumbnail utilization

- **WHEN** an authenticated admin opens the Storage page
- **THEN** thumbnail storage utilization is visible alongside segment storage areas

### Requirement: Reserve headroom is configurable

The system SHALL expose `storage_min_free_bytes` as a configurable setting that defines minimum free bytes required for selecting a storage area. The default value SHALL be `21474836480` (20 GiB).

#### Scenario: Headroom default is bootstrapped

- **WHEN** the server starts against a fresh database
- **THEN** `storage_min_free_bytes` is initialized to `21474836480`

#### Scenario: Admin updates reserve headroom

- **WHEN** an authenticated admin updates `storage_min_free_bytes`
- **THEN** subsequent storage placement decisions use the updated value
